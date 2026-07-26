package sftpserver

import (
	"fmt"
	"time"

	"gamepanel/beacon/internal/activity"
)

// Action represents an SFTP action type.
type Action string

const (
	ActionFileRead   Action = "sftp.file.read"
	ActionFileWrite  Action = "sftp.file.write"
	ActionFileCreate Action = "sftp.file.create"
	ActionFileDelete Action = "sftp.file.delete"
	ActionFileMkdir  Action = "sftp.file.mkdir"
	ActionFileRename Action = "sftp.file.rename"
	ActionFileChmod  Action = "sftp.file.chmod"
	ActionFileList   Action = "sftp.file.list"
	ActionFileStat   Action = "sftp.file.stat"
)

// Event represents a structured SFTP event.
type Event struct {
	Action    Action    `json:"action"`
	Path      string    `json:"path"`
	Target    string    `json:"target,omitempty"`
	UserID    string    `json:"userId"`
	ServerID  string    `json:"serverId"`
	IP        string    `json:"ip"`
	Client    string    `json:"client,omitempty"`
	SessionID string    `json:"sessionId"`
	Timestamp time.Time `json:"timestamp"`
}

func (e Event) String() string {
	return fmt.Sprintf("%s %s by %s on %s", e.Action, e.Path, e.UserID, e.ServerID)
}

// Publisher defines an interface for publishing SFTP events.
type Publisher interface {
	PublishEvent(event Event)
}

// PublisherFunc is an adapter to allow ordinary functions as Publisher.
type PublisherFunc func(Event)

func (f PublisherFunc) PublishEvent(event Event) {
	if f != nil {
		f(event)
	}
}

// MultiPublisher fans out events to multiple publishers.
type MultiPublisher struct {
	publishers []Publisher
}

func NewMultiPublisher(publishers ...Publisher) *MultiPublisher {
	return &MultiPublisher{publishers: publishers}
}

func (m *MultiPublisher) PublishEvent(event Event) {
	for _, p := range m.publishers {
		p.PublishEvent(event)
	}
}

// ActivityDBPublisher publishes events to the activity database.
type ActivityDBPublisher struct {
	db *activity.Database
}

func NewActivityDBPublisher(db *activity.Database) *ActivityDBPublisher {
	return &ActivityDBPublisher{db: db}
}

func (p *ActivityDBPublisher) PublishEvent(event Event) {
	if p == nil || p.db == nil {
		return
	}
	_ = p.db.Record(nil, activity.Activity{
		Event:    string(event.Action),
		User:     event.UserID,
		ServerID: event.ServerID,
		IP:       event.IP,
		Timestamp: event.Timestamp,
		Metadata: map[string]interface{}{
			"path":      event.Path,
			"target":    event.Target,
			"client":    event.Client,
			"sessionId": event.SessionID,
		},
	})
}
