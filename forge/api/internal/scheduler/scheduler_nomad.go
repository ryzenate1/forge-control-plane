package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

type NomadScheduler struct {
	config NomadConfig
}

// NewNomadScheduler refuses to build a scheduler without an explicit agent
// address and datacenter: defaulting addr to the control-plane host's
// loopback or inventing "dc1" would silently submit jobs somewhere the node
// configuration never named.
func NewNomadScheduler(cfg NomadConfig) (*NomadScheduler, error) {
	cfg.Addr = strings.TrimSpace(cfg.Addr)
	cfg.Datacenter = strings.TrimSpace(cfg.Datacenter)
	if cfg.Addr == "" {
		return nil, fmt.Errorf("nomad scheduler requires an explicit addr: refusing to assume a Nomad agent on the control-plane host")
	}
	if err := validateNomadAddr(cfg.Addr); err != nil {
		return nil, err
	}
	if cfg.Datacenter == "" {
		return nil, fmt.Errorf("nomad scheduler requires an explicit datacenter: job specs must name one and inventing one would place workloads the caller never chose")
	}
	if ns := strings.TrimSpace(cfg.Namespace); ns != "" {
		if err := validateResourceIdentifier("namespace", ns); err != nil {
			return nil, err
		}
	}
	return &NomadScheduler{config: cfg}, nil
}

func (s *NomadScheduler) Type() SchedulerType {
	return SchedulerTypeNomad
}

func (s *NomadScheduler) Name() string {
	return "nomad"
}

func (s *NomadScheduler) nomad(ctx context.Context, args ...string) (string, error) {
	if err := validateNomadAddr(s.config.Addr); err != nil {
		return "", err
	}
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
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("nomad %s: %w\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

func validateNomadAddr(addr string) error {
	u, err := url.Parse(strings.TrimSpace(addr))
	if err != nil || u.Host == "" || u.User != nil {
		return fmt.Errorf("invalid Nomad address")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("Nomad address must use HTTP or HTTPS")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	isLoopback := strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !isLoopback {
		return fmt.Errorf("remote Nomad address must use HTTPS")
	}
	return nil
}

func (s *NomadScheduler) Deploy(ctx context.Context, req DeployRequest) (DeployResponse, error) {
	name := req.Name
	if err := validateResourceIdentifier("workload name", name); err != nil {
		return DeployResponse{}, err
	}
	jobSpec := s.buildJobSpec(name, req)

	tmpfile, err := writeTempHCL(jobSpec)
	if err != nil {
		return DeployResponse{}, fmt.Errorf("write job spec: %w", err)
	}
	defer removeTempFile(tmpfile)

	// -detach: without it `nomad job run` monitors the deployment in the
	// foreground forever, so the submit call would hang instead of returning.
	if _, err := s.nomad(ctx, "job", "run", "-detach", tmpfile); err != nil {
		return DeployResponse{}, fmt.Errorf("nomad job run: %w", err)
	}

	status, err := s.GetStatus(ctx, name)
	if err != nil {
		// The job was submitted but its state could not be read back. Report
		// pending — the deployment exists and may still converge — never a
		// success-shaped status for a state that was never observed.
		status = "pending"
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
	if err := validateResourceIdentifier("workload name", name); err != nil {
		return err
	}
	_, err := s.nomad(ctx, "job", "stop", name)
	return err
}

// Start cannot honestly restart a stopped job: Nomad needs the full job spec
// resubmitted and this scheduler is stateless, so it says so instead of
// posing as a restart of a live job.
func (s *NomadScheduler) Start(ctx context.Context, name string) error {
	if err := validateResourceIdentifier("workload name", name); err != nil {
		return err
	}
	status, err := s.GetStatus(ctx, name)
	if err != nil {
		return fmt.Errorf("start nomad job %q: could not read current status: %w", name, err)
	}
	switch status {
	case "running", "pending":
		return nil
	default:
		return fmt.Errorf("nomad job %q is %q: restarting it requires resubmitting the full job spec via Deploy, which this stateless scheduler does not retain", name, status)
	}
}

func (s *NomadScheduler) Restart(ctx context.Context, name string) error {
	if err := validateResourceIdentifier("workload name", name); err != nil {
		return err
	}
	// `job restart` is Nomad's own rolling-replace primitive. The previous
	// implementation stopped allocations one by one and reported success even
	// when the job had no allocations at all — work never performed.
	_, err := s.nomad(ctx, "job", "restart", "-yes", "-detach", name)
	return err
}

func (s *NomadScheduler) Scale(ctx context.Context, req ScaleRequest) error {
	if err := validateResourceIdentifier("workload name", req.Name); err != nil {
		return err
	}
	if req.Replicas < 0 {
		return fmt.Errorf("replica count must not be negative, got %d", req.Replicas)
	}
	_, err := s.nomad(ctx, "job", "scale", req.Name, fmt.Sprintf("%d", req.Replicas))
	return err
}

func (s *NomadScheduler) GetStatus(ctx context.Context, name string) (string, error) {
	if err := validateResourceIdentifier("workload name", name); err != nil {
		return "unknown", err
	}
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
	if strings.TrimSpace(job.Status) == "" {
		// Nomad answers with an empty document for unknown job IDs; an
		// unobserved status is not a status.
		return "unknown", fmt.Errorf("nomad job %q returned no status", name)
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
		portLabels += fmt.Sprintf("      %q,\n", p.Name)
	}
	volumes := ""
	for _, m := range req.Mounts {
		mode := "rw"
		if m.ReadOnly {
			mode = "ro"
		}
		volumes += fmt.Sprintf("          %q,\n", m.Source+":"+m.Target+":"+mode)
	}
	volumeConfig := ""
	if volumes != "" {
		volumeConfig = "        volumes = [\n" + volumes + "        ]\n"
	}
	commandConfig := ""
	if len(req.Command) > 0 {
		commandConfig = fmt.Sprintf("        command = %q\n", req.Command[0])
		if len(req.Command) > 1 {
			commandConfig += "        args = ["
			for i, arg := range req.Command[1:] {
				if i > 0 {
					commandConfig += ", "
				}
				commandConfig += fmt.Sprintf("%q", arg)
			}
			commandConfig += "]\n"
		}
	}

	return fmt.Sprintf(`job %q {
  datacenters = [%q]
  type = "service"

  group %q {
    count = %d

    network {
      mode = "host"
%s
    }

    task %q {
      driver = "docker"

      config {
        image = %q
        ports = [%s]
%s%s
      }

      env {
%s
      }

      resources {
        memory = %d
        cpu = %d
      }
    }
  }
}
`, name, s.config.Datacenter, name, replicas, ports, name, req.Image, portLabels, volumeConfig, commandConfig, envVars, memMB, cpuMHz)
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
