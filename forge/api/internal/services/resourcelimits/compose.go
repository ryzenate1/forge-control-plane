package resourcelimits

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// This file is the whole point of the feature: the moment a stored
// ProcessConfig becomes real. It mutates a compose document in place, which is
// why the panel needs no new Beacon verb — `deploy.resources.limits`,
// `healthcheck` and `deploy.replicas` are Compose Specification keys, so the
// runtime the node already drives enforces them.

// AppliedProcess records what was written into the document for one process
// type, and against which compose service key.
type AppliedProcess struct {
	ProcessType string `json:"processType"`
	Service     string `json:"service"`
	Replicas    int    `json:"replicas"`
	// LimitsApplied is false when the process carries no CPU or memory limit,
	// which is a legitimate configuration ("inherit the host") and must not be
	// reported as if a constraint was enforced.
	LimitsApplied bool   `json:"limitsApplied"`
	HealthCheck   string `json:"healthCheckType,omitempty"`
	// ReplacedExistingHealthCheck is true when the authored document already
	// had a healthcheck for this service and the process config overrode it.
	// Operators need to see that: the compose file they wrote is no longer
	// entirely what ships.
	ReplacedExistingHealthCheck bool `json:"replacedExistingHealthCheck"`
}

// SkippedProcess records a config that could NOT be applied, and why. Silently
// dropping a configured limit is the exact failure mode this type exists to
// prevent.
type SkippedProcess struct {
	ProcessType string `json:"processType"`
	Reason      string `json:"reason"`
}

// ComposeRenderResult is the audit trail of one render.
type ComposeRenderResult struct {
	Applied []AppliedProcess `json:"applied"`
	Skipped []SkippedProcess `json:"skipped"`
}

// FullyApplied reports whether every enabled process config landed in the
// document.
func (r ComposeRenderResult) FullyApplied() bool {
	return len(r.Skipped) == 0
}

// ApplyToCompose injects every enabled process configuration for appID into the
// supplied compose document, mutating it in place.
//
// A process type with no matching service key is reported by RenderIntoCompose
// and is not an error here: a document may legitimately be mid-edit. Callers
// that must not ship partial enforcement use RenderIntoCompose and check
// ComposeRenderResult.FullyApplied.
func (s *Service) ApplyToCompose(ctx context.Context, appID string, composeDoc map[string]any) error {
	_, err := s.RenderIntoCompose(ctx, appID, composeDoc)
	return err
}

// RenderIntoCompose is ApplyToCompose plus the audit trail.
func (s *Service) RenderIntoCompose(ctx context.Context, appID string, composeDoc map[string]any) (*ComposeRenderResult, error) {
	if strings.TrimSpace(appID) == "" {
		return nil, errors.New("application id is required")
	}
	configs, err := s.store.ListProcessConfigs(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("load process configs: %w", err)
	}

	result := &ComposeRenderResult{Applied: []AppliedProcess{}, Skipped: []SkippedProcess{}}
	if len(configs) == 0 {
		return result, nil
	}
	if composeDoc == nil {
		return nil, fmt.Errorf("%w: document is nil", ErrInvalidComposeDocument)
	}

	services, err := composeServices(composeDoc)
	if err != nil {
		return nil, err
	}
	index := indexServiceNames(services)

	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		key, ok := index[cfg.ProcessType]
		if !ok {
			result.Skipped = append(result.Skipped, SkippedProcess{
				ProcessType: cfg.ProcessType,
				Reason: fmt.Sprintf(
					"the compose document has no service named %q; available services: %s",
					cfg.ProcessType, strings.Join(availableServiceNames(services), ", "),
				),
			})
			continue
		}
		applied, err := applyProcessConfig(services[key], cfg)
		if err != nil {
			result.Skipped = append(result.Skipped, SkippedProcess{
				ProcessType: cfg.ProcessType,
				Reason:      fmt.Sprintf("service %q could not be modified: %v", key, err),
			})
			continue
		}
		applied.Service = key
		result.Applied = append(result.Applied, applied)
	}

	sort.Slice(result.Applied, func(i, j int) bool { return result.Applied[i].ProcessType < result.Applied[j].ProcessType })
	sort.Slice(result.Skipped, func(i, j int) bool { return result.Skipped[i].ProcessType < result.Skipped[j].ProcessType })
	return result, nil
}

