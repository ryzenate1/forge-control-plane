package installer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gamepanel/beacon/internal/installer/operations"
	"gamepanel/beacon/internal/runtime"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

type installerLogWriter struct {
	buffer *bytes.Buffer
}

func newInstallerLogWriter() *installerLogWriter {
	return &installerLogWriter{buffer: new(bytes.Buffer)}
}

func (w *installerLogWriter) Write(p []byte) (int, error) {
	if w == nil {
		return 0, nil
	}
	return w.buffer.Write(p)
}

func (w *installerLogWriter) Bytes() []byte {
	if w == nil {
		return nil
	}
	b := w.buffer.Bytes()
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

type Script struct {
	Script         string
	Container      ScriptContainer
	SkipEggScripts bool
	Steps          []operations.Step
}

type ScriptContainer struct {
	Image       string
	Env         []string
	Privileged  bool
	WorkingDir  string
	Binds       []string
	NetworkMode string
}

type Installer struct {
	runtime runtime.Runtime
	client  *client.Client
}

func NewInstaller(r runtime.Runtime, c *client.Client) *Installer {
	return &Installer{runtime: r, client: c}
}

func (i *Installer) Run(ctx context.Context, dataDir string, s Script) ([]byte, error) {
	if s.SkipEggScripts || (strings.TrimSpace(s.Script) == "" && len(s.Steps) == 0) {
		return nil, nil
	}
	if i == nil || i.runtime == nil {
		return nil, errors.New("installer: runtime is not initialized")
	}

	if len(s.Steps) > 0 {
		return i.runSteps(ctx, dataDir, s)
	}
	return i.runScript(ctx, dataDir, s)
}

func (i *Installer) runSteps(ctx context.Context, dataDir string, s Script) ([]byte, error) {
	var logBuf bytes.Buffer

	if err := operations.ExecuteSteps(ctx, dataDir, s.Steps); err != nil {
		logBuf.WriteString(fmt.Sprintf("ERROR: %s\n", err))
		return logBuf.Bytes(), err
	}

	logBuf.WriteString("Installation completed successfully.\n")
	return logBuf.Bytes(), nil
}

func (i *Installer) runScript(ctx context.Context, dataDir string, s Script) ([]byte, error) {
	if i.client == nil {
		return nil, errors.New("installer: docker client is not initialized")
	}
	scriptPath := filepath.Join(dataDir, "install.sh")
	logFile := filepath.Join(dataDir, "install.log")

	scriptContent := "#!/bin/bash\nset -e\n" + s.Script

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(logFile, []byte(""), 0o600); err != nil {
		return nil, err
	}

	binds := append([]string{
		dataDir + ":/home/container:rw",
	}, s.Container.Binds...)

	var env []string
	for _, kv := range s.Container.Env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			env = append(env, fmt.Sprintf("%s=%s", parts[0], parts[1]))
		}
	}

	installCmd := []string{"bash", "/home/container/install.sh"}

	start := container.Config{
		Image:           s.Container.Image,
		Cmd:             installCmd,
		Env:             env,
		WorkingDir:      "/home/container",
		AttachStdin:     true,
		AttachStdout:    true,
		AttachStderr:    true,
		OpenStdin:       true,
		StdinOnce:       false,
		NetworkDisabled: true,
	}
	hostConfig := container.HostConfig{
		Resources:      container.Resources{Memory: 512 * 1024 * 1024},
		Binds:          binds,
		NetworkMode:    container.NetworkMode(s.Container.NetworkMode),
		ReadonlyRootfs: true,
	}

	createResp, err := i.client.ContainerCreate(ctx, &start, &hostConfig, nil, nil, "")
	if err != nil {
		return nil, err
	}
	containerID := createResp.ID
	// Whatever happens below, the install container must not outlive this call:
	// a stopped leftover keeps the name, the bind mount over the server root and
	// the read-only rootfs, so the next install fails with a conflict nobody can
	// explain from the panel. Removal uses a context detached from ctx so a
	// cancelled install can still clean up after itself.
	defer func() {
		rmCtx, rmCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer rmCancel()
		_ = i.client.ContainerRemove(rmCtx, containerID, container.RemoveOptions{Force: true})
	}()

	session, err := i.runtime.AttachConsole(ctx, containerID)
	if err != nil {
		_ = i.runtime.Kill(ctx, containerID)
		return nil, fmt.Errorf("installer: attach install console: %w", err)
	}
	// The console session is the only copy of the installer output the runtime
	// keeps open; leaving it attached pins a stream for the life of the daemon.
	defer func() { _ = session.Close() }()

	// A failed start has to fail the install. Swallowing it here meant the poll
	// loop below saw a container that was never running, read an exit code of 0
	// for it, and reported an installation that never executed as successful.
	if err := i.runtime.Start(ctx, containerID); err != nil {
		_ = i.runtime.Kill(ctx, containerID)
		return nil, fmt.Errorf("installer: start install container: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	var exitCode int
	var containerErr string
	for {
		inspected, inspectErr := i.client.ContainerInspect(waitCtx, containerID)
		if inspectErr != nil {
			return nil, fmt.Errorf("installer: inspect install container: %w", inspectErr)
		}
		if !inspected.State.Running {
			exitCode = inspected.State.ExitCode
			// OOM-killed and never-started containers report a reason instead of
			// an exit code that would only be a number.
			if inspected.State.OOMKilled {
				containerErr = "installation container was killed for exceeding its memory limit"
			} else if inspected.State.Error != "" {
				containerErr = inspected.State.Error
			}
			break
		}
		select {
		case <-waitCtx.Done():
			_ = i.runtime.Kill(ctx, containerID)
			return nil, fmt.Errorf("installer: installation timed out after 30m: %w", waitCtx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}

	readCloser, err := i.client.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return nil, fmt.Errorf("installer: read install logs: %w", err)
	}
	defer readCloser.Close()
	logs, _ := io.ReadAll(readCloser)

	// Persist the log before deciding the outcome, so a failed install still has
	// its output on disk for the operator to read.
	if err := os.WriteFile(logFile, logs, 0o600); err != nil {
		return logs, err
	}

	// The exit code is the authority on whether the script ran to completion.
	// It used to be read, logged and discarded, which let a `set -e` script that
	// died half way through be reported as an installed server.
	if exitCode != 0 {
		if containerErr != "" {
			return logs, fmt.Errorf("installer: installation failed (%s): exit code %d", containerErr, exitCode)
		}
		return logs, fmt.Errorf("installer: installation script failed with exit code %d", exitCode)
	}
	if containerErr != "" {
		return logs, fmt.Errorf("installer: installation did not complete: %s", containerErr)
	}
	// A script that prints its own ERROR/FATAL lines but still exits 0 is the
	// egg's own reporting convention; honour it rather than calling the install
	// clean.
	if strings.Contains(string(logs), "ERROR:") || strings.Contains(string(logs), "FATAL:") {
		return logs, errors.New("installer: installation script reported errors")
	}

	return logs, nil
}

func ensureImage(ctx context.Context, cli *client.Client, imageName string) error {
	_, _, err := cli.ImageInspectWithRaw(ctx, imageName)
	if err == nil {
		return nil
	}
	pull, err := cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return err
	}
	defer pull.Close()
	_, _ = io.Copy(io.Discard, pull)
	return nil
}
