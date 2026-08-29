package websocketlimiter

import (
	"sync"
)

type ConnectionManager struct {
	mu           sync.Mutex
	conns        map[string]int
	maxPerServer int
}

func NewConnectionManager(maxPerServer int) *ConnectionManager {
	if maxPerServer <= 0 {
		maxPerServer = 30
	}
	return &ConnectionManager{
		conns:        make(map[string]int),
		maxPerServer: maxPerServer,
	}
}

// Acquire atomically checks and reserves a connection slot.
func (cm *ConnectionManager) Acquire(serverID string) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.conns[serverID] >= cm.maxPerServer {
		return false
	}
	cm.conns[serverID]++
	return true
}

func (cm *ConnectionManager) Disconnected(serverID string) {
	cm.mu.Lock()
	if cm.conns[serverID] > 0 {
		cm.conns[serverID]--
	}
	if cm.conns[serverID] == 0 {
		delete(cm.conns, serverID)
	}
	cm.mu.Unlock()
}

func (cm *ConnectionManager) Count(serverID string) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.conns[serverID]
}
