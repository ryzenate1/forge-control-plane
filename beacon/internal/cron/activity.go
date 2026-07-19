package cron

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"gamepanel/beacon/internal/remote"
)

type ServerManager interface {
	GetPanelClient() remote.Client
	GetActivityEvents(limit int) ([]ActivityEvent, error)
	DeleteActivityEvents(ids []int) error
}

type ActivityEvent struct {
	ID        int
	User      string
	Server    string
	Event     string
	Metadata  map[string]interface{}
	IP        string
	Timestamp time.Time
}

type ActivityCron struct {
	manager  ServerManager
	maxBatch int
	mu       sync.Mutex
	running  bool
}

func NewActivityCron(manager ServerManager, maxBatch int) *ActivityCron {
	if maxBatch <= 0 {
		maxBatch = 100
	}
	return &ActivityCron{
		manager:  manager,
		maxBatch: maxBatch,
	}
}

func (ac *ActivityCron) Run(ctx context.Context) error {
	ac.mu.Lock()
	if ac.running {
		ac.mu.Unlock()
		return nil
	}
	ac.running = true
	ac.mu.Unlock()
	defer func() {
		ac.mu.Lock()
		ac.running = false
		ac.mu.Unlock()
	}()

	events, err := ac.manager.GetActivityEvents(ac.maxBatch)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	var filtered []ActivityEvent
	for _, e := range events {
		if strings.HasPrefix(e.Event, "server:sftp.") {
			continue
		}
		filtered = append(filtered, e)
	}
	if len(filtered) == 0 {
		return nil
	}

	activities := make([]remote.Activity, 0, len(filtered))
	var ids []int
	for _, e := range filtered {
		ip := e.IP
		if ip != "" && net.ParseIP(ip) == nil {
			ip = ""
		}
		activities = append(activities, remote.Activity{
			Event:     e.Event,
			User:      e.User,
			Server:    e.Server,
			IP:        ip,
			Timestamp: e.Timestamp.UTC().Format(time.RFC3339),
			Metadata:  e.Metadata,
		})
		ids = append(ids, e.ID)
	}

	client := ac.manager.GetPanelClient()
	if client == nil {
		return nil
	}

	if err := client.SendActivityLogs(ctx, activities); err != nil {
		return err
	}

	return ac.manager.DeleteActivityEvents(ids)
}
