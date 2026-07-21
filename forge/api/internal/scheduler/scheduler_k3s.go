package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type K3sScheduler struct {
	config K3sConfig
}

func NewK3sScheduler(cfg K3sConfig) *K3sScheduler {
	if cfg.Namespace == "" {
		cfg.Namespace = "default"
	}
	return &K3sScheduler{config: cfg}
}

func (s *K3sScheduler) Type() SchedulerType {
	return SchedulerTypeK3s
}

func (s *K3sScheduler) Name() string {
	return "k3s"
}

func (s *K3sScheduler) kubectl(ctx context.Context, args ...string) (string, error) {
	baseArgs := []string{}
	if s.config.KubeconfigPath != "" {
		baseArgs = append(baseArgs, "--kubeconfig", s.config.KubeconfigPath)
	}
	if s.config.KubeAPI != "" {
		baseArgs = append(baseArgs, "--server", s.config.KubeAPI)
	}
	baseArgs = append(baseArgs, "-n", s.config.Namespace)
	baseArgs = append(baseArgs, args...)
	cmd := exec.CommandContext(ctx, "kubectl", baseArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("kubectl %s: %w\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

func (s *K3sScheduler) Deploy(ctx context.Context, req DeployRequest) (DeployResponse, error) {
	name := sanitizeName(req.Name)

	deploymentYAML := s.buildDeployment(name, req)
	if _, err := s.kubectl(ctx, "apply", "-f", "-", "--filename", "-", "--filename", deploymentYAML); err != nil {
		return DeployResponse{}, fmt.Errorf("apply deployment: %w", err)
	}

	serviceYAML := s.buildService(name, req)
	if _, err := s.kubectl(ctx, "apply", "-f", "-", "--filename", "-", "--filename", serviceYAML); err != nil {
		return DeployResponse{}, fmt.Errorf("apply service: %w", err)
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

func (s *K3sScheduler) Stop(ctx context.Context, name string) error {
	name = sanitizeName(name)
	_, err := s.kubectl(ctx, "scale", "deployment", name, "--replicas=0")
	return err
}

func (s *K3sScheduler) Start(ctx context.Context, name string) error {
	name = sanitizeName(name)
	_, err := s.kubectl(ctx, "scale", "deployment", name, "--replicas=1")
	return err
}

func (s *K3sScheduler) Restart(ctx context.Context, name string) error {
	name = sanitizeName(name)
	_, err := s.kubectl(ctx, "rollout", "restart", "deployment", name)
	return err
}

func (s *K3sScheduler) Scale(ctx context.Context, req ScaleRequest) error {
	name := sanitizeName(req.Name)
	_, err := s.kubectl(ctx, "scale", "deployment", name, fmt.Sprintf("--replicas=%d", req.Replicas))
	return err
}

func (s *K3sScheduler) GetStatus(ctx context.Context, name string) (string, error) {
	name = sanitizeName(name)
	out, err := s.kubectl(ctx, "get", "deployment", name, "-o", "jsonpath={.status.conditions[?(@.type==\"Available\")].status}")
	if err != nil {
		return "unknown", err
	}
	status := strings.TrimSpace(out)
	switch status {
	case "True":
		return "running", nil
	case "False":
		return "degraded", nil
	default:
		return "unknown", nil
	}
}

func (s *K3sScheduler) GetLogs(ctx context.Context, name string, tail int) ([]LogEntry, error) {
	name = sanitizeName(name)
	tailArg := fmt.Sprintf("--tail=%d", tail)
	if tail <= 0 {
		tailArg = "--tail=100"
	}
	out, err := s.kubectl(ctx, "logs", "deployment/"+name, tailArg)
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

func (s *K3sScheduler) GetEvents(ctx context.Context, name string) ([]Event, error) {
	name = sanitizeName(name)
	out, err := s.kubectl(ctx, "get", "events", "--field-selector", fmt.Sprintf("involvedObject.name=%s", name), "-o", "json")
	if err != nil {
		return nil, err
	}
	var eventList struct {
		Items []struct {
			Type      string `json:"type"`
			Reason    string `json:"reason"`
			Message   string `json:"message"`
			Timestamp string `json:"lastTimestamp"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &eventList); err != nil {
		return nil, err
	}
	events := make([]Event, 0, len(eventList.Items))
	for _, e := range eventList.Items {
		events = append(events, Event{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   e.Message,
			Timestamp: e.Timestamp,
		})
	}
	return events, nil
}

func (s *K3sScheduler) GetResources(ctx context.Context, name string) (ResourceUsage, error) {
	name = sanitizeName(name)
	out, err := s.kubectl(ctx, "top", "pod", "-l", fmt.Sprintf("app=%s", name), "--no-headers")
	if err != nil {
		return ResourceUsage{}, err
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 0 {
		return ResourceUsage{}, nil
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 3 {
		return ResourceUsage{}, nil
	}
	cpuStr := fields[1]
	memStr := fields[2]
	cpuPercent := parseCPUPercent(cpuStr)
	memMB := parseMemoryMB(memStr)
	return ResourceUsage{CPUPercent: cpuPercent, MemoryMB: memMB}, nil
}

func (s *K3sScheduler) buildDeployment(name string, req DeployRequest) string {
	replicas := req.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	envVars := ""
	for k, v := range req.Env {
		envVars += fmt.Sprintf("            - name: %s\n              value: %q\n", k, v)
	}
	ports := ""
	for _, p := range req.Ports {
		ports += fmt.Sprintf("            - containerPort: %d\n              protocol: %s\n", p.TargetPort, p.Protocol)
	}
	mounts := ""
	for _, m := range req.Mounts {
		mounts += fmt.Sprintf("            - mountPath: %q\n              name: vol-%s\n", m.Target, sanitizeName(m.Source))
	}
	volumes := ""
	for _, m := range req.Mounts {
		volumes += fmt.Sprintf("        - name: vol-%s\n          hostPath:\n            path: %q\n            type: DirectoryOrCreate\n", sanitizeName(m.Source), m.Source)
	}
	cmdStr := ""
	if len(req.Command) > 0 {
		cmdStr = fmt.Sprintf("            command: [%s]", quoteJoin(req.Command))
	}
	memoryLimit := ""
	cpuLimit := ""
	if req.MemoryMB > 0 {
		memoryLimit = fmt.Sprintf("            memory: %dMi", req.MemoryMB)
	}
	if req.CPUMHz > 0 {
		cpuLimit = fmt.Sprintf("            cpu: %dm", req.CPUMHz)
	}

	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  labels:
    app: %s
spec:
  replicas: %d
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      containers:
      - name: %s
        image: %s
%s
        env:
%s
        ports:
%s
        resources:
          limits:
%s
%s
        volumeMounts:
%s
      volumes:
%s
`, name, name, replicas, name, name, name, req.Image, cmdStr, envVars, ports, memoryLimit, cpuLimit, mounts, volumes)
}

func (s *K3sScheduler) buildService(name string, req DeployRequest) string {
	ports := ""
	for _, p := range req.Ports {
		np := ""
		if p.NodePort > 0 {
			np = fmt.Sprintf("    nodePort: %d", p.NodePort)
		}
		ports += fmt.Sprintf("    - name: %s\n      port: %d\n      targetPort: %d\n      protocol: %s\n%s\n", p.Name, p.Port, p.TargetPort, p.Protocol, np)
	}
	return fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: %s-svc
  labels:
    app: %s
spec:
  selector:
    app: %s
  type: NodePort
  ports:
%s
`, name, name, name, ports)
}

func sanitizeName(name string) string {
	s := strings.NewReplacer(
		"_", "-",
		".", "-",
		" ", "-",
		"/", "-",
	).Replace(name)
	s = strings.ToLower(s)
	return s
}

func quoteJoin(strs []string) string {
	quoted := make([]string, len(strs))
	for i, s := range strs {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(quoted, ", ")
}

func parseCPUPercent(s string) float64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "m") {
		milli := strings.TrimSuffix(s, "m")
		var v float64
		fmt.Sscanf(milli, "%f", &v)
		return v / 1000.0 * 100
	}
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v * 100
}

func parseMemoryMB(s string) int64 {
	s = strings.TrimSpace(s)
	var v float64
	if strings.HasSuffix(s, "Mi") {
		fmt.Sscanf(s, "%fMi", &v)
		return int64(v)
	}
	if strings.HasSuffix(s, "Ki") {
		fmt.Sscanf(s, "%fKi", &v)
		return int64(v / 1024)
	}
	fmt.Sscanf(s, "%f", &v)
	return int64(v)
}
