package compose

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type GitStackConfig struct {
	SourceID         string `json:"sourceId,omitempty"`
	RepositoryURL    string `json:"repositoryUrl,omitempty"`
	RepositoryPath   string `json:"repositoryPath,omitempty"`
	Branch           string `json:"branch,omitempty"`
	CommitSHA        string `json:"commitSha,omitempty"`
	DesiredCommitSHA string `json:"desiredCommitSha,omitempty"`
	PreviousCommit   string `json:"previousCommit,omitempty"`
	PreviousCompose  string `json:"previousCompose,omitempty"`
	AutoUpdate       bool   `json:"autoUpdate"`
	PollIntervalSec  int    `json:"pollIntervalSec,omitempty"`
	WebhookID        string `json:"webhookId,omitempty"`
	UpdateStatus     string `json:"updateStatus,omitempty"`
	UpdateError      string `json:"updateError,omitempty"`
	CredentialID     string `json:"credentialId,omitempty"`
}

type GitUpdatePreview struct {
	StackID         string    `json:"stackId"`
	CurrentCommit   string    `json:"currentCommit"`
	DesiredCommit   string    `json:"desiredCommit"`
	CurrentBranch   string    `json:"currentBranch"`
	HasUpdate       bool      `json:"hasUpdate"`
	ServicesChanged []string  `json:"servicesChanged,omitempty"`
	PreviewYAML     string    `json:"previewYaml,omitempty"`
	CheckedAt       time.Time `json:"checkedAt"`
}

type GitServiceState struct {
	Name       string `json:"name"`
	Image      string `json:"image"`
	Status     string `json:"status"`
	State      string `json:"state"`
	Ports      string `json:"ports"`
	UpdateOK   bool   `json:"updateOk"`
	UpdateMsg  string `json:"updateMsg,omitempty"`
}

