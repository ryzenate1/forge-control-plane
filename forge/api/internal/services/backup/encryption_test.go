package backup

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	plaintext := []byte("this is sensitive backup data that must be encrypted")

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if bytes.Equal(plaintext, ciphertext) {
		t.Error("encrypted data should not equal plaintext")
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypted data does not match original: got %s, want %s", decrypted, plaintext)
	}
}

func TestEncryptedDataNotPlaintext(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	plaintext := []byte("backup-content-that-should-be-hidden")

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if string(ciphertext) == string(plaintext) {
		t.Error("ciphertext should not equal plaintext")
	}

	if bytes.Contains(ciphertext, plaintext) {
		t.Error("ciphertext should not contain plaintext")
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	originalKey := make([]byte, 32)
	if _, err := rand.Read(originalKey); err != nil {
		t.Fatalf("generate original key: %v", err)
	}
	wrongKey := make([]byte, 32)
	if _, err := rand.Read(wrongKey); err != nil {
		t.Fatalf("generate wrong key: %v", err)
	}

	plaintext := []byte("data encrypted with correct key")

	ciphertext, err := Encrypt(plaintext, originalKey)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	_, err = Decrypt(ciphertext, wrongKey)
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestDecryptWithNilKey(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	plaintext := []byte("test-data")
	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	_, err = Decrypt(ciphertext, nil)
	if err == nil {
		t.Error("expected error when decrypting with nil key")
	}
}

func TestCompressDecompressRoundTrip(t *testing.T) {
	original := []byte("this is data that will be compressed to save space in backups")
	for i := 0; i < 10; i++ {
		original = append(original, original...)
	}

	compressed, err := Compress(original)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}

	if len(compressed) >= len(original) {
		t.Logf("compressed size (%d) >= original (%d) — expected for small data", len(compressed), len(original))
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}

	if !bytes.Equal(original, decompressed) {
		t.Errorf("decompressed data does not match original")
	}
}

func TestCompressEmptyData(t *testing.T) {
	compressed, err := Compress([]byte{})
	if err != nil {
		t.Fatalf("compress empty: %v", err)
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress empty: %v", err)
	}
	if len(decompressed) != 0 {
		t.Errorf("expected empty result, got %d bytes", len(decompressed))
	}
}

func TestDecompressInvalidData(t *testing.T) {
	_, err := Decompress([]byte("this is not gzip compressed data"))
	if err == nil {
		t.Error("expected error when decompressing invalid data")
	}
}

func TestEncryptThenCompressRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	original := []byte("data that will be encrypted then compressed for backup storage")

	ciphertext, err := Encrypt(original, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	compressed, err := Compress(ciphertext)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}

	decrypted, err := Decrypt(decompressed, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if !bytes.Equal(original, decrypted) {
		t.Errorf("round-trip result does not match original")
	}
}

func TestEncryptLargeData(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	largeData := make([]byte, 1024*100)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	ciphertext, err := Encrypt(largeData, key)
	if err != nil {
		t.Fatalf("encrypt large data: %v", err)
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt large data: %v", err)
	}

	if !bytes.Equal(largeData, decrypted) {
		t.Errorf("large data round-trip failed")
	}
}
