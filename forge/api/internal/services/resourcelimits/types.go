// Package resourcelimits stores per-application process configuration — CPU and
// memory limits, replica counts and health-check definitions — and applies it
// to the compose document an application releases.
//
// It is the Forge counterpart to three Dokku plugins:
//
//   - `resource` — per-process CPU/memory limits, applied at container create;
//   - `checks`   — deploy-gating health verification with rollback;
//   - `ps`       — replica counts per process type.
//
// Dokku persists these as dotted keys in per-plugin property files and renders
// them into `docker run` flags from a scheduler hook. Forge has the same hook
// available in a better place: the deploy artefact is a compose document, and
// compose natively expresses deploy.resources.limits, healthcheck and
// deploy.replicas. So this package mutates the document instead of shelling out
// flags, and Docker enforces the result.
//
// Layering follows the panel convention: handlers -> Service -> Store. The
// Store is defined and implemented here (over the shared pgx pool) rather than
// in internal/store so the feature stays fully additive.
package resourcelimits

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Unit constants for the two limit fields. Both are stored in the database in
// these canonical units; conversion for display or for compose rendering only
// ever happens at the boundary.
const (
	// NanoCoresPerCore matches Docker's own convention: cpu_limit is a count of
	// nanoCores, where 1_000_000_000 is one full core. Storing an integer keeps
	// fractional cores exact.
	NanoCoresPerCore = 1_000_000_000

	// MinNanoCores is Docker's smallest enforceable CPU quota (1 microcore).
	MinNanoCores = 1_000
	// MaxNanoCores is an upper bound we consider honest for a single process.
	MaxNanoCores = 64 * NanoCoresPerCore

	// BytesPerMB is used by the API's convenience MB fields.
	BytesPerMB = 1_048_576

	// MinMemoryBytes is one megabyte — below that a container is not going to
	// run anything real, and a zero here would mean "unlimited" in Docker,
	// which is almost never what a caller who typed "0" meant.
	MinMemoryBytes = BytesPerMB
	// MaxMemoryBytes is 64 TiB, an upper sanity bound, not a platform limit.
	MaxMemoryBytes = 64 * 1024 * BytesPerMB * 1024

	// MaxReplicas bounds a single process type. Beyond this the honest answer
	// is "use an autoscaler", not "multiply this container".
	MaxReplicas = 64
)

// HealthCheckType enumerates the supported probe kinds. These map 1:1 onto the
// three things Docker's HEALTHCHECK can express.
type HealthCheckType string

const (
	HealthCheckNone    HealthCheckType = ""
	HealthCheckHTTP    HealthCheckType = "http"
	HealthCheckTCP     HealthCheckType = "tcp"
	HealthCheckCommand HealthCheckType = "command"
)

func (t HealthCheckType) Valid() bool {
	switch t {
	case HealthCheckNone, HealthCheckHTTP, HealthCheckTCP, HealthCheckCommand:
		return true
	}
	return false
}

// HealthCheck timings (seconds) bounded the same way the schema does, so a
// request that slips past the service is still rejected by Postgres rather
// than being written as nonsense.
const (
	DefaultHealthCheckInterval     = 30
	DefaultHealthCheckTimeout      = 10
	DefaultHealthCheckRetries      = 3
	DefaultHealthCheckStartPeriod  = 40
	MaxHealthCheckWindowSeconds    = 3600
	MaxHealthCheckRetries          = 60
	defaultHealthCheckHTTPPort     = 80
	defaultHealthCheckCommandStyle = "CMD-SHELL"
)

// Sentinel errors. Handlers translate these to HTTP statuses; matching on the
// message text would make the API surface fragile.
var (
	// ErrApplicationNotFound is returned when the application does not exist.
	ErrApplicationNotFound = errors.New("application not found")
	// ErrProcessConfigNotFound is returned when the (app, process type) row is
	// absent. Delete and scale treat it as "nothing to do", never as success.
	ErrProcessConfigNotFound = errors.New("process configuration not found")
	// ErrInvalidComposeDocument is returned when the document handed to
	// ApplyToCompose is not a compose document we can safely edit.
	ErrInvalidComposeDocument = errors.New("invalid compose document")
	// ErrNoBindableApplication is returned when a server has no application
	// bound to it, so there is nothing whose process types could be configured.
	ErrNoBindableApplication = errors.New("no application is bound to this server")
)

