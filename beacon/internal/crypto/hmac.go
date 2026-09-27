package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MinSecretBytes is the smallest key accepted for a panel/daemon HMAC. A
// shorter key is a misconfiguration: it is brute-forceable, so signing and
// verification both refuse it rather than silently producing a weak MAC.
const MinSecretBytes = 32

// MaxSkew bounds how far a signed request's timestamp may drift from local
// time. Anything outside this window is rejected.
const MaxSkew = 5 * time.Minute

// NonceLen is the required hex length of a replay nonce (16 random bytes).
const NonceLen = 32

var (
	ErrWeakSecret     = errors.New("HMAC secret must contain at least 32 bytes")
	ErrBadSecret      = errors.New("HMAC secret must not be empty")
	ErrBadSignature   = errors.New("invalid signature")
	ErrBadTimestamp   = errors.New("invalid signature timestamp")
	ErrBadNonce       = errors.New("invalid or replayed nonce")
	ErrSignatureShort = errors.New("signature must not be empty")
)

// domain separates this construction from any other HMAC over the same key.
const domain = "forge-beacon-v1\n"

// Sign returns the hex-encoded HMAC-SHA256 of data using the given secret.
func Sign(secret string, data []byte) (string, error) {
	sum, err := rawSum(secret, data)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sum), nil
}

// SignB64 returns the URL-safe base64-encoded HMAC-SHA256 of data using the
// given secret.
func SignB64(secret string, data []byte) (string, error) {
	sum, err := rawSum(secret, data)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(sum), nil
}

func rawSum(secret string, data []byte) ([]byte, error) {
	if secret == "" {
		return nil, ErrBadSecret
	}
	if len(secret) < MinSecretBytes {
		return nil, ErrWeakSecret
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return mac.Sum(nil), nil
}

// Equal performs a constant-time comparison of two signature strings.
//
// hmac.Equal rejects a length mismatch before comparing, which is correct for
// MACs but leaks nothing usable here: both operands are fixed-width digests
// once encoded. Callers must treat a false result as "deny"; it never means
// "unknown, allow".
func Equal(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return hmac.Equal([]byte(a), []byte(b))
}

// CanonicalString builds the MAC input for a signed request. Every field is
// length-prefixed so bytes cannot migrate across a field boundary: an
// attacker who can influence the URI or the nonce must not be able to shift
// a "\n" and re-use a signature issued for a different request.
func CanonicalString(method, requestURI, timestamp string, body []byte, nonce string) []byte {
	var out []byte
	out = append(out, domain...)
	for _, field := range []string{method, requestURI, timestamp, nonce} {
		out = strconv.AppendInt(out, int64(len(field)), 10)
		out = append(out, ':')
		out = append(out, field...)
	}
	out = strconv.AppendInt(out, int64(len(body)), 10)
	out = append(out, ':')
	out = append(out, body...)
	return out
}

// ValidTimestamp reports whether an RFC3339 timestamp is inside the allowed
// skew window. An unparseable or empty timestamp is invalid, never accepted.
func ValidTimestamp(timestamp string, now time.Time) bool {
	timestamp = strings.TrimSpace(timestamp)
	if timestamp == "" {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return false
	}
	drift := now.Sub(parsed)
	if drift < 0 {
		drift = -drift
	}
	return drift <= MaxSkew
}

// ValidNonce reports whether a nonce is well formed. Format validation is
// separate from replay storage so a caller cannot skip one by accident.
func ValidNonce(nonce string) bool {
	if len(nonce) != NonceLen {
		return false
	}
	if _, err := hex.DecodeString(nonce); err != nil {
		return false
	}
	return true
}

// SignRequest returns the hex signature for a request. It is the counterpart
// of VerifyRequest so the panel and the daemon cannot drift apart.
func SignRequest(secret, method, requestURI, timestamp string, body []byte, nonce string) (string, error) {
	sum, err := rawSum(secret, CanonicalString(method, requestURI, timestamp, body, nonce))
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sum), nil
}

// VerifyRequest checks a request signature against method, URI, timestamp,
// nonce and body. It fails closed on every unusable input: short secret, bad
// timestamp, malformed nonce, empty signature, length mismatch.
func VerifyRequest(secret, method, requestURI, timestamp, nonce, provided string, body []byte) error {
	if len(secret) < MinSecretBytes {
		if secret == "" {
			return ErrBadSecret
		}
		return ErrWeakSecret
	}
	if !ValidNonce(nonce) {
		return ErrBadNonce
	}
	if !ValidTimestamp(timestamp, time.Now()) {
		return ErrBadTimestamp
	}
	if provided == "" {
		return ErrSignatureShort
	}
	want, err := rawSum(secret, CanonicalString(method, requestURI, timestamp, body, nonce))
	if err != nil {
		return err
	}
	got, err := decodeSignature(provided)
	if err != nil {
		return ErrBadSignature
	}
	// Both operands are 32 bytes after decoding, so the constant-time
	// comparison cannot be short-circuited by a length difference; the
	// explicit length guard only rejects truncated encodings.
	if len(got) != len(want) || subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrBadSignature
	}
	return nil
}

// decodeSignature accepts either hex or unpadded base64url encodings of the
// digest, rejecting anything that is not exactly one SHA-256 MAC.
func decodeSignature(provided string) ([]byte, error) {
	provided = strings.TrimSpace(provided)
	if provided == "" {
		return nil, ErrSignatureShort
	}
	if got, err := hex.DecodeString(provided); err == nil && len(got) == sha256.Size {
		return got, nil
	}
	got, err := base64.RawURLEncoding.DecodeString(provided)
	if err != nil || len(got) != sha256.Size {
		return nil, ErrBadSignature
	}
	return got, nil
}

// NonceCache stores seen nonces for the replay window. A missing entry means
// "not seen yet"; a full cache never silently accepts a nonce it could not
// record, and never evicts an entry that is still inside the window.
type NonceCache struct {
	mu       sync.Mutex
	window   time.Duration
	entries  map[string]time.Time
	maxEntry int
	overflow bool
}

func NewNonceCache(maxEntries int, window time.Duration) *NonceCache {
	if maxEntries <= 0 {
		maxEntries = 8192
	}
	if window <= 0 || window > MaxSkew {
		window = MaxSkew
	}
	return &NonceCache{
		window:   window,
		entries:  make(map[string]time.Time, 64),
		maxEntry: maxEntries,
	}
}

// Use records nonce and reports whether it had not been seen. An invalid
// nonce, or one the cache has no room for, is rejected.
func (c *NonceCache) Use(nonce string, issued time.Time) bool {
	if c == nil || !ValidNonce(nonce) {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]time.Time, 64)
	}
	now := time.Now()
	c.sweepLocked(now)
	if _, seen := c.entries[nonce]; seen {
		return false
	}
	if len(c.entries) >= c.maxEntry {
		// Every slot is a live nonce: evicting one would re-open a replay
		// window, so refuse the new request instead.
		c.overflow = true
		return false
	}
	expires := issued.Add(c.window)
	if !expires.After(now) {
		expires = now.Add(c.window)
	}
	c.entries[nonce] = expires
	return true
}

// Overflowed reports whether the cache had to reject nonces for lack of room.
func (c *NonceCache) Overflowed() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.overflow
}

func (c *NonceCache) sweepLocked(now time.Time) {
	for nonce, expires := range c.entries {
		if !expires.After(now) {
			delete(c.entries, nonce)
		}
	}
}
