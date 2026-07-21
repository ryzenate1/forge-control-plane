package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type NomadScheduler struct {
	config NomadConfig
}

func NewNomadScheduler(cfg NomadConfig) *NomadScheduler {
	if cfg.Addr == "" {
		cfg.Addr = "http://127.0.0.1:4646"
	}
	if cfg.Region == "" {
		cfg.Region = "global"
	}
	if cfg.Datacenter == "" {
		cfg.Datacenter = "dc1"
	}
	return &NomadScheduler{config: cfg}
}

func (s *NomadScheduler) Type() SchedulerType {
	return SchedulerTypeNomad
}

func (s *NomadScheduler) Name() string {
	return "nomad"
}

func (s *NomadScheduler) nomad(ctx context.Context, args ...string) (string, error) {
	env := []string{}
	if s.config.Addr != "" {
		env = append(env, fmt.Sprintf("NOMAD_ADDR=%s", s.config.Addr))
	}
	if s.config.Region != "" {
		env = append(env, fmt.Sprintf("NOMAD_REGION=%s", s.config.Region))
	}
	if s.config.Namespace != "" {
		env = append(env, fmt.Sprintf("NOMAD_NAMESPACE=%s", s.config.Namespace))
	}
	cmd := exec.CommandContext(ctx, "nomad", args...)
	cmd.Env = append(cmd.Env, env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("nomad %s: %w\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

func (s *NomadScheduler) Deploy(ctx context.Context, req DeployRequest) (DeployResponse, error) {
	name := sanitizeName(req.Name)
	jobSpec := s.buildJobSpec(name, req)

	tmpfile, err := writeTempHCL(jobSpec)
	if err != nil {
		return DeployResponse{}, fmt.Errorf("write job spec: %w", err)
	}
	defer removeTempFile(tmpfile)

	if _, err := s.nomad(ctx, "job", "run", tmpfile); err != nil {
		return DeployResponse{}, fmt.Errorf("nomad job run: %w", err)
	}

	status, err := s.GetStatus(ctx, name)
	if err != nil {
		status = "unknown"
	}

	endpoints := make([]ServiceEndpoint, 0)
	for _, p := range req.Ports {
		endpoints = append(endpoints, ServiceEndpoint{
			Name: p.Name,
			Port: p.Port,
		})
	}

	return DeployResponse{
		Name:      name,
		Status:    status,
		Endpoints: endpoints,
	}, nil
}

func (s *NomadScheduler) Stop(ctx context.Context, name string) error {
	name = sanitizeName(name)
	_, err := s.nomad(ctx, "job", "stop", name)
	return err
}

func (s *NomadScheduler) Start(ctx context.Context, name string) error {
	return s.Restart(ctx, name)
}

func (s *NomadScheduler) Restart(ctx context.Context, name string) error {
	name = sanitizeName(name)
	out, err := s.nomad(ctx, "job", "status", name, "-json")
	if err != nil {
		return err
	}
	var jobStatus struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal([]byte(out), &jobStatus); err != nil {
		return err
	}
	allocOut, err := s.nomad(ctx, "job", "allocations", name, "-json")
	if err != nil {
		return err
	}
	var allocs []struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal([]byte(allocOut), &allocs); err != nil {
		return err
	}
	for _, alloc := range allocs {
		if _, err := s.nomad(ctx, "alloc", "stop", alloc.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *NomadScheduler) Scale(ctx context.Context, req ScaleRequest) error {
	name := sanitizeName(req.Name)
	_, err := s.nomad(ctx, "job", "scale", name, fmt.Sprintf("%d", req.Replicas))
	return err
}

func (s *NomadScheduler) GetStatus(ctx context.Context, name string) (string, error) {
	name = sanitizeName(name)
	out, err := s.nomad(ctx, "job", "status", name, "-json")
	if err != nil {
		return "unknown", err
	}
	var job struct {
		Status string `json:"Status"`
	}
	if err := json.Unmarshal([]byte(out), &job); err != nil {
		return "unknown", err
	}
	switch job.Status {
	case "running":
		return "running", nil
	case "pending":
		return "pending", nil
	case "dead":
		return "stopped", nil
	default:
		return job.Status, nil
	}
}

func (s *NomadScheduler) GetLogs(ctx context.Context, name string, tail int) ([]LogEntry, error) {
	name = sanitizeName(name)
	allocOut, err := s.nomad(ctx, "job", "allocations", name, "-json")
	if err != nil {
		return nil, err
	}
	var allocs []struct {
		ID string `json:"ID"`
	}
	if err := json.Unmarshal([]byte(allocOut), &allocs); err != nil {
		return nil, err
	}
	if len(allocs) == 0 {
		return nil, nil
	}
	allocID := allocs[0].ID
	tailStr := fmt.Sprintf("-n=%d", tail)
	if tail <= 0 {
		tailStr = "-n=100"
	}
	out, err := s.nomad(ctx, "alloc", "logs", allocID, tailStr)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	entries := make([]LogEntry, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		entries = append(entries, LogEntry{Message: line, Source: "stdout"})
	}
	return entries, nil
}

func (s *NomadScheduler) GetEvents(ctx context.Context, name string) ([]Event, error) {
	name = sanitizeName(name)
	out, err := s.nomad(ctx, "job", "status", name, "-json", "-verbose")
	if err != nil {
		return nil, err
	}
	var job struct {
		TaskGroups []struct {
			Name   string `json:"Name"`
			Events []struct {
				Type      string `json:"Type"`
				Message   string `json:"DisplayMessage"`
				Timestamp string `json:"Time"`
			} `json:"Events,omitempty"`
		} `json:"TaskGroups,omitempty"`
	}
	if err := json.Unmarshal([]byte(out), &job); err != nil {
		return nil, err
	}
	events := make([]Event, 0)
	for _, tg := range job.TaskGroups {
		for _, e := range tg.Events {
			events = append(events, Event{
				Type:      e.Type,
				Reason:    tg.Name,
				Message:   e.Message,
				Timestamp: e.Timestamp,
			})
		}
	}
	return events, nil
}

func (s *NomadScheduler) GetResources(ctx context.Context, name string) (ResourceUsage, error) {
	name = sanitizeName(name)
	out, err := s.nomad(ctx, "job", "status", name, "-json")
	if err != nil {
		return ResourceUsage{}, err
	}
	var job struct {
		TaskGroups []struct {
			Tasks []struct {
				Resources struct {
					MemoryMB int64 `json:"MemoryMB"`
					CPU      int64 `json:"CPU"`
				} `json:"Resources"`
			} `json:"Tasks"`
		} `json:"TaskGroups"`
	}
	if err := json.Unmarshal([]byte(out), &job); err != nil {
		return ResourceUsage{}, err
	}
	var totalMem, totalCPU int64
	for _, tg := range job.TaskGroups {
		for _, t := range tg.Tasks {
			totalMem += t.Resources.MemoryMB
			totalCPU += t.Resources.CPU
		}
	}
	return ResourceUsage{
		MemoryMB: totalMem,
	}, nil
}

func (s *NomadScheduler) buildJobSpec(name string, req DeployRequest) string {
	replicas := req.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	memMB := int(req.MemoryMB)
	if memMB <= 0 {
		memMB = 512
	}
	cpuMHz := int(req.CPUMHz)
	if cpuMHz <= 0 {
		cpuMHz = 500
	}
	envVars := ""
	for k, v := range req.Env {
		envVars += fmt.Sprintf("      %q = %q\n", k, v)
	}
	ports := ""
	portLabels := ""
	for _, p := range req.Ports {
		ports += fmt.Sprintf("    port %q { static = %d }\n", p.Name, p.Port)
		portLabels += fmt.Sprintf("      \"%s\",\n", p.Name)
	}
	mounts := ""
	for _, m := range req.Mounts {
		mounts += fmt.Sprintf("    template {\n      data = <<EOH\n%s\nEOH\n      destination = %q\n    }\n", "", m.Target)
	}

	return fmt.Sprintf(`job "%s" {
  datacenters = ["%s"]
  type = "service"

  group "%s" {
    count = %d

    network {
      mode = "host"
%s
    }

    task "%s" {
      driver = "docker"

      config {
        image = "%s"
        ports = [%s]
      }

      env {
%s
      }

      resources {
        memory = %d
        cpu = %d
      }
%s
    }
  }
}
`, name, s.config.Datacenter, name, replicas, ports, name, req.Image, portLabels, envVars, memMB, cpuMHz, mounts)
}

func writeTempHCL(content string) (string, error) {
	f, err := os.CreateTemp("", "nomad-job-*.hcl")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func removeTempFile(path string) {
	os.Remove(path)
}
