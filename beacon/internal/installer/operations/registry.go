package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

var installLocks sync.Map

type Step struct {
	Type      string          `json:"type"`
	Args      json.RawMessage `json:"args,omitempty"`
	Condition *Condition      `json:"condition,omitempty"`
}

func ExecuteSteps(ctx context.Context, serverDir string, steps []Step) error {
	canonical, err := filepath.EvalSymlinks(serverDir)
	if err != nil {
		return fmt.Errorf("resolve server directory: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("server directory must be an existing directory")
	}
	lockValue, _ := installLocks.LoadOrStore(canonical, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer func() {
		lock.Unlock()
		installLocks.CompareAndDelete(canonical, lock)
	}()

	parent := filepath.Dir(canonical)
	staging, err := os.MkdirTemp(parent, "."+filepath.Base(canonical)+".install-*")
	if err != nil {
		return fmt.Errorf("create installation transaction: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := cloneTree(ctx, canonical, staging); err != nil {
		return fmt.Errorf("snapshot installation directory: %w", err)
	}

	for i, step := range steps {
		factory, ok := GetFactory(step.Type)
		if !ok {
			return fmt.Errorf("step %d: unknown operation type %q", i, step.Type)
		}
		op, err := factory(step.Args)
		if err != nil {
			return fmt.Errorf("step %d (%s): build: %w", i, step.Type, err)
		}
		ok, err = step.Condition.ShouldExecute(staging)
		if err != nil {
			return fmt.Errorf("step %d (%s): condition: %w", i, step.Type, err)
		}
		if !ok {
			continue
		}
		if err := op.Execute(ctx, staging); err != nil {
			return fmt.Errorf("step %d (%s): %w", i, step.Type, err)
		}
	}
	rollback := canonical + ".install-rollback"
	if err := os.RemoveAll(rollback); err != nil {
		return fmt.Errorf("clear stale installation rollback: %w", err)
	}
	if err := os.Rename(canonical, rollback); err != nil {
		return fmt.Errorf("preserve installation rollback: %w", err)
	}
	if err := os.Rename(staging, canonical); err != nil {
		restoreErr := os.Rename(rollback, canonical)
		return fmt.Errorf("activate installation: %w (restore: %v)", err, restoreErr)
	}
	if err := SyncDirectory(parent); err != nil {
		_ = os.Rename(canonical, staging)
		_ = os.Rename(rollback, canonical)
		return fmt.Errorf("sync installation activation: %w", err)
	}
	if err := os.RemoveAll(rollback); err != nil {
		return fmt.Errorf("remove installation rollback: %w", err)
	}
	return SyncDirectory(parent)
}

func cloneTree(ctx context.Context, source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("installation tree contains symbolic link %q", relative)
		}
		if entry.IsDir() {
			return os.Mkdir(target, info.Mode().Perm()&0o750)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("installation tree contains unsupported file %q", relative)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()&0o750)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := input.Close()
		syncErr := output.Sync()
		outputCloseErr := output.Close()
		return errors.Join(copyErr, closeErr, syncErr, outputCloseErr)
	})
}

func StepsFromJSON(data []byte) ([]Step, error) {
	var steps []Step
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil, fmt.Errorf("parse steps: %w", err)
	}
	return steps, nil
}
