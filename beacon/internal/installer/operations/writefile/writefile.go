package writefile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

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
	dest := operations.ResolvePath(serverDir, op.Dest)
	if err := operations.EnsureParentDir(dest); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	mode := os.FileMode(0o644)
	if op.Mode > 0 {
		mode = os.FileMode(op.Mode)
	}

	if err := os.WriteFile(dest, []byte(op.Content), mode); err != nil {
		return fmt.Errorf("write %q: %w", dest, err)
	}
	return nil
}
