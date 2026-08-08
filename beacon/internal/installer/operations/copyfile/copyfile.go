package copyfile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gamepanel/beacon/internal/installer/operations"
)

type CopyFile struct {
	Source string `json:"source"`
	Dest   string `json:"dest"`
}

func init() {
	operations.Register("copyFile", factory)
}

func factory(args json.RawMessage) (operations.Operation, error) {
	var op CopyFile
	if err := json.Unmarshal(args, &op); err != nil {
		return nil, fmt.Errorf("copyFile: %w", err)
	}
	if op.Source == "" || op.Dest == "" {
		return nil, fmt.Errorf("copyFile: source and dest are required")
	}
	return &op, nil
}

func (op *CopyFile) Execute(ctx context.Context, serverDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source, err := operations.ResolvePath(serverDir, op.Source)
	if err != nil {
		return err
	}
	dest, err := operations.ResolvePath(serverDir, op.Dest)
	if err != nil {
		return err
	}

	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	srcInfo, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("stat %q: %w", source, err)
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("copy source %q must not be a symlink", source)
	}

	if srcInfo.IsDir() {
		return copyDir(ctx, source, dest)
	}
	return copyFile(ctx, source, dest, srcInfo.Mode())
}

func copyFile(ctx context.Context, src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %q: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm()&0o0777&^0o6000)
	if err != nil {
		return fmt.Errorf("create %q: %w", dst, err)
	}

	if _, err := io.Copy(out, &contextReader{ctx: ctx, reader: in}); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy %q -> %q: %w", src, dst, err)
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return fmt.Errorf("sync %q: %w", dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %q: %w", dst, err)
	}
	return nil
}

func copyDir(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("copy source %q must not be a symlink", src)
	}
	if err := os.MkdirAll(dst, srcInfo.Mode().Perm()&^0o6000); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		info, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("copy source %q contains symlink %q", src, srcPath)
		}
		if info.IsDir() {
			if err := copyDir(ctx, srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("copy source %q contains unsupported file %q", src, srcPath)
			}
			if err := copyFile(ctx, srcPath, dstPath, info.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
