package trafficmanager

import (
	"context"
	"time"

	"gamepanel/forge/internal/services/domains"
)

type NodeResolver interface {
	ResolveTargetHost(ctx context.Context, serverID string, nodeID string) string
}

type AdapterKind string

const (
	AdapterCaddy   AdapterKind = "caddy"
	AdapterTraefik AdapterKind = "traefik"
)

type HealthStatus string

const (
	HealthUnknown  HealthStatus = "unknown"
	HealthHealthy  HealthStatus = "healthy"
	HealthDegraded HealthStatus = "degraded"
	HealthDown     HealthStatus = "down"
)

type AdapterHealth struct {
	Status   HealthStatus  `json:"status"`
	Message  string        `json:"message,omitempty"`
	Uptime   time.Duration `json:"uptime,omitempty"`
	Version  string        `json:"version,omitempty"`
	ErrCount int           `json:"errCount"`
}

type GatewayAdapter interface {
	Kind() AdapterKind

	UpdateRoutes(ctx context.Context, rules []*RoutingRule, policies map[string]*TrafficPolicy) error

	RemoveRoutes(ctx context.Context, ruleIDs []string) error

	GetActiveConnections() map[string]int

	UpdateDomainRoutes(ctx context.Context, domainRoutes []domains.VerifiedDomainRoute) error

	SetCertificate(ctx context.Context, cert CertConfig) error

	RemoveCertificate(ctx context.Context, domains []string) error

	ValidateConfig(ctx context.Context) error

	Reload(ctx context.Context) error

	Rollback(ctx context.Context) error

	CleanupStale(ctx context.Context, activeRuleIDs map[string]bool) error

	Health(ctx context.Context) AdapterHealth

	SetUpstreamHealth(ctx context.Context, ruleID string, targetHost string, targetPort int, healthy bool) error
}
