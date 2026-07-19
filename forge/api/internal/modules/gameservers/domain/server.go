package domain

import "time"

type PowerSignal string

const (
	PowerStart   PowerSignal = "start"
	PowerStop    PowerSignal = "stop"
	PowerRestart PowerSignal = "restart"
	PowerKill    PowerSignal = "kill"
)

func (s PowerSignal) Valid() bool {
	return s == PowerStart || s == PowerStop || s == PowerRestart || s == PowerKill
}

type Server struct {
	ID                 string
	WorkloadID         string
	Name               string
	NodeID             string
	TemplateID         string
	OwnerID            string
	Suspended          bool
	DesiredState       string
	ObservedState      string
	DesiredGeneration  int64
	ObservedGeneration int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Allocation struct {
	ID, NodeID, IP string
	Port           int
	Protocol       string
	ServerID       string
}
type Template struct {
	ID, Name, Image, Startup string
	Variables                []Variable
}
type Variable struct {
	Key, Name, DefaultValue string
	Required                bool
	Secret                  bool
}
type Schedule struct {
	ID, ServerID, Name, CronExpression, Timezone string
	Enabled                                      bool
}
