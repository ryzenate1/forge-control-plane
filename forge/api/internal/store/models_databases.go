package store

type ServerDatabase struct {
	ID                string  `json:"id"`
	DatabaseName      string  `json:"database"`
	Username          string  `json:"username"`
	Remote            string  `json:"remote"`
	Engine            string  `json:"engine"`
	Host              string  `json:"host"`
	Port              int     `json:"port"`
	MaxConnections    *int    `json:"maxConnections,omitempty"`
	ProvisioningState string  `json:"provisioningState"`
	ProvisioningError string  `json:"provisioningError,omitempty"`
	Password          *string `json:"password,omitempty"`
}

type DatabaseHost struct {
	ID            string   `json:"id"`
	NodeID        string   `json:"nodeId,omitempty"`
	NodeIDs       []string `json:"nodeIds,omitempty"`
	NodeName      string   `json:"nodeName,omitempty"`
	NodeNames     []string `json:"nodeNames,omitempty"`
	Engine        string   `json:"engine"`
	Name          string   `json:"name"`
	Host          string   `json:"host"`
	Port          int      `json:"port"`
	Username      string   `json:"username"`
	TLSMode       string   `json:"tlsMode"`
	TLSCA         string   `json:"-"`
	TLSServerName string   `json:"tlsServerName,omitempty"`
	MaxDatabases  *int     `json:"maxDatabases,omitempty"`
	Databases     int      `json:"databases"`
}

type CreateDatabaseHostRequest struct {
	NodeID        string
	NodeIDs       []string
	Engine        string
	Name          string
	Host          string
	Port          int
	Username      string
	Password      string
	TLSMode       string
	TLSCA         string
	TLSServerName string
	MaxDatabases  *int
}

type UpdateDatabaseHostRequest struct {
	NodeID   string
	NodeIDs  []string
	Engine   string
	Name     string
	Host     string
	Port     int
	Username string
	Password string
	TLSMode  string
	// TLSCA is nil when an update omits tlsCa, preserving the configured CA.
	// An empty string explicitly clears it.
	TLSCA         *string
	TLSServerName string
	MaxDatabases  *int
}

type CreateServerDatabaseRequest struct {
	Database       string
	Remote         string
	MaxConnections *int
}
