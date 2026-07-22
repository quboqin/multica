package broker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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
	WorkspaceID pgtype.UUID
	ProfileID   pgtype.UUID
	ConnectorID string
	Capability  string
	Params      json.RawMessage
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
		return profile, nil
	}
	if err != pgx.ErrNoRows {
		return db.CredentialProfile{}, err
	}
	return s.queries.CreateCredentialProfile(ctx, db.CreateCredentialProfileParams{
		WorkspaceID:    in.WorkspaceID,
		AuthorizedByID: in.UserID,
		ConnectorID:    connector.ID,
		Label:          label,
		Status:         StatusPending,
	})
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
	session, err = s.queries.CompleteCredentialLoginSession(ctx, session.ID)
	if err != nil {
		return CompleteLoginSessionResult{}, err
	}
	return CompleteLoginSessionResult{Profile: profile, Session: session}, nil
}

func (s *Service) RevokeProfile(ctx context.Context, workspaceID, profileID pgtype.UUID) (db.CredentialProfile, error) {
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

func (s *Service) RunCrawl(ctx context.Context, in RunCrawlInput) (WorkerCrawlResponse, error) {
	if err := ValidateSafeJSON(in.Params); err != nil {
		return WorkerCrawlResponse{}, err
	}
	profile, err := s.resolveCrawlProfile(ctx, in)
	if err != nil {
		return WorkerCrawlResponse{}, err
	}
	if profile.Status != StatusActive {
		return WorkerCrawlResponse{}, ErrProfileNotActive
	}
	if s.worker == nil || !s.worker.Configured() {
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
		return WorkerCrawlResponse{}, err
	}
	switch resp.Status {
	case StatusNeedReauth:
		_, _ = s.queries.UpdateCredentialProfileStatus(ctx, db.UpdateCredentialProfileStatusParams{
			ID:     profile.ID,
			Status: StatusNeedReauth,
		})
	case "completed", "ok", "success":
		_, _ = s.queries.TouchCredentialProfileLastUsed(ctx, profile.ID)
	}
	return resp, nil
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