// ValidationError marks an error the caller can fix by sending different
// values, as opposed to a failure inside the platform. Without it a handler can
// only guess at the difference by matching message text, which makes the HTTP
// status whatever the last rewording happened to produce.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// invalidf builds a ValidationError. Every rejection this package raises about
// a value the caller supplied goes through it.
func invalidf(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// asValidation relabels an error from Validate/Normalize as a caller error.
// Errors that already carry the mark pass through unchanged so the wrapping is
// idempotent.
func asValidation(err error) error {
	if err == nil {
		return nil
	}
	var existing *ValidationError
	if errors.As(err, &existing) {
		return err
	}
	return &ValidationError{Message: err.Error()}
}

// ProcessConfig is one (application, process type) row.
//
// Nil vs zero matters for the two limits: a nil CPULimit means "no CPU
// constraint" while a zero would be nonsense, and the same for MemoryLimit.
// Replicas is intentionally NOT a pointer — "unset" and "scale to zero" are
// different states, and compose can express both, so the zero value is real.
type ProcessConfig struct {
	ID            string `json:"id"`
	ApplicationID string `json:"applicationId"`
	ProcessType   string `json:"processType"`

	CPULimit    *int64 `json:"cpuLimitNanoCores"`
	MemoryLimit *int64 `json:"memoryLimitBytes"`
	Replicas    int    `json:"replicas"`
	// HealthCheckGating is "a failing probe rolls the release back". It is
	// stored, not derived: an operator who only wants visibility sets it false
	// and keeps the check.
	HealthCheckGating bool `json:"healthCheckGating"`

	HealthCheckType        HealthCheckType `json:"healthCheckType"`
	HealthCheckPath        string          `json:"healthCheckPath"`
	HealthCheckPort        int             `json:"healthCheckPort"`
	HealthCheckCommand     string          `json:"healthCheckCommand"`
	HealthCheckInterval    int             `json:"healthCheckIntervalSeconds"`
	HealthCheckTimeout     int             `json:"healthCheckTimeoutSeconds"`
	HealthCheckRetries     int             `json:"healthCheckRetries"`
	HealthCheckStartPeriod int             `json:"healthCheckStartPeriodSeconds"`

	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ApplicationRef is the minimal application identity the per-server tab needs
// in order to call the app-scoped routes.
type ApplicationRef struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ServerID     string `json:"serverId"`
	Organization string `json:"organizationId"`
	SourceType   string `json:"sourceType"`
}

// HasHealthCheck reports whether a probe is defined and should be rendered
// into the compose document.
func (c ProcessConfig) HasHealthCheck() bool {
	return c.Enabled && c.HealthCheckType != HealthCheckNone
}

// GatesDeploy reports whether a failing probe of this process type must roll
// the current release back. Both the probe and the operator's decision to trust
// it are required — a check that nobody enabled gates nothing.
func (c ProcessConfig) GatesDeploy() bool {
	return c.HasHealthCheck() && c.HealthCheckGating
}

// GraceWindowSeconds is how long the platform waits for the first passing
// probe before it considers the rollout verdict final: the container's start
// period, plus every retry (each retry costs one interval plus the time the
// probe itself is allowed to hang).
func (c ProcessConfig) GraceWindowSeconds() int {
	if !c.HasHealthCheck() {
		return 0
	}
	interval := c.HealthCheckInterval
	if interval <= 0 {
		interval = DefaultHealthCheckInterval
	}
	timeout := c.HealthCheckTimeout
	if timeout <= 0 {
		timeout = DefaultHealthCheckTimeout
	}
	retries := c.HealthCheckRetries
	if retries <= 0 {
		retries = DefaultHealthCheckRetries
	}
	startPeriod := c.HealthCheckStartPeriod
	if startPeriod < 0 {
		startPeriod = DefaultHealthCheckStartPeriod
	}
	window := startPeriod + retries*(interval+timeout)
	if window > MaxHealthCheckWindowSeconds {
		return MaxHealthCheckWindowSeconds
	}
	return window
}

// HealthObservation is one probe result for one process on one server.
type HealthObservation struct {
	ID            string    `json:"id"`
	ServerID      string    `json:"serverId"`
	ApplicationID *string   `json:"applicationId,omitempty"`
	ProcessType   string    `json:"processType"`
	Healthy       bool      `json:"healthy"`
	Detail        string    `json:"detail"`
	ObservedAt    time.Time `json:"observedAt"`
}

// ProcessHealthState is the roll-up the UI renders for a single process type:
// what was asked for, what was last seen, and whether that is enough to trust
// the current release.
//
// Status is one of "healthy", "unhealthy", "pending" or "unknown". "pending"
// means the grace window is still open and nothing has failed yet; "unknown"
// means no process owns a check at all, which is explicitly NOT health.
type ProcessHealthState struct {
	ProcessType    string     `json:"processType"`
	Replicas       int        `json:"replicas"`
	CPULimit       *int64     `json:"cpuLimitNanoCores,omitempty"`
	MemoryLimit    *int64     `json:"memoryLimitBytes,omitempty"`
	HealthCheck    string     `json:"healthCheckType"`
	Gating         bool       `json:"healthCheckGating"`
	Enabled        bool       `json:"enabled"`
	Status         string     `json:"status"`
	Detail         string     `json:"detail,omitempty"`
	LastObservedAt *time.Time `json:"lastObservedAt,omitempty"`
	ObservedCount  int        `json:"observedCount"`
	GraceSeconds   int        `json:"graceWindowSeconds"`
}

// Health status values reported by ProcessHealthState.Status.
const (
	HealthStatusHealthy   = "healthy"
	HealthStatusUnhealthy = "unhealthy"
	HealthStatusPending   = "pending"
	HealthStatusUnknown   = "unknown"
)

// ProcessConfigInput is the upsert payload. Pointer fields mean "leave the
// stored value alone" for Update, and "not specified" for Upsert, where the
// service fills in the documented defaults.
type ProcessConfigInput struct {
	CPULimit      *int64 `json:"cpuLimitNanoCores"`
	MemoryLimit   *int64 `json:"memoryLimitBytes"`
	MemoryLimitMB *int64 `json:"memoryLimitMb"`
	Replicas      *int   `json:"replicas"`

	HealthCheckType        *string `json:"healthCheckType"`
	HealthCheckPath        *string `json:"healthCheckPath"`
	HealthCheckPort        *int    `json:"healthCheckPort"`
	HealthCheckCommand     *string `json:"healthCheckCommand"`
	HealthCheckInterval    *int    `json:"healthCheckIntervalSeconds"`
	HealthCheckTimeout     *int    `json:"healthCheckTimeoutSeconds"`
	HealthCheckRetries     *int    `json:"healthCheckRetries"`
	HealthCheckStartPeriod *int    `json:"healthCheckStartPeriodSeconds"`

	Enabled *bool `json:"enabled"`
	// HealthCheckGating makes a failing probe roll the release back. It defaults
	// to true so that "define a check" means "gate on the check", matching
	// Dokku's `checks` plugin, where a failing check aborts the deploy unless
	// the operator explicitly opts out. Set it false to keep probing while
	// downgrading the result to observation only.
	HealthCheckGating *bool `json:"healthCheckGating"`
}

// Validate normalises and checks the process type plus limit fields. It is
// exported so handlers can reject a bad body with 400 before touching the
// database.
func (c ProcessConfig) Validate() error {
	if _, err := NormalizeProcessType(c.ProcessType); err != nil {
		return err
	}
	if c.CPULimit != nil {
		if *c.CPULimit < MinNanoCores || *c.CPULimit > MaxNanoCores {
			return invalidf("cpu limit must be between %d and %d nanoCores, got %d", MinNanoCores, MaxNanoCores, *c.CPULimit)
		}
	}
	if c.MemoryLimit != nil {
		if *c.MemoryLimit < MinMemoryBytes || *c.MemoryLimit > MaxMemoryBytes {
			return invalidf("memory limit must be between %d and %d bytes, got %d", MinMemoryBytes, MaxMemoryBytes, *c.MemoryLimit)
		}
	}
	if c.Replicas < 0 || c.Replicas > MaxReplicas {
		return invalidf("replicas must be between 0 and %d", MaxReplicas)
	}
	if !c.HealthCheckType.Valid() {
		return invalidf("health check type must be one of http, tcp, command or empty to clear")
	}
	if err := validateWindow("interval", c.HealthCheckInterval); err != nil {
		return err
	}
	if err := validateWindow("timeout", c.HealthCheckTimeout); err != nil {
		return err
	}
	if err := validateWindow("start period", c.HealthCheckStartPeriod); err != nil {
		return err
	}
	if c.HealthCheckRetries < 1 || c.HealthCheckRetries > MaxHealthCheckRetries {
		return invalidf("retries must be between 1 and %d", MaxHealthCheckRetries)
	}
	if c.HealthCheckPort < 0 || c.HealthCheckPort > 65535 {
		return invalidf("health check port must be between 0 and 65535")
	}
	switch c.HealthCheckType {
	case HealthCheckCommand:
		if strings.TrimSpace(c.HealthCheckCommand) == "" {
			return invalidf("command health checks require a command")
		}
	case HealthCheckTCP:
		if c.HealthCheckPort == 0 {
			return invalidf("tcp health checks require a port")
		}
	case HealthCheckHTTP:
		// Path and port get defaults in Normalize, so only reject a clearly
		// broken request here.
		if !strings.HasPrefix(c.HealthCheckPath, "/") {
			return invalidf("http health check path must start with /")
		}
	case HealthCheckNone:
	}
	return nil
}

func validateWindow(field string, seconds int) error {
	if seconds < 0 || seconds > MaxHealthCheckWindowSeconds {
		return invalidf("health check %s must be between 0 and %d seconds", field, MaxHealthCheckWindowSeconds)
	}
	return nil
}

// NormalizeProcessType validates a Dokku-style process type ("web", "worker",
// "release"). These land in a VARCHAR(32) and are used verbatim as compose
// service keys, so the character class is restricted instead of relying on the
// database to complain later.
func NormalizeProcessType(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "", invalidf("process type is required")
	}
	if len(value) > 32 {
		return "", invalidf("process type must be 32 characters or fewer")
	}
	for i, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
			// Separators may not lead or trail: ".web" and "web." are not
			// service names anyone typed on purpose.
			if i == 0 || i == len(value)-1 {
				return "", invalidf("process type may not start or end with a separator")
			}
		default:
			return "", invalidf("process type may only contain letters, digits, '-', '_' and '.'")
		}
	}
	return value, nil
}

