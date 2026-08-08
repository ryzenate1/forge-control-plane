package downloadextract

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	// ExpectedSHA256 is the required hex-encoded SHA-256 digest of the
	// downloaded archive. The archive is verified before extraction.
	ExpectedSHA256 string `json:"expectedSha256,omitempty"`
	MaxBytes       int64  `json:"maxBytes,omitempty"`
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
	decoded, err := hex.DecodeString(op.ExpectedSHA256)
	if err != nil || len(decoded) != sha256.Size {
		return nil, fmt.Errorf("downloadExtract: expectedSha256 must be a 64-character SHA-256 digest")
	}
	if op.MaxBytes <= 0 {
		op.MaxBytes = 8 << 30
	}
	return &op, nil
}

func (op *DownloadExtract) Execute(ctx context.Context, serverDir string) error {
	if op.MaxBytes <= 0 {
		op.MaxBytes = 8 << 30
	}
	decoded, err := hex.DecodeString(op.ExpectedSHA256)
	if err != nil || len(decoded) != sha256.Size {
		return errors.New("expectedSha256 must be a valid SHA-256 digest")
	}
	dest, err := operations.ResolvePath(serverDir, op.Dest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return fmt.Errorf("create dest dir %q: %w", dest, err)
	}

	timeout := time.Duration(op.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	client := operations.SecureHTTPClient(timeout)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, op.URL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if err := operations.ValidateDownloadURL(req.URL); err != nil {
		return fmt.Errorf("validate download URL: %w", err)
	}

	resp, err := operations.DoWithRetry(ctx, client, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, op.URL, nil)
	})
	if err != nil {
		return fmt.Errorf("get %q: %w", op.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %q: unexpected status %d", op.URL, resp.StatusCode)
	}
	if resp.ContentLength > op.MaxBytes {
		return fmt.Errorf("archive exceeds %d-byte limit", op.MaxBytes)
	}
	requiredSpace := op.MaxBytes
	if resp.ContentLength > 0 {
		requiredSpace = resp.ContentLength
	}
	if err := operations.EnsureDiskSpace(serverDir, requiredSpace); err != nil {
		return err
	}

	// Download the full archive to a temp file first so it can be hashed in
	// its entirety and verified *before* any of its contents are extracted.
	// Streaming straight into the extractor would mean a checksum mismatch
	// is only discoverable after files have already been written to disk.
	archivePath, err := op.downloadToTemp(resp.Body, filepath.Dir(dest))
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

	format, err := detectArchiveFormat(archive)
	if err != nil {
		return err
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}
	switch format {
	case "zip":
		return op.extractZip(ctx, archivePath, dest)
	case "gzip":
		return op.extractTarGz(ctx, archive, dest)
	case "tar":
		return op.extractTar(ctx, archive, dest)
	}
	return fmt.Errorf("unsupported archive format for %q", op.URL)
}

