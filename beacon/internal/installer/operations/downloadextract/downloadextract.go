package downloadextract

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gamepanel/beacon/internal/installer/operations"
)

type DownloadExtract struct {
	URL     string `json:"url"`
	Dest    string `json:"dest"`
	Strip   int    `json:"strip,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
	// ExpectedSHA256 is an optional hex-encoded SHA-256 digest of the
	// downloaded archive. When set, the archive is hashed in full before
	// any of its contents are extracted; a mismatch aborts the operation
	// and deletes the downloaded artifact. When empty, integrity
	// verification is skipped and a warning is logged.
	ExpectedSHA256 string `json:"expectedSha256,omitempty"`
}

func init() {
	operations.Register("downloadExtract", factory)
}

func factory(args json.RawMessage) (operations.Operation, error) {
	var op DownloadExtract
	if err := json.Unmarshal(args, &op); err != nil {
		return nil, fmt.Errorf("downloadExtract: %w", err)
	}
	if op.URL == "" || op.Dest == "" {
		return nil, fmt.Errorf("downloadExtract: url and dest are required")
	}
	return &op, nil
}

func (op *DownloadExtract) Execute(ctx context.Context, serverDir string) error {
	dest := operations.ResolvePath(serverDir, op.Dest)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create dest dir %q: %w", dest, err)
	}

	client := &http.Client{Timeout: time.Duration(op.Timeout) * time.Second}
	if op.Timeout <= 0 {
		client.Timeout = 10 * time.Minute
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, op.URL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("get %q: %w", op.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %q: unexpected status %d", op.URL, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	disposition := resp.Header.Get("Content-Disposition")

	// Download the full archive to a temp file first so it can be hashed in
	// its entirety and verified *before* any of its contents are extracted.
	// Streaming straight into the extractor would mean a checksum mismatch
	// is only discoverable after files have already been written to disk.
	archivePath, err := op.downloadToTemp(resp.Body)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	if err := op.verifyChecksum(archivePath); err != nil {
		return err
	}

	archive, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open downloaded archive: %w", err)
	}
	defer archive.Close()

	if strings.Contains(contentType, "zip") || strings.HasSuffix(op.URL, ".zip") || strings.Contains(disposition, ".zip") {
		return op.extractZip(ctx, archivePath, dest)
	}
	if strings.Contains(contentType, "gzip") || strings.HasSuffix(op.URL, ".tar.gz") || strings.HasSuffix(op.URL, ".tgz") || strings.Contains(disposition, ".tar.gz") {
		return op.extractTarGz(archive, dest)
	}
	if strings.HasSuffix(op.URL, ".tar") || strings.Contains(contentType, "tar") {
		return op.extractTar(archive, dest)
	}
	return fmt.Errorf("unsupported archive format for %q (content-type: %s)", op.URL, contentType)
}

// downloadToTemp copies r into a new temporary file and returns its path.
func (op *DownloadExtract) downloadToTemp(r io.Reader) (string, error) {
	tmp, err := os.CreateTemp("", "gamepanel-dl-*.archive")
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}
	defer tmp.Close()

	if _, err := io.Copy(tmp, r); err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("copy to temp: %w", err)
	}
	return tmp.Name(), nil
}

// verifyChecksum checks the downloaded archive's SHA-256 digest against
// ExpectedSHA256, if one was supplied. If no checksum was supplied,
// verification is skipped but a warning is logged so the gap is visible.
func (op *DownloadExtract) verifyChecksum(path string) error {
	if op.ExpectedSHA256 == "" {
		log.Printf("[installer] warning: downloadExtract %q has no expectedSha256; integrity verification skipped", op.URL)
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open downloaded archive for checksum: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("hash downloaded archive: %w", err)
	}

	got := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(got, op.ExpectedSHA256) {
		return fmt.Errorf("downloaded archive sha256 mismatch: got %s, expected %s", got, op.ExpectedSHA256)
	}
	return nil
}

func (op *DownloadExtract) extractZip(ctx context.Context, archivePath, dest string) error {
	zipReader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zipReader.Close()

	for _, f := range zipReader.File {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		name := op.stripPath(f.Name)
		if name == "" {
			continue
		}

		target := filepath.Join(dest, name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)) {
			continue
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0o755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %q: %w", f.Name, err)
		}

		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return fmt.Errorf("create %q: %w", target, err)
		}

		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return fmt.Errorf("write %q: %w", target, err)
		}
	}
	return nil
}

func (op *DownloadExtract) extractTarGz(r io.Reader, dest string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gzr.Close()
	return op.extractTar(gzr, dest)
}

func (op *DownloadExtract) extractTar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		name := op.stripPath(header.Name)
		if name == "" {
			continue
		}

		target := filepath.Join(dest, name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("mkdir %q: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("mkdir: %w", err)
			}
			out, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("create %q: %w", target, err)
			}
			_, err = io.Copy(out, tr)
			out.Close()
			if err != nil {
				return fmt.Errorf("write %q: %w", target, err)
			}
		}
	}
	return nil
}

func (op *DownloadExtract) stripPath(name string) string {
	if op.Strip <= 0 {
		return name
	}
	parts := strings.SplitN(name, "/", op.Strip+1)
	if len(parts) <= op.Strip {
		return ""
	}
	return parts[op.Strip]
}
