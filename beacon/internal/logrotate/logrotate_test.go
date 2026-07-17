package logrotate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewRotatingWriterCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	w, err := NewRotatingWriter(path, DefaultConfig())
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}
	defer w.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("log file should have been created")
	}
}

func TestWriteWithinLimitDoesNotRotate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	cfg := Config{
		MaxSize:    1024,
		MaxBackups: 3,
		MaxAge:     time.Hour,
		Compress:   false,
	}

	w, err := NewRotatingWriter(path, cfg)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}
	defer w.Close()

	data := []byte("hello world\n")
	for i := 0; i < 5; i++ {
		_, err := w.Write(data)
		if err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}
	}

	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatal("rotation should not have occurred")
	}
}

func TestWriteExceedingMaxSizeTriggersRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	cfg := Config{
		MaxSize:    50,
		MaxBackups: 3,
		MaxAge:     time.Hour,
		Compress:   false,
	}

	w, err := NewRotatingWriter(path, cfg)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}
	defer w.Close()

	data := []byte(strings.Repeat("x", 60))
	_, err = w.Write(data)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := os.Stat(path + ".1"); os.IsNotExist(err) {
		t.Fatal("rotation should have created .1 backup")
	}
}

func TestMultipleRotationsCreateNumberedBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	cfg := Config{
		MaxSize:    50,
		MaxBackups: 5,
		MaxAge:     time.Hour,
		Compress:   false,
	}

	w, err := NewRotatingWriter(path, cfg)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}
	defer w.Close()

	data := []byte(strings.Repeat("x", 60))
	for i := 0; i < 3; i++ {
		_, err := w.Write(data)
		if err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}
	}

	for i := 1; i <= 2; i++ {
		backup := filepath.Join(dir, "test.log."+strings.Repeat("", 0)+string(rune('0'+i)))
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			t.Fatalf("backup %d should exist", i)
		}
	}
}

func TestMaxBackupsEnforcement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	cfg := Config{
		MaxSize:    50,
		MaxBackups: 2,
		MaxAge:     time.Hour,
		Compress:   false,
	}

	w, err := NewRotatingWriter(path, cfg)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}
	defer w.Close()

	data := []byte(strings.Repeat("x", 60))
	for i := 0; i < 5; i++ {
		_, err := w.Write(data)
		if err != nil {
			t.Fatalf("write %d failed: %v", i, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Fatal("backup .3 should have been deleted (MaxBackups=2)")
	}
}

func TestCloseWorks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	w, err := NewRotatingWriter(path, DefaultConfig())
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("second Close should not error, got: %v", err)
	}
}

func TestDefaultConfigReturnsSensibleValues(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxSize != 10*1024*1024 {
		t.Fatalf("expected MaxSize 10MB, got %d", cfg.MaxSize)
	}
	if cfg.MaxBackups != 5 {
		t.Fatalf("expected MaxBackups 5, got %d", cfg.MaxBackups)
	}
	if cfg.MaxAge != 7*24*time.Hour {
		t.Fatalf("expected MaxAge 7 days, got %s", cfg.MaxAge)
	}
	if !cfg.Compress {
		t.Fatal("expected Compress to be true by default")
	}
}

func TestWriteWithCompression(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	cfg := Config{
		MaxSize:    50,
		MaxBackups: 3,
		MaxAge:     time.Hour,
		Compress:   true,
	}

	w, err := NewRotatingWriter(path, cfg)
	if err != nil {
		t.Fatalf("NewRotatingWriter failed: %v", err)
	}
	defer w.Close()

	data := []byte(strings.Repeat("x", 60))
	_, err = w.Write(data)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := os.Stat(path + ".1.gz"); os.IsNotExist(err) {
		t.Fatal("compressed backup .1.gz should exist")
	}
}
