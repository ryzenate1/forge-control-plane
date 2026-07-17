package symlink

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"gamepanel/beacon/internal/installer/operations"
)

type Symlink struct {
	Source string `json:"source"`
	Dest   string `json:"dest"`
}

func init() {
	operations.Register("symlink", factory)
}

func factory(args json.RawMessage) (operations.Operation, error) {
	var op Symlink
	if err := json.Unmarshal(args, &op); err != nil {
		return nil, fmt.Errorf("symlink: %w", err)
	}
	if op.Source == "" || op.Dest == "" {
		return nil, fmt.Errorf("symlink: source and dest are required")
	}
	return &op, nil
}

func (op *Symlink) Execute(ctx context.Context, serverDir string) error {
	source := operations.ResolvePath(serverDir, op.Source)
	dest := operations.ResolvePath(serverDir, op.Dest)

	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	if _, err := os.Lstat(dest); err == nil {
		os.Remove(dest)
	}

	if err := os.Symlink(source, dest); err != nil {
		return fmt.Errorf("symlink %q -> %q: %w", source, dest, err)
	}
	return nil
}
