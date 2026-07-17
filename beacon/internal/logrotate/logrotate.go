package logrotate

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Config struct {
	MaxSize    int64
	MaxBackups int
	MaxAge     time.Duration
	Compress   bool
}

func DefaultConfig() Config {
	return Config{
		MaxSize:    10 * 1024 * 1024,
		MaxBackups: 5,
		MaxAge:     7 * 24 * time.Hour,
		Compress:   true,
	}
}

type RotatingWriter struct {
	mu     sync.Mutex
	file   *os.File
	path   string
	config Config
	size   int64
}

func NewRotatingWriter(path string, config Config) (*RotatingWriter, error) {
	if config.MaxSize <= 0 {
		config.MaxSize = 10 * 1024 * 1024
	}
	if config.MaxBackups <= 0 {
		config.MaxBackups = 5
	}
	if config.MaxAge <= 0 {
		config.MaxAge = 7 * 24 * time.Hour
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("stat log file: %w", err)
	}

	return &RotatingWriter{
		file:   f,
		path:   path,
		config: config,
		size:   info.Size(),
	}, nil
}

func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.size+int64(len(p)) > w.config.MaxSize {
		if err := w.rotate(); err != nil {
			return 0, fmt.Errorf("rotate: %w", err)
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *RotatingWriter) rotate() error {
	if w.file != nil {
		w.file.Close()
	}

	for i := w.config.MaxBackups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", w.path, i)
		dst := fmt.Sprintf("%s.%d", w.path, i+1)
		os.Rename(src, dst)

		if w.config.Compress {
			os.Rename(src+".gz", dst+".gz")
		}
	}

	if w.config.Compress {
		gzDst := w.path + ".1.gz"
		os.Remove(gzDst)
		if err := compressFile(w.path, gzDst); err == nil {
			os.Remove(w.path)
		}
	} else {
		os.Rename(w.path, w.path+".1")
	}

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("reopen log file: %w", err)
	}
	w.file = f
	w.size = 0

	go w.cleanup()

	return nil
}

func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}

func (w *RotatingWriter) cleanup() {
	now := time.Now()

	pattern := w.path + ".*"
	if w.config.Compress {
		pattern = w.path + ".*.gz"
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	sort.Strings(matches)

	if len(matches) > w.config.MaxBackups {
		for _, m := range matches[:len(matches)-w.config.MaxBackups] {
			os.Remove(m)
		}
		matches = matches[len(matches)-w.config.MaxBackups:]
	}

	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > w.config.MaxAge {
			os.Remove(m)
		}
	}
}

func compressFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()

	if _, err := io.Copy(gz, in); err != nil {
		return err
	}

	return nil
}

func WriteSystemConfig(logDir, serviceName string) error {
	dir := "/etc/logrotate.d"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	content := fmt.Sprintf(`%s/*.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0644 root root
}
`, logDir)

	path := filepath.Join(dir, serviceName)
	return os.WriteFile(path, []byte(content), 0o644)
}
