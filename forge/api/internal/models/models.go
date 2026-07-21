package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Base columns shared by integer-primary-key models. The gorm struct-tag
// values are placeholders only; this package does not import gorm.
type BaseModel struct {
	ID        uint64    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// UUIDModel is the base for UUID-primary-key models.
type UUIDModel struct {
	ID string `json:"id" gorm:"primaryKey;type:uuid"`
}

// Server is the domain-agnostic server record.
type Server struct {
	UUIDModel
	Name        string            `json:"name" gorm:"type:text;not null"`
	Description string            `json:"description" gorm:"type:text"`
	OwnerID     string            `json:"owner_id" gorm:"type:uuid;index;not null"`
	NodeID      string            `json:"node_id" gorm:"type:uuid;index;not null"`
	Status      string            `json:"status" gorm:"type:text;default:'stopped'"`
	Suspended   bool              `json:"suspended" gorm:"default:false"`
	Installing  bool              `json:"installing" gorm:"default:false"`
	Installed   bool              `json:"installed" gorm:"default:false"`
	DiskMB      int               `json:"disk_mb" gorm:"default:0"`
	MemoryMB    int               `json:"memory_mb" gorm:"default:0"`
	CPUShares   int               `json:"cpu_shares" gorm:"default:0"`
	SwapMB      int               `json:"swap_mb" gorm:"default:0"`
	EggID       string            `json:"egg_id" gorm:"type:text"`
	NestID      string            `json:"nest_id" gorm:"type:text"`
	Container   Container         `json:"container" gorm:"type:jsonb"`
	Env         map[string]string `json:"env" gorm:"type:jsonb"`
	CreatedAt   time.Time         `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time         `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt   *time.Time        `json:"deleted_at,omitempty" gorm:"index"`
}

func (s *Server) Validate() error {
	if s.Name == "" {
		return errors.New("name is required")
	}
	if s.OwnerID == "" {
		return errors.New("owner_id is required")
	}
	if s.NodeID == "" {
		return errors.New("node_id is required")
	}
	return nil
}

// Container captures the docker image and startup command for a server.
type Container struct {
	Image          string `json:"image" gorm:"type:text"`
	StartupCommand string `json:"startup_command" gorm:"type:text"`
}

// User is the domain-agnostic user record.
type User struct {
	BaseModel
	Email        string     `json:"email" gorm:"type:text;uniqueIndex;not null"`
	Username     string     `json:"username" gorm:"type:text;uniqueIndex;not null"`
	PasswordHash string     `json:"-" gorm:"type:text;not null"`
	DisplayName  string     `json:"display_name" gorm:"type:text"`
	RoleID       uint64     `json:"role_id" gorm:"index"`
	Status       string     `json:"status" gorm:"type:text;default:'active'"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty" gorm:"index"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty" gorm:"index"`
}

func (u *User) Validate() error {
	if u.Email == "" {
		return errors.New("email is required")
	}
	if u.Username == "" {
		return errors.New("username is required")
	}
	if u.PasswordHash == "" {
		return errors.New("password_hash is required")
	}
	return nil
}

// Role groups permissions for users.
type Role struct {
	BaseModel
	Name        string     `json:"name" gorm:"type:text;uniqueIndex;not null"`
	Description string     `json:"description" gorm:"type:text"`
	IsAdmin     bool       `json:"is_admin" gorm:"default:false"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" gorm:"index"`
}

func (r *Role) Validate() error {
	if r.Name == "" {
		return errors.New("name is required")
	}
	return nil
}

// Permission is a single grantable capability.
type Permission struct {
	BaseModel
	Name        string `json:"name" gorm:"type:text;uniqueIndex;not null"`
	Description string `json:"description" gorm:"type:text"`
}

func (p *Permission) Validate() error {
	if p.Name == "" {
		return errors.New("name is required")
	}
	return nil
}

// RolePermission joins roles to permissions.
type RolePermission struct {
	RoleID       uint64    `json:"role_id" gorm:"primaryKey;index"`
	PermissionID uint64    `json:"permission_id" gorm:"primaryKey;index"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// Scope is an OAuth/API-key scope.
type Scope struct {
	BaseModel
	Name        string `json:"name" gorm:"type:text;uniqueIndex;not null"`
	Description string `json:"description" gorm:"type:text"`
	IsDefault   bool   `json:"is_default" gorm:"default:false"`
}

func (s *Scope) Validate() error {
	if s.Name == "" {
		return errors.New("name is required")
	}
	return nil
}

// UserScope joins users to scopes.
type UserScope struct {
	UserID    uint64    `json:"user_id" gorm:"primaryKey;index"`
	ScopeID   uint64    `json:"scope_id" gorm:"primaryKey;index"`
	GrantedAt time.Time `json:"granted_at" gorm:"autoCreateTime"`
}

// Session is an authenticated user session.
type Session struct {
	ID        string     `json:"id" gorm:"primaryKey;type:text"`
	UserID    uint64     `json:"user_id" gorm:"index;not null"`
	TokenHash string     `json:"-" gorm:"type:text;not null"`
	IssuedAt  time.Time  `json:"issued_at" gorm:"autoCreateTime"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" gorm:"index"`
	IP        string     `json:"ip" gorm:"type:text"`
	UserAgent string     `json:"user_agent" gorm:"type:text"`
}

func (s *Session) Validate() error {
	if s.UserID == 0 {
		return errors.New("user_id is required")
	}
	if s.TokenHash == "" {
		return errors.New("token_hash is required")
	}
	return nil
}

// Node is a daemon host.
type Node struct {
	UUIDModel
	Name            string      `json:"name" gorm:"type:text;not null"`
	FQDN            string      `json:"fqdn" gorm:"type:text"`
	Scheme          string      `json:"scheme" gorm:"type:text;default:'https'"`
	ListenPort      int         `json:"listen_port" gorm:"default:9090"`
	DaemonTokenHash string      `json:"-" gorm:"type:text;not null"`
	MemoryMB        int         `json:"memory_mb" gorm:"default:0"`
	DiskMB          int         `json:"disk_mb" gorm:"default:0"`
	Location        string      `json:"location" gorm:"type:text"`
	Tags            StringArray `json:"tags" gorm:"type:text[]"`
	RuntimeProvider string      `json:"runtime_provider" gorm:"type:text;default:'docker'"`
}

func (n *Node) Validate() error {
	if n.Name == "" {
		return errors.New("name is required")
	}
	if n.FQDN == "" {
		return errors.New("fqdn is required")
	}
	return nil
}

// Backup is a server backup record.
type Backup struct {
	UUIDModel
	ServerID    string     `json:"server_id" gorm:"type:uuid;index;not null"`
	Adapter     string     `json:"adapter" gorm:"type:text"`
	Status      string     `json:"status" gorm:"type:text;not null;default:'pending'"`
	SizeBytes   int64      `json:"size_bytes" gorm:"default:0"`
	Checksum    string     `json:"checksum" gorm:"type:text"`
	CreatedAt   time.Time  `json:"created_at" gorm:"autoCreateTime;index"`
	CompletedAt *time.Time `json:"completed_at,omitempty" gorm:"index"`
}

func (b *Backup) Validate() error {
	if b.ServerID == "" {
		return errors.New("server_id is required")
	}
	return nil
}

// AuditEvent is a security-relevant action record.
type AuditEvent struct {
	BaseModel
	EventType      string            `json:"event_type" gorm:"type:text;index;not null"`
	ActorUserID    *uint64           `json:"actor_user_id,omitempty" gorm:"index"`
	TargetUserID   *uint64           `json:"target_user_id,omitempty" gorm:"index"`
	TargetServerID *string           `json:"target_server_id,omitempty" gorm:"type:uuid;index"`
	IP             string            `json:"ip" gorm:"type:text"`
	UserAgent      string            `json:"user_agent" gorm:"type:text"`
	Metadata       map[string]string `json:"metadata" gorm:"type:jsonb"`
}

// StringArray is a []string that serializes to a JSON array for storage and
// can also parse Postgres text[] literals of the form {a,b}. It implements
// driver.Valuer and sql.Scanner so it can be used with database/sql drivers
// without needing gorm.
type StringArray []string

// Value serializes the slice as a JSON array. A nil slice becomes a JSON null.
func (s StringArray) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal([]string(s))
}