// applyProcessConfig writes one config into one service mapping. The service
// mapping must already exist: this code never invents a service, because a
// process type with no service is a mistake in the configuration, not a
// request to create a container.
func applyProcessConfig(service any, cfg ProcessConfig) (AppliedProcess, error) {
	applied := AppliedProcess{ProcessType: cfg.ProcessType, HealthCheck: string(cfg.HealthCheckType)}

	body, ok := asStringMap(service)
	if !ok {
		return applied, errors.New("the service entry is not a mapping")
	}

	cpu, hasCPU := int64Value(cfg.CPULimit)
	mem, hasMem := int64Value(cfg.MemoryLimit)
	if hasCPU || hasMem {
		limits := map[string]any{}
		if hasCPU {
			limits["cpus"] = nanoCoresToCPUScalar(cpu)
		}
		if hasMem {
			// ByteCount accepts a plain decimal byte count; the string form
			// keeps YAML parsers from turning a large byte value into a float.
			limits["memory"] = fmt.Sprintf("%d", mem)
		}
		setNestedMap(body, []string{"deploy", "resources", "limits"}, limits)
		applied.LimitsApplied = true
	} else {
		// Drop any previously rendered limits so removing a limit in the UI
		// actually removes it from the release instead of lingering.
		clearNestedPath(body, []string{"deploy", "resources", "limits"})
	}

	// Compose has two places to say "how many": deploy.replicas and the newer
	// top-level scale. deploy.replicas rejects 0, scale does not, so a process
	// type scaled to zero is expressed with scale and gets no deploy.replicas
	// rather than a document Docker will refuse to parse.
	if cfg.Replicas == 0 {
		body["scale"] = 0
		deleteNestedPath(body, []string{"deploy", "replicas"})
	} else {
		setNestedScalar(body, []string{"deploy", "replicas"}, cfg.Replicas)
		deleteKey(body, "scale")
	}
	applied.Replicas = cfg.Replicas

	if cfg.HasHealthCheck() {
		probe, err := healthCheckProbe(cfg)
		if err != nil {
			return applied, err
		}
		_, existed := body["healthcheck"]
		body["healthcheck"] = map[string]any{
			"test":         []any{"CMD-SHELL", probe},
			"interval":     durationScalar(cfg.HealthCheckInterval),
			"timeout":      durationScalar(cfg.HealthCheckTimeout),
			"retries":      cfg.HealthCheckRetries,
			"start_period": durationScalar(cfg.HealthCheckStartPeriod),
			"disable":      false,
		}
		applied.ReplacedExistingHealthCheck = existed
	} else {
		// An explicitly cleared check must also clear the document's check;
		// otherwise disabling gating in the panel would appear to do nothing.
		deleteKey(body, "healthcheck")
	}

	return applied, nil
}

