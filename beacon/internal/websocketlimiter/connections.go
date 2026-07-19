package websocketlimiter

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
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

func (cm *ConnectionManager) CanConnect(serverID string) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.conns[serverID] < cm.maxPerServer
}

func (cm *ConnectionManager) Connected(serverID string) {
	cm.mu.Lock()
	cm.conns[serverID]++
	cm.mu.Unlock()
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

type GlobalRateLimiter struct {
	limiter *rate.Limiter
}

func NewGlobalRateLimiter() *GlobalRateLimiter {
	return &GlobalRateLimiter{
		limiter: rate.NewLimiter(rate.Every(200*time.Millisecond), 10),
	}
}

func (g *GlobalRateLimiter) Allow() bool {
	return g.limiter.Allow()
}
