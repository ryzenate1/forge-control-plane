package tokens

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// maxTracked bounds how many one-time entries are held. The window for a
// ticket is short, so the cap only needs to cover peak concurrent issuance.
const maxTracked = 65536

var ErrStoreFull = errors.New("one-time token store is at capacity")

// TokenStore records one-time token usage. Two independent states are kept:
// tickets this process issued (so a re-issue can be detected) and tickets
// already spent (so a replay is refused). Absence from "issued" is not
// authorization - a token minted by the panel is legitimately unknown here -
// but absence from "spent" is what makes a spend succeed, and a store that
// has dropped spent entries never claims an entry is unused.
type TokenStore struct {
	mu     sync.Mutex
	tokens map[string]time.Time
	spent  map[string]time.Time
	lost   bool
}

func NewTokenStore() *TokenStore {
	return &TokenStore{
		tokens: make(map[string]time.Time),
		spent:  make(map[string]time.Time),
	}
}

// IsValid reports whether uniqueID was issued here and has not expired. An
// empty or unknown id is invalid; an unreachable state is never "valid".
func (ts *TokenStore) IsValid(uniqueID string) bool {
	if ts == nil {
		return false
	}
	uniqueID = NormalizeID(uniqueID)
	if uniqueID == "" {
		return false
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()

	expiry, exists := ts.tokens[uniqueID]
	if !exists {
		return false
	}

	if !time.Now().Before(expiry) {
		delete(ts.tokens, uniqueID)
		return false
	}
	if ts.spent[uniqueID] != (time.Time{}) {
		return false
	}
	return true
}

// Consume validates and atomically removes a one-time token, marking it spent
// so later reads of the same id refuse it. The spend is remembered for as long
// as the token itself could have been presented.
func (ts *TokenStore) Consume(uniqueID string) bool {
	return ts.ConsumeUntil(uniqueID, time.Now().Add(absoluteMaxLifetime))
}

// ConsumeUntil spends a ticket and records the spend only until the moment the
// credential itself stops being usable. Holding spent entries longer than the
// token's lifetime fills the store with entries that can never be replayed,
// which eventually trips the overflow breaker and denies every ticket.
func (ts *TokenStore) ConsumeUntil(uniqueID string, until time.Time) bool {
	if ts == nil {
		return false
	}
	uniqueID = NormalizeID(uniqueID)
	if uniqueID == "" {
		return false
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()

	now := time.Now()
	if entry, seen := ts.spent[uniqueID]; seen && now.Before(entry) {
		return false
	}
	expiry, exists := ts.tokens[uniqueID]
	if !exists {
		// A token minted elsewhere (the panel) is legitimately unknown here.
		// The signature already authenticated it; record the spend so the
		// replay is refused regardless of where it came from.
		expiry = until
	}
	if !expiry.After(now) {
		if exists {
			delete(ts.tokens, uniqueID)
		}
		return false
	}
	if ts.spent == nil {
		ts.spent = make(map[string]time.Time)
	}
	if len(ts.spent) >= maxTracked {
		ts.sweepSpentLocked(now)
		if len(ts.spent) >= maxTracked {
			// Cannot record the spend, so cannot guarantee single use: deny.
			ts.lost = true
			return false
		}
	}
	delete(ts.tokens, uniqueID)
	ts.spent[uniqueID] = expiry
	return true
}

// Redeemed reports whether a ticket was already spent. When the store has had
// to drop spent entries, every id is treated as redeemed: an incomplete
// revocation list must not be read as "clean".
func (ts *TokenStore) Redeemed(uniqueID string) bool {
	if ts == nil {
		return true
	}
	uniqueID = NormalizeID(uniqueID)
	if uniqueID == "" {
		return true
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.lost {
		return true
	}
	expiry, seen := ts.spent[uniqueID]
	if !seen {
		return false
	}
	if !time.Now().Before(expiry) {
		delete(ts.spent, uniqueID)
		return false
	}
	return true
}

// Add records an issued ticket. It returns false when the entry could not be
// stored, so a caller never reports success for a ticket that is not
// actually tracked.
func (ts *TokenStore) Add(uniqueID string, expiry time.Time) bool {
	if ts == nil {
		return false
	}
	uniqueID = NormalizeID(uniqueID)
	if uniqueID == "" || !time.Now().Before(expiry) {
		return false
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.tokens == nil {
		ts.tokens = make(map[string]time.Time)
	}
	if len(ts.tokens) >= maxTracked {
		ts.sweepLocked(time.Now())
		if len(ts.tokens) >= maxTracked {
			ts.lost = true
			return false
		}
	}
	ts.tokens[uniqueID] = expiry
	return true
}

func (ts *TokenStore) Cleanup() {
	if ts == nil {
		return
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	now := time.Now()
	ts.sweepLocked(now)
	ts.sweepSpentLocked(now)
}

func (ts *TokenStore) sweepLocked(now time.Time) {
	for id, expiry := range ts.tokens {
		if !now.Before(expiry) {
			delete(ts.tokens, id)
		}
	}
}

func (ts *TokenStore) sweepSpentLocked(now time.Time) {
	for id, expiry := range ts.spent {
		if !now.Before(expiry) {
			delete(ts.spent, id)
		}
	}
}

// LostEntries reports whether the store overflowed and can no longer answer
// replay questions reliably. Callers must treat this as degraded.
func (ts *TokenStore) LostEntries() bool {
	if ts == nil {
		return true
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.lost
}

// NormalizeID trims a ticket id before it is used as a store key so that
// surrounding whitespace cannot produce two identities for one ticket.
func NormalizeID(id string) string { return strings.TrimSpace(id) }
