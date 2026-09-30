package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type K3sScheduler struct {
	config K3sConfig
}

// NewK3sScheduler refuses to build a scheduler without an explicit cluster
// target: with neither kubeconfigPath nor kubeApi set, kubectl would silently
// act on whatever ambient context the control-plane host happens to have —
// a different cluster than the node asked for.
func NewK3sScheduler(cfg K3sConfig) (*K3sScheduler, error) {
	if strings.TrimSpace(cfg.KubeconfigPath) == "" && strings.TrimSpace(cfg.KubeAPI) == "" {
		return nil, fmt.Errorf("k3s scheduler requires an explicit target: set kubeconfigPath or kubeApi instead of relying on the ambient kubectl context")
	}
	if path := strings.TrimSpace(cfg.KubeconfigPath); path != "" {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("k3s kubeconfigPath %q is not readable: %w", path, err)
		}
		cfg.KubeconfigPath = path
	}
	if cfg.Namespace == "" {
		cfg.Namespace = "default"
	} else if err := validateResourceIdentifier("namespace", cfg.Namespace); err != nil {
		return nil, err
	}
	return &K3sScheduler{config: cfg}, nil
}

// validateResourceIdentifier rejects instead of rewrites: silently
// sanitizing a caller-supplied name (e.g. "My App" → "my-app") would point
// every later operation at a different resource than the one requested.
func validateResourceIdentifier(kind, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%s is required", kind)
	}
	if name != sanitizeName(name) {
		return fmt.Errorf("%s %q is not a valid resource identifier: use lowercase letters, digits and single hyphens, at most 63 characters", kind, name)
	}
	return nil
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
	name := req.Name
	if err := validateResourceIdentifier("workload name", name); err != nil {
		return DeployResponse{}, err
	}

	deploymentYAML, err := s.buildDeployment(name, req)
	if err != nil {
		return DeployResponse{}, fmt.Errorf("build deployment manifest: %w", err)
	}
	if _, err := s.kubectlInput(ctx, deploymentYAML, "apply", "-f", "-"); err != nil {
		return DeployResponse{}, fmt.Errorf("apply deployment: %w", err)
	}

	serviceYAML, err := s.buildService(name, req)
	if err != nil {
		return DeployResponse{}, fmt.Errorf("build service manifest: %w", err)
	}
	if _, err := s.kubectlInput(ctx, serviceYAML, "apply", "-f", "-"); err != nil {
		return DeployResponse{}, fmt.Errorf("apply service: %w", err)
	}

	status, err := s.GetStatus(ctx, name)
	if err != nil {
		// The deployment was applied but its state could not be read back.
		// Report pending — it exists and may still converge — never a
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

// stoppedReplicasAnnotation records the replica count a deployment had before
// Stop scaled it to zero, so Start restores what was actually running instead
// of inventing a replica count.
const stoppedReplicasAnnotation = "scheduler.forge/stopped-replicas"

// getDeployment reads the deployment object once so Stop/Start act on state
// observed from the platform, never on a guess.
func (s *K3sScheduler) getDeployment(ctx context.Context, name string) (specReplicas *int64, annotations map[string]string, err error) {
	out, err := s.kubectl(ctx, "get", "deployment", name, "-o", "json")
	if err != nil {
		return nil, nil, err
	}
	var dep struct {
		Metadata struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
		Spec struct {
			Replicas *int64 `json:"replicas"`
		} `json:"spec"`
	}
	if err := json.Unmarshal([]byte(out), &dep); err != nil {
		return nil, nil, fmt.Errorf("parse deployment %q: %w", name, err)
	}
	return dep.Spec.Replicas, dep.Metadata.Annotations, nil
}

func (s *K3sScheduler) Stop(ctx context.Context, name string) error {
	if err := validateResourceIdentifier("workload name", name); err != nil {
		return err
	}
	replicas, _, err := s.getDeployment(ctx, name)
	if err != nil {
		return fmt.Errorf("read deployment %q before stop: %w", name, err)
	}
	if replicas != nil && *replicas == 0 {
		return nil // already stopped
	}
	if replicas == nil {
		return fmt.Errorf("deployment %q does not report a replica count; refusing to stop it without recording how to start it again", name)
	}
	if _, err := s.kubectl(ctx, "annotate", "deployment", name,
		fmt.Sprintf("%s=%d", stoppedReplicasAnnotation, *replicas), "--overwrite"); err != nil {
		return fmt.Errorf("record pre-stop replica count for %q: %w", name, err)
	}
	_, err = s.kubectl(ctx, "scale", "deployment", name, "--replicas=0")
	return err
}

func (s *K3sScheduler) Start(ctx context.Context, name string) error {
	if err := validateResourceIdentifier("workload name", name); err != nil {
		return err
	}
	replicas, annotations, err := s.getDeployment(ctx, name)
	if err != nil {
		return fmt.Errorf("read deployment %q before start: %w", name, err)
	}
	if replicas != nil && *replicas > 0 {
		return nil // already running
	}
	prev, ok := annotations[stoppedReplicasAnnotation]
	if !ok {
		return fmt.Errorf("deployment %q is scaled to zero but has no %s annotation; refusing to guess a replica count to start it with", name, stoppedReplicasAnnotation)
	}
	target, err := strconv.ParseInt(strings.TrimSpace(prev), 10, 32)
	if err != nil || target <= 0 {
		return fmt.Errorf("deployment %q has an unusable %s annotation %q", name, stoppedReplicasAnnotation, prev)
	}
	_, err = s.kubectl(ctx, "scale", "deployment", name, fmt.Sprintf("--replicas=%d", target))
	return err
}

func (s *K3sScheduler) Restart(ctx context.Context, name string) error {
	if err := validateResourceIdentifier("workload name", name); err != nil {
		return err
	}
	_, err := s.kubectl(ctx, "rollout", "restart", "deployment", name)
	return err
}

func (s *K3sScheduler) Scale(ctx context.Context, req ScaleRequest) error {
	if err := validateResourceIdentifier("workload name", req.Name); err != nil {
		return err
	}
	if req.Replicas < 0 {
		return fmt.Errorf("replica count must not be negative, got %d", req.Replicas)
	}
	_, err := s.kubectl(ctx, "scale", "deployment", req.Name, fmt.Sprintf("--replicas=%d", req.Replicas))
	return err
}

func (s *K3sScheduler) GetStatus(ctx context.Context, name string) (string, error) {
	if err := validateResourceIdentifier("workload name", name); err != nil {
		return "unknown", err
	}
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
		// An unrecognized or empty condition is not a success signal. Report
		// pending so callers keep waiting instead of declaring victory over
		// a state they never observed.
		return "pending", nil
	}
}

