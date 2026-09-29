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
	defaultOfflineTimeout   = 30 * time.Second
	heartbeatProbeInterval  = 15 * time.Second
	minOfflineCheckInterval = time.Second
	panelProbeTimeout       = 10 * time.Second
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

	state   int32
	stopCh  chan struct{}
	stopped chan struct{}
	// stopCtx is done once stopCh closes, so context.AfterFunc can bound an
	// in-flight probe by shutdown without spawning a watcher per round-trip.
	stopCtx    context.Context
	stopCancel context.CancelFunc
	mu         sync.Mutex
	lastHb     time.Time
	onHB       func()
	attempts   int64
	startOnce  sync.Once
	stopOnce   sync.Once
	started    chan struct{}
	newClient  func() Client
	// Circuit breaker: consecutive failed round-trips. Reset on any success.
	consecutiveFails int
}

func NewReconnectClient(panelURL, token string, offlineTimeout time.Duration) *ReconnectClient {
	stopCtx, stopCancel := context.WithCancel(context.Background())
	return &ReconnectClient{
		inner:          NewClient(panelURL, token),
		panelURL:       panelURL,
		token:          token,
		offlineTimeout: normalizeOfflineTimeout(offlineTimeout),
		state:          int32(StateDisconnected),
		stopCh:         make(chan struct{}),
		stopped:        make(chan struct{}),
		stopCtx:        stopCtx,
		stopCancel:     stopCancel,
		started:        make(chan struct{}),
		newClient:      func() Client { return NewClient(panelURL, token) },
	}
}

func NewReconnectClientWithClient(inner Client, reconnect func() Client, offlineTimeout time.Duration) *ReconnectClient {
	if reconnect == nil {
		reconnect = func() Client { return inner }
	}
	stopCtx, stopCancel := context.WithCancel(context.Background())
	client := &ReconnectClient{
		inner: inner, offlineTimeout: normalizeOfflineTimeout(offlineTimeout),
		state: int32(StateDisconnected), stopCh: make(chan struct{}),
		stopped: make(chan struct{}), stopCtx: stopCtx, stopCancel: stopCancel,
		started: make(chan struct{}), newClient: reconnect,
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

	ticker := time.NewTicker(heartbeatProbeInterval)
	defer ticker.Stop()
	offlineCheck := time.NewTicker(rc.offlineCheckInterval())
	defer offlineCheck.Stop()

	backoff := initialReconnectBackoff

	// The channel is only healthy once a round-trip has proved it. Marking
	// StateConnected before the first probe would let a node that cannot reach
	// the panel advertise itself as healthy.
	atomic.StoreInt32(&rc.state, int32(StateConnecting))
	if err := rc.probe(ctx); err != nil {
		atomic.StoreInt32(&rc.state, int32(StateDisconnected))
		log.Print("[reconnect] panel unreachable at startup, retrying in the background")
	} else {
		rc.markConnected()
	}

	for {
		select {
		case <-ctx.Done():
			atomic.StoreInt32(&rc.state, int32(StateDisconnected))
			return
		case <-rc.stopCh:
			atomic.StoreInt32(&rc.state, int32(StateDisconnected))
			return
		case <-ticker.C:
			if err := rc.probe(ctx); err != nil {
				// One failed round-trip is a blip, not an outage: the heartbeat
				// clock simply stops advancing and the offline detector below
				// decides when the link is really gone.
				continue
			}
			rc.markConnected()
			backoff = initialReconnectBackoff
		case <-offlineCheck.C:
			if !rc.linkExpired() {
				continue
			}
			if ConnState(atomic.LoadInt32(&rc.state)) != StateReconnecting {
				atomic.StoreInt32(&rc.state, int32(StateReconnecting))
				log.Printf("[reconnect] no panel round-trip for %v, reconnecting", rc.offlineTimeout)
			}
			var ok bool
			backoff, ok = rc.doReconnect(ctx, backoff)
			if ok {
				backoff = initialReconnectBackoff
			}
		}
	}
}

// probe performs one cheap authenticated panel round-trip through the current
// inner client. Any error means the link is not provably healthy; only success
// may advance the heartbeat clock.
func (rc *ReconnectClient) probe(ctx context.Context) error {
	probeCtx, cancel := rc.probeContext(ctx, panelProbeTimeout)
	defer cancel()
	return rc.roundTrip(probeCtx, rc.Inner())
}

// roundTrip executes a probe against a specific client and keeps the failure
// bookkeeping, so an outage is reported on transition rather than on every tick.
func (rc *ReconnectClient) roundTrip(ctx context.Context, c Client) error {
	if c == nil {
		return errors.New("no panel client")
	}
	_, err := c.GetServers(ctx, 1)
	if err == nil {
		return nil
	}
	atomic.AddInt64(&rc.attempts, 1)
	rc.mu.Lock()
	rc.consecutiveFails++
	fails := rc.consecutiveFails
	rc.mu.Unlock()
	if fails == 1 || fails%circuitBreakerFailures == 0 {
		log.Printf("[reconnect] panel round-trip failed (%d consecutive): %v", fails, err)
	}
	return err
}

// markConnected records a proven-healthy link: the attempt counters reset and a
// link that had been down says so once, rather than on every subsequent beat.
func (rc *ReconnectClient) markConnected() {
	rc.mu.Lock()
	recovered := rc.consecutiveFails
	rc.consecutiveFails = 0
	rc.lastHb = time.Now()
	onHB := rc.onHB
	rc.mu.Unlock()

	atomic.StoreInt32(&rc.state, int32(StateConnected))
	if recovered > 0 {
		log.Printf("[reconnect] panel reachable again after %d failed round-trip(s)", recovered)
	}
	if onHB != nil {
		onHB()
	}
}

// linkExpired reports whether the last proven round-trip is older than the
// offline timeout. A link that never succeeded has no reading to age: unknown
// is not connected.
func (rc *ReconnectClient) linkExpired() bool {
	rc.mu.Lock()
	last := rc.lastHb
	rc.mu.Unlock()
	return last.IsZero() || time.Since(last) > rc.offlineTimeout
}

// probeContext bounds a round-trip by the parent context, the probe timeout and
// Stop(), so no probe can outlive shutdown.
//
// Shutdown cancels stopCtx alongside closing stopCh, so context.AfterFunc can
// watch shutdown directly instead of spawning a watcher goroutine per probe.
// The returned CancelFunc stops that watch as well as the timeout, and is safe
// to call more than once.
func (rc *ReconnectClient) probeContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	stopWatch := context.AfterFunc(rc.stopCtx, cancel)
	return probeCtx, func() {
		stopWatch()
		cancel()
	}
}

