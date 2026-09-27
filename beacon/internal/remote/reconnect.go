package remote

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// defaultOfflineTimeout is used when a caller does not configure one; a
	// zero timeout would make the offline ticker panic and the offline check
	// fire on every tick.
	defaultOfflineTimeout = 30 * time.Second
	heartbeatProbeInterval = 15 * time.Second
	minOfflineCheckInterval = time.Second
	panelProbeTimeout      = 10 * time.Second
	initialReconnectBackoff = time.Second
	maxReconnectBackoff     = 5 * time.Minute
	// circuitBreakerFailures caps how often the loop retries a panel that is
	// clearly down: after this many consecutive failed round-trips the backoff
	// is pinned at maxReconnectBackoff until something succeeds.
	circuitBreakerFailures = 10
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
	// Circuit breaker: consecutive failed round-trips. Reset on any success.
	consecutiveFails int
}

func NewReconnectClient(panelURL, token string, offlineTimeout time.Duration) *ReconnectClient {
	return &ReconnectClient{
		inner:          NewClient(panelURL, token),
		panelURL:       panelURL,
		token:          token,
		offlineTimeout: normalizeOfflineTimeout(offlineTimeout),
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
		inner: inner, offlineTimeout: normalizeOfflineTimeout(offlineTimeout),
		state: int32(StateDisconnected), stopCh: make(chan struct{}),
		stopped: make(chan struct{}), started: make(chan struct{}), newClient: reconnect,
	}
	return client
}

// normalizeOfflineTimeout keeps a misconfigured (zero, negative or absurdly
// small) timeout from panicking the offline ticker or busy-looping it.
func normalizeOfflineTimeout(value time.Duration) time.Duration {
	if value < minOfflineCheckInterval*2 {
		return defaultOfflineTimeout
	}
	return value
}

func (rc *ReconnectClient) offlineCheckInterval() time.Duration {
	interval := rc.offlineTimeout / 2
	if interval < minOfflineCheckInterval {
		return minOfflineCheckInterval
	}
	return interval
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
			// Real probe: only a successful panel round-trip refreshes the
			// heartbeat. A blind timer refresh would mask an outage and
			// prevent the offline detector below from ever firing.
			if err := rc.probe(ctx); err != nil {
				log.Printf("[reconnect] heartbeat probe failed: %v", err)
				continue
			}
			rc.mu.Lock()
			rc.lastHb = time.Now()
			rc.consecutiveFails = 0
			if rc.onHB != nil {
				rc.onHB()
			}
			rc.mu.Unlock()
			backoff = 1 * time.Second
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
				var ok bool
				backoff, ok = rc.doReconnect(ctx, backoff, maxBackoff)
				if ok {
					backoff = 1 * time.Second
				}
			}
		}
	}
}

// probe performs a cheap authenticated panel round-trip. Any error means the
// link is not healthy; only success may advance lastHb.
func (rc *ReconnectClient) probe(ctx context.Context) error {
	inner := rc.Inner()
	if inner == nil {
		return errors.New("no panel client")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := inner.GetServers(probeCtx, 1)
	return err
}

func (rc *ReconnectClient) doReconnect(ctx context.Context, backoff, maxBackoff time.Duration) (time.Duration, bool) {
	select {
	case <-time.After(backoff):
	case <-ctx.Done():
		return backoff, false
	case <-rc.stopCh:
		return backoff, false
	}

	candidate := rc.newClient()
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	_, err := candidate.GetServers(probeCtx, 1)
	cancel()
	if err != nil {
		rc.mu.Lock()
		rc.consecutiveFails++
		fails := rc.consecutiveFails
		rc.mu.Unlock()
		log.Printf("[reconnect] reconnect probe failed (consecutive %d): %v", fails, err)
		// Circuit breaker: after 10 consecutive failures, stop hammering and
		// cap at maxBackoff until a probe succeeds.
		nextBackoff := time.Duration(float64(backoff) * 2.0)
		if nextBackoff > maxBackoff {
			nextBackoff = maxBackoff
		}
		if fails >= 10 {
			nextBackoff = maxBackoff
		}
		jitter := secureDurationJitter(nextBackoff / 4)
		return nextBackoff - nextBackoff/8 + jitter, false
	}

	rc.mu.Lock()
	rc.inner = candidate
	rc.lastHb = time.Now()
	rc.consecutiveFails = 0
	rc.mu.Unlock()

	atomic.StoreInt32(&rc.state, int32(StateConnected))
	log.Printf("[reconnect] reconnected successfully")

	return 1 * time.Second, true
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
