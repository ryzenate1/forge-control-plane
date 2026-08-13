package notifications

import (
	"fmt"
	"strings"
	"time"
)

// EventDescriptor describes one dispatchable notification event type for the
// notifications engine. It is the contract shared by the GET
// /api/v1/notifications/events catalog, the subscription matrix in the admin
// UI, and the template renderer in the router.
//
// Names follow the Dokploy-inspired dotted convention (deploy.success,
// backup.completed, ...). LegacyAliases preserves the event names that were
// already stored in notification_event_subscriptions by the pre-engine
// notification service ("deployment.complete", "node.down", ...), so existing
// subscriptions keep firing without a data migration.
type EventDescriptor struct {
	Type          string            `json:"type"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Category      string            `json:"category"`
	Severity      NotificationLevel `json:"severity"`
	LegacyAliases []string          `json:"legacyAliases,omitempty"`
	InternalTypes []string          `json:"internalTypes,omitempty"`
	DefaultTmpl   string            `json:"-"`
}

// NotificationLevel is the normalized severity of a notification. Channel
// notifiers translate it into their own color/style vocabulary.
type NotificationLevel string

const (
	LevelInfo     NotificationLevel = "info"
	LevelSuccess  NotificationLevel = "success"
	LevelWarning  NotificationLevel = "warning"
	LevelError    NotificationLevel = "error"
	LevelCritical NotificationLevel = "critical"
)

// defaultTemplate renders any event uniformly when the catalog has no specific
// template (and the subscription carries none).
const defaultTemplate = `[{{.Event}}] {{.Summary}} ({{.ResourceType}} {{.ResourceID}}) at {{.Timestamp}}`

// NotificationCatalog lists every event type a user can subscribe to.
// InternalTypes wire the entry to events.Registry event names published by the
// control plane; entries without InternalTypes (ssl.expiring, preview.created)
// are dispatched by calling Router.DispatchEvent directly from the owning
// service, and are advertised here so subscriptions can exist before the
// producer is wired.
var NotificationCatalog = []EventDescriptor{
	{Type: "deploy.success", Name: "Deployment succeeded", Category: "delivery", Severity: LevelSuccess,
		Description:   "An application or server deployment finished successfully",
		LegacyAliases: []string{"deployment.complete"}, InternalTypes: []string{"DeploymentCompleted", "AppDeployed"},
		DefaultTmpl: "Deployment of {{.Resource}} finished successfully."},
	{Type: "deploy.failed", Name: "Deployment failed", Category: "delivery", Severity: LevelError,
		Description:   "An application or server deployment failed",
		LegacyAliases: []string{"deployment.failed"}, InternalTypes: []string{"DeploymentFailed", "AppFailed"},
		DefaultTmpl: "Deployment of {{.Resource}} failed. Check the deployment logs for details."},
	{Type: "deploy.rolled_back", Name: "Deployment rolled back", Category: "delivery", Severity: LevelWarning,
		Description:   "A deployment was automatically or manually rolled back",
		InternalTypes: []string{"DeploymentRolledBack", "compose_stack_rolled_back"},
		DefaultTmpl:   "{{.ResourceType}} {{.Resource}} was rolled back."},
	{Type: "server.crash", Name: "Server crash", Category: "servers", Severity: LevelCritical,
		Description:   "A game server process crashed",
		LegacyAliases: []string{"server.crash"}, InternalTypes: []string{"ServerCrashed"},
		DefaultTmpl: "Server {{.Resource}} crashed{{with field .Payload \"crash_count\"}} ({{.}} consecutive times){{end}}."},
	{Type: "server.started", Name: "Server started", Category: "servers", Severity: LevelInfo,
		Description:   "A server was powered on",
		LegacyAliases: []string{"server.startup"}, InternalTypes: []string{"ServerStarted", "server:started"},
		DefaultTmpl: "Server {{.Resource}} started."},
	{Type: "server.stopped", Name: "Server stopped", Category: "servers", Severity: LevelInfo,
		Description:   "A server was powered off or killed",
		InternalTypes: []string{"ServerStopped", "server:stopped"},
		DefaultTmpl:   "Server {{.Resource}} stopped."},
	{Type: "server.restarted", Name: "Server restarted", Category: "servers", Severity: LevelInfo,
		Description:   "A server was restarted",
		InternalTypes: []string{"ServerRestarted", "server:restarted"},
		DefaultTmpl:   "Server {{.Resource}} restarted."},
	{Type: "server.installed", Name: "Server install complete", Category: "servers", Severity: LevelSuccess,
		Description:   "A server installation completed",
		LegacyAliases: []string{"server.install.complete"}, InternalTypes: []string{"ServerInstallCompleted", "server:installed"},
		DefaultTmpl: "Installation completed for server {{.Resource}}."},
	{Type: "backup.completed", Name: "Backup completed", Category: "backups", Severity: LevelSuccess,
		Description:   "A backup finished successfully",
		LegacyAliases: []string{"backup.complete"}, InternalTypes: []string{"ServerBackupCreated"},
		DefaultTmpl: "Backup {{coalesce (field .Payload \"backup_name\" \"uuid\") .ResourceID}} completed for {{.Resource}}."},
	{Type: "backup.failed", Name: "Backup failed", Category: "backups", Severity: LevelError,
		Description:   "A backup attempt failed",
		LegacyAliases: []string{"backup.failed"}, InternalTypes: []string{"ServerBackupFailed"},
		DefaultTmpl: "Backup {{coalesce (field .Payload \"backup_name\" \"uuid\") .ResourceID}} failed{{with field .Payload \"error\"}}: {{.}}{{end}}."},
	{Type: "backup.restored", Name: "Backup restored", Category: "backups", Severity: LevelInfo,
		Description:   "A backup was restored onto a server",
		InternalTypes: []string{"ServerBackupRestored"},
		DefaultTmpl:   "Backup {{coalesce (field .Payload \"backup_name\" \"uuid\") .ResourceID}} was restored onto {{.Resource}}."},
	{Type: "node.offline", Name: "Node offline", Category: "infrastructure", Severity: LevelCritical,
		Description:   "A node stopped responding to heartbeats",
		LegacyAliases: []string{"node.down"}, InternalTypes: []string{"NodeOffline"},
		DefaultTmpl: "Node {{.Resource}} went offline."},
	{Type: "node.online", Name: "Node online", Category: "infrastructure", Severity: LevelInfo,
		Description:   "A node came back online",
		LegacyAliases: []string{"node.up"}, InternalTypes: []string{"NodeOnline", "NodeRecovered"},
		DefaultTmpl: "Node {{.Resource}} is back online."},
	{Type: "ssl.expiring", Name: "SSL certificate expiring", Category: "networking", Severity: LevelWarning,
		Description: "A TLS certificate is close to its expiry date",
		DefaultTmpl: "The certificate for {{.Resource}} expires{{with field .Payload \"expires_at\" \"expiresAt\"}} on {{.}}{{end}}."},
	{Type: "preview.created", Name: "Preview environment created", Category: "delivery", Severity: LevelInfo,
		Description: "A pull-request preview environment was created",
		DefaultTmpl: "Preview environment {{.Resource}} was created{{with field .Payload \"url\"}} at {{.}}{{end}}{{with field .Payload \"branch\"}} for branch {{.}}{{end}}."},
	{Type: "compose.deployed", Name: "Compose stack deployed", Category: "delivery", Severity: LevelSuccess,
		Description:   "A Docker Compose stack deployment finished",
		InternalTypes: []string{"compose_stack_deployed"},
		DefaultTmpl:   "Compose stack {{.Resource}} deployed{{with field .Payload \"commit\"}} at {{.}}{{end}}."},
	{Type: "compose.failed", Name: "Compose stack failed", Category: "delivery", Severity: LevelError,
		Description:   "A Docker Compose stack update or rollback failed",
		InternalTypes: []string{"compose_stack_update_failed", "compose_stack_rollback_failed"},
		DefaultTmpl:   "Compose stack {{.Resource}} failed{{with field .Payload \"error\"}}: {{.}}{{end}}."},
}

// catalogByType indexes the catalog by canonical event name.
var catalogByType = func() map[string]EventDescriptor {
	index := make(map[string]EventDescriptor, len(NotificationCatalog))
	for _, entry := range NotificationCatalog {
		index[entry.Type] = entry
	}
	return index
}()

// catalogByAlias indexes the catalog by legacy name and internal event type.
var catalogByAlias = func() map[string]EventDescriptor {
	index := make(map[string]EventDescriptor, len(NotificationCatalog)*4)
	for _, entry := range NotificationCatalog {
		for _, alias := range entry.LegacyAliases {
			index[alias] = entry
		}
		for _, internal := range entry.InternalTypes {
			index[internal] = entry
		}
	}
	return index
}()

// LookupEvent returns the catalog entry for a canonical event type.
func LookupEvent(eventType string) (EventDescriptor, bool) {
	entry, ok := catalogByType[eventType]
	return entry, ok
}

// IsKnownEventName reports whether name resolves to a catalog entry (as a
// canonical type, legacy alias or internal event-bus name).
func IsKnownEventName(name string) bool {
	if _, ok := catalogByType[name]; ok {
		return true
	}
	_, ok := catalogByAlias[name]
	return ok
}

// ResolveEvent normalizes any accepted event name (canonical, legacy alias or
// internal event-bus type) to its catalog entry. Unknown names are returned
// unchanged as a synthetic descriptor so custom DispatchEvent callers still
// fan out to exact/wildcard subscriptions.
func ResolveEvent(name string) EventDescriptor {
	if entry, ok := catalogByType[name]; ok {
		return entry
	}
	if entry, ok := catalogByAlias[name]; ok {
		return entry
	}
	return EventDescriptor{
		Type: name, Name: name, Category: "custom", Severity: LevelInfo,
		Description: "Custom event dispatched by a service or integration.",
		DefaultTmpl: defaultTemplate,
	}
}

// EventNamesFor returns every name an incoming event should match against:
// the canonical type plus legacy aliases. Subscriptions may have been created
// with either spelling.
func (e EventDescriptor) EventNamesFor() []string {
	names := make([]string, 0, 1+len(e.LegacyAliases))
	names = append(names, e.Type)
	names = append(names, e.LegacyAliases...)
	return names
}

// renderData is the document handed to text/template. Payload keys are also
// flattened onto the top level of the template data by the router so
// {{.server_id}} works alongside {{index .Payload "server_id"}}.
type renderData struct {
	Event        string            `json:"event"`
	Title        string            `json:"title"`
	Summary      string            `json:"summary"`
	Severity     NotificationLevel `json:"severity"`
	ResourceType string            `json:"resource_type"`
	ResourceID   string            `json:"resource_id"`
	Resource     string            `json:"resource"`
	Timestamp    string            `json:"timestamp"`
	Payload      map[string]any    `json:"payload"`
}

// templateFuncs exposes helpers used by the catalog's default templates.
var templateFuncs = map[string]any{
	// field returns the first present, stringifiable payload key.
	"field": func(payload map[string]any, keys ...string) string {
		for _, key := range keys {
			if value, ok := payload[key]; ok {
				if text, valid := stringifyTemplateValue(value); valid {
					return text
				}
			}
		}
		return ""
	},
	// coalesce returns the first non-empty argument.
	"coalesce": func(values ...string) string {
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				return value
			}
		}
		return ""
	},
	"time": func(layout string, value time.Time) string { return value.Format(layout) },
}

func stringifyTemplateValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case int, int32, int64, float32, float64, bool:
		return fmt.Sprintf("%v", typed), true
	case time.Time:
		return typed.Format(time.RFC3339), true
	default:
		return "", false
	}
}
