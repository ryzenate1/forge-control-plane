package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type EnrollmentState string

const (
	EnrollmentPending  EnrollmentState = "pending"
	EnrollmentApproved EnrollmentState = "approved"
	EnrollmentRejected EnrollmentState = "rejected"
	EnrollmentRevoked  EnrollmentState = "revoked"
	EnrollmentExpired  EnrollmentState = "expired"
)

// EnrollmentToken represents a single enrollment token's lifecycle state.
//
// Token holds the plaintext token value. It is only ever kept in memory
// (for the lifetime of the process) and is never persisted to disk: it is
// tagged json:"-" so it is excluded from marshaling. Only TokenHash (the
// hex-encoded SHA-256 digest of the token) is written to
// enrollment_tokens.json. Incoming tokens are validated by hashing them
// and comparing against TokenHash using a constant-time comparison.
type EnrollmentToken struct {
	Token      string          `json:"-"`
	TokenHash  string          `json:"tokenHash"`
	NodeID     string          `json:"nodeId"`
	CreatedAt  time.Time       `json:"createdAt"`
	ExpiresAt  time.Time       `json:"expiresAt"`
	State      EnrollmentState `json:"state"`
	ApprovedBy string          `json:"approvedBy,omitempty"`
	Reason     string          `json:"reason,omitempty"`
}

// hashToken returns the hex-encoded SHA-256 digest of a raw enrollment
// token. Only this digest is ever persisted to disk.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// tokensEqual performs a constant-time comparison of two hex-encoded
// token hashes.
func tokensEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// EnrollmentManager keeps tokens indexed by their SHA-256 hash (not the
// plaintext value), so that only hashes ever need to be written to disk.
type EnrollmentManager struct {
	mu         sync.RWMutex
	tokens     map[string]*EnrollmentToken // keyed by TokenHash
	nodeIDs    map[string]*EnrollmentToken
	storageDir string
}

func NewEnrollmentManager(storageDir string) *EnrollmentManager {
	mgr := &EnrollmentManager{
		tokens:     make(map[string]*EnrollmentToken),
		nodeIDs:    make(map[string]*EnrollmentToken),
		storageDir: storageDir,
	}
	mgr.load()
	return mgr
}

func (m *EnrollmentManager) GenerateToken(nodeID string, ttl time.Duration) (*EnrollmentToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.nodeIDs[nodeID]; ok {
		if existing.State == EnrollmentApproved || existing.State == EnrollmentPending {
			return nil, fmt.Errorf("node %s already has an active enrollment", nodeID)
		}
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	tokenHash := hashToken(token)

	now := time.Now().UTC()
	et := &EnrollmentToken{
		Token:     token,
		TokenHash: tokenHash,
		NodeID:    nodeID,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
		State:     EnrollmentPending,
	}
	m.tokens[tokenHash] = et
	m.nodeIDs[nodeID] = et
	if err := m.save(); err != nil {
		log.Printf("[enrollment] failed to persist: %v", err)
	}
	return et, nil
}

func (m *EnrollmentManager) ValidateToken(token string) (*EnrollmentToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tokenHash := hashToken(token)
	et, ok := m.tokens[tokenHash]
	if !ok || !tokensEqual(et.TokenHash, tokenHash) {
		return nil, errors.New("enrollment token not found")
	}
	if et.State == EnrollmentRevoked {
		return nil, errors.New("enrollment token has been revoked")
	}
	if et.State == EnrollmentExpired {
		return nil, errors.New("enrollment token has expired")
	}
	if time.Now().UTC().After(et.ExpiresAt) {
		return nil, errors.New("enrollment token has expired")
	}
	if et.State != EnrollmentApproved && et.State != EnrollmentPending {
		return nil, fmt.Errorf("enrollment token state is %s", et.State)
	}
	return et, nil
}

func (m *EnrollmentManager) Approve(token, approvedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	et, ok := m.tokens[hashToken(token)]
	if !ok {
		return errors.New("enrollment token not found")
	}
	if et.State != EnrollmentPending {
		return fmt.Errorf("cannot approve token in state %s", et.State)
	}
	et.State = EnrollmentApproved
	et.ApprovedBy = approvedBy
	return m.save()
}

func (m *EnrollmentManager) Reject(token, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	et, ok := m.tokens[hashToken(token)]
	if !ok {
		return errors.New("enrollment token not found")
	}
	if et.State != EnrollmentPending {
		return fmt.Errorf("cannot reject token in state %s", et.State)
	}
	et.State = EnrollmentRejected
	et.Reason = reason
	return m.save()
}

func (m *EnrollmentManager) Revoke(token, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	et, ok := m.tokens[hashToken(token)]
	if !ok {
		return errors.New("enrollment token not found")
	}
	et.State = EnrollmentRevoked
	et.Reason = reason
	return m.save()
}

func (m *EnrollmentManager) RevokeByNodeID(nodeID, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	et, ok := m.nodeIDs[nodeID]
	if !ok {
		return errors.New("node not found in enrollment registry")
	}
	et.State = EnrollmentRevoked
	et.Reason = reason
	return m.save()
}

func (m *EnrollmentManager) GetByNodeID(nodeID string) *EnrollmentToken {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.nodeIDs[nodeID]
}

func (m *EnrollmentManager) List() []*EnrollmentToken {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*EnrollmentToken, 0, len(m.tokens))
	for _, et := range m.tokens {
		result = append(result, et)
	}
	return result
}