type GitDeployResult struct {
	StackID      string           `json:"stackId"`
	CommitSHA    string            `json:"commitSha"`
	Branch       string            `json:"branch"`
	Services     []GitServiceState `json:"services,omitempty"`
	Status       string            `json:"status"`
	Error        string            `json:"error,omitempty"`
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationIssue struct {
	Field    string `json:"field"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type ValidateResult struct {
	Valid    bool              `json:"valid"`
	Errors   []ValidationError `json:"errors,omitempty"`
	Warnings []ValidationError `json:"warnings,omitempty"`
	Summary  *ParsedCompose    `json:"summary,omitempty"`
}

type rawCompose struct {
	Version  string                `yaml:"version,omitempty"`
	Name     string                `yaml:"name,omitempty"`
	Services map[string]rawService `yaml:"services,omitempty"`
	Networks map[string]rawNetwork `yaml:"networks,omitempty"`
	Volumes  map[string]rawVolume  `yaml:"volumes,omitempty"`
	Secrets  map[string]rawSecret  `yaml:"secrets,omitempty"`
	Configs  map[string]rawConfig  `yaml:"configs,omitempty"`
	Include  []rawInclude          `yaml:"include,omitempty"`
}

type rawService struct {
	Image       string                 `yaml:"image,omitempty"`
	Build       map[string]interface{} `yaml:"build,omitempty"`
	Ports       []interface{}          `yaml:"ports,omitempty"`
	Environment interface{}            `yaml:"environment,omitempty"`
	Volumes     []interface{}          `yaml:"volumes,omitempty"`
	DependsOn   interface{}            `yaml:"depends_on,omitempty"`
	Profiles    []string               `yaml:"profiles,omitempty"`
	Restart     string                 `yaml:"restart,omitempty"`
	Command     interface{}            `yaml:"command,omitempty"`
	Entrypoint  interface{}            `yaml:"entrypoint,omitempty"`
	HealthCheck map[string]interface{} `yaml:"healthcheck,omitempty"`
	Deploy      map[string]interface{} `yaml:"deploy,omitempty"`
	Secrets     []interface{}          `yaml:"secrets,omitempty"`
	Configs     []interface{}          `yaml:"configs,omitempty"`
}

type rawNetwork struct {
	Driver   string            `yaml:"driver,omitempty"`
	External interface{}       `yaml:"external,omitempty"`
	Labels   map[string]string `yaml:"labels,omitempty"`
}

type rawVolume struct {
	Driver   string            `yaml:"driver,omitempty"`
	External interface{}       `yaml:"external,omitempty"`
	Labels   map[string]string `yaml:"labels,omitempty"`
}

type rawSecret struct {
	File     string `yaml:"file,omitempty"`
	External bool   `yaml:"external,omitempty"`
}

type rawConfig struct {
	File     string `yaml:"file,omitempty"`
	External bool   `yaml:"external,omitempty"`
}

type rawInclude struct {
	Path       interface{} `yaml:"path,omitempty"`
	ProjectDir string      `yaml:"project_directory,omitempty"`
	EnvFile    interface{} `yaml:"env_file,omitempty"`
}

func (s *Service) Parse(content []byte, workingDir string) (*ParsedCompose, error) {
	return s.ParseComposeYAML(content, workingDir, nil)
}

func (s *Service) Validate(content []byte, workingDir string) *ValidateResult {
	return s.ValidateCompose(content, workingDir)
}

func (s *Service) ParseComposeYAML(content []byte, workingDir string, envVars map[string]string) (*ParsedCompose, error) {
	var err error
	content, err = interpolateEnv(content, envVars)
	if err != nil {
		return nil, err
	}

	var raw rawCompose
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse compose YAML: %w", err)
	}

	projectName := raw.Name
	if projectName == "" && workingDir != "" {
		projectName = filepath.Base(workingDir)
	}

	parsed := &ParsedCompose{
		Version:  raw.Version,
		Name:     projectName,
		Services: make([]ServiceSummary, 0, len(raw.Services)),
		Networks: make([]NetworkSummary, 0, len(raw.Networks)),
		Volumes:  make([]VolumeSummary, 0, len(raw.Volumes)),
		Secrets:  make([]SecretSummary, 0, len(raw.Secrets)),
		Configs:  make([]ConfigSummary, 0, len(raw.Configs)),
	}

	for name, svc := range raw.Services {
		summary := ServiceSummary{
			Name:        name,
			Image:       svc.Image,
			Ports:       normalizePorts(svc.Ports),
			Environment: normalizeEnv(svc.Environment),
			Volumes:     normalizeVolumes(svc.Volumes),
			DependsOn:   normalizeDependsOn(svc.DependsOn),
			Profiles:    svc.Profiles,
			Restart:     svc.Restart,
		}

		summary.Command = stringifyValue(svc.Command)
		summary.Entrypoint = stringifyValue(svc.Entrypoint)

		if svc.Build != nil {
			summary.Build = normalizeBuild(svc.Build)
		}
		if svc.HealthCheck != nil {
			summary.HealthCheck = normalizeHealthCheck(svc.HealthCheck)
		}
		if svc.Deploy != nil {
			summary.Deploy = normalizeDeploy(svc.Deploy)
		}

		parsed.Services = append(parsed.Services, summary)
	}

	for name, net := range raw.Networks {
		parsed.Networks = append(parsed.Networks, NetworkSummary{
			Name:     name,
			Driver:   net.Driver,
			External: isExternal(net.External),
			Labels:   net.Labels,
		})
	}

	for name, vol := range raw.Volumes {
		parsed.Volumes = append(parsed.Volumes, VolumeSummary{
			Name:     name,
			Driver:   vol.Driver,
			External: isExternal(vol.External),
			Labels:   vol.Labels,
		})
	}

	for name, sec := range raw.Secrets {
		parsed.Secrets = append(parsed.Secrets, SecretSummary{
			Name:     name,
			File:     sec.File,
			External: sec.External,
		})
	}

	for name, cfg := range raw.Configs {
		parsed.Configs = append(parsed.Configs, ConfigSummary{
			Name:     name,
			File:     cfg.File,
			External: cfg.External,
		})
	}

	return parsed, nil
}

func (s *Service) ValidateCompose(content []byte, workingDir string) *ValidateResult {
	result := &ValidateResult{Valid: true}

	parsed, err := s.ParseComposeYAML(content, workingDir, nil)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "yaml",
			Message: err.Error(),
		})
		return result
	}

	result.Summary = parsed

	if len(parsed.Services) == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Field:   "services",
			Message: "at least one service is required",
		})
		return result
	}

	for _, svc := range parsed.Services {
		if svc.Image == "" && svc.Build == nil {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Field:   fmt.Sprintf("services.%s", svc.Name),
				Message: "service must specify either 'image' or 'build'",
			})
		}
		if len(svc.Profiles) > 0 {
			result.Warnings = append(result.Warnings, ValidationError{
				Field:   fmt.Sprintf("services.%s.profiles", svc.Name),
				Message: "profiles are not currently enforced by Forge",
			})
		}
		if svc.HealthCheck != nil {
			result.Warnings = append(result.Warnings, ValidationError{
				Field:   fmt.Sprintf("services.%s.healthcheck", svc.Name),
				Message: "healthcheck support is not yet implemented in Forge",
			})
		}
		if svc.Deploy != nil {
			result.Warnings = append(result.Warnings, ValidationError{
				Field:   fmt.Sprintf("services.%s.deploy", svc.Name),
				Message: "deploy section is not yet fully supported in Forge",
			})
		}
	}

	var rawMap map[string]interface{}
	if err := yaml.Unmarshal(content, &rawMap); err == nil {
		issues := ValidateComposeSecurity(rawMap)
		for _, issue := range issues {
			if issue.Severity == "error" {
				result.Valid = false
				result.Errors = append(result.Errors, ValidationError{
					Field:   issue.Field,
					Message: issue.Message,
				})
			} else {
				result.Warnings = append(result.Warnings, ValidationError{
					Field:   issue.Field,
					Message: issue.Message,
				})
			}
		}
	}

	for _, sec := range parsed.Secrets {
		if sec.External {
			result.Warnings = append(result.Warnings, ValidationError{
				Field:   fmt.Sprintf("secrets.%s", sec.Name),
				Message: "external secrets are not yet supported in Forge",
			})
		}
	}

	for _, cfg := range parsed.Configs {
		if cfg.External {
			result.Warnings = append(result.Warnings, ValidationError{
				Field:   fmt.Sprintf("configs.%s", cfg.Name),
				Message: "external configs are not yet supported in Forge",
			})
		}
	}

	return result
}

func (s *Service) ImportComposeProject(content []byte, name, workingDir string) (*ParsedCompose, error) {
	return s.ParseComposeYAML(content, workingDir, nil)
}

func ParseSummary(content []byte, workingDir string) (*ParsedCompose, error) {
	var s Service
	return s.ParseComposeYAML(content, workingDir, nil)
}

func ValidateSummary(content []byte, workingDir string) *ValidateResult {
	var s Service
	return s.ValidateCompose(content, workingDir)
}

var composeVarRe = regexp.MustCompile(`\$\{([^}]+)\}|\$\$`)

func interpolateEnv(content []byte, envVars map[string]string) ([]byte, error) {
	if envVars == nil {
		envVars = map[string]string{}
	}
	var err error
	out := composeVarRe.ReplaceAllFunc(content, func(match []byte) []byte {
		if err != nil {
			return nil
		}
		if string(match) == "$$" {
			return []byte("$")
		}
		inner := string(match[2 : len(match)-1])

		if idx := strings.Index(inner, ":-"); idx >= 0 {
			name := inner[:idx]
			def := inner[idx+2:]
			if val, ok := envVars[name]; ok && val != "" {
				return []byte(val)
			}
			return []byte(def)
		}

		if idx := strings.Index(inner, "-"); idx >= 0 {
			name := inner[:idx]
			def := inner[idx+1:]
			if val, ok := envVars[name]; ok {
				return []byte(val)
			}
			return []byte(def)
		}

		if idx := strings.Index(inner, ":?"); idx >= 0 {
			name := inner[:idx]
			msg := inner[idx+2:]
			if val, ok := envVars[name]; ok && val != "" {
				return []byte(val)
			}
			if msg == "" {
				err = fmt.Errorf("required variable %q is unset", name)
			} else {
				err = fmt.Errorf("%s", msg)
			}
			return nil
		}

		if idx := strings.Index(inner, "?"); idx >= 0 {
			name := inner[:idx]
			msg := inner[idx+1:]
			if val, ok := envVars[name]; ok {
				return []byte(val)
			}
			if msg == "" {
				err = fmt.Errorf("required variable %q is unset", name)
			} else {
				err = fmt.Errorf("%s", msg)
			}
			return nil
		}

		if val, ok := envVars[inner]; ok {
			return []byte(val)
		}
		return []byte("")
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func ValidateComposeSecurity(rawCompose interface{}) []ValidationIssue {
	var issues []ValidationIssue

	compose, ok := rawCompose.(map[string]interface{})
	if !ok {
		return issues
	}

	services, ok := compose["services"].(map[string]interface{})
	if !ok {
		return issues
	}

	for name, svcRaw := range services {
		svc, ok := svcRaw.(map[string]interface{})
		if !ok {
			continue
		}

		if privileged, ok := svc["privileged"]; ok {
			if b, ok := privileged.(bool); ok && b {
				issues = append(issues, ValidationIssue{
					Field:    fmt.Sprintf("services.%s.privileged", name),
					Message:  "privileged mode is not allowed for security reasons",
					Severity: "error",
				})
			}
		}

		if networkMode, ok := svc["network_mode"]; ok {
			if mode, ok := networkMode.(string); ok && mode == "host" {
				issues = append(issues, ValidationIssue{
					Field:    fmt.Sprintf("services.%s.network_mode", name),
					Message:  "network_mode 'host' is not allowed for security reasons",
					Severity: "error",
				})
			}
		}

		if pid, ok := svc["pid"]; ok {
			if p, ok := pid.(string); ok && p == "host" {
				issues = append(issues, ValidationIssue{
					Field:    fmt.Sprintf("services.%s.pid", name),
					Message:  "pid 'host' is not allowed for security reasons",
					Severity: "error",
				})
			}
		}

		if ipc, ok := svc["ipc"]; ok {
			if i, ok := ipc.(string); ok && i == "host" {
				issues = append(issues, ValidationIssue{
					Field:    fmt.Sprintf("services.%s.ipc", name),
					Message:  "ipc 'host' is not allowed for security reasons",
					Severity: "error",
				})
			}
		}

		checkVolumesSecurity(name, svc["volumes"], &issues)
		checkCapAddSecurity(name, svc["cap_add"], &issues)
		checkDevicesSecurity(name, svc["devices"], &issues)

		if _, hasSecOpt := svc["security_opt"]; hasSecOpt {
			issues = append(issues, ValidationIssue{
				Field:    fmt.Sprintf("services.%s.security_opt", name),
				Message:  "security_opt overrides may weaken container isolation",
				Severity: "warning",
			})
		}

		if usernsMode, ok := svc["userns_mode"]; ok {
			if u, ok := usernsMode.(string); ok && u == "host" {
				issues = append(issues, ValidationIssue{
					Field:    fmt.Sprintf("services.%s.userns_mode", name),
					Message:  "userns_mode 'host' disables user namespace remapping",
					Severity: "warning",
				})
			}
		}

		if _, hasCN := svc["container_name"]; hasCN {
			issues = append(issues, ValidationIssue{
				Field:    fmt.Sprintf("services.%s.container_name", name),
				Message:  "container_name may conflict with other containers on the node",
				Severity: "warning",
			})
		}

		if restart, ok := svc["restart"]; ok {
			if r, ok := restart.(string); ok && r == "always" {
				issues = append(issues, ValidationIssue{
					Field:    fmt.Sprintf("services.%s.restart", name),
					Message:  "restart 'always' may conflict with platform lifecycle management",
					Severity: "warning",
				})
			}
		}
	}

	return issues
}

var dangerousCapabilities = map[string]bool{
	"SYS_ADMIN":  true,
	"SYS_RAWIO":  true,
	"SYS_PTRACE": true,
	"SYS_MODULE": true,
	"SYS_BOOT":   true,
	"NET_ADMIN":  true,
	"NET_RAW":    true,
	"ALL":        true,
}

func checkCapAddSecurity(svcName string, capAdd interface{}, issues *[]ValidationIssue) {
	if capAdd == nil {
		return
	}
	caps, ok := capAdd.([]interface{})
	if !ok {
		return
	}
	for _, capItem := range caps {
		capStr := fmt.Sprintf("%v", capItem)
		if dangerousCapabilities[capStr] {
			*issues = append(*issues, ValidationIssue{
				Field:    fmt.Sprintf("services.%s.cap_add", svcName),
				Message:  fmt.Sprintf("cap_add '%s' is not allowed for security reasons", capStr),
				Severity: "error",
			})
		} else {
			*issues = append(*issues, ValidationIssue{
				Field:    fmt.Sprintf("services.%s.cap_add", svcName),
				Message:  fmt.Sprintf("cap_add '%s' may grant unnecessary privileges", capStr),
				Severity: "warning",
			})
		}
	}
}

func checkDevicesSecurity(svcName string, devices interface{}, issues *[]ValidationIssue) {
	if devices == nil {
		return
	}
	devs, ok := devices.([]interface{})
	if !ok {
		return
	}
	for _, d := range devs {
		var path string
		switch v := d.(type) {
		case string:
			path = v
		case map[string]interface{}:
			if p, ok := v["source"]; ok {
				path = fmt.Sprintf("%v", p)
			}
		}
		if strings.HasPrefix(path, "/dev") {
			*issues = append(*issues, ValidationIssue{
				Field:    fmt.Sprintf("services.%s.devices", svcName),
				Message:  fmt.Sprintf("device mapping '%s' is not allowed for security reasons", path),
				Severity: "error",
			})
		}
	}
}

func checkVolumesSecurity(svcName string, volumes interface{}, issues *[]ValidationIssue) {
	if volumes == nil {
		return
	}
	vols, ok := volumes.([]interface{})
	if !ok {
		return
	}
	for _, v := range vols {
		var source string
		switch val := v.(type) {
		case string:
			parts := strings.SplitN(val, ":", 2)
			if len(parts) > 0 {
				source = parts[0]
			}
		case map[string]interface{}:
			if s, ok := val["source"]; ok {
				source = fmt.Sprintf("%v", s)
			}
		}
		if source == "" {
			continue
		}
		lower := strings.ToLower(source)
		if strings.HasPrefix(lower, "/var/run/docker.sock") || strings.Contains(lower, ":/var/run/docker.sock") {
			*issues = append(*issues, ValidationIssue{
				Field:    fmt.Sprintf("services.%s.volumes", svcName),
				Message:  "mounting docker.sock is not allowed for security reasons",
				Severity: "error",
			})
		}
		if strings.HasPrefix(lower, "/proc") || strings.HasPrefix(lower, "/sys") {
			*issues = append(*issues, ValidationIssue{
				Field:    fmt.Sprintf("services.%s.volumes", svcName),
				Message:  fmt.Sprintf("mounting %s is not allowed for security reasons", source),
				Severity: "error",
			})
		}
		if strings.HasPrefix(source, "/") && isSensitiveHostPath(source) {
			*issues = append(*issues, ValidationIssue{
				Field:    fmt.Sprintf("services.%s.volumes", svcName),
				Message:  fmt.Sprintf("host path mount '%s' may expose sensitive host files", source),
				Severity: "warning",
			})
		}
	}
}

var sensitiveHostPaths = []string{"/", "/root", "/etc", "/home"}

func isSensitiveHostPath(path string) bool {
	for _, p := range sensitiveHostPaths {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

func normalizePorts(ports []interface{}) []string {
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		switch v := p.(type) {
		case string:
			out = append(out, v)
		case int:
			out = append(out, fmt.Sprintf("%d", v))
		case float64:
			out = append(out, fmt.Sprintf("%.0f", v))
		}
	}
	return out
}

func normalizeEnv(env interface{}) map[string]string {
	out := make(map[string]string)
	switch v := env.(type) {
	case map[string]interface{}:
		for k, val := range v {
			out[k] = fmt.Sprintf("%v", val)
		}
	case []interface{}:
		for _, item := range v {
			switch e := item.(type) {
			case string:
				parts := strings.SplitN(e, "=", 2)
				if len(parts) == 2 {
					out[parts[0]] = parts[1]
				} else {
					out[parts[0]] = ""
				}
			case map[string]interface{}:
				if key, ok := e["name"]; ok {
					if val, ok := e["value"]; ok {
						out[fmt.Sprintf("%v", key)] = fmt.Sprintf("%v", val)
					}
				}
			}
		}
	}
	return out
}

func normalizeVolumes(vols []interface{}) []string {
	out := make([]string, 0, len(vols))
	for _, v := range vols {
		switch val := v.(type) {
		case string:
			out = append(out, val)
		case map[string]interface{}:
			var parts []string
			if t, ok := val["type"]; ok && fmt.Sprintf("%v", t) == "tmpfs" {
				if target, ok := val["target"]; ok {
					out = append(out, fmt.Sprintf("tmpfs:%v", target))
				}
				continue
			}
			if src, ok := val["source"]; ok {
				parts = append(parts, fmt.Sprintf("%v", src))
			}
			if target, ok := val["target"]; ok {
				parts = append(parts, fmt.Sprintf("%v", target))
			}
			if ro, ok := val["read_only"]; ok {
				if b, ok := ro.(bool); ok && b {
					parts = append(parts, ":ro")
				}
			}
			out = append(out, strings.Join(parts, ":"))
		}
	}
	return out
}

func normalizeDependsOn(dep interface{}) []string {
	out := make([]string, 0)
	switch v := dep.(type) {
	case []interface{}:
		for _, item := range v {
			switch d := item.(type) {
			case string:
				out = append(out, d)
			case map[string]interface{}:
				for key := range d {
					out = append(out, key)
				}
			}
		}
	case map[string]interface{}:
		for key := range v {
			out = append(out, key)
		}
	}
	return out
}

func normalizeBuild(build map[string]interface{}) *BuildSummary {
	b := &BuildSummary{}
	if ctx, ok := build["context"]; ok {
		b.Context = fmt.Sprintf("%v", ctx)
	}
	if df, ok := build["dockerfile"]; ok {
		b.Dockerfile = fmt.Sprintf("%v", df)
	}
	if args, ok := build["args"]; ok {
		if argMap, ok := args.(map[string]interface{}); ok {
			b.Args = make(map[string]string)
			for k, v := range argMap {
				b.Args[k] = fmt.Sprintf("%v", v)
			}
		}
	}
	if tgt, ok := build["target"]; ok {
		b.Target = fmt.Sprintf("%v", tgt)
	}
	return b
}

func normalizeHealthCheck(hc map[string]interface{}) *HealthSummary {
	h := &HealthSummary{}
	if test, ok := hc["test"]; ok {
		switch v := test.(type) {
		case []interface{}:
			for _, item := range v {
				h.Test = append(h.Test, fmt.Sprintf("%v", item))
			}
		case string:
			h.Test = []string{"CMD-SHELL", v}
		}
	}
	if interval, ok := hc["interval"]; ok {
		h.Interval = fmt.Sprintf("%v", interval)
	}
	if timeout, ok := hc["timeout"]; ok {
		h.Timeout = fmt.Sprintf("%v", timeout)
	}
	if retries, ok := hc["retries"]; ok {
		h.Retries = intFromInterface(retries)
	}
	if sp, ok := hc["start_period"]; ok {
		h.StartPeriod = fmt.Sprintf("%v", sp)
	}
	if disable, ok := hc["disable"]; ok {
		if b, ok := disable.(bool); ok {
			h.Disable = b
		}
	}
	return h
}

func normalizeDeploy(deploy map[string]interface{}) *DeploySummary {
	d := &DeploySummary{}
	if mode, ok := deploy["mode"]; ok {
		d.Mode = fmt.Sprintf("%v", mode)
	}
	if replicas, ok := deploy["replicas"]; ok {
		d.Replicas = intFromInterface(replicas)
	}
	if resources, ok := deploy["resources"]; ok {
		if resMap, ok := resources.(map[string]interface{}); ok {
			d.Resources = &ResourceSummary{}
			if limits, ok := resMap["limits"]; ok {
				d.Resources.Limits = stringMap(limits)
			}
			if reservations, ok := resMap["reservations"]; ok {
				d.Resources.Reservations = stringMap(reservations)
			}
		}
	}
	return d
}

func isExternal(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case map[string]interface{}:
		return true
	}
	return false
}

func stringMap(v interface{}) map[string]string {
	out := make(map[string]string)
	switch val := v.(type) {
	case map[string]interface{}:
		for k, item := range val {
			out[k] = fmt.Sprintf("%v", item)
		}
	}
	return out
}
