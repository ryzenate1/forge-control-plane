package store

type Allocation struct {
	ID            string  `json:"id"`
	Node          string  `json:"node"`
	Server        *string `json:"server,omitempty"`
	IP            string  `json:"ip"`
	Port          int     `json:"port"`
	ContainerPort int     `json:"containerPort"`
	Protocol      string  `json:"protocol"`
	Alias         *string `json:"alias,omitempty"`
	Notes         string  `json:"notes"`
}

type CreateAllocationRequest struct {
	NodeID        string
	IP            string
	Port          int
	ContainerPort int
	Protocol      string
	Alias         string
	Notes         string
}

type UpdateAllocationRequest struct {
	Alias string
	Notes string
}
