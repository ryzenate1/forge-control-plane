package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
)

// Sign returns the hex-encoded HMAC-SHA256 of data using the given secret.
func Sign(secret string, data []byte) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("HMAC secret must contain at least 32 bytes")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// SignB64 returns the URL-safe base64-encoded HMAC-SHA256 of data using the
// given secret.
func SignB64(secret string, data []byte) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("HMAC secret must contain at least 32 bytes")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// Equal performs a constant-time comparison of two signature strings.
func Equal(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}