// Scan accepts a JSON array (as bytes/string) or a Postgres text[] literal
// such as `{a,b}`. It never panics on unexpected input.
func (s *StringArray) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return errors.New("models: unsupported scan source for StringArray")
	}
	if len(raw) == 0 {
		*s = nil
		return nil
	}
	// Postgres array literal: {a,b}
	if raw[0] == '{' {
		trimmed := strings.Trim(string(raw), "{}")
		if trimmed == "" {
			*s = StringArray{}
			return nil
		}
		parts := strings.Split(trimmed, ",")
		out := make(StringArray, 0, len(parts))
		for _, p := range parts {
			out = append(out, unquoteArrayElement(p))
		}
		*s = out
		return nil
	}
	// JSON array fallback.
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return err
	}
	*s = StringArray(arr)
	return nil
}

func unquoteArrayElement(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// SourceType represents the origin of an application workload.
type SourceType string

const (
	SourceTypeGit         SourceType = "GIT"
	SourceTypeDockerImage SourceType = "DOCKER_IMAGE"
	SourceTypeCompose     SourceType = "COMPOSE"
)

// ServiceStatus represents the observed runtime condition.
type ServiceStatus string

const (
	ServiceStatusIdle      ServiceStatus = "idle"
	ServiceStatusRunning   ServiceStatus = "running"
	ServiceStatusDone      ServiceStatus = "done"
	ServiceStatusError     ServiceStatus = "error"
	ServiceStatusDeploying ServiceStatus = "deploying"
)

// AppDesiredState is the operator-intended state for an application.
type AppDesiredState string

const (
	AppDesiredStateRunning AppDesiredState = "running"
	AppDesiredStateStopped AppDesiredState = "stopped"
	AppDesiredStateRemoved AppDesiredState = "removed"
)
