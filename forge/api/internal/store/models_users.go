package store

import (
	"time"
)

type User struct {
	ID              string  `json:"id"`
	ExternalID      string  `json:"externalId"`
	Email           string  `json:"email"`
	Username        string  `json:"username"`
	NameFirst       string  `json:"nameFirst"`
	NameLast        string  `json:"nameLast"`
	Role            string  `json:"role"`
	UseTOTP         bool    `json:"useTotp"`
	TOTPSecret      *string `json:"totpSecret,omitempty"`
	RootAdmin       bool    `json:"rootAdmin"`
	Language        string  `json:"language"`
	CPULimit        int     `json:"cpuLimit"`
	MemoryMBLimit   int     `json:"memoryMbLimit"`
	DiskMBLimit     int     `json:"diskMbLimit"`
	BackupLimit     int     `json:"backupLimit"`
	DatabaseLimit   int     `json:"databaseLimit"`
	AllocationLimit int     `json:"allocationLimit"`
	SubuserLimit    int     `json:"subuserLimit"`
	ScheduleLimit   int     `json:"scheduleLimit"`
	ServerLimit     int     `json:"serverLimit"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
	SessionVersion  int64   `json:"-"`
	Disabled        bool    `json:"disabled"`
}

type ServerSubuser struct {
	ID          string    `json:"id"`
	ServerID    string    `json:"serverId"`
	UserID      string    `json:"userId"`
	Email       string    `json:"email"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type UpsertServerSubuserRequest struct {
	Email       string
	Permissions []string
}

type SubuserInvitation struct {
	ID          string     `json:"id"`
	ServerID    string     `json:"serverId"`
	Email       string     `json:"email"`
	Permissions []string   `json:"permissions"`
	Token       string     `json:"token"`
	CreatedBy   *string    `json:"createdBy,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	AcceptedAt  *time.Time `json:"acceptedAt,omitempty"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
}

type CreateSubuserInvitationRequest struct {
	Email       string   `json:"email"`
	Permissions []string `json:"permissions"`
}

type SFTPAuthResult struct {
	UserID      string   `json:"user"`
	ServerID    string   `json:"server"`
	Permissions []string `json:"permissions"`
	DiskLimitMB int64    `json:"diskLimitMb"`
	Suspended   bool     `json:"suspended"`
	ReadOnly    bool     `json:"readOnly"`
}

type CreateUserRequest struct {
	Email           string
	Password        string
	Role            string
	CPULimit        int
	MemoryMBLimit   int
	DiskMBLimit     int
	BackupLimit     int
	DatabaseLimit   int
	AllocationLimit int
	SubuserLimit    int
	ScheduleLimit   int
	ServerLimit     int
}

type UpdateUserRequest struct {
	Email           string
	Password        string
	Role            string
	CPULimit        *int
	MemoryMBLimit   *int
	DiskMBLimit     *int
	BackupLimit     *int
	DatabaseLimit   *int
	AllocationLimit *int
	SubuserLimit    *int
	ScheduleLimit   *int
	ServerLimit     *int
}
