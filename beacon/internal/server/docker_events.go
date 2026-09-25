package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

// Real-time Docker events feed — node side.
//
// Beacon tails the local Docker event stream, keeps only container lifecycle
// actions, and pushes them in batches to the panel's /api/remote/docker/events
// ingest endpoint, signed with the same node credential and HMAC scheme as the
// heartbeat and the health-report forward.
//
// Delivery is best effort by design: an unreachable panel never fails the
// daemon or the event source. Events accumulate in a bounded in-memory buffer
// (oldest dropped first) and are re-sent on the next flush; a batch the panel
// rejects outright (4xx) is dropped instead of retried forever.

const (
	// dockerEventFlushInterval is the max latency of an idle feed; the count
	// threshold exists so a burst is delivered before the timer expires.
	dockerEventFlushInterval = 5 * time.Second
	dockerEventFlushCount    = 50
	// dockerEventBufferLimit bounds the offline backlog. Past 1000 events the
	// oldest are dropped: a timeline that skipped a weekend of churn is more
	// useful than one that never filled, and unbounded buffering is a memory
	// leak with extra steps.
	dockerEventBufferLimit = 1000
	// dockerEventMaxBatch caps a single POST; the panel enforces the same limit.
	dockerEventMaxBatch       = 200
	dockerEventRequestTimeout = 15 * time.Second
	dockerEventBackoffStart   = time.Second
	dockerEventBackoffMax     = time.Minute
	// dockerEventAttrLimit caps one actor attribute value on the way out. Docker
	// puts labels and env summaries in there, which can be much larger than the
	// few bytes worth shipping to a timeline.
	dockerEventAttrLimit = 512
	// dockerEventPath is the panel ingest route (handlers_docker_events.go).
	dockerEventPath = "/api/remote/docker/events"
)

// dockerEventActions are the container lifecycle events the feed tracks. Docker
// emits many more (attach, exec, rename, update…); those are noise for an
// operational timeline and are filtered at the daemon so they never cost a
// websocket frame.
var dockerEventActions = []string{"start", "stop", "die", "kill", "oom", "recreate", "destroy"}

// errDockerEventRejected marks a panel response that retrying cannot fix (4xx).
// The flush loop drops the batch instead of requeueing it, and says so in the log.
var errDockerEventRejected = errors.New("panel rejected docker event batch")

// dockerEventRecord is the wire format the panel expects:
// {type, containerID, containerName, image, actorAttributes, timestamp}.
type dockerEventRecord struct {
	Type            string            `json:"type"`
	ContainerID     string            `json:"containerID"`
	ContainerName   string            `json:"containerName"`
	Image           string            `json:"image"`
	ActorAttributes map[string]string `json:"actorAttributes,omitempty"`
	Timestamp       string            `json:"timestamp"`
}

// dockerEventBatchRequest is the ingest envelope.
type dockerEventBatchRequest struct {
	Events []dockerEventRecord `json:"events"`
}

// dockerEventBuffer is the bounded hand-off between the Docker tail and the
// HTTP flusher. It is the only place events can be lost, and it reports every
// loss as a counter instead of a silent overwrite.
type dockerEventBuffer struct {
	mu      sync.Mutex
	pending []dockerEventRecord
	dropped uint64
	// notify has capacity 1 and is signalled whenever the batch threshold is
	// crossed, so a burst is flushed without waiting for the ticker.
	notify chan struct{}
}

func newDockerEventBuffer() *dockerEventBuffer {
	return &dockerEventBuffer{notify: make(chan struct{}, 1)}
}

func (b *dockerEventBuffer) add(record dockerEventRecord) {
	b.mu.Lock()
	b.pending = append(b.pending, record)
	overflow := len(b.pending) - dockerEventBufferLimit
	if overflow > 0 {
		// Drop the oldest, not the newest: the feed is read most-recent-first, so
		// the entries about to fall off the end of the retention window are the
		// least useful ones to keep re-sending.
		b.pending = append([]dockerEventRecord(nil), b.pending[overflow:]...)
		b.dropped += uint64(overflow)
	}
	threshold := len(b.pending) >= dockerEventFlushCount
	b.mu.Unlock()
	if threshold {
		b.signal()
	}
}