// downloadToTemp copies r into a new temporary file and returns its path.
func (op *DownloadExtract) downloadToTemp(r io.Reader, dir string) (string, error) {
	tmp, err := os.CreateTemp(dir, ".gamepanel-dl-*.archive")
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}
	defer tmp.Close()

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", err
	}
	written, err := io.Copy(tmp, io.LimitReader(r, op.MaxBytes+1))
	if err != nil || written > op.MaxBytes {
		_ = os.Remove(tmp.Name())
		if err != nil {
			return "", fmt.Errorf("copy to temp: %w", err)
		}
		return "", fmt.Errorf("archive exceeds %d-byte limit", op.MaxBytes)
	}
	if err := tmp.Sync(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func (op *DownloadExtract) verifyChecksum(path string) error {
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

	var extracted int64
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

		target, err := archiveTarget(dest, name)
		if err != nil {
			return fmt.Errorf("unsafe zip entry %q: %w", f.Name, err)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, normalizedArchiveDirMode(f.Mode())); err != nil {
				return err
			}
			continue
		}
		if !f.Mode().IsRegular() {
			return fmt.Errorf("unsupported zip entry type %q", f.Name)
		}
		if f.UncompressedSize64 > uint64(op.MaxBytes) || extracted > op.MaxBytes-int64(f.UncompressedSize64) {
			return fmt.Errorf("zip extraction exceeds %d-byte limit", op.MaxBytes)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %q: %w", f.Name, err)
		}

		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, normalizedArchiveFileMode(f.Mode()))
		if err != nil {
			rc.Close()
			return fmt.Errorf("create %q: %w", target, err)
		}

		written, copyErr := io.Copy(out, &contextReader{ctx: ctx, reader: io.LimitReader(rc, int64(f.UncompressedSize64)+1)})
		syncErr := out.Sync()
		closeOutErr := out.Close()
		closeInErr := rc.Close()
		if err := errors.Join(copyErr, syncErr, closeOutErr, closeInErr); err != nil {
			return fmt.Errorf("write %q: %w", target, err)
		}
		if written != int64(f.UncompressedSize64) {
			return fmt.Errorf("zip entry %q size mismatch", f.Name)
		}
		extracted += written
	}
	return operations.SyncDirectory(dest)
}

func (op *DownloadExtract) extractTarGz(ctx context.Context, r io.Reader, dest string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gzr.Close()
	return op.extractTar(ctx, gzr, dest)
}

func (op *DownloadExtract) extractTar(ctx context.Context, r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	var extracted int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
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

		target, err := archiveTarget(dest, name)
		if err != nil {
			return fmt.Errorf("unsafe tar entry %q: %w", header.Name, err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, normalizedArchiveDirMode(os.FileMode(header.Mode))); err != nil {
				return fmt.Errorf("mkdir %q: %w", target, err)
			}
		case tar.TypeReg:
			if header.Size < 0 || header.Size > op.MaxBytes || extracted > op.MaxBytes-header.Size {
				return fmt.Errorf("tar entry %q has invalid size", header.Name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return fmt.Errorf("mkdir: %w", err)
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, normalizedArchiveFileMode(os.FileMode(header.Mode)))
			if err != nil {
				return fmt.Errorf("create %q: %w", target, err)
			}
			written, copyErr := io.Copy(out, &contextReader{ctx: ctx, reader: io.LimitReader(tr, header.Size)})
			syncErr := out.Sync()
			closeErr := out.Close()
			if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
				return fmt.Errorf("write %q: %w", target, err)
			}
			if written != header.Size {
				return fmt.Errorf("tar entry %q was truncated", header.Name)
			}
			extracted += written
		default:
			return fmt.Errorf("unsupported tar entry type %d", header.Typeflag)
		}
	}
	return operations.SyncDirectory(dest)
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

func detectArchiveFormat(file *os.File) (string, error) {
	header := make([]byte, 512)
	n, err := io.ReadFull(file, header)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", err
	}
	header = header[:n]
	if len(header) >= 4 && header[0] == 'P' && header[1] == 'K' &&
		(header[2] == 3 || header[2] == 5 || header[2] == 7) {
		return "zip", nil
	}
	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		return "gzip", nil
	}
	if len(header) >= 262 && string(header[257:262]) == "ustar" {
		return "tar", nil
	}
	return "", errors.New("download is not a recognized ZIP, gzip, or tar archive")
}

func archiveTarget(root, name string) (string, error) {
	name = filepath.ToSlash(name)
	if strings.ContainsRune(name, '\x00') || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return "", errors.New("absolute or malformed path")
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes destination")
	}
	target := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes destination")
	}
	return target, nil
}

func normalizedArchiveFileMode(mode os.FileMode) os.FileMode {
	mode &= 0o666
	if mode == 0 {
		return 0o600
	}
	return mode
}

func normalizedArchiveDirMode(mode os.FileMode) os.FileMode {
	mode &= 0o777
	mode &^= 0o022
	if mode == 0 {
		return 0o750
	}
	return mode
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
