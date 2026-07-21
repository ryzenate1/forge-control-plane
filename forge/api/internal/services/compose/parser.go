package compose

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

// ComposeConfig represents the parsed Compose configuration from compose-go library
type ComposeConfig = types.Project

// ForgeAppConfig represents the normalized Forge application configuration
type ForgeAppConfig struct {
	Version    string              `json:"version"`
	Name       string              `json:"name"`
	Services   []ForgeServiceConfig `json:"services"`
	Networks   []ForgeNetworkConfig `json:"networks"`
	Volumes    []ForgeVolumeConfig  `json:"volumes"`
	Secrets    []ForgeSecretConfig  `json:"secrets"`
	Configs    []ForgeConfigConfig  `json:"configs"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// ForgeServiceConfig represents a normalized service configuration for Forge
type ForgeServiceConfig struct {
	Name        string                     `json:"name"`
	Image       string                     `json:"image,omitempty"`
	Build       *ForgeBuildConfig          `json:"build,omitempty"`
	Ports       []string                   `json:"ports,omitempty"`
	Environment map[string]string          `json:"environment,omitempty"`
	Volumes     []string                   `json:"volumes,omitempty"`
	DependsOn   []string                   `json:"depends_on,omitempty"`
	Profiles    []string                   `json:"profiles,omitempty"`
	Restart     string                     `json:"restart,omitempty"`
	Command     string                     `json:"command,omitempty"`
	Entrypoint  string                     `json:"entrypoint,omitempty"`
	HealthCheck *ForgeHealthCheckConfig     `json:"healthcheck,omitempty"`
	Deploy      *ForgeDeployConfig          `json:"deploy,omitempty"`
	Networks    []string                   `json:"networks,omitempty"`
	Labels      map[string]string          `json:"labels,omitempty"`
	Extensions  map[string]interface{}     `json:"extensions,omitempty"`
}

// ForgeBuildConfig represents build configuration
type ForgeBuildConfig struct {
	Context    string            `json:"context,omitempty"`
	Dockerfile string            `json:"dockerfile,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	Target     string            `json:"target,omitempty"`
	CacheFrom  []string          `json:"cache_from,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Network    string            `json:"network,omitempty"`
}

// ForgeHealthCheckConfig represents health check configuration
type ForgeHealthCheckConfig struct {
	Test        []string `json:"test,omitempty"`
	Interval    string   `json:"interval,omitempty"`
	Timeout     string   `json:"timeout,omitempty"`
	Retries     int      `json:"retries,omitempty"`
	StartPeriod string   `json:"start_period,omitempty"`
	Disable     bool     `json:"disable,omitempty"`
}

// ForgeDeployConfig represents deploy configuration
type ForgeDeployConfig struct {
	Mode       string             `json:"mode,omitempty"`
	Replicas   int                `json:"replicas,omitempty"`
	Resources  *ForgeResourceConfig `json:"resources,omitempty"`
	Placement  *ForgePlacementConfig `json:"placement,omitempty"`
	Restart    string             `json:"restart_policy,omitempty"`
	Update     *ForgeUpdateConfig   `json:"update_config,omitempty"`
	Rollback   *ForgeRollbackConfig `json:"rollback_config,omitempty"`
	Labels     map[string]string    `json:"labels,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// ForgeResourceConfig represents resource limits and reservations
type ForgeResourceConfig struct {
	Limits      map[string]string `json:"limits,omitempty"`
	Reservations map[string]string `json:"reservations,omitempty"`
}

// ForgePlacementConfig represents placement constraints
type ForgePlacementConfig struct {
	Constraints []string          `json:"constraints,omitempty"`
	Preferences []ForgePreference `json:"preferences,omitempty"`
	MaxReplicas uint              `json:"max_replicas,omitempty"`
}

// ForgePreference represents placement preferences
type ForgePreference struct {
	Spread string `json:"spread,omitempty"`
}

// ForgeUpdateConfig represents update configuration
type ForgeUpdateConfig struct {
	Parallelism   uint   `json:"parallelism,omitempty"`
	Delay        string `json:"delay,omitempty"`
	FailureAction string `json:"failure_action,omitempty"`
	Monitor      string `json:"monitor,omitempty"`
	MaxFailure   uint   `json:"max_failure_ratio,omitempty"`
	Order        string `json:"order,omitempty"`
}

// ForgeRollbackConfig represents rollback configuration
type ForgeRollbackConfig struct {
	Parallelism   uint   `json:"parallelism,omitempty"`
	Delay        string `json:"delay,omitempty"`
	FailureAction string `json:"failure_action,omitempty"`
	Monitor      string `json:"monitor,omitempty"`
	MaxFailure   uint   `json:"max_failure_ratio,omitempty"`
	Order        string `json:"order,omitempty"`
}

// ForgeNetworkConfig represents network configuration
type ForgeNetworkConfig struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver,omitempty"`
	DriverOpts map[string]string `json:"driver_opts,omitempty"`
	External   bool              `json:"external,omitempty"`
	Internal   bool              `json:"internal,omitempty"`
	Attachable bool              `json:"attachable,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// ForgeVolumeConfig represents volume configuration
type ForgeVolumeConfig struct {
	Name       string            `json:"name"`
	Driver     string            `json:"driver,omitempty"`
	DriverOpts map[string]string `json:"driver_opts,omitempty"`
	External   bool              `json:"external,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// ForgeSecretConfig represents secret configuration
type ForgeSecretConfig struct {
	Name       string            `json:"name"`
	File       string            `json:"file,omitempty"`
	External   bool              `json:"external,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Driver     string            `json:"driver,omitempty"`
	DriverOpts map[string]string `json:"driver_opts,omitempty"`
	Template   string            `json:"template_drv,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// ForgeConfigConfig represents config configuration
type ForgeConfigConfig struct {
	Name       string            `json:"name"`
	File       string            `json:"file,omitempty"`
	External   bool              `json:"external,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Template   string            `json:"template_drv,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// ComposeParser provides full Compose specification parsing using compose-spec library
type ComposeParser struct {
	workingDir    string
	configDetails *types.ConfigDetails
}

// GetServiceNames returns the names of all services in a parsed project
func (p *ComposeParser) GetServiceNames(project *types.Project) []string {
	if project == nil {
		return nil
	}
	names := make([]string, 0, len(project.Services))
	for name := range project.Services {
		names = append(names, name)
	}
	return names
}

// GetServiceByName returns a service from a parsed project by name
func (p *ComposeParser) GetServiceByName(project *types.Project, name string) (*types.ServiceConfig, error) {
	if project == nil {
		return nil, fmt.Errorf("project is nil")
	}
	svc, ok := project.Services[name]
	if !ok {
		return nil, fmt.Errorf("service %q not found", name)
	}
	return &svc, nil
}

// ParseComposeWithMultipleFiles parses multiple Compose files together
func (p *ComposeParser) ParseComposeWithMultipleFiles(files map[string][]byte) (*types.Project, error) {
	configDetails := &types.ConfigDetails{
		WorkingDir: p.workingDir,
		ConfigFiles: make([]types.ConfigFile, 0, len(files)),
		Environment: map[string]string{},
	}
	for filename, content := range files {
		configDetails.ConfigFiles = append(configDetails.ConfigFiles, types.ConfigFile{
			Filename: filename,
			Content:  content,
		})
	}
	return loader.LoadWithContext(context.Background(), *configDetails, func(opts *loader.Options) {
		opts.SetProjectName("default", false)
	})
}

// CheckComposeVersionCompatibility checks if a Compose file version is supported
func (p *ComposeParser) CheckComposeVersionCompatibility(version string) error {
	if version == "" {
		return nil
	}
	v, err := strconv.ParseFloat(version, 64)
	if err != nil {
		return fmt.Errorf("invalid compose version: %s", version)
	}
	if v < 2.0 {
		return fmt.Errorf("compose version %s is not supported (minimum is 2.0)", version)
	}
	return nil
}

// NewComposeParser creates a new Compose parser instance
func NewComposeParser(workingDir string) *ComposeParser {
	return &ComposeParser{
		workingDir: workingDir,
		configDetails: &types.ConfigDetails{
			WorkingDir: workingDir,
			ConfigFiles: []types.ConfigFile{},
			Environment: map[string]string{},
		},
	}
}

// NewDefaultComposeParser creates a Compose parser with default settings
func NewDefaultComposeParser(workingDir string) *ComposeParser {
	return NewComposeParser(workingDir)
}

// ParseComposeYAML parses Compose YAML content and returns a ComposeConfig
func ParseComposeYAML(yamlContent string) (*ComposeConfig, error) {
	configDetails := &types.ConfigDetails{
		WorkingDir: "",
		ConfigFiles: []types.ConfigFile{
			{
				Filename: "docker-compose.yml",
				Content:  []byte(yamlContent),
			},
		},
		Environment: map[string]string{},
	}

	project, err := loader.LoadWithContext(context.Background(), *configDetails, func(opts *loader.Options) {
		opts.SetProjectName("default", false)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse compose YAML: %w", err)
	}

	return (*ComposeConfig)(project), nil
}

// NormalizeToForge converts a ComposeConfig to ForgeAppConfig
func NormalizeToForge(config *ComposeConfig) (*ForgeAppConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("compose config cannot be nil")
	}

	project := (*types.Project)(config)

	forgeConfig := &ForgeAppConfig{
		Version:  "3.8",
		Name:     project.Name,
		Services: make([]ForgeServiceConfig, 0, len(project.Services)),
		Networks: make([]ForgeNetworkConfig, 0, len(project.Networks)),
		Volumes:  make([]ForgeVolumeConfig, 0, len(project.Volumes)),
		Secrets:  make([]ForgeSecretConfig, 0, len(project.Secrets)),
		Configs:  make([]ForgeConfigConfig, 0, len(project.Configs)),
		Extensions: make(map[string]interface{}),
	}

	// Parse services
	for name, svc := range project.Services {
		serviceConfig := normalizeServiceToForge(name, svc)
		forgeConfig.Services = append(forgeConfig.Services, *serviceConfig)
	}

	// Parse networks
	for name, net := range project.Networks {
		networkConfig := normalizeNetworkToForge(name, net)
		forgeConfig.Networks = append(forgeConfig.Networks, *networkConfig)
	}

	// Parse volumes
	for name, vol := range project.Volumes {
		volumeConfig := normalizeVolumeToForge(name, vol)
		forgeConfig.Volumes = append(forgeConfig.Volumes, *volumeConfig)
	}

	// Parse secrets
	for name, sec := range project.Secrets {
		secretConfig := normalizeSecretToForge(name, sec)
		forgeConfig.Secrets = append(forgeConfig.Secrets, *secretConfig)
	}

	// Parse configs
	for name, cfg := range project.Configs {
		configConfig := normalizeConfigToForge(name, cfg)
		forgeConfig.Configs = append(forgeConfig.Configs, *configConfig)
	}

	// Handle extensions (x- fields)
	if project.Extensions != nil {
		for k, v := range project.Extensions {
			forgeConfig.Extensions[k] = v
		}
	}

	return forgeConfig, nil
}

// ValidateCompose validates a ComposeConfig
func ValidateCompose(config *ComposeConfig) error {
	if config == nil {
		return fmt.Errorf("compose config cannot be nil")
	}

	project := (*types.Project)(config)

	// Basic validation - check if we have at least one service
	if len(project.Services) == 0 {
		return fmt.Errorf("compose file must contain at least one service")
	}

	// Validate each service
	for serviceName, service := range project.Services {
		// Check that service has either image or build
		if service.Image == "" && service.Build == nil {
			return fmt.Errorf("service '%s' must specify either 'image' or 'build'", serviceName)
		}

		// Validate healthcheck if present
		if service.HealthCheck != nil {
			if len(service.HealthCheck.Test) == 0 {
				return fmt.Errorf("service '%s' healthcheck must specify test command", serviceName)
			}
		}

		// Validate deploy configuration if present
		if service.Deploy != nil {
			if service.Deploy.Replicas != nil && *service.Deploy.Replicas < 0 {
				return fmt.Errorf("service '%s' deploy replicas must be non-negative", serviceName)
			}
		}
	}

	return nil
}

// ParseComposeString parses Compose YAML content from a string
func (p *ComposeParser) ParseComposeString(content string, filename string) (*types.Project, error) {
	if p.configDetails == nil {
		p.configDetails = &types.ConfigDetails{
			WorkingDir: p.workingDir,
			ConfigFiles: []types.ConfigFile{},
			Environment: map[string]string{},
		}
	}

	configFile := types.ConfigFile{
		Filename: filename,
		Content:  []byte(content),
	}

	p.configDetails.ConfigFiles = []types.ConfigFile{configFile}

	project, err := loader.LoadWithContext(context.Background(), *p.configDetails, func(opts *loader.Options) {
		opts.SetProjectName("default", false)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse compose file: %w", err)
	}

	return project, nil
}

// NormalizeToForgeModels converts a parsed Compose project to Forge internal models
func (p *ComposeParser) NormalizeToForgeModels(project *types.Project) (*ParsedCompose, error) {
	if project == nil {
		return nil, fmt.Errorf("project cannot be nil")
	}

	parsed := &ParsedCompose{
		Version:  "3.8",
		Name:     project.Name,
		Services: make([]ServiceSummary, 0, len(project.Services)),
		Networks: make([]NetworkSummary, 0, len(project.Networks)),
		Volumes:  make([]VolumeSummary, 0, len(project.Volumes)),
		Secrets:  make([]SecretSummary, 0, len(project.Secrets)),
		Configs:  make([]ConfigSummary, 0, len(project.Configs)),
	}

	// Parse services
	for name, svc := range project.Services {
		serviceSummary := p.normalizeService(name, svc)
		parsed.Services = append(parsed.Services, *serviceSummary)
	}

	// Parse networks
	for name, net := range project.Networks {
		networkSummary := p.normalizeNetwork(name, net)
		parsed.Networks = append(parsed.Networks, *networkSummary)
	}

	// Parse volumes
	for name, vol := range project.Volumes {
		volumeSummary := p.normalizeVolume(name, vol)
		parsed.Volumes = append(parsed.Volumes, *volumeSummary)
	}

	// Parse secrets
	for name, sec := range project.Secrets {
		secretSummary := p.normalizeSecret(name, sec)
		parsed.Secrets = append(parsed.Secrets, *secretSummary)
	}

	// Parse configs
	for name, cfg := range project.Configs {
		configSummary := p.normalizeConfig(name, cfg)
		parsed.Configs = append(parsed.Configs, *configSummary)
	}

	return parsed, nil
}

// ParsedCompose represents a parsed Compose configuration
type ParsedCompose struct {
	Version  string           `json:"version,omitempty"`
	Name     string           `json:"name,omitempty"`
	Services []ServiceSummary `json:"services"`
	Networks []NetworkSummary `json:"networks,omitempty"`
	Volumes  []VolumeSummary  `json:"volumes,omitempty"`
	Secrets  []SecretSummary  `json:"secrets,omitempty"`
	Configs  []ConfigSummary  `json:"configs,omitempty"`
}

// ServiceSummary represents a service summary
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
	HealthCheck *HealthSummary     `json:"healthcheck,omitempty"`
	Deploy      *DeploySummary     `json:"deploy,omitempty"`
}

// BuildSummary represents build configuration
type BuildSummary struct {
	Context    string            `json:"context,omitempty"`
	Dockerfile string            `json:"dockerfile,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	Target     string            `json:"target,omitempty"`
}

// HealthSummary represents health check configuration
type HealthSummary struct {
	Test        []string `json:"test,omitempty"`
	Interval    string   `json:"interval,omitempty"`
	Timeout     string   `json:"timeout,omitempty"`
	Retries     int      `json:"retries,omitempty"`
	StartPeriod string   `json:"start_period,omitempty"`
	Disable     bool     `json:"disable,omitempty"`
}

// DeploySummary represents deploy configuration
type DeploySummary struct {
	Mode      string            `json:"mode,omitempty"`
	Replicas  int               `json:"replicas,omitempty"`
	Resources *ResourceSummary `json:"resources,omitempty"`
}

// ResourceSummary represents resource limits and reservations
type ResourceSummary struct {
	Limits      map[string]string `json:"limits,omitempty"`
	Reservations map[string]string `json:"reservations,omitempty"`
}

// NetworkSummary represents network configuration
type NetworkSummary struct {
	Name     string            `json:"name"`
	Driver   string            `json:"driver,omitempty"`
	External bool              `json:"external,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// VolumeSummary represents volume configuration
type VolumeSummary struct {
	Name     string            `json:"name"`
	Driver   string            `json:"driver,omitempty"`
	External bool              `json:"external,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// SecretSummary represents secret configuration
type SecretSummary struct {
	Name     string `json:"name"`
	File     string `json:"file,omitempty"`
	External bool   `json:"external,omitempty"`
}

// ConfigSummary represents config configuration
type ConfigSummary struct {
	Name     string `json:"name"`
	File     string `json:"file,omitempty"`
	External bool   `json:"external,omitempty"`
}

// normalizeService converts a Compose service to a ServiceSummary
func (p *ComposeParser) normalizeService(name string, svc types.ServiceConfig) *ServiceSummary {
	summary := &ServiceSummary{
		Name:        name,
		Image:       svc.Image,
		Ports:       normalizePortsFromCompose(svc.Ports),
		Environment: normalizeEnvFromCompose(svc.Environment),
		Volumes:     normalizeVolumesFromCompose(svc.Volumes),
		DependsOn:   normalizeDependsOnFromCompose(svc.DependsOn),
		Profiles:    svc.Profiles,
		Restart:     stringifyValue(svc.Restart),
		Command:     stringifyValue(svc.Command),
		Entrypoint:  stringifyValue(svc.Entrypoint),
	}

	// Handle build configuration
	if svc.Build != nil {
		summary.Build = &BuildSummary{
			Context:    svc.Build.Context,
			Dockerfile: svc.Build.Dockerfile,
			Args:       mapMappingWithEqualsToMap(svc.Build.Args),
			Target:     svc.Build.Target,
		}
	}

	// Handle health check
	if svc.HealthCheck != nil {
		summary.HealthCheck = &HealthSummary{
			Test:        svc.HealthCheck.Test,
			Interval:    durationPtrToString(svc.HealthCheck.Interval),
			Timeout:     durationPtrToString(svc.HealthCheck.Timeout),
			Retries:     intFromInterface(svc.HealthCheck.Retries),
			StartPeriod: durationPtrToString(svc.HealthCheck.StartPeriod),
			Disable:    boolValue(svc.HealthCheck.Disable),
		}
	}

	// Handle deploy configuration
	if svc.Deploy != nil {
		summary.Deploy = &DeploySummary{
			Mode:      stringifyValue(svc.Deploy.Mode),
			Replicas:  intFromInterface(svc.Deploy.Replicas),
			Resources: normalizeResources(svc.Deploy.Resources),
		}
	}

	return summary
}

// normalizeNetwork converts a Compose network to a NetworkSummary
func (p *ComposeParser) normalizeNetwork(name string, net types.NetworkConfig) *NetworkSummary {
	return &NetworkSummary{
		Name:     name,
		Driver:   net.Driver,
		External: boolValue(net.External),
		Labels:   mapLabelsToMap(net.Labels),
	}
}

// normalizeVolume converts a Compose volume to a VolumeSummary
func (p *ComposeParser) normalizeVolume(name string, vol types.VolumeConfig) *VolumeSummary {
	return &VolumeSummary{
		Name:     name,
		Driver:   vol.Driver,
		External: boolValue(vol.External),
		Labels:   mapLabelsToMap(vol.Labels),
	}
}

// normalizeSecret converts a Compose secret to a SecretSummary
func (p *ComposeParser) normalizeSecret(name string, sec types.SecretConfig) *SecretSummary {
	return &SecretSummary{
		Name:     name,
		File:     sec.File,
		External: boolValue(sec.External),
	}
}

// normalizeConfig converts a Compose config to a ConfigSummary
func (p *ComposeParser) normalizeConfig(name string, cfg types.ConfigObjConfig) *ConfigSummary {
	return &ConfigSummary{
		Name:     name,
		File:     cfg.File,
		External: boolValue(cfg.External),
	}
}

// normalizeServiceToForge converts a Compose service to a ForgeServiceConfig
func normalizeServiceToForge(name string, svc types.ServiceConfig) *ForgeServiceConfig {
	config := &ForgeServiceConfig{
		Name:        name,
		Image:       svc.Image,
		Ports:       normalizePortsToForge(svc.Ports),
		Environment: normalizeEnvToForge(svc.Environment),
		Volumes:     normalizeVolumesToForge(svc.Volumes),
		DependsOn:   normalizeDependsOnToForge(svc.DependsOn),
		Profiles:    svc.Profiles,
		Restart:     stringifyValue(svc.Restart),
		Command:     stringifyValue(svc.Command),
		Entrypoint:  stringifyValue(svc.Entrypoint),
		Labels:      mapLabelsToMap(svc.Labels),
		Extensions:  make(map[string]interface{}),
	}

	// Handle build configuration
	if svc.Build != nil {
		config.Build = &ForgeBuildConfig{
			Context:    svc.Build.Context,
			Dockerfile: svc.Build.Dockerfile,
			Args:       mapMappingWithEqualsToMap(svc.Build.Args),
			Target:     svc.Build.Target,
			CacheFrom:  svc.Build.CacheFrom,
			Labels:     mapLabelsToMap(svc.Build.Labels),
			Network:    svc.Build.Network,
		}
	}

	// Handle health check
	if svc.HealthCheck != nil {
		hc := &ForgeHealthCheckConfig{
			Test:    svc.HealthCheck.Test,
			Disable: svc.HealthCheck.Disable,
		}
		if svc.HealthCheck.Interval != nil {
			hc.Interval = durationPtrToString(svc.HealthCheck.Interval)
		}
		if svc.HealthCheck.Timeout != nil {
			hc.Timeout = durationPtrToString(svc.HealthCheck.Timeout)
		}
		if svc.HealthCheck.Retries != nil {
			hc.Retries = intFromInterface(svc.HealthCheck.Retries)
		}
		if svc.HealthCheck.StartPeriod != nil {
			hc.StartPeriod = durationPtrToString(svc.HealthCheck.StartPeriod)
		}
		config.HealthCheck = hc
	}

	// Handle deploy configuration - simplified to avoid type issues
	if svc.Deploy != nil {
		config.Deploy = &ForgeDeployConfig{
			Mode:    stringifyValue(svc.Deploy.Mode),
			Replicas: intFromInterface(svc.Deploy.Replicas),
			Resources: normalizeResourcesToForge(svc.Deploy.Resources),
			Labels:  mapLabelsToMap(svc.Deploy.Labels),
			Extensions: make(map[string]interface{}),
		}
	}

	// Handle networks
	for _, net := range svc.Networks {
		config.Networks = append(config.Networks, stringifyValue(net))
	}

	// Handle extensions
	for k, v := range svc.Extensions {
		config.Extensions[k] = v
	}

	return config
}

// normalizeNetworkToForge converts a Compose network to a ForgeNetworkConfig
func normalizeNetworkToForge(name string, net types.NetworkConfig) *ForgeNetworkConfig {
	return &ForgeNetworkConfig{
		Name:       name,
		Driver:     net.Driver,
		DriverOpts: mapOptionsToMap(net.DriverOpts),
		External:   boolValue(net.External),
		Internal:   boolValue(net.Internal),
		Attachable: boolValue(net.Attachable),
		Labels:     mapLabelsToMap(net.Labels),
		Extensions: make(map[string]interface{}),
	}
}

// normalizeVolumeToForge converts a Compose volume to a ForgeVolumeConfig
func normalizeVolumeToForge(name string, vol types.VolumeConfig) *ForgeVolumeConfig {
	return &ForgeVolumeConfig{
		Name:       name,
		Driver:     vol.Driver,
		DriverOpts: mapOptionsToMap(vol.DriverOpts),
		External:   boolValue(vol.External),
		Labels:     mapLabelsToMap(vol.Labels),
		Extensions: make(map[string]interface{}),
	}
}

// normalizeSecretToForge converts a Compose secret to a ForgeSecretConfig
func normalizeSecretToForge(name string, sec types.SecretConfig) *ForgeSecretConfig {
	return &ForgeSecretConfig{
		Name:       name,
		File:       sec.File,
		External:   boolValue(sec.External),
		Labels:     mapLabelsToMap(sec.Labels),
		Driver:     sec.Driver,
		DriverOpts: mapOptionsToMap(sec.DriverOpts),
		Template:   sec.TemplateDriver,
		Extensions: make(map[string]interface{}),
	}
}

// normalizeConfigToForge converts a Compose config to a ForgeConfigConfig
func normalizeConfigToForge(name string, cfg types.ConfigObjConfig) *ForgeConfigConfig {
	return &ForgeConfigConfig{
		Name:       name,
		File:       cfg.File,
		External:   boolValue(cfg.External),
		Labels:     mapLabelsToMap(cfg.Labels),
		Template:   cfg.TemplateDriver,
		Extensions: make(map[string]interface{}),
	}
}

// Helper functions

// normalizePortsToForge converts Compose port mappings to string slices
func normalizePortsToForge(ports []types.ServicePortConfig) []string {
	if len(ports) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(ports))
	for _, port := range ports {
		portStr := fmt.Sprintf("%s:%d", port.Published, port.Target)
		if port.Protocol != "" {
			portStr += "/" + port.Protocol
		}
		result = append(result, portStr)
	}
	return result
}

// normalizeEnvToForge converts Compose environment variables to map
func normalizeEnvToForge(env types.MappingWithEquals) map[string]string {
	if len(env) == 0 {
		return map[string]string{}
	}

	result := make(map[string]string)
	for k, v := range env {
		if v != nil {
			result[k] = *v
		} else {
			result[k] = ""
		}
	}
	return result
}

// normalizeVolumesToForge converts Compose volume mounts to string slices
func normalizeVolumesToForge(volumes []types.ServiceVolumeConfig) []string {
	if len(volumes) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(volumes))
	for _, vol := range volumes {
		volumeStr := vol.Source + ":" + vol.Target
		if vol.ReadOnly {
			volumeStr += ":ro"
		}
		result = append(result, volumeStr)
	}
	return result
}

// normalizeDependsOnToForge converts Compose dependencies to string slices
func normalizeDependsOnToForge(deps types.DependsOnConfig) []string {
	if len(deps) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(deps))
	for serviceName, condition := range deps {
		if condition.Condition != "" {
			result = append(result, fmt.Sprintf("%s:%s", serviceName, condition.Condition))
		} else {
			result = append(result, serviceName)
		}
	}
	return result
}

// normalizeResourcesToForge converts Compose resource limits to ForgeResourceConfig
func normalizeResourcesToForge(resources types.Resources) *ForgeResourceConfig {
	if resources.Limits == nil && resources.Reservations == nil {
		return nil
	}

	result := &ForgeResourceConfig{
		Limits:      make(map[string]string),
		Reservations: make(map[string]string),
	}

	if resources.Limits != nil {
		if resources.Limits.NanoCPUs != 0 {
			result.Limits["cpu"] = fmt.Sprintf("%g", resources.Limits.NanoCPUs)
		}
		if resources.Limits.MemoryBytes != 0 {
			result.Limits["memory"] = fmt.Sprintf("%d", resources.Limits.MemoryBytes)
		}
	}

	if resources.Reservations != nil {
		if resources.Reservations.NanoCPUs != 0 {
			result.Reservations["cpu"] = fmt.Sprintf("%g", resources.Reservations.NanoCPUs)
		}
		if resources.Reservations.MemoryBytes != 0 {
			result.Reservations["memory"] = fmt.Sprintf("%d", resources.Reservations.MemoryBytes)
		}
	}

	return result
}

// normalizePortsFromCompose converts Compose port mappings to string slices
func normalizePortsFromCompose(ports []types.ServicePortConfig) []string {
	if len(ports) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(ports))
	for _, port := range ports {
		portStr := fmt.Sprintf("%s:%d", port.Published, port.Target)
		if port.Protocol != "" {
			portStr += "/" + port.Protocol
		}
		result = append(result, portStr)
	}
	return result
}

// normalizeEnvFromCompose converts Compose environment variables to map
func normalizeEnvFromCompose(env types.MappingWithEquals) map[string]string {
	if len(env) == 0 {
		return map[string]string{}
	}

	result := make(map[string]string)
	for k, v := range env {
		if v != nil {
			result[k] = *v
		} else {
			result[k] = ""
		}
	}
	return result
}

// normalizeVolumesFromCompose converts Compose volume mounts to string slices
func normalizeVolumesFromCompose(volumes []types.ServiceVolumeConfig) []string {
	if len(volumes) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(volumes))
	for _, vol := range volumes {
		volumeStr := vol.Source + ":" + vol.Target
		if vol.ReadOnly {
			volumeStr += ":ro"
		}
		result = append(result, volumeStr)
	}
	return result
}

// normalizeDependsOnFromCompose converts Compose dependencies to string slices
func normalizeDependsOnFromCompose(deps types.DependsOnConfig) []string {
	if len(deps) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(deps))
	for serviceName, condition := range deps {
		if condition.Condition != "" {
			result = append(result, fmt.Sprintf("%s:%s", serviceName, condition.Condition))
		} else {
			result = append(result, serviceName)
		}
	}
	return result
}

// normalizeResources converts Compose resource limits to ResourceSummary
func normalizeResources(resources types.Resources) *ResourceSummary {
	if resources.Limits == nil && resources.Reservations == nil {
		return nil
	}

	result := &ResourceSummary{
		Limits:      make(map[string]string),
		Reservations: make(map[string]string),
	}

	if resources.Limits != nil {
		if resources.Limits.NanoCPUs != 0 {
			result.Limits["cpu"] = fmt.Sprintf("%g", resources.Limits.NanoCPUs)
		}
		if resources.Limits.MemoryBytes != 0 {
			result.Limits["memory"] = fmt.Sprintf("%d", resources.Limits.MemoryBytes)
		}
	}

	if resources.Reservations != nil {
		if resources.Reservations.NanoCPUs != 0 {
			result.Reservations["cpu"] = fmt.Sprintf("%g", resources.Reservations.NanoCPUs)
		}
		if resources.Reservations.MemoryBytes != 0 {
			result.Reservations["memory"] = fmt.Sprintf("%d", resources.Reservations.MemoryBytes)
		}
	}

	return result
}

// Helper functions for type conversion

// mapLabelsToMap converts types.Labels to map[string]string
func mapLabelsToMap(labels types.Labels) map[string]string {
	if len(labels) == 0 {
		return nil
	}

	result := make(map[string]string)
	for k, v := range labels {
		result[k] = v
	}
	return result
}

// mapOptionsToMap converts types.Options to map[string]string
func mapOptionsToMap(opts types.Options) map[string]string {
	if len(opts) == 0 {
		return nil
	}

	result := make(map[string]string)
	for k, v := range opts {
		result[k] = v
	}
	return result
}

// mapMappingWithEqualsToMap converts types.MappingWithEquals to map[string]string
func mapMappingWithEqualsToMap(mapping types.MappingWithEquals) map[string]string {
	if len(mapping) == 0 {
		return nil
	}

	result := make(map[string]string)
	for k, v := range mapping {
		if v != nil {
			result[k] = *v
		} else {
			result[k] = ""
		}
	}
	return result
}

// convertStringMappingToMap converts types.Mapping to map[string]string
func convertStringMappingToMap(mapping types.Mapping) map[string]string {
	if len(mapping) == 0 {
		return nil
	}

	result := make(map[string]string)
	for k, v := range mapping {
		result[k] = v
	}
	return result
}

// durationPtrToString converts a *types.Duration to a string representation
func durationPtrToString(d *types.Duration) string {
	if d == nil {
		return ""
	}
	return d.String()
}

// stringifyValue converts various types to string
func stringifyValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []string:
		return strings.Join(val, " ")
	case []interface{}:
		strs := make([]string, len(val))
		for i, item := range val {
			strs[i] = fmt.Sprintf("%v", item)
		}
		return strings.Join(strs, " ")
	default:
		return fmt.Sprintf("%v", val)
	}
}

// intFromInterface attempts to convert an interface{} to int
func intFromInterface(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case *int:
		if val != nil {
			return *val
		}
	case int32:
		return int(val)
	case *int32:
		if val != nil {
			return int(*val)
		}
	case int64:
		return int(val)
	case *int64:
		if val != nil {
			return int(*val)
		}
	case uint:
		return int(val)
	case *uint:
		if val != nil {
			return int(*val)
		}
	case uint32:
		return int(val)
	case *uint32:
		if val != nil {
			return int(*val)
		}
	case uint64:
		return int(val)
	case *uint64:
		if val != nil {
			return int(*val)
		}
	}
	return 0
}

// boolValue converts various boolean types to bool
func boolValue(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	default:
		return false
	}
}