// requeue puts an unsent batch back at the front of the buffer, applying the
// same cap. A node that cannot reach the panel for an hour therefore keeps the
// most recent dockerEventBufferLimit events and loses exactly the rest.
func (b *dockerEventBuffer) requeue(records []dockerEventRecord) {
	if len(records) == 0 {
		return
	}
	b.mu.Lock()
	combined := make([]dockerEventRecord, 0, len(records)+len(b.pending))
	combined = append(combined, records...)
	combined = append(combined, b.pending...)
	if overflow := len(combined) - dockerEventBufferLimit; overflow > 0 {
		combined = combined[overflow:]
		b.dropped += uint64(overflow)
	}
	b.pending = combined
	b.mu.Unlock()
	b.signal()
}

// drain removes up to limit records from the front of the buffer.
func (b *dockerEventBuffer) drain(limit int) []dockerEventRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.pending) == 0 {
		return nil
	}
	count := len(b.pending)
	if limit > 0 && count > limit {
		count = limit
	}
	out := make([]dockerEventRecord, count)
	copy(out, b.pending[:count])
	remaining := make([]dockerEventRecord, len(b.pending)-count)
	copy(remaining, b.pending[count:])
	b.pending = remaining
	return out
}

func (b *dockerEventBuffer) len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending)
}

func (b *dockerEventBuffer) dropCount() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dropped
}

func (b *dockerEventBuffer) signal() {
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

// StartDockerEventStream connects the local Docker event stream to the panel
// feed. It blocks until ctx is cancelled and is expected to run as a goroutine
// from the daemon's background wiring.
//
// It returns immediately (with a log line, not an error) when there is no panel
// to talk to or no node credential to sign with, because a standalone Beacon
// has no feed to fill; it also honours DAEMON_DOCKER_EVENTS=false as an
// operator kill switch.
func (s *Server) StartDockerEventStream(ctx context.Context) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DAEMON_DOCKER_EVENTS")), "false") {
		log.Printf("docker event stream disabled by DAEMON_DOCKER_EVENTS")
		return
	}
	panelBase := dockerEventPanelBase()
	if panelBase == "" || s.token == "" {
		log.Printf("docker event stream disabled: panel API URL and node token are both required")
		return
	}

	buffer := newDockerEventBuffer()
	go s.flushDockerEvents(ctx, panelBase, buffer)
	log.Printf("docker event stream started (actions: %s, flush: %s/%d events, panel: %s%s)",
		strings.Join(dockerEventActions, ", "), dockerEventFlushInterval, dockerEventFlushCount, panelBase, dockerEventPath)

	backoff := dockerEventBackoffStart
	for ctx.Err() == nil {
		sawEvent, err := s.tailDockerEvents(ctx, buffer)
		if ctx.Err() != nil {
			break
		}
		if sawEvent {
			// A stream that carried traffic before dying is a transient failure, not
			// a misconfiguration; reset so a flapping socket does not settle at the
			// maximum backoff.
			backoff = dockerEventBackoffStart
		}
		if err != nil {
			log.Printf("docker event stream ended, retrying in %s: %v", backoff, err)
		} else {
			log.Printf("docker event stream closed, retrying in %s", backoff)
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff *= 2; backoff > dockerEventBackoffMax {
			backoff = dockerEventBackoffMax
		}
	}

	if pending := buffer.len(); pending > 0 || buffer.dropCount() > 0 {
		log.Printf("docker event stream stopping: %d buffered, %d dropped for buffer overflow", pending, buffer.dropCount())
	}
}