func (m *EnrollmentManager) PruneExpired() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	pruned := 0
	for token, et := range m.tokens {
		if now.After(et.ExpiresAt) && et.State != EnrollmentApproved {
			et.State = EnrollmentExpired
			pruned++
			delete(m.tokens, token)
			delete(m.nodeIDs, et.NodeID)
		}
	}
	if pruned > 0 {
		_ = m.save()
	}
	return pruned
}

func (m *EnrollmentManager) save() error {
	if m.storageDir == "" {
		return nil
	}
	if err := os.MkdirAll(m.storageDir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(m.storageDir, "enrollment_tokens.json")
	data := struct {
		Tokens  []*EnrollmentToken `json:"tokens"`
		Updated time.Time          `json:"updated"`
	}{
		Tokens:  m.List(),
		Updated: time.Now().UTC(),
	}
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func (m *EnrollmentManager) load() {
	if m.storageDir == "" {
		return
	}
	path := filepath.Join(m.storageDir, "enrollment_tokens.json")
	body, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var data struct {
		Tokens  []*EnrollmentToken `json:"tokens"`
		Updated time.Time          `json:"updated"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		log.Printf("[enrollment] failed to load tokens: %v", err)
		return
	}
	for _, et := range data.Tokens {
		// Only TokenHash is ever persisted; plaintext tokens loaded from
		// disk are never available here (Token is not serialized), so
		// tokens can no longer be validated by their raw value after a
		// restart until re-issued. Index strictly by TokenHash.
		if et.TokenHash == "" {
			continue
		}
		m.tokens[et.TokenHash] = et
		m.nodeIDs[et.NodeID] = et
	}
}

func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Token         string `json:"token"`
		NodeID        string `json:"nodeId"`
		BeaconVersion string `json:"beaconVersion"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid enroll request")
		return
	}
	if body.Token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	if body.NodeID == "" {
		writeError(w, http.StatusBadRequest, "nodeId is required")
		return
	}

	et, err := s.enrollmentMgr.ValidateToken(body.Token)
	if err != nil {
		log.Printf("[beacon] enrollment rejected for node %s: %v", body.NodeID, err)
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"enrolled": false,
			"reason":   err.Error(),
		})
		return
	}

	if et.NodeID != body.NodeID {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"enrolled": false,
			"reason":   "token does not match node id",
		})
		return
	}

	compat := CheckVersionCompatibility(body.BeaconVersion, "")
	if !compat.Compatible {
		writeJSON(w, http.StatusOK, map[string]any{
			"enrolled":   false,
			"compatible": false,
			"reason":     compat.Message,
			"minVersion": compat.MinBeaconVersion,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"enrolled":     true,
		"nodeId":       body.NodeID,
		"compatible":   true,
		"capabilities": s.collectCapabilities(),
	})
}

func (s *Server) handleEnrollmentStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	nodeID := r.URL.Query().Get("nodeId")
	token := r.URL.Query().Get("token")
	if nodeID == "" && token == "" {
		writeError(w, http.StatusBadRequest, "nodeId or token query parameter required")
		return
	}
	var et *EnrollmentToken
	if token != "" {
		var err error
		et, err = s.enrollmentMgr.ValidateToken(token)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"valid": false, "reason": err.Error()})
			return
		}
	} else {
		et = s.enrollmentMgr.GetByNodeID(nodeID)
		if et == nil {
			writeJSON(w, http.StatusOK, map[string]any{"valid": false, "reason": "no enrollment found"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":     et.State == EnrollmentApproved || et.State == EnrollmentPending,
		"state":     et.State,
		"nodeId":    et.NodeID,
		"expiresAt": et.ExpiresAt.Format(time.RFC3339),
		"createdAt": et.CreatedAt.Format(time.RFC3339),
	})
}
