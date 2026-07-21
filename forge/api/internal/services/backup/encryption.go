package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const purposeEncryptionKey = "gamepanel-backup-encryption"

const (
	aesKeySize    = 32
	nonceSize     = 12
	encryptionOverhead = nonceSize + 16
)

func GetEncryptionKeyFromEnv() []byte {
	key, err := parseMasterKey(os.Getenv("FORGE_MASTER_KEY"))
	if err == nil {
		return key
	}
	return nil
}

func parseMasterKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("FORGE_MASTER_KEY is empty")
	}
	if len(raw) == 64 {
		decoded, err := hex.DecodeString(raw)
		if err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 32 {
		return nil, errors.New("FORGE_MASTER_KEY must be 32 bytes as hex (64 chars) or base64")
	}
	return decoded, nil
}

func GenerateEncryptionKey() ([]byte, error) {
	key := make([]byte, aesKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate encryption key: %w", err)
	}
	return key, nil
}

func validateEncryptionKey(key []byte) error {
	if len(key) == 0 {
		return errors.New("encryption key is empty")
	}
	if len(key) != aesKeySize {
		return fmt.Errorf("encryption key must be %d bytes, got %d", aesKeySize, len(key))
	}
	return nil
}

func generateNonce() ([]byte, error) {
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return nonce, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new GCM: %w", err)
	}
	return gcm, nil
}

// deriveEncryptionKey derives a per-purpose key using HKDF-SHA256, following
// the Kopia reference model for key separation.
func deriveEncryptionKey(masterKey []byte, purpose string) ([]byte, error) {
	if len(masterKey) == 0 {
		return nil, errors.New("master key is empty")
	}
	derived, err := hkdf.Key(sha256.New, masterKey, nil, purpose, aesKeySize)
	if err != nil {
		return nil, fmt.Errorf("hkdf derive: %w", err)
	}
	return derived, nil
}

func Encrypt(data []byte, key []byte) ([]byte, error) {
	if err := validateEncryptionKey(key); err != nil {
		return nil, err
	}
	derivedKey, err := deriveEncryptionKey(key, purposeEncryptionKey)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(derivedKey)
	if err != nil {
		return nil, err
	}
	nonce, err := generateNonce()
	if err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, data, nil)
	out := make([]byte, nonceSize+len(ciphertext))
	copy(out[:nonceSize], nonce)
	copy(out[nonceSize:], ciphertext)
	return out, nil
}

func Decrypt(data []byte, key []byte) ([]byte, error) {
	if err := validateEncryptionKey(key); err != nil {
		return nil, err
	}
	if len(data) < nonceSize+16 {
		return nil, errors.New("ciphertext too short")
	}
	derivedKey, err := deriveEncryptionKey(key, purposeEncryptionKey)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(derivedKey)
	if err != nil {
		return nil, err
	}
	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

func EncryptReader(r io.Reader, key []byte) (io.Reader, error) {
	if err := validateEncryptionKey(key); err != nil {
		return nil, err
	}
	derivedKey, err := deriveEncryptionKey(key, purposeEncryptionKey)
	if err != nil {
		return nil, err
	}
	input, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read input for encryption: %w", err)
	}
	encrypted, err := encryptWithKey(input, derivedKey)
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	go func() {
		_, writeErr := pw.Write(encrypted)
		if writeErr != nil {
			pw.CloseWithError(writeErr)
		} else {
			pw.Close()
		}
	}()
	return pr, nil
}

func DecryptReader(r io.Reader, key []byte) (io.Reader, error) {
	if err := validateEncryptionKey(key); err != nil {
		return nil, err
	}
	derivedKey, err := deriveEncryptionKey(key, purposeEncryptionKey)
	if err != nil {
		return nil, err
	}
	input, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read input for decryption: %w", err)
	}
	decrypted, err := decryptWithKey(input, derivedKey)
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	go func() {
		_, writeErr := pw.Write(decrypted)
		if writeErr != nil {
			pw.CloseWithError(writeErr)
		} else {
			pw.Close()
		}
	}()
	return pr, nil
}

// encryptWithKey is the low-level encrypt using the already-derived key.
func encryptWithKey(data []byte, derivedKey []byte) ([]byte, error) {
	gcm, err := newGCM(derivedKey)
	if err != nil {
		return nil, err
	}
	nonce, err := generateNonce()
	if err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, data, nil)
	out := make([]byte, nonceSize+len(ciphertext))
	copy(out[:nonceSize], nonce)
	copy(out[nonceSize:], ciphertext)
	return out, nil
}

// decryptWithKey is the low-level decrypt using the already-derived key.
func decryptWithKey(data []byte, derivedKey []byte) ([]byte, error) {
	if len(data) < nonceSize+16 {
		return nil, errors.New("ciphertext too short")
	}
	gcm, err := newGCM(derivedKey)
	if err != nil {
		return nil, err
	}
	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}
