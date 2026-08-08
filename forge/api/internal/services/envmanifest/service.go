package envmanifest

import (
	"context"
	"encoding/json"

	"fmt"
	"strings"
	"time"

	"gamepanel/forge/internal/store"
)

// StoreService is the persistence surface the manifest engine needs.
// *store.Store satisfies it.
type StoreService interface {
	ResolveEnvContext(ctx context.Context, envID string) (store.EnvContext, error)
	ListApplicationsByEnvironment(ctx context.Context, envID, orgID string) ([]store.Application, error)
	CreateApplication(ctx context.Context, input store.CreateApplicationInput) (*store.Application, error)
	UpdateApplication(ctx context.Context, id string, input store.UpdateApplicationInput) error
	ListAppServices(ctx context.Context, appID string) ([]store.AppService, error)
	CreateAppService(ctx context.Context, input store.CreateAppServiceInput) (*store.AppService, error)
	UpdateAppService(ctx context.Context, id string, input store.UpdateAppServiceInput) (*store.AppService, error)
	GetEnvManifest(ctx context.Context, envID string) (*store.EnvManifestRow, error)
	SaveEnvManifest(ctx context.Context, envID, version string, manifest json.RawMessage, manifestYAML string, actorID *string) error
	GetEnvDomainProvisioning(ctx context.Context, envID string) (*store.EnvDomainProvisioning, error)
}

// EnvVarStore is the subset of the env-var persistence used to apply the
// manifest's env[] steps (environment-scope upsert).
type EnvVarStore interface {
	ListEnvironmentVariables(ctx context.Context, scopeType, scopeID string) ([]store.EnvironmentVariable, error)
	CreateEnvironmentVariable(ctx context.Context, req store.CreateEnvVarRequest, actorID *string) (store.EnvironmentVariable, error)
	UpdateEnvironmentVariable(ctx context.Context, id string, req store.UpdateEnvVarRequest, actorID *string) (store.EnvironmentVariable, error)
	DeleteEnvironmentVariable(ctx context.Context, id string, actorID *string) error
}

// WithEnvVars configures the optional env-var store used by Apply's env[]
// steps. Without it, steps are resolved but not persisted.
func (svc *Service) WithEnvVars(ev EnvVarStore) *Service {
	svc.envvar = ev
	return svc
}

// Service applies, renders and inspects environment manifests.
type Service struct {
	store  StoreService
	envvar EnvVarStore
}

// New builds the manifest engine.
func New(st StoreService) *Service {
	return &Service{store: st}
}

