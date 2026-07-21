package health

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

type DockerCheck struct {
	label string
}

func NewDockerCheck() *DockerCheck {
	return &DockerCheck{
		label: "Docker Daemon",
	}
}

func (c *DockerCheck) Name() string   { return "docker" }
func (c *DockerCheck) Label() string  { return c.label }
func (c *DockerCheck) Critical() bool { return true }

func (c *DockerCheck) Run(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Name:   c.Name(),
		Label:  c.Label(),
		Status: StatusOK,
	}

	if err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}").Run(); err != nil {
		result.Status = StatusFailed
		result.Message = "Docker daemon is not reachable"
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}\n{{.OSType}}\n{{.Architecture}}\n{{.NCPU}}\n{{.MemTotal}}").Output()
	if err != nil {
		result.Status = StatusFailed
		result.Message = "Docker info query failed"
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	details := make(map[string]any)
	if len(lines) > 0 {
		details["version"] = lines[0]
	}
	if len(lines) > 1 {
		details["os"] = lines[1]
	}
	if len(lines) > 2 {
		details["architecture"] = lines[2]
	}
	if len(lines) > 3 {
		details["cpus"] = lines[3]
	}
	if len(lines) > 4 {
		details["totalMemory"] = lines[4]
	}

	result.Details = details
	result.Message = "Docker daemon is healthy"
	result.LatencyMs = time.Since(start).Milliseconds()

	return result
}
