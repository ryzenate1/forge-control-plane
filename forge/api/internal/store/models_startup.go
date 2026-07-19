package store

type StartupVariable struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	EnvVariable  string `json:"env_variable"`
	DefaultValue string `json:"default_value"`
	ServerValue  string `json:"server_value"`
	IsEditable   bool   `json:"is_editable"`
	Rules        string `json:"rules"`
}

type StartupDetails struct {
	StartupCommand    string            `json:"startup_command"`
	RawStartupCommand string            `json:"raw_startup_command"`
	DockerImages      map[string]string `json:"docker_images"`
	Variables         []StartupVariable `json:"variables"`
}
