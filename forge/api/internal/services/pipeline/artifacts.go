package pipeline

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ArtifactHandler persists pipeline artifact payloads on the local
// filesystem. Layout (relative to the configured base dir, default
// "data/pipelines"):
//
//	data/pipelines/artifacts/<runId>/<stageId>/<name>
//
// Only the relative path below the artifacts root is stored in the database,
// so a future S3-backed handler could implement the same interface without
// schema churn. There is no S3 storage currently wired into the pipeline
// service — local FS is the only backend (s3 was deliberately not reused
// because the backup S3 layer is policy-scoped to backup objects).
type ArtifactHandler struct {
	baseDir string
}

// NewArtifactHandler builds a handler rooted at dataDir/pipelines/artifacts
// (dataDir defaults to "data"; the default artifact root is therefore
// data/pipelines/artifacts). The directory is created on first use.
func NewArtifactHandler(dataDir string) *ArtifactHandler {
	if dataDir == "" {
		dataDir = "data"
	}
	return &ArtifactHandler{baseDir: filepath.Join(dataDir, "pipelines", "artifacts")}
}

// Save writes artifact payload bytes and returns the relative path as stored
// in the database (e.g. "<runId>/<stageId>/<name>").
func (h *ArtifactHandler) Save(runID, stageID, name string, payload []byte) (string, error) {
	safeName := sanitizeArtifactName(name)
	if safeName == "" {
		return "", fmt.Errorf("invalid artifact name")
	}
	rel := filepath.Join(runID, stageID, safeName)
	abs := filepath.Join(h.baseDir, runID, stageID)
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create artifact dir: %w", err)
	}
	dest := filepath.Join(abs, safeName)
	if !strings.HasPrefix(dest, filepath.Clean(h.baseDir)+string(filepath.Separator)) {
		return "", fmt.Errorf("artifact path escapes root")
	}
	if err := os.WriteFile(dest, payload, 0o644); err != nil {
		return "", fmt.Errorf("write artifact: %w", err)
	}
	return rel, nil
}

// Open returns a read handle for the stored artifact. Callers must close it.
func (h *ArtifactHandler) Open(relativePath string) (*os.File, error) {
	clean := filepath.Clean(relativePath)
	abs := filepath.Join(h.baseDir, clean)
	if !strings.HasPrefix(abs, filepath.Clean(h.baseDir)+string(filepath.Separator)) {
		return nil, fmt.Errorf("artifact path escapes root")
	}
	f, err := os.Open(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("open artifact: %w", err)
	}
	return f, nil
}

// Read loads a stored artifact fully into memory (artifacts are capped at
// upload time by the HTTP layer).
func (h *ArtifactHandler) Read(relativePath string) ([]byte, error) {
	f, err := h.Open(relativePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := &bytes.Buffer{}
	if _, err := buf.ReadFrom(f); err != nil {
		return nil, fmt.Errorf("read artifact: %w", err)
	}
	return buf.Bytes(), nil
}

// Delete removes the file for a stored artifact reference. Missing files are
// tolerated (the DB row owns lifecycle; the file may already be gone).
func (h *ArtifactHandler) Delete(relativePath string) error {
	clean := filepath.Clean(relativePath)
	abs := filepath.Join(h.baseDir, clean)
	if !strings.HasPrefix(abs, filepath.Clean(h.baseDir)+string(filepath.Separator)) {
		return fmt.Errorf("artifact path escapes root")
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove artifact: %w", err)
	}
	return nil
}

func sanitizeArtifactName(name string) string {
	name = filepath.Base(strings.TrimSpace(strings.ReplaceAll(name, "\\", "/")))
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, ".")
	return name
}