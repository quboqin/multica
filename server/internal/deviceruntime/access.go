package deviceruntime

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const runtimeAccessHeader = "X-Multica-Device-Token"
const runtimeSessionHeader = "X-Multica-Preview-Session"

type runtimeAccessClaim struct {
	sessionID string
	expiresAt time.Time
}

type runtimeAccessStore struct {
	mu     sync.Mutex
	tokens map[string]runtimeAccessClaim
}

func newRuntimeAccessStore() *runtimeAccessStore {
	return &runtimeAccessStore{tokens: make(map[string]runtimeAccessClaim)}
}

func (s *runtimeAccessStore) mint(sessionID string) string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		panic("generate Device Runtime access token: " + err.Error())
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for existing, claim := range s.tokens {
		if now.After(claim.expiresAt) {
			delete(s.tokens, existing)
		}
	}
	s.tokens[token] = runtimeAccessClaim{sessionID: strings.TrimSpace(sessionID), expiresAt: now.Add(4 * time.Hour)}
	return token
}

func (s *runtimeAccessStore) claim(token string) (runtimeAccessClaim, bool) {
	if token == "" {
		return runtimeAccessClaim{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	claim, ok := s.tokens[token]
	if !ok || time.Now().After(claim.expiresAt) {
		delete(s.tokens, token)
		return runtimeAccessClaim{}, false
	}
	return claim, true
}

type runtimeSessionContextKey struct{}

func (s Server) protectAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if claim, ok := s.access.claim(strings.TrimSpace(r.Header.Get(runtimeAccessHeader))); ok {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), runtimeSessionContextKey{}, claim.sessionID)))
			return
		}
		if validBearer(r, s.AccessToken) {
			sessionID := strings.TrimSpace(r.Header.Get(runtimeSessionHeader))
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), runtimeSessionContextKey{}, sessionID)))
			return
		}
		// Native clients remain compatible on loopback when no host secret is
		// configured. Browser requests always require a console-minted token.
		if s.AccessToken == "" && !isBrowserRequest(r) && isLoopbackRemote(r.RemoteAddr) {
			next.ServeHTTP(w, r)
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Device Runtime authorization required"})
	})
}

func validBearer(r *http.Request, expected string) bool {
	if expected == "" {
		return false
	}
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	provided, ok := strings.CutPrefix(value, "Bearer ")
	return ok && len(provided) == len(expected) && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func runtimeSessionID(r *http.Request) string {
	value, _ := r.Context().Value(runtimeSessionContextKey{}).(string)
	return strings.TrimSpace(value)
}

func isBrowserRequest(r *http.Request) bool {
	return r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != ""
}

func isLoopbackRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return false
	}
	return net.ParseIP(host).IsLoopback()
}
