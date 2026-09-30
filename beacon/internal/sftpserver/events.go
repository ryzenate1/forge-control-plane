package sftpserver

import (
	"context"
	"fmt"
	"sync"
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
	db     *activity.Database
	queue  chan Event
	mu     sync.RWMutex
	closed bool
}

func NewActivityDBPublisher(db *activity.Database) *ActivityDBPublisher {
	publisher := &ActivityDBPublisher{db: db}
	if db != nil {
		publisher.queue = make(chan Event, 1024)
		go publisher.run()
	}
	return publisher
}

func (p *ActivityDBPublisher) PublishEvent(event Event) {
	if p == nil || p.db == nil || p.queue == nil {
		return
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return
	}
	select {
	case p.queue <- event:
	default:
		// SFTP I/O must never block on telemetry. The bounded queue prevents
		// unbounded goroutine growth when the activity database is unavailable.
	}
}

func (p *ActivityDBPublisher) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed && p.queue != nil {
		p.closed = true
		close(p.queue)
	}
}

func (p *ActivityDBPublisher) run() {
	for event := range p.queue {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = p.db.Record(ctx, activity.Activity{
			Event:     string(event.Action),
			User:      event.UserID,
			ServerID:  event.ServerID,
			IP:        event.IP,
			Timestamp: event.Timestamp,
			Metadata: map[string]interface{}{
				"path":      event.Path,
				"target":    event.Target,
				"client":    event.Client,
				"sessionId": event.SessionID,
			},
		})
		cancel()
	}
}
