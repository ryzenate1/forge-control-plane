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
	source := operations.ResolvePath(serverDir, op.Source)
	dest := operations.ResolvePath(serverDir, op.Dest)

	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	srcInfo, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat %q: %w", source, err)
	}

	if srcInfo.IsDir() {
		return copyDir(source, dest)
	}
	return copyFile(source, dest)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %q: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %q: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %q -> %q: %w", src, dst, err)
	}
	return nil
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}
