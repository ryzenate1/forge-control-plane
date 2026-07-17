package runcommand

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunCommand(t *testing.T) {
	dir := t.TempDir()
	op := &RunCommand{Command: "touch", Args: []string{"created.txt"}}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	exists, err := fileExistsInDir(dir, "created.txt")
	if err != nil {
		t.Fatalf("check file: %v", err)
	}
	if !exists {
		t.Fatal("expected file to exist")
	}
}

func TestRunCommandShell(t *testing.T) {
	dir := t.TempDir()
	op := &RunCommand{Command: "echo hello > shell_test.txt", Shell: true}
	if err := op.Execute(context.Background(), dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	exists, err := fileExistsInDir(dir, "shell_test.txt")
	if err != nil {
		t.Fatalf("check file: %v", err)
	}
	if !exists {
		t.Fatal("expected file to exist")
	}
}

func TestRunCommandFactory(t *testing.T) {
	op, err := factory([]byte(`{"command": "ls", "shell": true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rc, ok := op.(*RunCommand)
	if !ok {
		t.Fatal("expected *RunCommand type")
	}
	if rc.Command != "ls" {
		t.Fatalf("unexpected command: %s", rc.Command)
	}
	if !rc.Shell {
		t.Fatal("expected shell=true")
	}
}

func TestRunCommandFactoryMissingCmd(t *testing.T) {
	_, err := factory([]byte(`{}`))
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func fileExistsInDir(dir, name string) (bool, error) {
	_, err := os.Stat(filepath.Join(dir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
