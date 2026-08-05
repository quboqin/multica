package broker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type Service struct {
	queries    *db.Queries
	registry   Registry
	worker     WorkerClient
	sessionTTL time.Duration
}

type StartLoginSessionInput struct {
	WorkspaceID pgtype.UUID
	UserID      pgtype.UUID
	ProfileID   pgtype.UUID
	ConnectorID string
	Label       string
}

type StartLoginSessionResult struct {
	Profile db.CredentialProfile
	Session db.CredentialLoginSession
}

type RunCrawlInput struct {
	WorkspaceID      pgtype.UUID
	RequestingUserID pgtype.UUID
	ProfileID        pgtype.UUID
	ConnectorID      string
	Capability       string
	Params           json.RawMessage
}

type CompleteLoginSessionInput struct {
	SessionToken string
	Ciphertext   []byte
	KeyVersion   string
	ExpiresHint  pgtype.Timestamptz
}

type CompleteLoginSessionResult struct {
	Profile db.CredentialProfile
	Session db.CredentialLoginSession
}

func NewService(queries *db.Queries, worker WorkerClient) *Service {
	return NewServiceWithRegistry(queries, worker, DefaultRegistry())
}

func NewServiceWithRegistry(queries *db.Queries, worker WorkerClient, registry Registry) *Service {
	if worker == nil {
		worker = NewDisabledWorkerClient()
	}
	return &Service{
		queries:    queries,
		registry:   registry,
		worker:     worker,
		sessionTTL: 15 * time.Minute,
	}
}

func (s *Service) SetWorker(worker WorkerClient) {
	if worker == nil {
		worker = NewDisabledWorkerClient()
	}
	s.worker = worker
}

func (s *Service) ListConnectors() []Connector {
	return s.registry.List()
}

func (s *Service) ListProfiles(ctx context.Context, workspaceID pgtype.UUID) ([]db.CredentialProfile, error) {
	return s.queries.ListCredentialProfilesForWorkspace(ctx, workspaceID)
}

func (s *Service) GetProfile(ctx context.Context, workspaceID, profileID pgtype.UUID) (db.CredentialProfile, error) {
	return s.queries.GetCredentialProfileForWorkspace(ctx, db.GetCredentialProfileForWorkspaceParams{
		ID:          profileID,
		WorkspaceID: workspaceID,
	})
}

func (s *Service) CanManageProfile(ctx context.Context, profileID, userID pgtype.UUID) (bool, error) {
	return s.queries.IsCredentialProfileManager(ctx, db.IsCredentialProfileManagerParams{
		ProfileID: profileID,
		UserID:    userID,
	})
}

func (s *Service) ListProfileManagers(ctx context.Context, profileID pgtype.UUID) ([]db.ListCredentialProfileManagersRow, error) {
	return s.queries.ListCredentialProfileManagers(ctx, profileID)
}

func (s *Service) StartLoginSession(ctx context.Context, in StartLoginSessionInput) (StartLoginSessionResult, error) {
	connector, ok := s.registry.Get(strings.TrimSpace(in.ConnectorID))
	if !ok {
		return StartLoginSessionResult{}, ErrConnectorUnknown
	}
	if s.worker == nil || !s.worker.Configured() {
		return StartLoginSessionResult{}, ErrWorkerNotConfigured
	}
	profile, err := s.startLoginSessionProfile(ctx, in, connector)
	if err != nil {
		return StartLoginSessionResult{}, err
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return StartLoginSessionResult{}, err
	}
	workerResp, err := s.worker.StartLoginSession(ctx, WorkerLoginSessionRequest{
		ProfileID:    util.UUIDToString(profile.ID),
		ConnectorID:  connector.ID,
		SessionToken: token,
		LoginURL:     connector.LoginURL,
	})
	if err != nil {
		return StartLoginSessionResult{}, err
	}
	browserURL := strings.TrimSpace(workerResp.BrowserURL)
	if browserURL == "" {
		browserURL = connector.LoginURL
	}
	ttl := s.sessionTTL
	if workerResp.ExpiresInSeconds > 0 {
		ttl = time.Duration(workerResp.ExpiresInSeconds) * time.Second
	}
	_, _ = s.queries.AbortPendingCredentialLoginSessions(ctx, profile.ID)
	session, err := s.queries.CreateCredentialLoginSession(ctx, db.CreateCredentialLoginSessionParams{
		ProfileID:   profile.ID,
		UserID:      in.UserID,
		ConnectorID: connector.ID,
		TokenHash:   tokenHash,
		BrowserUrl:  browserURL,
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(ttl), Valid: true},
	})
	if err != nil {
		return StartLoginSessionResult{}, err
	}
	return StartLoginSessionResult{Profile: profile, Session: session}, nil
}

