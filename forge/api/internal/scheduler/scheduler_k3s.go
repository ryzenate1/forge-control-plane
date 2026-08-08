package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
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
	return s.kubectlInput(ctx, "", args...)
}

func (s *K3sScheduler) kubectlInput(ctx context.Context, input string, args ...string) (string, error) {
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
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
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
	if _, err := s.kubectlInput(ctx, deploymentYAML, "apply", "-f", "-"); err != nil {
		return DeployResponse{}, fmt.Errorf("apply deployment: %w", err)
	}

	serviceYAML := s.buildService(name, req)
	if _, err := s.kubectlInput(ctx, serviceYAML, "apply", "-f", "-"); err != nil {
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
	envVars := make([]map[string]any, 0, len(req.Env))
	for k, v := range req.Env {
		envVars = append(envVars, map[string]any{"name": k, "value": v})
	}
	ports := make([]map[string]any, 0, len(req.Ports))
	for _, p := range req.Ports {
		ports = append(ports, map[string]any{
			"containerPort": p.TargetPort,
			"protocol":      strings.ToUpper(p.Protocol),
		})
	}
	mounts := make([]map[string]any, 0, len(req.Mounts))
	volumes := make([]map[string]any, 0, len(req.Mounts))
	for _, m := range req.Mounts {
		volumeName := "vol-" + sanitizeName(m.Source)
		mounts = append(mounts, map[string]any{
			"mountPath": m.Target,
			"name":      volumeName,
			"readOnly":  m.ReadOnly,
		})
		volumes = append(volumes, map[string]any{
			"name": volumeName,
			"hostPath": map[string]any{
				"path": m.Source,
				"type": "DirectoryOrCreate",
			},
		})
	}
	container := map[string]any{
		"name":         name,
		"image":        req.Image,
		"env":          envVars,
		"ports":        ports,
		"volumeMounts": mounts,
	}
	if len(req.Command) > 0 {
		container["command"] = req.Command
	}
	limits := map[string]any{}
	if req.MemoryMB > 0 {
		limits["memory"] = fmt.Sprintf("%dMi", req.MemoryMB)
	}
	if req.CPUMHz > 0 {
		limits["cpu"] = fmt.Sprintf("%dm", req.CPUMHz)
	}
	if len(limits) > 0 {
		container["resources"] = map[string]any{"limits": limits}
	}

	manifest := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": name, "labels": map[string]string{"app": name}},
		"spec": map[string]any{
			"replicas": replicas,
			"selector": map[string]any{"matchLabels": map[string]string{"app": name}},
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]string{"app": name}},
				"spec": map[string]any{
					"containers": []map[string]any{container},
					"volumes":    volumes,
				},
			},
		},
	}
	data, _ := yaml.Marshal(manifest)
	return string(data)
}

func (s *K3sScheduler) buildService(name string, req DeployRequest) string {
	ports := make([]map[string]any, 0, len(req.Ports))
	for _, p := range req.Ports {
		port := map[string]any{
			"name":       p.Name,
			"port":       p.Port,
			"targetPort": p.TargetPort,
			"protocol":   strings.ToUpper(p.Protocol),
		}
		if p.NodePort > 0 {
			port["nodePort"] = p.NodePort
		}
		ports = append(ports, port)
	}
	manifest := map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": name + "-svc", "labels": map[string]string{"app": name}},
		"spec": map[string]any{
			"selector": map[string]string{"app": name},
			"type":     "NodePort",
			"ports":    ports,
		},
	}
	data, _ := yaml.Marshal(manifest)
	return string(data)
}

func sanitizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastHyphen := false
	for _, r := range name {
		isAlphaNumeric := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if isAlphaNumeric {
			b.WriteRune(r)
			lastHyphen = false
		} else if !lastHyphen && b.Len() > 0 {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "app"
	}
	if len(s) > 63 {
		s = strings.TrimRight(s[:63], "-")
	}
	return s
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
