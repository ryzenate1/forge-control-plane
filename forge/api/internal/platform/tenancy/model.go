package tenancy

import "time"

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
	return Scope{OrganizationID: "default", ProjectID: "default", EnvironmentID: "default"}
}
