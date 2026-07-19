package removefile

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"gamepanel/beacon/internal/installer/operations"
)

type RemoveFile struct {
	Target string `json:"target"`
	Recursive bool `json:"recursive,omitempty"`
}

func init() {
	operations.Register("removeFile", factory)
}

func factory(args json.RawMessage) (operations.Operation, error) {
	var op RemoveFile
	if err := json.Unmarshal(args, &op); err != nil {
		return nil, fmt.Errorf("removeFile: %w", err)
	}
	if op.Target == "" {
		return nil, fmt.Errorf("removeFile: target is required")
	}
	return &op, nil
}

func (op *RemoveFile) Execute(ctx context.Context, serverDir string) error {
	target := operations.ResolvePath(serverDir, op.Target)

	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %q: %w", target, err)
	}

	if info.IsDir() && op.Recursive {
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("remove dir %q: %w", target, err)
		}
		return nil
	}

	if info.IsDir() {
		entries, err := os.ReadDir(target)
		if err != nil {
			return fmt.Errorf("read dir %q: %w", target, err)
		}
		if len(entries) > 0 {
			return fmt.Errorf("directory %q is not empty (set recursive=true to remove)", target)
		}
	}

	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("remove %q: %w", target, err)
	}
	return nil
}
