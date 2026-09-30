package extract

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gamepanel/beacon/internal/installer/operations"
)

// Extract unpacks a local archive (zip, tar, tar.gz) from source to destination.
// Puffer spec: {"type":"extract","source":"archive.zip","destination":"./"}
type Extract struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Dest        string `json:"dest"` // alias
	Strip       int    `json:"strip,omitempty"`
}

func init() {
	operations.Register("extract", factory)
}

func factory(args json.RawMessage) (operations.Operation, error) {
	var op Extract
	if err := json.Unmarshal(args, &op); err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}
	if op.Source == "" {
		return nil, fmt.Errorf("extract: source is required")
	}
	if op.Destination == "" {
		op.Destination = op.Dest
	}
	if op.Destination == "" {
		op.Destination = "."
	}
	return &op, nil
}

func (op *Extract) Execute(ctx context.Context, serverDir string) error {
	src, err := operations.ResolvePath(serverDir, op.Source)
	if err != nil {
		return err
	}
	dest, err := operations.ResolvePath(serverDir, op.Destination)
	if err != nil {
		return err
	}
	// dest may be existing directory or not; ensure it exists
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return fmt.Errorf("create dest dir %q: %w", dest, err)
	}
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source %q: %w", src, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source %q is not a regular file", src)
	}
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source %q: %w", src, err)
	}
	defer f.Close()

	format, err := detectFormat(f)
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	switch format {
	case "zip":
		return extractZip(ctx, f, src, dest, op.Strip)
	case "gzip":
		return extractTarGz(ctx, f, dest, op.Strip)
	case "tar":
		return extractTar(ctx, f, dest, op.Strip)
	default:
		return fmt.Errorf("unsupported archive format for %q", src)
	}
}

func detectFormat(f *os.File) (string, error) {
	header := make([]byte, 512)
	n, err := io.ReadFull(f, header)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", err
	}
	header = header[:n]
	if len(header) >= 4 && header[0] == 'P' && header[1] == 'K' && (header[2] == 3 || header[2] == 5 || header[2] == 7) {
		return "zip", nil
	}
	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		return "gzip", nil
	}
	if len(header) >= 262 && string(header[257:262]) == "ustar" {
		return "tar", nil
	}
	return "", errors.New("not a recognized ZIP, gzip, or tar archive")
}

func extractZip(ctx context.Context, file *os.File, archivePath, dest string, strip int) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(file, info.Size())
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	for _, f := range zr.File {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		name := stripPath(f.Name, strip)
		if name == "" {
			continue
		}
		target, err := archiveTarget(dest, name)
		if err != nil {
			return fmt.Errorf("unsafe zip entry %q: %w", f.Name, err)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, normalizeDirMode(f.Mode())); err != nil {
				return err
			}
			continue
		}
		if !f.Mode().IsRegular() {
			return fmt.Errorf("unsupported zip entry type %q", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return fmt.Errorf("mkdir: %w", err)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %q: %w", f.Name, err)
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, normalizeFileMode(f.Mode()))
		if err != nil {
			rc.Close()
			return fmt.Errorf("create %q: %w", target, err)
		}
		written, copyErr := io.Copy(out, &ctxReader{ctx: ctx, r: io.LimitReader(rc, 1<<30)})
		syncErr := out.Sync()
		closeOutErr := out.Close()
		closeInErr := rc.Close()
		if err := errors.Join(copyErr, syncErr, closeOutErr, closeInErr); err != nil {
			return fmt.Errorf("write %q: %w", target, err)
		}
		_ = written
	}
	return operations.SyncDirectory(dest)
}

func extractTarGz(ctx context.Context, r io.Reader, dest string, strip int) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gzr.Close()
	return extractTar(ctx, gzr, dest, strip)
}

func extractTar(ctx context.Context, r io.Reader, dest string, strip int) error {
	tr := tar.NewReader(r)
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
		name := stripPath(header.Name, strip)
		if name == "" {
			continue
		}
		target, err := archiveTarget(dest, name)
		if err != nil {
			return fmt.Errorf("unsafe tar entry %q: %w", header.Name, err)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, normalizeDirMode(os.FileMode(header.Mode))); err != nil {
				return fmt.Errorf("mkdir %q: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return fmt.Errorf("mkdir: %w", err)
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, normalizeFileMode(os.FileMode(header.Mode)))
			if err != nil {
				return fmt.Errorf("create %q: %w", target, err)
			}
			written, copyErr := io.Copy(out, &ctxReader{ctx: ctx, r: io.LimitReader(tr, header.Size)})
			syncErr := out.Sync()
			closeErr := out.Close()
			if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
				return fmt.Errorf("write %q: %w", target, err)
			}
			if written != header.Size {
				return fmt.Errorf("tar entry %q truncated", header.Name)
			}
		default:
			return fmt.Errorf("unsupported tar entry type %d", header.Typeflag)
		}
	}
	return operations.SyncDirectory(dest)
}

func stripPath(name string, strip int) string {
	if strip <= 0 {
		return name
	}
	parts := strings.SplitN(name, "/", strip+1)
	if len(parts) <= strip {
		return ""
	}
	return parts[strip]
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

func normalizeFileMode(mode os.FileMode) os.FileMode {
	mode &= 0o666
	if mode == 0 {
		return 0o600
	}
	return mode
}

func normalizeDirMode(mode os.FileMode) os.FileMode {
	mode &= 0o777
	mode &^= 0o022
	if mode == 0 {
		return 0o750
	}
	return mode
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *ctxReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