// int64Value dereferences a limit pointer for the compose renderer, which needs
// to distinguish "absent" from "zero".
func int64Value(v *int64) (int64, bool) {
	if v == nil {
		return 0, false
	}
	return *v, *v > 0
}

// Normalize fills in the defaults the database also enforces, so a caller that
// omitted the optional fields gets a config identical to the one Postgres would
// have stored — and so the compose renderer never sees a zero-valued interval.
//
// Replicas is deliberately untouched: "scale to zero" is a real intent (Dokku's
// `ps:scale web=0` stops a process type), so only the caller — which knows
// whether the field was present in the request — may decide the default.
func (c *ProcessConfig) Normalize() error {
	processType, err := NormalizeProcessType(c.ProcessType)
	if err != nil {
		return err
	}
	c.ProcessType = processType

	if c.CPULimit != nil && *c.CPULimit == 0 {
		c.CPULimit = nil
	}
	if c.MemoryLimit != nil && *c.MemoryLimit == 0 {
		c.MemoryLimit = nil
	}
	if c.Replicas < 0 {
		return invalidf("replicas must not be negative")
	}

	// The four timings are stored NOT NULL with a >=1 CHECK even for a process
	// that has no check, so they are defaulted unconditionally. Leaving them at
	// zero would make an honest "no health check" row fail at the database
	// instead of at the (correct) place.
	if c.HealthCheckInterval <= 0 {
		c.HealthCheckInterval = DefaultHealthCheckInterval
	}
	if c.HealthCheckTimeout <= 0 {
		c.HealthCheckTimeout = DefaultHealthCheckTimeout
	}
	if c.HealthCheckRetries <= 0 {
		c.HealthCheckRetries = DefaultHealthCheckRetries
	}
	if c.HealthCheckStartPeriod <= 0 {
		c.HealthCheckStartPeriod = DefaultHealthCheckStartPeriod
	}

	if c.HealthCheckType == HealthCheckNone {
		c.HealthCheckPath = ""
		c.HealthCheckCommand = ""
		c.HealthCheckPort = 0
		// Nothing to gate on. Keeping the flag true would be a stored promise
		// the config cannot keep.
		c.HealthCheckGating = false
	} else {
		switch c.HealthCheckType {
		case HealthCheckHTTP:
			if strings.TrimSpace(c.HealthCheckPath) == "" {
				c.HealthCheckPath = "/healthz"
			}
			if !strings.HasPrefix(c.HealthCheckPath, "/") {
				c.HealthCheckPath = "/" + c.HealthCheckPath
			}
			if c.HealthCheckPort == 0 {
				c.HealthCheckPort = defaultHealthCheckHTTPPort
			}
			c.HealthCheckCommand = ""
		case HealthCheckTCP:
			if c.HealthCheckPort == 0 {
				return invalidf("tcp health checks require a port")
			}
			c.HealthCheckPath = ""
			c.HealthCheckCommand = ""
		case HealthCheckCommand:
			c.HealthCheckPath = ""
			c.HealthCheckPort = 0
		}
	}
	return nil
}