func (s *K3sScheduler) GetLogs(ctx context.Context, name string, tail int) ([]LogEntry, error) {
	if err := validateResourceIdentifier("workload name", name); err != nil {
		return nil, err
	}
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
	if err := validateResourceIdentifier("workload name", name); err != nil {
		return nil, err
	}
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
	if err := validateResourceIdentifier("workload name", name); err != nil {
		return ResourceUsage{}, err
	}
	out, err := s.kubectl(ctx, "top", "pod", "-l", fmt.Sprintf("app=%s", name), "--no-headers")
	if err != nil {
		return ResourceUsage{}, err
	}
	var first []string
	for _, line := range strings.Split(out, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			first = fields
			break
		}
	}
	if first == nil {
		return ResourceUsage{}, fmt.Errorf("metrics-server reported no pod usage for %q; resources are not available", name)
	}
	if len(first) < 3 {
		return ResourceUsage{}, fmt.Errorf("unexpected metrics-server output for %q: %q", name, strings.Join(first, " "))
	}
	cpuPercent, err := parseCPUPercent(first[1])
	if err != nil {
		return ResourceUsage{}, fmt.Errorf("parse CPU usage for %q: %w", name, err)
	}
	memMB, err := parseMemoryMB(first[2])
	if err != nil {
		return ResourceUsage{}, fmt.Errorf("parse memory usage for %q: %w", name, err)
	}
	// DiskMB stays nil: metrics-server does not report filesystem usage, and
	// "not reported" must not be encoded as zero.
	return ResourceUsage{CPUPercent: &cpuPercent, MemoryMB: &memMB}, nil
}

func (s *K3sScheduler) buildDeployment(name string, req DeployRequest) (string, error) {
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
	for i, m := range req.Mounts {
		// Indexed volume names keep two sources that collapse to the same
		// sanitized form from aliasing onto one volume.
		volumeName := fmt.Sprintf("vol-%d-%s", i, sanitizeName(m.Source))
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
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *K3sScheduler) buildService(name string, req DeployRequest) (string, error) {
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
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return "", err
	}
	return string(data), nil
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

// parseCPUPercent converts a metrics-server CPU reading (nanocores "n",
// millicores "m", or whole cores) into a percentage of one core. Unparseable
// input is an error: silently returning 0 would report "idle" for a
// measurement that was never read.
func parseCPUPercent(s string) (float64, error) {
	s = strings.TrimSpace(s)
	var cores float64
	switch {
	case strings.HasSuffix(s, "n"):
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "n"), 64)
		if err != nil {
			return 0, fmt.Errorf("unparseable CPU measurement %q", s)
		}
		cores = v / 1e9
	case strings.HasSuffix(s, "m"):
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "m"), 64)
		if err != nil {
			return 0, fmt.Errorf("unparseable CPU measurement %q", s)
		}
		cores = v / 1000
	default:
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, fmt.Errorf("unparseable CPU measurement %q", s)
		}
		cores = v
	}
	return cores * 100, nil
}

// parseMemoryMB converts a metrics-server memory reading (Ki/Mi/Gi/Ti or a
// bare byte count) into megabytes, erroring instead of reporting zero for
// input it cannot read.
func parseMemoryMB(s string) (int64, error) {
	s = strings.TrimSpace(s)
	var numStr string
	var mbPerUnit float64
	switch {
	case strings.HasSuffix(s, "Ti"):
		numStr, mbPerUnit = strings.TrimSuffix(s, "Ti"), 1024*1024
	case strings.HasSuffix(s, "Gi"):
		numStr, mbPerUnit = strings.TrimSuffix(s, "Gi"), 1024
	case strings.HasSuffix(s, "Mi"):
		numStr, mbPerUnit = strings.TrimSuffix(s, "Mi"), 1
	case strings.HasSuffix(s, "Ki"):
		numStr, mbPerUnit = strings.TrimSuffix(s, "Ki"), 1.0/1024
	case strings.HasSuffix(s, "M"):
		numStr, mbPerUnit = strings.TrimSuffix(s, "M"), 1
	case strings.HasSuffix(s, "K"):
		numStr, mbPerUnit = strings.TrimSuffix(s, "K"), 1.0/1024
	case strings.HasSuffix(s, "G"):
		numStr, mbPerUnit = strings.TrimSuffix(s, "G"), 1024
	default:
		numStr, mbPerUnit = s, 1.0/(1024*1024) // bare value is bytes
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("unparseable memory measurement %q", s)
	}
	return int64(v * mbPerUnit), nil
}
