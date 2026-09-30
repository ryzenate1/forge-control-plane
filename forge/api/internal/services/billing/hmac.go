package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// hmacSHA256Hex returns the lowercase hex HMAC-SHA256 of body with key.
func hmacSHA256Hex(body []byte, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// hmacConstantTimeEqual compares two hex digests in constant time.
func hmacConstantTimeEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}