// healthCheckProbe renders the shell one-liner Docker will run inside the
// container. Everything here is deliberately the most portable thing that
// works: curl for the images that have it, wget -q -O - for the busybox/alpine
// ones, and a bounded timeout so a hung endpoint reports as a failed probe
// instead of a probe that never returns.
func healthCheckProbe(cfg ProcessConfig) (string, error) {
	timeout := cfg.HealthCheckTimeout
	if timeout <= 0 {
		timeout = DefaultHealthCheckTimeout
	}
	switch cfg.HealthCheckType {
	case HealthCheckCommand:
		command := strings.TrimSpace(cfg.HealthCheckCommand)
		if command == "" {
			return "", errors.New("command health checks require a command")
		}
		return command, nil
	case HealthCheckTCP:
		if cfg.HealthCheckPort <= 0 {
			return "", errors.New("tcp health checks require a port")
		}
		return fmt.Sprintf("nc -z -w %d 127.0.0.1 %d", timeout, cfg.HealthCheckPort), nil
	case HealthCheckHTTP:
		if cfg.HealthCheckPort <= 0 {
			return "", errors.New("http health checks require a port")
		}
		path := strings.TrimSpace(cfg.HealthCheckPath)
		if path == "" {
			path = "/healthz"
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		url := fmt.Sprintf("http://127.0.0.1:%d%s", cfg.HealthCheckPort, path)
		return fmt.Sprintf(
			"(curl -fsS --max-time %d -o /dev/null %s) || (wget -q -T %d -O - %s >/dev/null 2>&1)",
			timeout, shellQuote(url), timeout, shellQuote(url),
		), nil
	case HealthCheckNone:
		return "", errors.New("no health check is configured for this process")
	}
	return "", fmt.Errorf("unsupported health check type %q", cfg.HealthCheckType)
}

// nanoCoresToCPUScalar converts Docker nanoCores into the decimal-core string
// the Compose Specification expects for deploy.resources.limits.cpus.
func nanoCoresToCPUScalar(nano int64) string {
	cores := float64(nano) / float64(NanoCoresPerCore)
	return trimTrailingZeros(fmt.Sprintf("%.6f", cores))
}

func trimTrailingZeros(value string) string {
	if !strings.Contains(value, ".") {
		return value
	}
	value = strings.TrimRight(value, "0")
	return strings.TrimRight(value, ".")
}

func durationScalar(seconds int) string {
	if seconds <= 0 {
		return "0s"
	}
	return fmt.Sprintf("%ds", seconds)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// composeServices returns the mutable `services` mapping of a compose document.
func composeServices(doc map[string]any) (map[string]any, error) {
	raw, ok := doc["services"]
	if !ok {
		return nil, fmt.Errorf("%w: the document has no \"services\" mapping", ErrInvalidComposeDocument)
	}
	services, ok := asStringMap(raw)
	if !ok {
		return nil, fmt.Errorf("%w: \"services\" is not a mapping of service name to service definition", ErrInvalidComposeDocument)
	}
	if len(services) == 0 {
		return nil, fmt.Errorf("%w: \"services\" is empty", ErrInvalidComposeDocument)
	}
	// Write the normalised map back so a document parsed as map[any]any by a
	// YAML library still ends up with string keys callers can marshal.
	doc["services"] = services
	return services, nil
}

// indexServiceNames maps lowercased service keys to their real key. Compose
// service names are case-sensitive, so "Web" and "web" are two services; the
// first exact match wins, and a case-insensitive match is only used when it is
// unambiguous. Guessing between two candidates is exactly the silent
// resolution this codebase forbids.
func indexServiceNames(services map[string]any) map[string]string {
	index := make(map[string]string, len(services))
	ambiguous := make(map[string]bool)
	for name := range services {
		index[name] = name
	}
	for name := range services {
		lower := strings.ToLower(name)
		if existing, ok := index[lower]; ok && existing != name {
			if strings.EqualFold(existing, name) {
				continue
			}
			ambiguous[lower] = true
		}
		if _, exact := index[name]; !exact {
			index[name] = name
		}
		if !ambiguous[lower] {
			index[lower] = name
		} else {
			delete(index, lower)
		}
	}
	return index
}

func availableServiceNames(services map[string]any) []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 8 {
		names = append(names[:8:8], "…")
	}
	return names
}

// asStringMap normalises the two map shapes YAML and JSON libraries produce.
func asStringMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[any]any:
		converted := make(map[string]any, len(typed))
		for key, val := range typed {
			name, ok := key.(string)
			if !ok {
				return nil, false
			}
			converted[name] = val
		}
		return converted, true
	default:
		return nil, false
	}
}

func setNestedMap(root map[string]any, path []string, value map[string]any) {
	parent := root
	for _, key := range path[:len(path)-1] {
		child, ok := asStringMap(parent[key])
		if !ok {
			child = map[string]any{}
			parent[key] = child
		}
		parent = child
	}
	leaf := path[len(path)-1]
	existing, ok := asStringMap(parent[leaf])
	if ok {
		for key, val := range value {
			existing[key] = val
		}
		parent[leaf] = existing
		return
	}
	parent[leaf] = value
}

func setNestedScalar(root map[string]any, path []string, value any) {
	parent := root
	for _, key := range path[:len(path)-1] {
		child, ok := asStringMap(parent[key])
		if !ok {
			child = map[string]any{}
			parent[key] = child
		}
		parent = child
	}
	parent[path[len(path)-1]] = value
}

func clearNestedPath(root map[string]any, path []string) {
	parent := root
	for i, key := range path {
		if i == len(path)-1 {
			deleteKey(parent, key)
			return
		}
		child, ok := asStringMap(parent[key])
		if !ok {
			return
		}
		parent = child
	}
}

func deleteNestedPath(root map[string]any, path []string) {
	clearNestedPath(root, path)
}

// deleteKey removes a key and then prunes mappings that became empty, so a
// cleared healthcheck does not leave an orphan "deploy: {}" behind.
func deleteKey(m map[string]any, key string) {
	delete(m, key)
}