func (s *Service) startLoginSessionProfile(ctx context.Context, in StartLoginSessionInput, connector Connector) (db.CredentialProfile, error) {
	if in.ProfileID.Valid {
		profile, err := s.GetProfile(ctx, in.WorkspaceID, in.ProfileID)
		if err != nil {
			return db.CredentialProfile{}, err
		}
		if profile.ConnectorID != connector.ID {
			return db.CredentialProfile{}, ErrProfileConnectorMismatch
		}
		if err := s.requireProfileManager(ctx, profile.ID, in.UserID); err != nil {
			return db.CredentialProfile{}, err
		}
		return profile, nil
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = connector.DisplayName
	}
	profile, err := s.queries.GetCredentialProfileForWorkspaceByConnector(ctx, db.GetCredentialProfileForWorkspaceByConnectorParams{
		WorkspaceID: in.WorkspaceID,
		ConnectorID: connector.ID,
	})
	if err == nil {
		if err := s.requireProfileManager(ctx, profile.ID, in.UserID); err != nil {
			return db.CredentialProfile{}, err
		}
		return profile, nil
	}
	if err != pgx.ErrNoRows {
		return db.CredentialProfile{}, err
	}
	profile, err = s.queries.CreateCredentialProfile(ctx, db.CreateCredentialProfileParams{
		WorkspaceID:    in.WorkspaceID,
		AuthorizedByID: in.UserID,
		ConnectorID:    connector.ID,
		Label:          label,
		Status:         StatusPending,
		Scope:          connector.Scope,
	})
	if err != nil {
		if connector.Scope == ScopeDeployment && isDeploymentProfileUniqueViolation(err) {
			// The unique index is the serialization point for first-time global
			// binding. Re-read the winner so a race never leaks a database error.
			if _, lookupErr := s.queries.GetDeploymentCredentialProfileByConnector(ctx, connector.ID); lookupErr == nil {
				return db.CredentialProfile{}, ErrDeploymentProfileBindingConflict
			}
		}
		return db.CredentialProfile{}, err
	}
	if err := s.queries.AddCredentialProfileManager(ctx, db.AddCredentialProfileManagerParams{
		ProfileID:   profile.ID,
		UserID:      in.UserID,
		GrantedByID: in.UserID,
	}); err != nil {
		return db.CredentialProfile{}, err
	}
	return profile, nil
}

func isDeploymentProfileUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "credential_profile_deployment_connector_idx"
}

func (s *Service) CompleteLoginSession(ctx context.Context, in CompleteLoginSessionInput) (CompleteLoginSessionResult, error) {
	tokenHash := hashSessionToken(strings.TrimSpace(in.SessionToken))
	session, err := s.queries.GetPendingCredentialLoginSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return CompleteLoginSessionResult{}, err
	}
	keyVersion := strings.TrimSpace(in.KeyVersion)
	if keyVersion == "" {
		keyVersion = "v1"
	}
	if err := s.queries.UpsertCredentialSecret(ctx, db.UpsertCredentialSecretParams{
		ProfileID:  session.ProfileID,
		Ciphertext: in.Ciphertext,
		KeyVersion: keyVersion,
	}); err != nil {
		return CompleteLoginSessionResult{}, err
	}
	profile, err := s.queries.UpdateCredentialProfileStatus(ctx, db.UpdateCredentialProfileStatusParams{
		ID:          session.ProfileID,
		Status:      StatusActive,
		ExpiresHint: in.ExpiresHint,
	})
	if err != nil {
		return CompleteLoginSessionResult{}, err
	}
	profile, err = s.queries.UpdateCredentialProfileAuthorizedBy(ctx, db.UpdateCredentialProfileAuthorizedByParams{
		ID:             session.ProfileID,
		AuthorizedByID: session.UserID,
	})
	if err != nil {
		return CompleteLoginSessionResult{}, err
	}
	if err := s.queries.AddCredentialProfileManager(ctx, db.AddCredentialProfileManagerParams{
		ProfileID:   profile.ID,
		UserID:      session.UserID,
		GrantedByID: session.UserID,
	}); err != nil {
		return CompleteLoginSessionResult{}, err
	}
	session, err = s.queries.CompleteCredentialLoginSession(ctx, session.ID)
	if err != nil {
		return CompleteLoginSessionResult{}, err
	}
	return CompleteLoginSessionResult{Profile: profile, Session: session}, nil
}

func (s *Service) RevokeProfile(ctx context.Context, workspaceID, userID, profileID pgtype.UUID) (db.CredentialProfile, error) {
	if _, err := s.GetProfile(ctx, workspaceID, profileID); err != nil {
		return db.CredentialProfile{}, err
	}
	if err := s.requireProfileManager(ctx, profileID, userID); err != nil {
		return db.CredentialProfile{}, err
	}
	profile, err := s.queries.RevokeCredentialProfileForWorkspace(ctx, db.RevokeCredentialProfileForWorkspaceParams{
		ID:          profileID,
		WorkspaceID: workspaceID,
	})
	if err != nil {
		return db.CredentialProfile{}, err
	}
	if err := s.queries.DeleteCredentialSecret(ctx, profile.ID); err != nil {
		return db.CredentialProfile{}, err
	}
	return profile, nil
}

