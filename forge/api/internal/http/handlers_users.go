package http

import "gamepanel/forge/internal/store"

// PublicUser is a sanitized representation of store.User safe for JSON
// responses. It intentionally omits sensitive fields such as the TOTP
// secret (and any future password hash / API key hash fields) that must
// never be echoed back to API clients, even admins.
//
// NOTE: this file does not exist elsewhere in the codebase; the user
// endpoints that previously serialized store.User directly live in
// handlers_servers.go (GET/POST /users, PATCH /users/:id). They have been
// updated to use ToPublicUser/ToPublicUsers below.
type PublicUser struct {
	ID              string `json:"id"`
	ExternalID      string `json:"externalId"`
	Email           string `json:"email"`
	Username        string `json:"username"`
	NameFirst       string `json:"nameFirst"`
	NameLast        string `json:"nameLast"`
	Role            string `json:"role"`
	UseTOTP         bool   `json:"useTotp"`
	RootAdmin       bool   `json:"rootAdmin"`
	Language        string `json:"language"`
	CPULimit        int    `json:"cpuLimit"`
	MemoryMBLimit   int    `json:"memoryMbLimit"`
	DiskMBLimit     int    `json:"diskMbLimit"`
	BackupLimit     int    `json:"backupLimit"`
	DatabaseLimit   int    `json:"databaseLimit"`
	AllocationLimit int    `json:"allocationLimit"`
	SubuserLimit    int    `json:"subuserLimit"`
	ScheduleLimit   int    `json:"scheduleLimit"`
	ServerLimit     int    `json:"serverLimit"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	Disabled        bool   `json:"disabled"`
}

// ToPublicUser strips sensitive fields (e.g. TOTPSecret) from a store.User
// before it is returned in an API response.
func ToPublicUser(u store.User) PublicUser {
	return PublicUser{
		ID:              u.ID,
		ExternalID:      u.ExternalID,
		Email:           u.Email,
		Username:        u.Username,
		NameFirst:       u.NameFirst,
		NameLast:        u.NameLast,
		Role:            u.Role,
		UseTOTP:         u.UseTOTP,
		RootAdmin:       u.RootAdmin,
		Language:        u.Language,
		CPULimit:        u.CPULimit,
		MemoryMBLimit:   u.MemoryMBLimit,
		DiskMBLimit:     u.DiskMBLimit,
		BackupLimit:     u.BackupLimit,
		DatabaseLimit:   u.DatabaseLimit,
		AllocationLimit: u.AllocationLimit,
		SubuserLimit:    u.SubuserLimit,
		ScheduleLimit:   u.ScheduleLimit,
		ServerLimit:     u.ServerLimit,
		CreatedAt:       u.CreatedAt,
		UpdatedAt:       u.UpdatedAt,
		Disabled:        u.Disabled,
	}
}

// ToPublicUsers maps a slice of store.User to their sanitized DTO form.
func ToPublicUsers(users []store.User) []PublicUser {
	out := make([]PublicUser, 0, len(users))
	for _, u := range users {
		out = append(out, ToPublicUser(u))
	}
	return out
}
