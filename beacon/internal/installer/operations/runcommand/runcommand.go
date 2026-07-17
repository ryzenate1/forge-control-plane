package runcommand

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gamepanel/beacon/internal/installer/operations"
)

type RunCommand struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Shell   bool     `json:"shell,omitempty"`
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

func (op *RunCommand) Execute(ctx context.Context, serverDir string) error {
	var cmd *exec.Cmd

	if op.Shell || len(op.Args) == 0 {
		fullCmd := op.Command
		if len(op.Args) > 0 {
			fullCmd = fullCmd + " " + strings.Join(op.Args, " ")
		}
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", fullCmd)
	} else {
		cmd = exec.CommandContext(ctx, op.Command, op.Args...)
	}

	cmd.Dir = serverDir
	cmd.Env = append(os.Environ(),
		"SERVER_DIR="+serverDir,
		"PWD="+serverDir,
		"HOME="+serverDir,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %q: %w\nstdout: %s\nstderr: %s", op.Command, err, stdout.String(), stderr.String())
	}
	return nil
}
