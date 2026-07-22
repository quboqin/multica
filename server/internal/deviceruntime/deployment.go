package deviceruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	deploymentStatusRunning  = "running"
	deploymentStatusFailed   = "failed"
	deploymentStatusCanceled = "canceled"
	deploymentStatusPending  = "pending"
)

type deploymentRequest struct {
	Serial    string `json:"serial"`
	SessionID string `json:"session_id"`
	Artifact  string `json:"artifact"`
	WebURL    string `json:"web_url"`
}

type deploymentSnapshot struct {
	ID             string `json:"id"`
	Serial         string `json:"serial"`
	Status         string `json:"status"`
	Phase          string `json:"phase"`
	ArtifactSHA256 string `json:"artifact_sha256"`
	InstallSkipped bool   `json:"install_skipped"`
	Error          string `json:"error,omitempty"`
	StartedAt      string `json:"started_at"`
	UpdatedAt      string `json:"updated_at"`
}

type deploymentJob struct {
	sessionID string
	snapshot  deploymentSnapshot
	cancel    context.CancelFunc
}

type deploymentStore struct {
	mu             sync.Mutex
	jobs           map[string]*deploymentJob
	activeBySerial map[string]string
	installed      map[string]string
}

func newDeploymentStore() *deploymentStore {
	return &deploymentStore{
		jobs:           make(map[string]*deploymentJob),
		activeBySerial: make(map[string]string),
		installed:      make(map[string]string),
	}
}

func (s *deploymentStore) launch(
	sessionID string,
	serial string,
	artifactHash string,
	run func(context.Context, func(string), string) (bool, error),
) (deploymentSnapshot, error) {
	s.mu.Lock()
	if activeID := s.activeBySerial[serial]; activeID != "" {
		if active := s.jobs[activeID]; active != nil && active.snapshot.Status == deploymentStatusPending {
			if active.sessionID != sessionID || active.snapshot.ArtifactSHA256 != artifactHash {
				s.mu.Unlock()
				return deploymentSnapshot{}, fmt.Errorf("deployment already in progress for device")
			}
			snapshot := active.snapshot
			s.mu.Unlock()
			return snapshot, nil
		}
	}
	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	job := &deploymentJob{
		sessionID: sessionID,
		snapshot: deploymentSnapshot{
			ID:             uuid.NewString(),
			Serial:         serial,
			Status:         deploymentStatusPending,
			Phase:          "queued",
			ArtifactSHA256: artifactHash,
			StartedAt:      now.Format(time.RFC3339Nano),
			UpdatedAt:      now.Format(time.RFC3339Nano),
		},
		cancel: cancel,
	}
	s.jobs[job.snapshot.ID] = job
	s.activeBySerial[serial] = job.snapshot.ID
	s.trimLocked(100)
	snapshot := job.snapshot
	s.mu.Unlock()

	go func() {
		defer cancel()
		update := func(phase string) {
			s.mu.Lock()
			job.snapshot.Phase = phase
			job.snapshot.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			s.mu.Unlock()
		}
		skipped, err := run(ctx, update, artifactHash)
		if contextErr := ctx.Err(); contextErr != nil {
			err = contextErr
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		job.snapshot.InstallSkipped = skipped
		job.snapshot.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		switch {
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			job.snapshot.Status = deploymentStatusCanceled
			job.snapshot.Phase = "canceled"
			if errors.Is(err, context.DeadlineExceeded) {
				job.snapshot.Status = deploymentStatusFailed
				job.snapshot.Phase = "failed"
				job.snapshot.Error = "deployment timed out"
			}
		case err != nil:
			job.snapshot.Status = deploymentStatusFailed
			job.snapshot.Phase = "failed"
			job.snapshot.Error = err.Error()
		default:
			job.snapshot.Status = deploymentStatusRunning
			job.snapshot.Phase = "ready"
		}
		if s.activeBySerial[serial] == job.snapshot.ID {
			delete(s.activeBySerial, serial)
		}
	}()

	return snapshot, nil
}

func (s *deploymentStore) snapshotFor(id, sessionID string) (deploymentSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[id]
	if job == nil || (sessionID != "" && job.sessionID != sessionID) {
		return deploymentSnapshot{}, false
	}
	return job.snapshot, true
}

func (s *deploymentStore) snapshot(id string) (deploymentSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[id]
	if job == nil {
		return deploymentSnapshot{}, false
	}
	return job.snapshot, true
}

func (s *deploymentStore) cancel(id string) (deploymentSnapshot, bool) {
	s.mu.Lock()
	job := s.jobs[id]
	if job == nil {
		s.mu.Unlock()
		return deploymentSnapshot{}, false
	}
	if job.snapshot.Status == deploymentStatusPending {
		job.cancel()
	}
	snapshot := job.snapshot
	s.mu.Unlock()
	return snapshot, true
}

func (s *deploymentStore) cancelFor(id, sessionID string) (deploymentSnapshot, bool) {
	s.mu.Lock()
	job := s.jobs[id]
	if job == nil || (sessionID != "" && job.sessionID != sessionID) {
		s.mu.Unlock()
		return deploymentSnapshot{}, false
	}
	if job.snapshot.Status == deploymentStatusPending {
		job.cancel()
	}
	snapshot := job.snapshot
	s.mu.Unlock()
	return snapshot, true
}

func (s *deploymentStore) installedHash(serial, packageName string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.installed[serial+"\n"+packageName]
}

func (s *deploymentStore) markInstalled(serial, packageName, hash string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.installed[serial+"\n"+packageName] = hash
}

func (s *deploymentStore) trimLocked(limit int) {
	if len(s.jobs) <= limit {
		return
	}
	var oldestID string
	var oldest time.Time
	for id, job := range s.jobs {
		if job.snapshot.Status == deploymentStatusPending {
			continue
		}
		updated, _ := time.Parse(time.RFC3339Nano, job.snapshot.UpdatedAt)
		if oldestID == "" || updated.Before(oldest) {
			oldestID, oldest = id, updated
		}
	}
	if oldestID != "" {
		delete(s.jobs, oldestID)
	}
}

func artifactSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open artifact: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash artifact: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s Server) runDeployment(
	ctx context.Context,
	request deploymentRequest,
	artifact string,
	artifactHash string,
	update func(string),
) (bool, error) {
	update("preparing")
	if err := s.ADB.ReverseWebPreview(ctx, request.Serial, request.WebURL); err != nil {
		return false, fmt.Errorf("configure device H5 reverse tunnel: %w", err)
	}
	skipped := false
	if s.deployments.installedHash(request.Serial, s.Package) == artifactHash {
		installed, err := s.ADB.PackageInstalled(ctx, request.Serial, s.Package)
		if err == nil && installed {
			skipped = true
		}
	}
	if !skipped {
		update("installing")
		if err := s.ADB.Install(ctx, request.Serial, artifact); err != nil {
			return false, err
		}
		s.deployments.markInstalled(request.Serial, s.Package, artifactHash)
	}
	update("starting")
	if err := s.ADB.Start(ctx, request.Serial, s.Component, request.WebURL); err != nil {
		return skipped, err
	}
	update("connecting")
	return skipped, nil
}
