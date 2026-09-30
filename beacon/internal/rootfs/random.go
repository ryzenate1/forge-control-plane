package rootfs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func randomName() (string, error) {
	var body [12]byte
	if _, err := rand.Read(body[:]); err != nil {
		return "", fmt.Errorf("generate secure temporary name: %w", err)
	}
	return hex.EncodeToString(body[:]), nil
}
