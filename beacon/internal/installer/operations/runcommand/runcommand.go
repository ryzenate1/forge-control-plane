package runcommand

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gamepanel/beacon/internal/installer/operations"
)

type RunCommand struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

func init() {
	operations.Register("runCommand", factory)
}

func factory(args json.RawMessage) (operations.Operation, error) {
	var op RunCommand
	if err := json.Unmarshal(args, &op); err != nil {
		return nil, fmt.Errorf("runCommand: %w", err)
	}
	if op.Command == "" {
		return nil, fmt.Errorf("runCommand: command is required")
	}
	return &op, nil
}

var allowedCommands = map[string]bool{
	"java":  true,
	"unzip": true, "tar": true, "chmod": true,
	"cp": true, "mv": true, "rm": true, "ln": true, "mkdir": true,
	"touch": true, "echo": true, "ls": true,
}

func (op *RunCommand) Execute(ctx context.Context, serverDir string) error {
	command := filepath.Base(strings.TrimSpace(op.Command))
	if command != op.Command || !allowedCommands[command] {
		return fmt.Errorf("runCommand: command %q is not in the allowed list", op.Command)
	}
	for _, arg := range op.Args {
		if strings.ContainsRune(arg, '\x00') || strings.ContainsAny(arg, "\n\r") {
			return fmt.Errorf("runCommand: argument contains invalid control characters")
		}
	}

	cmd := exec.CommandContext(ctx, command, op.Args...)
	cmd.Dir = serverDir
	cmd.Env = append(os.Environ(),
		"SERVER_DIR="+serverDir,
		"PWD="+serverDir,
		"HOME="+serverDir,
	)

	if err := cmd.Run(); err != nil {
		// Installer output can contain credentials supplied by an egg. Never
		// echo child output into API-visible errors.
		return fmt.Errorf("run %q: %w", command, err)
	}
	return nil
}
