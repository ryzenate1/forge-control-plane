package tenancy

import "time"

const (
	DefaultOrganizationID = "00000000-0000-0000-0000-000000000001"
	DefaultProjectID      = "00000000-0000-0000-0000-000000000002"
	DefaultEnvironmentID  = "00000000-0000-0000-0000-000000000003"
)

type Organization struct {
	ID, Name, Slug       string
	CreatedAt, UpdatedAt time.Time
}
type Project struct {
	ID, OrganizationID, Name, Slug string
	CreatedAt, UpdatedAt           time.Time
}
type Environment struct {
	ID, ProjectID, Name, Slug string
	Production                bool
	CreatedAt, UpdatedAt      time.Time
}

type Scope struct {
	OrganizationID string `json:"organizationId"`
	ProjectID      string `json:"projectId"`
	EnvironmentID  string `json:"environmentId"`
}

func DefaultScope() Scope {
	return Scope{OrganizationID: DefaultOrganizationID, ProjectID: DefaultProjectID, EnvironmentID: DefaultEnvironmentID}
}
