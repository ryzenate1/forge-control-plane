package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

type ParsedCompose struct {
	Version  string           `json:"version,omitempty"`
	Name     string           `json:"name,omitempty"`
	Services []ServiceSummary `json:"services"`
	Networks []NetworkSummary `json:"networks,omitempty"`
	Volumes  []VolumeSummary  `json:"volumes,omitempty"`
	Secrets  []SecretSummary  `json:"secrets,omitempty"`
	Configs  []ConfigSummary  `json:"configs,omitempty"`
}

type ServiceSummary struct {
	Name        string            `json:"name"`
	Image       string            `json:"image,omitempty"`
	Build       *BuildSummary     `json:"build,omitempty"`
	Ports       []string          `json:"ports,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Volumes     []string          `json:"volumes,omitempty"`
	DependsOn   []string          `json:"dependsOn,omitempty"`
	Profiles    []string          `json:"profiles,omitempty"`
	Restart     string            `json:"restart,omitempty"`
	Command     string            `json:"command,omitempty"`
	Entrypoint  string            `json:"entrypoint,omitempty"`
	HealthCheck *HealthSummary    `json:"healthcheck,omitempty"`
	Deploy      *DeploySummary    `json:"deploy,omitempty"`
}

type BuildSummary struct {
	Context    string            `json:"context,omitempty"`
	Dockerfile string            `json:"dockerfile,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	Target     string            `json:"target,omitempty"`
}

type HealthSummary struct {
	Test        []string `json:"test,omitempty"`
	Interval    string   `json:"interval,omitempty"`
	Timeout     string   `json:"timeout,omitempty"`
	Retries     int      `json:"retries,omitempty"`
	StartPeriod string   `json:"startPeriod,omitempty"`
	Disable     bool     `json:"disable,omitempty"`
}

type DeploySummary struct {
	Mode      string           `json:"mode,omitempty"`
	Replicas  int              `json:"replicas,omitempty"`
	Resources *ResourceSummary `json:"resources,omitempty"`
}

type ResourceSummary struct {
	Limits       map[string]string `json:"limits,omitempty"`
	Reservations map[string]string `json:"reservations,omitempty"`
}

type NetworkSummary struct {
	Name     string            `json:"name"`
	Driver   string            `json:"driver,omitempty"`
	External bool              `json:"external,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

type VolumeSummary struct {
	Name     string            `json:"name"`
	Driver   string            `json:"driver,omitempty"`
	External bool              `json:"external,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

type SecretSummary struct {
	Name     string `json:"name"`
	File     string `json:"file,omitempty"`
	External bool   `json:"external,omitempty"`
}

type ConfigSummary struct {
	Name     string `json:"name"`
	File     string `json:"file,omitempty"`
	External bool   `json:"external,omitempty"`
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
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
	return s.ParseComposeYAML(content, workingDir)
}

func (s *Service) Validate(content []byte, workingDir string) *ValidateResult {
	return s.ValidateCompose(content, workingDir)
}

func (s *Service) ParseComposeYAML(content []byte, workingDir string) (*ParsedCompose, error) {
	content = interpolateEnv(content)

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

	parsed, err := s.ParseComposeYAML(content, workingDir)
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
	return s.ParseComposeYAML(content, workingDir)
}

func ParseSummary(content []byte, workingDir string) (*ParsedCompose, error) {
	var s Service
	return s.ParseComposeYAML(content, workingDir)
}

func ValidateSummary(content []byte, workingDir string) *ValidateResult {
	var s Service
	return s.ValidateCompose(content, workingDir)
}

func interpolateEnv(content []byte) []byte {
	result := string(content)
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		result = strings.ReplaceAll(result, "${"+parts[0]+"}", parts[1])
		result = strings.ReplaceAll(result, "$"+parts[0], parts[1])
	}
	return []byte(result)
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

func stringifyValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []interface{}:
		parts := make([]string, len(val))
		for i, item := range val {
			parts[i] = fmt.Sprintf("%v", item)
		}
		return strings.Join(parts, " ")
	case []string:
		return strings.Join(val, " ")
	}
	return ""
}

func intFromInterface(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		var i int
		fmt.Sscanf(val, "%d", &i)
		return i
	}
	return 0
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
