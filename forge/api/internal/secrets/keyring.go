package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

const envelopePrefix = "forge:v1:"

var (
	ErrInvalidKey        = errors.New("master key must be exactly 32 bytes encoded as strict base64 or 64 hexadecimal characters")
	ErrMalformedEnvelope = errors.New("malformed encrypted secret")
	ErrUnknownKey        = errors.New("encrypted secret references an unavailable key")
)

type Keyring struct {
	activeID string
	keys     map[string][]byte
	random   io.Reader
}

func ParseKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if len(value) == 64 {
		decoded, err := hex.DecodeString(value)
		if err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return nil, ErrInvalidKey
	}
	return decoded, nil
}

func New(activeID, activeKey string, previous map[string]string) (*Keyring, error) {
	activeID = strings.TrimSpace(activeID)
	if activeID == "" || strings.Contains(activeID, ":") {
		return nil, errors.New("master key ID must be non-empty and cannot contain ':'")
	}
	key, err := ParseKey(activeKey)
	if err != nil {
		return nil, fmt.Errorf("active master key: %w", err)
	}
	ring := &Keyring{activeID: activeID, keys: map[string][]byte{activeID: key}, random: rand.Reader}
	for id, encoded := range previous {
		id = strings.TrimSpace(id)
		if id == "" || strings.Contains(id, ":") || id == activeID {
			return nil, errors.New("previous master key IDs must be unique, non-empty, and cannot contain ':'")
		}
		parsed, err := ParseKey(encoded)
		if err != nil {
			return nil, fmt.Errorf("previous master key %q: %w", id, err)
		}
		ring.keys[id] = parsed
	}
	return ring, nil
}

func (k *Keyring) ActiveKeyID() string {
	if k == nil {
		return ""
	}
	return k.activeID
}

func (k *Keyring) Encrypt(plaintext []byte, aad string) (string, error) {
	if k == nil {
		return "", errors.New("secret encryption is not configured")
	}
	block, err := aes.NewCipher(k.keys[k.activeID])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(k.random, nonce); err != nil {
		return "", fmt.Errorf("generate encryption nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, plaintext, []byte(aad))
	payload := append(nonce, sealed...)
	return envelopePrefix + k.activeID + ":" + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (k *Keyring) Decrypt(envelope, aad string) ([]byte, error) {
	if k == nil {
		return nil, errors.New("secret encryption is not configured")
	}
	if !strings.HasPrefix(envelope, envelopePrefix) {
		return nil, ErrMalformedEnvelope
	}
	rest := strings.TrimPrefix(envelope, envelopePrefix)
	parts := strings.SplitN(rest, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, ErrMalformedEnvelope
	}
	key, ok := k.keys[parts[0]]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownKey, parts[0])
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil {
		return nil, ErrMalformedEnvelope
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(payload) < gcm.NonceSize()+gcm.Overhead() {
		return nil, ErrMalformedEnvelope
	}
	plaintext, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], []byte(aad))
	if err != nil {
		return nil, errors.New("encrypted secret authentication failed")
	}
	return plaintext, nil
}

func (k *Keyring) NeedsRotation(envelope string) bool {
	return k != nil && !strings.HasPrefix(envelope, envelopePrefix+k.activeID+":")
}
