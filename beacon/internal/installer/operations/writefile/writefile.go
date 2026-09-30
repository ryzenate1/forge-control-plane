package writefile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gamepanel/beacon/internal/installer/operations"
)

type WriteFile struct {
	Dest    string `json:"dest"`
	Content string `json:"content"`
	Mode    int    `json:"mode,omitempty"`
}

func init() {
	operations.Register("writeFile", factory)
}

func factory(args json.RawMessage) (operations.Operation, error) {
	var op WriteFile
	if err := json.Unmarshal(args, &op); err != nil {
		return nil, fmt.Errorf("writeFile: %w", err)
	}
	if op.Dest == "" {
		return nil, fmt.Errorf("writeFile: dest is required")
	}
	return &op, nil
}

func (op *WriteFile) Execute(ctx context.Context, serverDir string) error {
	dest, err := operations.ResolvePath(serverDir, op.Dest)
	if err != nil {
		return err
	}
	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	mode := os.FileMode(0o644)
	if op.Mode > 0 {
		mode = os.FileMode(op.Mode)
	}
	mode &= 0o666
	if mode == 0 {
		return fmt.Errorf("writeFile: mode must grant owner read or write access")
	}

	temp, err := os.CreateTemp(filepath.Dir(dest), "."+filepath.Base(dest)+".write-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.WriteString(op.Content); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write %q: %w", dest, err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, dest); err != nil {
		return fmt.Errorf("write %q: %w", dest, err)
	}
	return operations.SyncDirectory(filepath.Dir(dest))
}
