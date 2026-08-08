// Package envmanifest implements the Phase 2 environment manifest
// (service-definition) engine: a schema type (EnvManifest), a YAML parser and
// an Apply/Render orchestration layer that creates and updates
// applications+services for an existing environment from a declarative
// document.
package envmanifest

import (
	"time"
)

// ManifestPort is a container-level port declaration. HostPort, when set,
// requests a host-side allocation; Protocol is "http", "https", "tcp" or
// "udp".
type ManifestPort struct {
	ContainerPort int    `yaml:"containerPort" json:"containerPort"`
	HostPort      int    `yaml:"hostPort,omitempty" json:"hostPort,omitempty"`
	Protocol      string `yaml:"protocol,omitempty" json:"protocol,omitempty"`
}

// ManifestService declares one service under the manifest application.
type ManifestService struct {
	Name           string            `yaml:"name" json:"name"`
	Image          string            `yaml:"image,omitempty" json:"image,omitempty"`
	ComposeService string            `yaml:"composeService,omitempty" json:"composeService,omitempty"`
	Replicas       int               `yaml:"replicas,omitempty" json:"replicas,omitempty"`
	Ports          []ManifestPort    `yaml:"ports,omitempty" json:"ports,omitempty"`
	Env            map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Groups         []string          `yaml:"groups,omitempty" json:"groups,omitempty"`
	DependsOn      []string          `yaml:"dependsOn,omitempty" json:"dependsOn,omitempty"`
	DesiredState   string            `yaml:"desiredState,omitempty" json:"desiredState,omitempty"`
}

// EnvStep is one ordered environment-variable step applied after services are
// provisioned. Setting Service attaches the variable to that service's scope
// instead of the environment scope.
type EnvStep struct {
	Key       string `yaml:"key" json:"key"`
	Value     string `yaml:"value" json:"value"`
	Service   string `yaml:"service,omitempty" json:"service,omitempty"`
	Sensitive bool   `yaml:"sensitive,omitempty" json:"sensitive,omitempty"`
}

// EnvManifest is the top-level service-definition document. Services declare
// the workload topology; Env holds ordered variable steps; Ports is an
// informational list of env-level exposed ports surfaced on the dashboard.
type EnvManifest struct {
	Version  string            `yaml:"version,omitempty" json:"version,omitempty"`
	Name     string            `yaml:"name" json:"name"`
	Domain   string            `yaml:"domain,omitempty" json:"domain,omitempty"`
	Ports    []ManifestPort    `yaml:"ports,omitempty" json:"ports,omitempty"`
	Services []ManifestService `yaml:"services" json:"services"`
	Env      []EnvStep         `yaml:"env,omitempty" json:"env,omitempty"`
}

// ApplyResult summarizes what Apply changed so callers can log/inspect the
// diff without re-querying.
type ApplyResult struct {
	EnvID           string       `json:"envId"`
	Manifest        *EnvManifest `json:"manifest"`
	AppliedAt       time.Time    `json:"appliedAt"`
	CreatedApps     []string     `json:"createdApps"`
	UpdatedApps     []string     `json:"updatedApps"`
	CreatedServices []string     `json:"createdServices"`
	UpdatedServices []string     `json:"updatedServices"`
	EnvStepsApplied int          `json:"envStepsApplied"`
	Warnings        []string     `json:"warnings,omitempty"`
}
