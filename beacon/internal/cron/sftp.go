package cron

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"gamepanel/beacon/internal/remote"
)

type SFTPCron struct {
	manager  ServerManager
	maxBatch int
	mu       sync.Mutex
	running  bool
}

func NewSFTPCron(manager ServerManager, maxBatch int) *SFTPCron {
	if maxBatch <= 0 {
		maxBatch = 100
	}
	return &SFTPCron{
		manager:  manager,
		maxBatch: maxBatch,
	}
}

type sftpDedupKey struct {
	User      string
	Server    string
	IP        string
	Event     string
	Timestamp string
}

func (sc *SFTPCron) Run(ctx context.Context) error {
	sc.mu.Lock()
	if sc.running {
		sc.mu.Unlock()
		return nil
	}
	sc.running = true
	sc.mu.Unlock()
	defer func() {
		sc.mu.Lock()
		sc.running = false
		sc.mu.Unlock()
	}()

	events, err := sc.manager.GetActivityEvents(sc.maxBatch)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	var sftpEvents []ActivityEvent
	for _, e := range events {
		if strings.HasPrefix(e.Event, "server:sftp.") {
			sftpEvents = append(sftpEvents, e)
		}
	}
	if len(sftpEvents) == 0 {
		return nil
	}

	merged := make(map[sftpDedupKey]*mergedSFTP)
	var orderedKeys []sftpDedupKey

	for _, e := range sftpEvents {
		ip := e.IP
		if ip != "" && net.ParseIP(ip) == nil {
			ip = ""
		}
		key := sftpDedupKey{
			User:      e.User,
			Server:    e.Server,
			IP:        ip,
			Event:     e.Event,
			Timestamp: e.Timestamp.Truncate(time.Minute).UTC().Format(time.RFC3339),
		}
		if existing, ok := merged[key]; ok {
			existing.IDs = append(existing.IDs, e.ID)
			if files, ok := e.Metadata["files"].([]interface{}); ok {
				existing.Files = append(existing.Files, files...)
			}
		} else {
			m := &mergedSFTP{
				IDs:       []int{e.ID},
				Event:     e.Event,
				User:      e.User,
				Server:    e.Server,
				IP:        ip,
				Timestamp: key.Timestamp,
			}
			if files, ok := e.Metadata["files"].([]interface{}); ok {
				m.Files = append(m.Files, files...)
			}
			merged[key] = m
			orderedKeys = append(orderedKeys, key)
		}
	}

	activities := make([]remote.Activity, 0, len(orderedKeys))
	var allIDs []int
	for _, key := range orderedKeys {
		m := merged[key]
		metadata := map[string]interface{}{}
		if len(m.Files) > 0 {
			metadata["files"] = m.Files
		}
		activities = append(activities, remote.Activity{
			Event:     m.Event,
			User:      m.User,
			Server:    m.Server,
			IP:        m.IP,
			Timestamp: m.Timestamp,
			Metadata:  metadata,
		})
		allIDs = append(allIDs, m.IDs...)
	}

	client := sc.manager.GetPanelClient()
	if client == nil {
		return nil
	}

	if err := client.SendActivityLogs(ctx, activities); err != nil {
		return err
	}

	return sc.deleteInChunks(allIDs)
}

func (sc *SFTPCron) deleteInChunks(ids []int) error {
	const chunkSize = 32000
	for i := 0; i < len(ids); i += chunkSize {
		end := i + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := sc.manager.DeleteActivityEvents(ids[i:end]); err != nil {
			return err
		}
	}
	return nil
}

type mergedSFTP struct {
	IDs       []int
	Event     string
	User      string
	Server    string
	IP        string
	Timestamp string
	Files     []interface{}
}