func (s *Service) AddProfileManager(ctx context.Context, workspaceID, actorID, profileID, userID pgtype.UUID) error {
	if _, err := s.GetProfile(ctx, workspaceID, profileID); err != nil {
		return err
	}
	if err := s.requireProfileManager(ctx, profileID, actorID); err != nil {
		return err
	}
	return s.queries.AddCredentialProfileManager(ctx, db.AddCredentialProfileManagerParams{
		ProfileID:   profileID,
		UserID:      userID,
		GrantedByID: actorID,
	})
}

func (s *Service) RemoveProfileManager(ctx context.Context, workspaceID, actorID, profileID, userID pgtype.UUID) error {
	if _, err := s.GetProfile(ctx, workspaceID, profileID); err != nil {
		return err
	}
	if err := s.requireProfileManager(ctx, profileID, actorID); err != nil {
		return err
	}
	removed, err := s.queries.RemoveCredentialProfileManager(ctx, db.RemoveCredentialProfileManagerParams{
		ProfileID: profileID,
		UserID:    userID,
	})
	if err != nil {
		return err
	}
	if removed == 0 {
		count, err := s.queries.CountCredentialProfileManagers(ctx, profileID)
		if err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastProfileManager
		}
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Service) requireProfileManager(ctx context.Context, profileID, userID pgtype.UUID) error {
	canManage, err := s.CanManageProfile(ctx, profileID, userID)
	if err != nil {
		return err
	}
	if !canManage {
		return ErrProfileManageForbidden
	}
	return nil
}

func (s *Service) RunCrawl(ctx context.Context, in RunCrawlInput) (WorkerCrawlResponse, error) {
	if err := ValidateSafeJSON(in.Params); err != nil {
		return WorkerCrawlResponse{}, err
	}
	profile, err := s.resolveCrawlProfile(ctx, in)
	if err != nil {
		return WorkerCrawlResponse{}, err
	}
	if profile.Status != StatusActive {
		s.recordUsageAudit(ctx, in, profile, StatusNeedReauth)
		return WorkerCrawlResponse{}, ErrProfileNotActive
	}
	if s.worker == nil || !s.worker.Configured() {
		s.recordUsageAudit(ctx, in, profile, "failed")
		return WorkerCrawlResponse{}, ErrWorkerNotConfigured
	}
	params := in.Params
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	resp, err := s.worker.RunCrawl(ctx, WorkerCrawlRequest{
		ProfileID:   util.UUIDToString(profile.ID),
		ConnectorID: profile.ConnectorID,
		Capability:  strings.TrimSpace(in.Capability),
		Params:      params,
	})
	if err != nil {
		s.recordUsageAudit(ctx, in, profile, "failed")
		return WorkerCrawlResponse{}, err
	}
	switch resp.Status {
	case StatusNeedReauth:
		s.recordUsageAudit(ctx, in, profile, StatusNeedReauth)
		_, _ = s.queries.UpdateCredentialProfileStatus(ctx, db.UpdateCredentialProfileStatusParams{
			ID:     profile.ID,
			Status: StatusNeedReauth,
		})
	case "completed", "ok", "success":
		s.recordUsageAudit(ctx, in, profile, "completed")
		_, _ = s.queries.TouchCredentialProfileLastUsed(ctx, profile.ID)
	default:
		s.recordUsageAudit(ctx, in, profile, "failed")
	}
	return resp, nil
}

func (s *Service) recordUsageAudit(ctx context.Context, in RunCrawlInput, profile db.CredentialProfile, outcome string) {
	_ = s.queries.CreateCredentialUsageAudit(ctx, db.CreateCredentialUsageAuditParams{
		ProfileID:     profile.ID,
		WorkspaceID:   in.WorkspaceID,
		RequestedByID: in.RequestingUserID,
		ConnectorID:   profile.ConnectorID,
		Capability:    strings.TrimSpace(in.Capability),
		Outcome:       outcome,
	})
}

func (s *Service) resolveCrawlProfile(ctx context.Context, in RunCrawlInput) (db.CredentialProfile, error) {
	if in.ProfileID.Valid {
		return s.GetProfile(ctx, in.WorkspaceID, in.ProfileID)
	}
	connectorID := strings.TrimSpace(in.ConnectorID)
	if connectorID == "" {
		return db.CredentialProfile{}, ErrProfileRequired
	}
	connector, ok := s.registry.Get(connectorID)
	if !ok {
		return db.CredentialProfile{}, ErrConnectorUnknown
	}
	profile, err := s.queries.GetActiveCredentialProfileForWorkspaceByConnector(ctx, db.GetActiveCredentialProfileForWorkspaceByConnectorParams{
		WorkspaceID: in.WorkspaceID,
		ConnectorID: connector.ID,
	})
	if err == nil {
		return profile, nil
	}
	return db.CredentialProfile{}, err
}

func DecodeCiphertextBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimSpace(s))
}

func newSessionToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashSessionToken(raw), nil
}

func hashSessionToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