// doReconnect waits out the current backoff, then proves a freshly built client
// can round-trip before promoting it. It returns the next backoff and whether
// the link is healthy again.
func (rc *ReconnectClient) doReconnect(ctx context.Context, backoff time.Duration) (time.Duration, bool) {
	if !waitOrCancelled(ctx, rc.stopCh, backoff) {
		return backoff, false
	}

	candidate := rc.newClient()
	probeCtx, cancel := rc.probeContext(ctx, panelProbeTimeout)
	err := rc.roundTrip(probeCtx, candidate)
	cancel()
	if err != nil {
		rc.mu.Lock()
		fails := rc.consecutiveFails
		rc.mu.Unlock()
		next := backoff * 2
		// Circuit breaker: past the threshold, stop hammering a panel that is
		// clearly down and hold at the cap until a round-trip succeeds.
		if fails >= circuitBreakerFailures || next > maxReconnectBackoff {
			next = maxReconnectBackoff
		}
		jitter := secureDurationJitter(next / 4)
		return next - next/8 + jitter, false
	}

	rc.mu.Lock()
	rc.inner = candidate
	rc.mu.Unlock()
	rc.markConnected()
	return initialReconnectBackoff, true
}

// waitOrCancelled sleeps for d unless ctx is cancelled or stopCh closes first.
// It reports whether the full delay elapsed.
func waitOrCancelled(ctx context.Context, stopCh <-chan struct{}, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	case <-stopCh:
		return false
	}
}

func (rc *ReconnectClient) Stop() {
	rc.stopOnce.Do(func() {
		close(rc.stopCh)
		rc.stopCancel()
	})
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