// tailDockerEvents runs one connection to the Docker event stream, buffering
// lifecycle events until the socket dies or ctx is cancelled. It reports whether
// it ever received an event, which the caller uses to decide on backoff.
func (s *Server) tailDockerEvents(ctx context.Context, buffer *dockerEventBuffer) (bool, error) {
	cli, err := s.adminDockerClient()
	if err != nil {
		return false, err
	}
	defer func() {
		_ = cli.Close()
	}()

	args := filters.NewArgs(filters.Arg("type", string(events.ContainerEventType)))
	for _, action := range dockerEventActions {
		args.Add("event", action)
	}

	stream, errs := cli.Events(ctx, events.ListOptions{Filters: args})
	sawEvent := false
	for {
		select {
		case message, ok := <-stream:
			if !ok {
				return sawEvent, errors.New("docker event stream closed by the daemon")
			}
			sawEvent = true
			if record, valid := dockerEventRecordFrom(message); valid {
				buffer.add(record)
			}
		case err, ok := <-errs:
			if ok && err != nil {
				return sawEvent, err
			}
			return sawEvent, errors.New("docker event stream reported an error")
		case <-ctx.Done():
			return sawEvent, nil
		}
	}
}

// dockerEventRecordFrom converts a Docker event into the ingest payload, and
// says whether it is worth shipping. The daemon-side filter should already have
// selected for these, so the action check is defence against a daemon that
// ignores or reinterprets the filter (and against a future widening of args).
func dockerEventRecordFrom(message events.Message) (dockerEventRecord, bool) {
	if message.Type != "" && message.Type != events.ContainerEventType {
		return dockerEventRecord{}, false
	}
	action := strings.ToLower(strings.TrimSpace(string(message.Action)))
	if action == "" || !isDockerLifecycleAction(action) {
		return dockerEventRecord{}, false
	}

	attributes := make(map[string]string, len(message.Actor.Attributes))
	for key, value := range message.Actor.Attributes {
		if len(value) > dockerEventAttrLimit {
			// Cut on a rune boundary: a UTF-8 label must not arrive at the panel
			// with a torn last character.
			cut := dockerEventAttrLimit
			for cut > 0 && !utf8.RuneStart(value[cut]) {
				cut--
			}
			value = value[:cut]
		}
		attributes[key] = value
	}

	// Docker reports event time as unix seconds (Time) plus a nanosecond
	// field (TimeNano); either may be absent on older daemons.
	var timestamp time.Time
	switch {
	case message.TimeNano > 0:
		timestamp = time.Unix(0, message.TimeNano)
	case message.Time > 0:
		timestamp = time.Unix(message.Time, 0)
	default:
		timestamp = time.Now()
	}

	containerID := strings.TrimSpace(message.Actor.ID)
	if containerID == "" {
		containerID = attributes["id"]
	}

	return dockerEventRecord{
		Type:            action,
		ContainerID:     containerID,
		ContainerName:   attributes["name"],
		Image:           attributes["image"],
		ActorAttributes: attributes,
		Timestamp:       timestamp.UTC().Format(time.RFC3339Nano),
	}, true
}

func isDockerLifecycleAction(action string) bool {
	for _, candidate := range dockerEventActions {
		if candidate == action {
			return true
		}
	}
	return false
}

// flushDockerEvents is the delivery half of the feed: flush on the timer, on the
// batch threshold, and once more on shutdown.
func (s *Server) flushDockerEvents(ctx context.Context, panelBase string, buffer *dockerEventBuffer) {
	ticker := time.NewTicker(dockerEventFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// The daemon context is already cancelled, so the final attempt cannot
			// inherit it; give it a short independent deadline and accept that a
			// node shutting down offline simply loses the backlog.
			s.flushDockerEventBatch(context.Background(), panelBase, buffer, 5*time.Second)
			return
		case <-ticker.C:
		case <-buffer.notify:
		}
		s.flushDockerEventBatch(ctx, panelBase, buffer, 0)
	}
}

// flushDockerEventBatch delivers whatever is buffered. When shutdownTimeout is
// non-zero the daemon context has already gone away, so the remaining backlog
// gets one short, independent deadline instead of a cancelled parent.
func (s *Server) flushDockerEventBatch(ctx context.Context, panelBase string, buffer *dockerEventBuffer, shutdownTimeout time.Duration) {
	if shutdownTimeout > 0 {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		s.deliverDockerEvents(shutdownCtx, panelBase, buffer)
		return
	}
	s.deliverDockerEvents(ctx, panelBase, buffer)
}