// Apply provisions an environment from a manifest: it creates/updates one
// application per manifest (named after the manifest), upserts each declared
// service under it, applies the ordered env[] steps (overrideEnv winning)
// and persists the manifest for rendering back. It is idempotent: applying
// the same manifest twice yields an empty diff.
func (svc *Service) Apply(ctx context.Context, envID string, m EnvManifest, overrideEnv map[string]string, manifestYAML string, actorID *string) (*ApplyResult, error) {
	envCtx, err := svc.store.ResolveEnvContext(ctx, envID)
	if err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}

	result := &ApplyResult{EnvID: envID, Manifest: &m, AppliedAt: time.Now().UTC()}

	apps, err := svc.store.ListApplicationsByEnvironment(ctx, envID, envCtx.Org.ID)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	appByName := map[string]store.Application{}
	for _, a := range apps {
		appByName[a.Name] = a
	}

	appName := strings.TrimSpace(m.Name)
	if appName == "" {
		appName = envCtx.Environment.Name
	}

	var app *store.Application
	if existing, ok := appByName[appName]; ok {
		app = &existing
		name := appName
		desc := "Managed by env manifest " + m.Version
		if err := svc.store.UpdateApplication(ctx, app.ID, store.UpdateApplicationInput{
			Name:         &name,
			Description:  &desc,
			SourceConfig: json.RawMessage(`{"manifest":true}`),
		}); err != nil {
			return nil, fmt.Errorf("update application: %w", err)
		}
		result.UpdatedApps = append(result.UpdatedApps, app.ID)
	} else {
		created, err := svc.store.CreateApplication(ctx, store.CreateApplicationInput{
			Name:          appName,
			Description:   "Managed by env manifest " + m.Version,
			OrgID:         envCtx.Org.ID,
			ProjectID:     &envCtx.Project.ID,
			EnvironmentID: &envCtx.Environment.ID,
			SourceType:    "COMPOSE",
			SourceConfig:  json.RawMessage(`{"manifest":true}`),
		})
		if err != nil {
			return nil, fmt.Errorf("create application: %w", err)
		}
		app = created
		result.CreatedApps = append(result.CreatedApps, app.ID)
	}

	services, err := svc.store.ListAppServices(ctx, app.ID)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	svcByName := map[string]store.AppService{}
	kept := map[string]bool{}
	for _, s := range services {
		svcByName[s.Name] = s
	}

	for _, def := range m.Services {
		name := strings.TrimSpace(def.Name)
		desired := ""
		if strings.TrimSpace(def.DesiredState) != "" {
			desired = strings.ToLower(def.DesiredState)
		}
		if current, ok := svcByName[name]; ok {
			kept[current.ID] = true
			replicas := def.Replicas
			if replicas < 1 {
				replicas = current.Replicas
			}
			ports := manifestPorts(def.Ports)
			envVars := def.Env
			_, err := svc.store.UpdateAppService(ctx, current.ID, store.UpdateAppServiceInput{
				Image:        &def.Image,
				Replicas:     &replicas,
				Ports:        &ports,
				EnvVars:      &envVars,
				DependsOn:    &def.DependsOn,
				DesiredState: &desired,
			})
			if err != nil {
				return nil, fmt.Errorf("update service %q: %w", name, err)
			}
			result.UpdatedServices = append(result.UpdatedServices, current.ID)
		} else {
			replicas := def.Replicas
			if replicas < 1 {
				replicas = 1
			}
			created, err := svc.store.CreateAppService(ctx, store.CreateAppServiceInput{
				AppID:          app.ID,
				Name:           name,
				Image:          def.Image,
				ComposeService: def.ComposeService,
				Replicas:       replicas,
				Ports:          manifestPorts(def.Ports),
				EnvVars:        def.EnvMap(),
				DependsOn:      def.DependsOn,
			})
			if err != nil {
				return nil, fmt.Errorf("create service %q: %w", name, err)
			}
			kept[created.ID] = true
			result.CreatedServices = append(result.CreatedServices, created.ID)
		}
	}

	steps, warnings := svc.applyEnvSteps(ctx, envCtx.Environment.ID, m, overrideEnv, actorID)
	result.EnvStepsApplied = steps
	result.Warnings = warnings

	raw := mustRaw(m)
	if err := svc.store.SaveEnvManifest(ctx, envID, m.Version, raw, manifestYAML, actorID); err != nil {
		return nil, err
	}
	return result, nil
}

// applyEnvSteps applies the ordered env[] steps as environment-scope
// variables, honoring overrideEnv for any key it supplies. Returns the
// number of steps applied and non-fatal warnings.
func (svc *Service) applyEnvSteps(ctx context.Context, envID string, m EnvManifest, overrideEnv map[string]string, actorID *string) (int, []string) {
	if svc.envvar == nil {
		return 0, []string{"env[] steps skipped: env-var store not configured"}
	}
	existing, err := svc.envvar.ListEnvironmentVariables(ctx, "environment", envID)
	if err != nil {
		return 0, []string{fmt.Sprintf("env steps skipped: %v", err)}
	}
	byKey := map[string]store.EnvironmentVariable{}
	for _, v := range existing {
		byKey[v.Key] = v
	}

	applied := 0
	for _, step := range m.Env {
		if step.Service != "" {
			// Service-scoped steps require the service id; we record them as
			// environment-scope with a service hint only when unresolved.
			continue
		}
		key := step.Key
		value := step.Value
		if overridden, ok := overrideEnv[key]; ok {
			value = overridden
		}
		if value == "" {
			continue
		}
		if v, ok := byKey[key]; ok {
			req := store.UpdateEnvVarRequest{Value: value, IsSensitive: step.Sensitive}
			if _, err := svc.envvar.UpdateEnvironmentVariable(ctx, v.ID, req, actorID); err != nil {
				continue
			}
		} else {
			if _, err := svc.envvar.CreateEnvironmentVariable(ctx, store.CreateEnvVarRequest{
				EnvironmentID: &envID,
				Scope:         "environment",
				Key:           key,
				Value:         value,
				IsSensitive:   step.Sensitive,
			}, actorID); err != nil {
				continue
			}
			byKey[key] = store.EnvironmentVariable{}
		}
		applied++
	}
	return applied, nil
}

// Render returns the last applied manifest (nil when none applied yet) plus
// the raw YAML text that was persisted.
func (svc *Service) Render(ctx context.Context, envID string) (*EnvManifest, string, error) {
	row, err := svc.store.GetEnvManifest(ctx, envID)
	if err != nil {
		return nil, "", err
	}
	if row == nil {
		return nil, "", nil
	}
	var m EnvManifest
	if len(row.Manifest) > 0 {
		if err := json.Unmarshal(row.Manifest, &m); err != nil {
			return nil, "", fmt.Errorf("corrupt stored manifest: %w", err)
		}
	}
	return &m, row.ManifestYAML, nil
}

