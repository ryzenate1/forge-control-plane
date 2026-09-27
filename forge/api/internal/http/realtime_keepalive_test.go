package http

import (
	"context"
	"sync"
	"testing"
	"time"

	gorilla "github.com/gorilla/websocket"
)

// recordingPinger stands in for a WebSocket connection and records every
// control frame written to it.
type recordingPinger struct {
	mu     sync.Mutex
	frames []int
	err    error
	wrote  chan struct{}
}

func newRecordingPinger() *recordingPinger {
	return &recordingPinger{wrote: make(chan struct{}, 8)}
}

func (p *recordingPinger) WriteControl(messageType int, _ []byte, _ time.Time) error {
	p.mu.Lock()
	if p.err != nil {
		err := p.err
		p.mu.Unlock()
		return err
	}
	p.frames = append(p.frames, messageType)
	p.mu.Unlock()
	select {
	case p.wrote <- struct{}{}:
	default:
	}
	return nil
}

func (p *recordingPinger) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.frames)
}

func (p *recordingPinger) pings() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, frame := range p.frames {
		if frame == gorilla.PingMessage {
			n++
		}
	}
	return n
}

func (p *recordingPinger) failWith(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err = err
}

func waitForWrite(t *testing.T, p *recordingPinger) {
	t.Helper()
	select {
	case <-p.wrote:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a control frame")
	}
}

// TestPumpKeepalivePingsClientNotOnlyUpstream is the regression test for the
// keepalive that pinged upstream alone. Both sockets enforce a read deadline
// that only an inbound pong extends, and a browser answers pings but never
// sends them, so an unpinged client socket is closed after one read-deadline
// interval of user inactivity while the workload is still streaming.
func TestPumpKeepalivePingsClientNotOnlyUpstream(t *testing.T) {
	upstream := newRecordingPinger()
	client := newRecordingPinger()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		pumpKeepalive(ctx, tick, upstream, client)
		close(done)
	}()

	tick <- time.Now()
	waitForWrite(t, upstream)
	waitForWrite(t, client)

	if got := upstream.pings(); got != 1 {
		t.Fatalf("upstream pings = %d, want 1", got)
	}
	if got := client.pings(); got != 1 {
		t.Fatalf("client pings = %d, want 1 (client-bound ping missing)", got)
	}

	tick <- time.Now()
	waitForWrite(t, upstream)
	waitForWrite(t, client)

	if got := client.pings(); got != 2 {
		t.Fatalf("client pings after second tick = %d, want 2", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pumpKeepalive did not stop on context cancellation")
	}
}

// TestPumpKeepaliveStopsOnWriteFailure asserts the loop exits once a peer's
// socket is gone rather than spinning against a dead connection.
func TestPumpKeepaliveStopsOnWriteFailure(t *testing.T) {
	upstream := newRecordingPinger()
	client := newRecordingPinger()
	client.failWith(gorilla.ErrCloseSent)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		pumpKeepalive(ctx, tick, upstream, client)
		close(done)
	}()

	tick <- time.Now()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pumpKeepalive kept running after a peer write failed")
	}

	if got := client.count(); got != 0 {
		t.Fatalf("client recorded %d frames, want 0 after a failing write", got)
	}
}
