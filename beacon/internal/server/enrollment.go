package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

// maxEnrollRequestBytes bounds the JSON body an enrollment request may carry.
const maxEnrollRequestBytes = 64 << 10

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

	nodeID = strings.TrimSpace(nodeID)
	// An empty node id would key every such token to "" and let any of them
	// enroll; a non-positive ttl would create a token that is already expired
	// yet still reported as issued. Reject both instead of issuing a credential
	// that cannot be honoured.
	if nodeID == "" {
		return nil, errors.New("node id is required to issue an enrollment token")
	}
	if ttl <= 0 {
		return nil, fmt.Errorf("enrollment token ttl must be positive, got %v", ttl)
	}

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
	// A token that only lives in this process is lost on restart, so the node
	// would present a credential the reloaded registry has never heard of.
	// Persistence is part of issuing the token: on failure the in-memory record
	// is withdrawn and the caller is told the token was not issued.
	if err := m.saveLocked(); err != nil {
		delete(m.tokens, tokenHash)
		delete(m.nodeIDs, nodeID)
		return nil, fmt.Errorf("persist enrollment token: %w", err)
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
	return m.saveLocked()
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
	return m.saveLocked()
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
	return m.saveLocked()
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
	return m.saveLocked()
}

func (m *EnrollmentManager) GetByNodeID(nodeID string) *EnrollmentToken {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.nodeIDs[nodeID]
}

func (m *EnrollmentManager) List() []*EnrollmentToken {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.listLocked()
}

// listLocked snapshots the registry. The caller must hold m.mu (read or write).
func (m *EnrollmentManager) listLocked() []*EnrollmentToken {
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
			pruned++
			delete(m.tokens, token)
			// Only drop the node index when it still points at this very token:
			// a node may hold a superseded record here while a newer token for
			// the same node is live, and deleting that entry would silently
			// un-enroll the node.
			if m.nodeIDs[et.NodeID] == et {
				delete(m.nodeIDs, et.NodeID)
			}
		}
	}
	if pruned > 0 {
		if err := m.saveLocked(); err != nil {
			log.Printf("[enrollment] pruned %d token(s) but could not persist the registry: %v", pruned, err)
		}
	}
	return pruned
}

// saveLocked writes the registry to disk. The caller must hold m.mu: sync.Mutex
// and sync.RWMutex are not reentrant, so taking the lock here (directly or
// through List) would deadlock the caller's goroutine and every later enrollment
// request with it.
func (m *EnrollmentManager) saveLocked() error {
	if m.storageDir == "" {
		return errors.New("enrollment storage directory is not configured")
	}
	if err := os.MkdirAll(m.storageDir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(m.storageDir, "enrollment_tokens.json")
	data := struct {
		Tokens  []*EnrollmentToken `json:"tokens"`
		Updated time.Time          `json:"updated"`
	}{
		Tokens:  m.listLocked(),
		Updated: time.Now().UTC(),
	}
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	// The registry is credential state: write it through a temporary file and
	// rename, so an interrupted write cannot leave a half-written registry that
	// load() then reads as "no tokens issued".
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	success := false
	defer func() {
		_ = temp.Close()
		if !success {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temp.Write(body); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	success = true
	return nil
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
	// Bound the request body before decoding: this endpoint accepts a credential
	// and an unbounded body would let a caller pin memory here.
	if err := json.NewDecoder(io.LimitReader(r.Body, maxEnrollRequestBytes)).Decode(&body); err != nil {
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

	// A pending token is not an approval. Enrolling before an operator
	// approves would let any holder of a freshly generated token join.
	if et.State != EnrollmentApproved {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"enrolled": false,
			"reason":   fmt.Sprintf("enrollment token state is %s; approval is required", et.State),
		})
		return
	}

	compat := CheckVersionCompatibility(body.BeaconVersion, "")
	if !compat.Compatible {
		// An enrollment that did not happen is a failure, not a 200: a caller
		// that only reads the status code must not be able to mark this node
		// enrolled. The reason stays in the body for the operator.
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
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
	// The status endpoint is a token oracle if a bare nodeId answers with
	// enrollment state: node authentication (HMAC, enforced by middleware)
	// plus the token itself are both required. A nodeId may additionally be
	// supplied but must match the token's binding.
	if token == "" {
		writeError(w, http.StatusBadRequest, "token query parameter required")
		return
	}
	et, err := s.enrollmentMgr.ValidateToken(token)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "reason": err.Error()})
		return
	}
	if nodeID != "" && nodeID != et.NodeID {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "reason": "token does not match node id"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":     et.State == EnrollmentApproved || et.State == EnrollmentPending,
		"state":     et.State,
		"nodeId":    et.NodeID,
		"expiresAt": et.ExpiresAt.Format(time.RFC3339),
		"createdAt": et.CreatedAt.Format(time.RFC3339),
	})
}
