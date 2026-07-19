package movefile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"gamepanel/beacon/internal/installer/operations"
)

type MoveFile struct {
	Source string `json:"source"`
	Dest   string `json:"dest"`
}

func init() {
	operations.Register("moveFile", factory)
}

func factory(args json.RawMessage) (operations.Operation, error) {
	var op MoveFile
	if err := json.Unmarshal(args, &op); err != nil {
		return nil, fmt.Errorf("moveFile: %w", err)
	}
	if op.Source == "" || op.Dest == "" {
		return nil, fmt.Errorf("moveFile: source and dest are required")
	}
	return &op, nil
}

func (op *MoveFile) Execute(ctx context.Context, serverDir string) error {
	source := operations.ResolvePath(serverDir, op.Source)
	dest := operations.ResolvePath(serverDir, op.Dest)

	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	if err := os.Rename(source, dest); err != nil {
		return fmt.Errorf("move %q -> %q: %w", source, dest, err)
	}
	return nil
}