// deliverDockerEvents empties the buffer one bounded batch at a time, stopping
// at the first batch that fits entirely (i.e. nothing else is queued) or that
// cannot be delivered.
func (s *Server) deliverDockerEvents(ctx context.Context, panelBase string, buffer *dockerEventBuffer) {
	for {
		batch := buffer.drain(dockerEventMaxBatch)
		if len(batch) == 0 {
			return
		}
		// The daemon context has no deadline of its own, so each attempt gets one:
		// a panel that accepts the connection and then stops answering must not be
		// able to park the flusher forever.
		requestCtx, cancel := context.WithTimeout(ctx, dockerEventRequestTimeout)
		err := s.sendDockerEvents(requestCtx, panelBase, batch)
		cancel()
		switch {
		case err == nil:
		case errors.Is(err, errDockerEventRejected):
			// The panel understood the batch and refused it (bad payload, unknown
			// node, replayed nonce). Re-sending the same bytes would fail again, so
			// drop it and keep the tail running.
			log.Printf("docker event batch dropped: %v", err)
		default:
			buffer.requeue(batch)
			log.Printf("docker event delivery failed (%d buffered): %v", buffer.len(), err)
			return
		}
		if len(batch) < dockerEventMaxBatch {
			return
		}
	}
}

// sendDockerEvents POSTs one batch, signed exactly like every other
// Beacon→panel call: bearer node token plus an HMAC over
// method, request URI, timestamp, nonce and body.
func (s *Server) sendDockerEvents(ctx context.Context, panelBase string, batch []dockerEventRecord) error {
	body, err := json.Marshal(dockerEventBatchRequest{Events: batch})
	if err != nil {
		// Encoding is local and deterministic; if it fails the batch is unusable
		// and requeueing it would loop forever.
		return fmt.Errorf("%w: encode batch: %s", errDockerEventRejected, err.Error())
	}

	requestURL := strings.TrimRight(panelBase, "/") + dockerEventPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build docker event request: %w", err)
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return fmt.Errorf("generate docker event nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)

	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.forge.v1+json")
	req.Header.Set("X-Panel-Timestamp", timestamp)
	req.Header.Set("X-Panel-Nonce", nonce)
	req.Header.Set("X-Panel-Signature", sign(s.token, req.Method, req.URL.RequestURI(), timestamp, body, nonce))

	// The request already carries a deadline, so the client itself needs no
	// timeout: one shared cancellation path covers slow headers and a stalled
	// response body.
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post docker events: %w", err)
	}
	defer res.Body.Close()
	// Drained (and bounded) so the connection can be reused and a chatty error
	// page cannot grow the daemon's heap.
	detail, _ := io.ReadAll(io.LimitReader(res.Body, 4096))

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	message := fmt.Sprintf("panel returned %s: %s", res.Status, strings.TrimSpace(string(detail)))
	if res.StatusCode >= 400 && res.StatusCode < 500 &&
		res.StatusCode != http.StatusRequestTimeout && res.StatusCode != http.StatusTooManyRequests {
		return fmt.Errorf("%w: %s", errDockerEventRejected, message)
	}
	return fmt.Errorf("panel would not accept docker events: %s", message)
}

// dockerEventPanelBase derives the panel root from either env spelling,
// tolerating a value that already carries the /api/remote or /api/v1 suffix.
// The same normalization is applied inline in server.go for the health-report
// forward; it is duplicated rather than shared because that call site owns its
// own request path and this file must not reshape it.
//
// A value with no scheme is rejected here instead of being concatenated into a
// relative URL: the feed is a background loop and an unsendable endpoint would
// show up only as a permanent stream of delivery errors.
func dockerEventPanelBase() string {
	panelBase := strings.TrimSpace(os.Getenv("PANEL_API_URL"))
	if panelBase == "" {
		panelBase = strings.TrimSpace(os.Getenv("WINGS_PANEL_URL"))
	}
	panelBase = strings.TrimRight(
		strings.TrimSuffix(strings.TrimSuffix(panelBase, "/api/remote"), "/api/v1"),
		"/",
	)
	if panelBase == "" || !strings.HasPrefix(panelBase, "http://") && !strings.HasPrefix(panelBase, "https://") {
		return ""
	}
	return panelBase
}
