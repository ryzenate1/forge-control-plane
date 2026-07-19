package store

type Mount struct {
	ID            string   `json:"id"`
	UUID          string   `json:"uuid"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Source        string   `json:"source"`
	Target        string   `json:"target"`
	ReadOnly      bool     `json:"readOnly"`
	UserMountable bool     `json:"userMountable"`
	NodeIDs       []string `json:"nodeIds"`
	TemplateIDs   []string `json:"templateIds"`
	ServerIDs     []string `json:"serverIds"`
}

type CreateMountRequest struct {
	Name          string
	Description   string
	Source        string
	Target        string
	ReadOnly      bool
	UserMountable bool
	NodeIDs       []string
	TemplateIDs   []string
}

type ServerMount struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Source        string `json:"source"`
	Target        string `json:"target"`
	ReadOnly      bool   `json:"read_only"`
	UserMountable bool   `json:"user_mountable"`
}
