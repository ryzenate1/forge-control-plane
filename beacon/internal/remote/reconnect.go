package remote

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type ConnState int32

const (
	StateDisconnected ConnState = iota
	StateConnecting
	StateConnected
	StateReconnecting
)

func (s ConnState) String() string {
	switch s {
	case StateDisconnected:
		return "disconnected"
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	default:
		return "unknown"
	}
}

type ReconnectClient struct {
	inner          Client
	panelURL       string
	token          string
	offlineTimeout time.Duration

	state     int32
	stopCh    chan struct{}
	stopped   chan struct{}
	mu        sync.Mutex
	lastHb    time.Time
	onHB      func()
	attempts  int64
	startOnce sync.Once
	stopOnce  sync.Once
	started   chan struct{}
	newClient func() Client
}

func NewReconnectClient(panelURL, token string, offlineTimeout time.Duration) *ReconnectClient {
	return &ReconnectClient{
		inner:          NewClient(panelURL, token),
		panelURL:       panelURL,
		token:          token,
		offlineTimeout: offlineTimeout,
		state:          int32(StateDisconnected),
		stopCh:         make(chan struct{}),
		stopped:        make(chan struct{}),
		started:        make(chan struct{}),
		newClient:      func() Client { return NewClient(panelURL, token) },
	}
}

func NewReconnectClientWithClient(inner Client, reconnect func() Client, offlineTimeout time.Duration) *ReconnectClient {
	if reconnect == nil {
		reconnect = func() Client { return inner }
	}
	client := &ReconnectClient{
		inner: inner, offlineTimeout: offlineTimeout,
		state: int32(StateDisconnected), stopCh: make(chan struct{}),
		stopped: make(chan struct{}), started: make(chan struct{}), newClient: reconnect,
	}
	return client
}

func (rc *ReconnectClient) Inner() Client {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.inner == nil {
		rc.inner = rc.newClient()
	}
	return rc.inner
}

func (rc *ReconnectClient) Start(ctx context.Context) {
	rc.startOnce.Do(func() {
		close(rc.started)
		rc.run(ctx)
	})
}

func (rc *ReconnectClient) run(ctx context.Context) {
	defer close(rc.stopped)
	atomic.StoreInt32(&rc.state, int32(StateConnected))
	rc.mu.Lock()
	rc.lastHb = time.Now()
	rc.mu.Unlock()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	offlineCheck := time.NewTicker(rc.offlineTimeout / 2)
	defer offlineCheck.Stop()

	backoff := 1 * time.Second
	maxBackoff := 5 * time.Minute

	for {
		select {
		case <-ctx.Done():
			atomic.StoreInt32(&rc.state, int32(StateDisconnected))
			return
		case <-rc.stopCh:
			atomic.StoreInt32(&rc.state, int32(StateDisconnected))
			return
		case <-ticker.C:
			rc.mu.Lock()
			rc.lastHb = time.Now()
			if rc.onHB != nil {
				rc.onHB()
			}
			rc.mu.Unlock()
		case <-offlineCheck.C:
			rc.mu.Lock()
			last := rc.lastHb
			rc.mu.Unlock()
			if !last.IsZero() && time.Since(last) > rc.offlineTimeout {
				cur := ConnState(atomic.LoadInt32(&rc.state))
				if cur == StateConnected || cur == StateReconnecting {
					atomic.StoreInt32(&rc.state, int32(StateReconnecting))
					atomic.AddInt64(&rc.attempts, 1)
					log.Printf("[reconnect] offline detected, reconnecting (attempt %d)...", atomic.LoadInt64(&rc.attempts))
				}
				backoff = rc.doReconnect(ctx, backoff, maxBackoff)
			}
		}
	}
}

func (rc *ReconnectClient) doReconnect(ctx context.Context, backoff, maxBackoff time.Duration) time.Duration {
	select {
	case <-time.After(backoff):
	case <-ctx.Done():
		return backoff
	case <-rc.stopCh:
		return backoff
	}

	rc.mu.Lock()
	rc.inner = rc.newClient()
	rc.lastHb = time.Now()
	rc.mu.Unlock()

	atomic.StoreInt32(&rc.state, int32(StateConnected))
	log.Printf("[reconnect] reconnected successfully")

	nextBackoff := time.Duration(float64(backoff) * 2.0)
	if nextBackoff > maxBackoff {
		nextBackoff = maxBackoff
	}
	jitter := secureDurationJitter(nextBackoff / 4)
	return nextBackoff - nextBackoff/8 + jitter
}

func (rc *ReconnectClient) Stop() {
	rc.stopOnce.Do(func() { close(rc.stopCh) })
	select {
	case <-rc.started:
		select {
		case <-rc.stopped:
		case <-time.After(5 * time.Second):
		}
	default:
	}
}

func secureDurationJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	var body [8]byte
	if _, err := rand.Read(body[:]); err != nil {
		return 0
	}
	return time.Duration(binary.LittleEndian.Uint64(body[:]) % uint64(max))
}

func (rc *ReconnectClient) State() ConnState {
	return ConnState(atomic.LoadInt32(&rc.state))
}

func (rc *ReconnectClient) SetOnHeartbeat(fn func()) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.onHB = fn
}

func (rc *ReconnectClient) Stats() map[string]any {
	return map[string]any{
		"state":            rc.State().String(),
		"attempts":         atomic.LoadInt64(&rc.attempts),
		"offlineTimeoutMs": rc.offlineTimeout.Milliseconds(),
	}
}
