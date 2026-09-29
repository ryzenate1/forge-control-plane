package tokens

import (
	"sync"
)

// maxDeniedIdentities bounds how many revocation entries the denylist holds.
// The list only ever contains denials, so an entry that had to be dropped can
// only mean a revoked principal might now be treated as allowed: overflowing
// trips the same degraded flag as the ticket store and every lookup answers
// "denied".
const maxDeniedIdentities = 65536

// anyServer is the key used for a principal revoked on every server.
const anyServer = "*"

// WebSocketDenylist records principals whose stream access has been revoked.
// It is the revocation side of the ticket layer: a ticket that has not expired
// still must not open a stream for a user who has since been deauthorized.
type WebSocketDenylist struct {
	mu       sync.RWMutex
	byServer map[string]map[string]bool
	lost     bool
}

func NewWebSocketDenylist() *WebSocketDenylist {
	return &WebSocketDenylist{
		byServer: make(map[string]map[string]bool),
	}
}

// DenyForServer revokes userID's stream access on one server. It reports false
// when the entry could not be recorded, so a caller never reports a revocation
// that did not happen.
func (d *WebSocketDenylist) DenyForServer(serverID, userID string) bool {
	return d.deny(serverID, userID)
}

// DenyUser revokes userID's stream access on every server, which is what a
// "deauthorize this user" action means. An empty user id is refused rather than
// treated as a wildcard that denies everyone.
func (d *WebSocketDenylist) DenyUser(userID string) bool {
	return d.deny(anyServer, userID)
}

func (d *WebSocketDenylist) deny(serverID, userID string) bool {
	if d == nil {
		return false
	}
	serverID, userID = NormalizeID(serverID), NormalizeID(userID)
	if serverID == "" || userID == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.byServer == nil {
		d.byServer = make(map[string]map[string]bool)
	}
	if d.countLocked() >= maxDeniedIdentities {
		// Refusing the eviction of a denial is what keeps the overflow
		// honest: the flag turns every later lookup into a denial.
		d.lost = true
		return false
	}
	if d.byServer[serverID] == nil {
		d.byServer[serverID] = make(map[string]bool)
	}
	d.byServer[serverID][userID] = true
	return true
}

// AllowForServer lifts a single-server revocation.
func (d *WebSocketDenylist) AllowForServer(serverID, userID string) {
	d.allow(serverID, userID)
}

// AllowUser lifts a node-wide revocation.
func (d *WebSocketDenylist) AllowUser(userID string) {
	d.allow(anyServer, userID)
}

func (d *WebSocketDenylist) allow(serverID, userID string) {
	if d == nil {
		return
	}
	serverID, userID = NormalizeID(serverID), NormalizeID(userID)
	d.mu.Lock()
	defer d.mu.Unlock()
	users, ok := d.byServer[serverID]
	if !ok {
		return
	}
	delete(users, userID)
	if len(users) == 0 {
		delete(d.byServer, serverID)
	}
}

// IsDenied reports whether the pair has been revoked, either for this server
// specifically or node-wide. A denylist that overflowed answers denied for
// every pair: a lost entry was a denial, and "unknown" is not "allowed".
func (d *WebSocketDenylist) IsDenied(serverID, userID string) bool {
	if d == nil {
		return true
	}
	serverID, userID = NormalizeID(serverID), NormalizeID(userID)
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.lost {
		return true
	}
	if userID == "" {
		// A stream that cannot be attributed to a principal cannot be checked
		// against the revocation list, so it is not allowed through.
		return true
	}
	if users, ok := d.byServer[anyServer]; ok && users[userID] {
		return true
	}
	if serverID == "" {
		return true
	}
	users, exists := d.byServer[serverID]
	if !exists {
		return false
	}
	return users[userID]
}

// LostEntries reports whether the denylist overflowed and can no longer answer
// revocation questions for the entries it had to drop.
func (d *WebSocketDenylist) LostEntries() bool {
	if d == nil {
		return true
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lost
}

// Reset clears every revocation. It is only safe at boot, when the process has
// no live streams to revoke; the degraded flag is cleared with the entries
// because nothing issued by this process is outstanding any more.
func (d *WebSocketDenylist) Reset() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.byServer = make(map[string]map[string]bool)
	d.lost = false
}

func (d *WebSocketDenylist) countLocked() int {
	total := 0
	for _, users := range d.byServer {
		total += len(users)
	}
	return total
}

// Len reports how many revocations are recorded, for dashboards and alerts.
func (d *WebSocketDenylist) Len() int {
	if d == nil {
		return 0
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.countLocked()
}
