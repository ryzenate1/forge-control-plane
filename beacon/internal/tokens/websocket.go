package tokens

import (
	"sync"
	"time"
)

var bootTime = time.Now()

type WebSocketDenylist struct {
	mu       sync.RWMutex
	byServer map[string]map[string]bool
}

func NewWebSocketDenylist() *WebSocketDenylist {
	return &WebSocketDenylist{
		byServer: make(map[string]map[string]bool),
	}
}

func (d *WebSocketDenylist) DenyForServer(serverID, userID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.byServer[serverID] == nil {
		d.byServer[serverID] = make(map[string]bool)
	}
	d.byServer[serverID][userID] = true
}

func (d *WebSocketDenylist) IsDenied(serverID, userID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	users, exists := d.byServer[serverID]
	if !exists {
		return false
	}
	return users[userID]
}

func IsBeforeBoot(issuedAt time.Time) bool {
	return issuedAt.Before(bootTime)
}
