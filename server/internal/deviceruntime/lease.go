package deviceruntime

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const runtimeLeaseTTL = 5 * time.Minute

type runtimeLease struct {
	sessionID string
	expiresAt time.Time
}

type runtimeLeaseStore struct {
	mu        sync.Mutex
	bySerial  map[string]runtimeLease
	bySession map[string]string
}

func newRuntimeLeaseStore() *runtimeLeaseStore {
	return &runtimeLeaseStore{
		bySerial:  make(map[string]runtimeLease),
		bySession: make(map[string]string),
	}
}

func (s *runtimeLeaseStore) acquire(sessionID, serial string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", fmt.Errorf("preview session id is required")
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.bySerial[serial]; ok && existing.sessionID != sessionID && now.Before(existing.expiresAt) {
		return "", fmt.Errorf("device is leased by another preview session")
	}
	previous := s.bySession[sessionID]
	if previous != "" && previous != serial {
		delete(s.bySerial, previous)
	}
	s.bySerial[serial] = runtimeLease{sessionID: sessionID, expiresAt: now.Add(runtimeLeaseTTL)}
	s.bySession[sessionID] = serial
	return previous, nil
}

func (s *runtimeLeaseStore) restore(sessionID, previous, failedSerial string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current := s.bySerial[failedSerial]; current.sessionID == sessionID {
		delete(s.bySerial, failedSerial)
	}
	if previous == "" {
		delete(s.bySession, sessionID)
		return
	}
	s.bySerial[previous] = runtimeLease{sessionID: sessionID, expiresAt: time.Now().Add(runtimeLeaseTTL)}
	s.bySession[sessionID] = previous
}

func (s *runtimeLeaseStore) validate(sessionID, serial string) bool {
	if sessionID == "" {
		return false
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.bySerial[serial]
	if !ok || lease.sessionID != sessionID || now.After(lease.expiresAt) {
		if ok && now.After(lease.expiresAt) {
			delete(s.bySerial, serial)
			if s.bySession[lease.sessionID] == serial {
				delete(s.bySession, lease.sessionID)
			}
		}
		return false
	}
	lease.expiresAt = now.Add(runtimeLeaseTTL)
	s.bySerial[serial] = lease
	return true
}

func (s Server) authorizeDeviceLease(w http.ResponseWriter, r *http.Request, serial string) bool {
	sessionID := runtimeSessionID(r)
	if sessionID == "" && (validBearer(r, s.AccessToken) || (s.AccessToken == "" && !isBrowserRequest(r) && isLoopbackRemote(r.RemoteAddr))) {
		return true
	}
	if s.leases.validate(sessionID, serial) {
		return true
	}
	writeJSON(w, http.StatusConflict, map[string]string{"error": "device lease does not belong to this preview session"})
	return false

}
func (s Server) acquireDeviceLease(w http.ResponseWriter, r *http.Request, serial string) bool {
	sessionID := runtimeSessionID(r)
	if sessionID == "" && (validBearer(r, s.AccessToken) || (s.AccessToken == "" && !isBrowserRequest(r) && isLoopbackRemote(r.RemoteAddr))) {
		return true
	}
	if _, err := s.leases.acquire(sessionID, serial); err == nil {
		return true
	} else {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return false
	}
}
