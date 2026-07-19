package http

import (
	"crypto/md5"
	"encoding/hex"
)

// md5Hex returns the lowercase hex md5 of the input. Used by the
// `accounts.json` endpoint to provide Gravatar hints.
func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
