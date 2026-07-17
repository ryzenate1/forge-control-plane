package rootfs

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

func randomName() string {
	var body [12]byte
	if _, err := rand.Read(body[:]); err == nil {
		return hex.EncodeToString(body[:])
	}
	return time.Now().UTC().Format("20060102150405.000000000")
}
