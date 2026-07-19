package tokens

import (
	"sync"
	"time"
)

type TokenStore struct {
	mu     sync.Mutex
	tokens map[string]time.Time
}

func NewTokenStore() *TokenStore {
	return &TokenStore{
		tokens: make(map[string]time.Time),
	}
}

func (ts *TokenStore) IsValid(uniqueID string) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	expiry, exists := ts.tokens[uniqueID]
	if !exists {
		return false
	}

	if time.Now().After(expiry) {
		delete(ts.tokens, uniqueID)
		return false
	}

	delete(ts.tokens, uniqueID)
	return true
}

func (ts *TokenStore) Add(uniqueID string, expiry time.Time) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.tokens[uniqueID] = expiry
}

func (ts *TokenStore) Cleanup() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	now := time.Now()
	for id, expiry := range ts.tokens {
		if now.After(expiry) {
			delete(ts.tokens, id)
		}
	}
}