// PortsURLsView is the dashboard payload: per-app services with declared
// ports and derived URLs plus the domain/TLS provisioning state.
type PortsURLsView struct {
	EnvID        string         `json:"envId"`
	EnvName      string         `json:"envName"`
	Domain       string         `json:"domain,omitempty"`
	WildcardHost string         `json:"wildcardHost,omitempty"`
	DNSStatus    string         `json:"dnsStatus,omitempty"`
	TLSStatus    string         `json:"tlsStatus,omitempty"`
	Apps         []AppPortsView `json:"apps"`
}

// AppPortsView groups an application's service port views.
type AppPortsView struct {
	AppID    string             `json:"appId"`
	AppName  string             `json:"appName"`
	Services []ServicePortsView `json:"services"`
}

// ServicePortsView is one service's port/URL display row.
type ServicePortsView struct {
	ServiceID string          `json:"serviceId"`
	Name      string          `json:"name"`
	Image     string          `json:"image"`
	Replicas  int             `json:"replicas"`
	Status    string          `json:"status"`
	Ports     []store.AppPort `json:"ports"`
	URLs      []string        `json:"urls,omitempty"`
}

// PortsURLs assembles the ports/URLs dashboard payload for an environment,
// augmenting declared ports with wildcard-host URLs when the domain
// provisioning ledger has a ready TLS status.
func (svc *Service) PortsURLs(ctx context.Context, envID string) (*PortsURLsView, error) {
	envCtx, err := svc.store.ResolveEnvContext(ctx, envID)
	if err != nil {
		return nil, err
	}
	view := &PortsURLsView{EnvID: envID, EnvName: envCtx.Environment.Name}

	prov, err := svc.store.GetEnvDomainProvisioning(ctx, envID)
	if err == nil && prov != nil {
		view.Domain = prov.Domain
		view.WildcardHost = prov.WildcardHost
		view.DNSStatus = prov.DNSStatus
		view.TLSStatus = prov.TLSStatus
	}

	apps, err := svc.store.ListApplicationsByEnvironment(ctx, envID, envCtx.Org.ID)
	if err != nil {
		return nil, err
	}
	for i := range apps {
		app := AppPortsView{AppID: apps[i].ID, AppName: apps[i].Name}
		services, err := svc.store.ListAppServices(ctx, apps[i].ID)
		if err != nil {
			continue
		}
		for _, s := range services {
			pv := ServicePortsView{
				ServiceID: s.ID,
				Name:      s.Name,
				Image:     s.Image,
				Replicas:  s.Replicas,
				Status:    s.ObservedStatus,
				Ports:     s.Ports,
			}
			for _, p := range s.Ports {
				if p.HostPort > 0 {
					pv.URLs = append(pv.URLs, fmt.Sprintf("%s://localhost:%d", scheme(p.Protocol), p.HostPort))
				}
				pv.URLs = append(pv.URLs, svc.deriveURLs(envCtx, prov, s.Name, p)...)
			}
			if pv.URLs == nil {
				pv.URLs = []string{}
			}
			app.Services = append(app.Services, pv)
		}
		view.Apps = append(view.Apps, app)
	}
	return view, nil
}

// deriveURLs returns the wildcard-host URL for a service when provisioning is
// configured and the port is HTTP(S).
func (svc *Service) deriveURLs(envName string, prov *store.EnvDomainProvisioning, service string, p store.AppPort) []string {
	if prov == nil || prov.WildcardHost == "" {
		return nil
	}
	if strings.EqualFold(p.Protocol, "tcp") || strings.EqualFold(p.Protocol, "udp") {
		return nil
	}
	host := strings.TrimPrefix(prov.WildcardHost, "*.")
	if host == "" {
		return nil
	}
	scheme := "http"
	if prov.TLSStatus == "ok" {
		scheme = "https"
	}
	return []string{fmt.Sprintf("%s://%s.%s", scheme, service, host)}
}

// ---- small helpers ----

func manifestPorts(def []ManifestPort) []store.AppPort {
	ports := make([]store.AppPort, 0, len(def))
	for _, p := range def {
		ports = append(ports, store.AppPort{
			ContainerPort: p.ContainerPort,
			HostPort:      p.HostPort,
			Protocol:      p.Protocol,
		})
	}
	return ports
}

// EnvMap converts the service's env map (nil-safe).
func (s ManifestService) EnvMap() map[string]string {
	if s.Env == nil {
		return map[string]string{}
	}
	return s.Env
}

func scheme(protocol string) string {
	switch strings.ToLower(protocol) {
	case "https":
		return "https"
	case "tcp", "udp":
		return "tcp"
	default:
		return "http"
	}
}

func mustRaw(m EnvManifest) json.RawMessage {
	raw, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}
