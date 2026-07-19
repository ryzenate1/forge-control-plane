package store

import "context"

type UserRepository interface {
	FindByID(ctx context.Context, id string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter UserFilter) ([]User, error)
}

type ServerRepository interface {
	FindByID(ctx context.Context, id string) (*Server, error)
	Create(ctx context.Context, server *Server) error
	Update(ctx context.Context, server *Server) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter ServerFilter) ([]Server, error)
	FindByNode(ctx context.Context, nodeID string) ([]Server, error)
	FindByOwner(ctx context.Context, ownerID string) ([]Server, error)
}

type NodeRepository interface {
	FindByID(ctx context.Context, id string) (*Node, error)
	Create(ctx context.Context, node *Node) error
	Update(ctx context.Context, node *Node) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter NodeFilter) ([]Node, error)
	FindOnline(ctx context.Context) ([]Node, error)
}

type AllocationRepository interface {
	FindByID(ctx context.Context, id string) (*Allocation, error)
	Create(ctx context.Context, allocation *Allocation) error
	Delete(ctx context.Context, id string) error
	ListByNode(ctx context.Context, nodeID string) ([]Allocation, error)
	ListFree(ctx context.Context, nodeID string) ([]Allocation, error)
	AssignToServer(ctx context.Context, id, serverID string) error
}

type BackupRepository interface {
	FindByID(ctx context.Context, id string) (*Backup, error)
	Create(ctx context.Context, backup *Backup) error
	Update(ctx context.Context, backup *Backup) error
	Delete(ctx context.Context, id string) error
	ListByServer(ctx context.Context, serverID string) ([]Backup, error)
}

type DatabaseHostRepository interface {
	FindByID(ctx context.Context, id string) (*DatabaseHost, error)
	Create(ctx context.Context, host *DatabaseHost) error
	Update(ctx context.Context, host *DatabaseHost) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]DatabaseHost, error)
}

type ScheduleRepository interface {
	FindByID(ctx context.Context, id string) (*Schedule, error)
	Create(ctx context.Context, schedule *Schedule) error
	Update(ctx context.Context, schedule *Schedule) error
	Delete(ctx context.Context, id string) error
	ListByServer(ctx context.Context, serverID string) ([]Schedule, error)
	ListDue(ctx context.Context) ([]Schedule, error)
}

type EggRepository interface {
	FindByID(ctx context.Context, id string) (*Egg, error)
	Create(ctx context.Context, egg *Egg) error
	Update(ctx context.Context, egg *Egg) error
	Delete(ctx context.Context, id string) error
	ListByNest(ctx context.Context, nestID string) ([]Egg, error)
}

type NestRepository interface {
	FindByID(ctx context.Context, id string) (*Nest, error)
	Create(ctx context.Context, nest *Nest) error
	Update(ctx context.Context, nest *Nest) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context) ([]Nest, error)
}

type UserFilter struct {
	Email  *string
	Role   *string
	Limit  int
	Offset int
}

type ServerFilter struct {
	NodeID  *string
	OwnerID *string
	Status  *string
	Limit   int
	Offset  int
}

type NodeFilter struct {
	RegionID *string
	Status   *string
	Limit    int
	Offset   int
}
