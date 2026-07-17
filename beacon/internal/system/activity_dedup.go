package system

import (
	"sync"
	"time"
)

// ActivityEntry represents a single SFTP activity log event recorded against
// a server. The combination of Action+Path is used as the deduplication key.
type ActivityEntry struct {
	Action    string
	Path      string
	IP        string
	User      string
	Client    string
	SessionID string
	Timestamp time.Time
}

// ActivityDedup aggregates SFTP file activity logs over a sliding window
// before flushing them to the API. This prevents flooding the panel API
// with thousands of individual file events during bulk operations.
//
// When the same action+path pair is recorded multiple times within the
// configured window for a given server, only the most recent entry is kept.
// At the end of each window interval the deduped entries are passed to the
// flush callback in batches no larger than maxBatch.
type ActivityDedup struct {
	mu        sync.Mutex
	entries   map[string]map[string]ActivityEntry // serverID -> dedupKey -> entry
	window    time.Duration
	maxBatch  int
	flushFunc func(serverID string, entries []ActivityEntry)
	ticker    *time.Ticker
	done      chan struct{}
}

// NewActivityDedup creates a new ActivityDedup with the given sliding window
// duration, maximum batch size, and flush callback. The flush function is
// invoked once per server per flush cycle with at most maxBatch entries at a
// time. Start must be called separately to begin background flushing.
func NewActivityDedup(window time.Duration, maxBatch int, flush func(string, []ActivityEntry)) *ActivityDedup {
	return &ActivityDedup{
		entries:   make(map[string]map[string]ActivityEntry),
		window:    window,
		maxBatch:  maxBatch,
		flushFunc: flush,
	}
}

// dedupKey returns a deduplication key for the given action and path pair.
func dedupKey(action, path string) string {
	return action + "\x00" + path
}

// Record adds an activity entry for the given server. If an entry with the
// same action+path already exists within the current window, only the entry
// with the latest timestamp is retained.
func (d *ActivityDedup) Record(serverID, action, path, ip string) {
	d.RecordDetailed(serverID, action, path, ip, "", "", "")
}

func (d *ActivityDedup) RecordDetailed(serverID, action, path, ip, user, client, sessionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := dedupKey(action, path)
	serverEntries, ok := d.entries[serverID]
	if !ok {
		serverEntries = make(map[string]ActivityEntry)
		d.entries[serverID] = serverEntries
	}

	entry := ActivityEntry{
		Action:    action,
		Path:      path,
		IP:        ip,
		User:      user,
		Client:    client,
		SessionID: sessionID,
		Timestamp: time.Now(),
	}

	// Only keep the latest timestamp for identical action+path.
	if existing, exists := serverEntries[key]; exists {
		if entry.Timestamp.After(existing.Timestamp) {
			serverEntries[key] = entry
		}
	} else {
		serverEntries[key] = entry
	}
}

// Flush manually drains all pending entries and invokes the flush callback
// for each server. Entries are batched into slices of at most maxBatch items.
func (d *ActivityDedup) Flush() {
	d.mu.Lock()
	// Swap out the entries map so we can release the lock before calling
	// the flush callback, avoiding holding the lock during potentially
	// slow network calls.
	pending := d.entries
	d.entries = make(map[string]map[string]ActivityEntry)
	d.mu.Unlock()

	for serverID, serverEntries := range pending {
		batch := make([]ActivityEntry, 0, len(serverEntries))
		for _, entry := range serverEntries {
			batch = append(batch, entry)
			if len(batch) >= d.maxBatch {
				d.flushFunc(serverID, batch)
				batch = make([]ActivityEntry, 0, d.maxBatch)
			}
		}
		if len(batch) > 0 {
			d.flushFunc(serverID, batch)
		}
	}
}

// Start begins the background ticker that automatically flushes pending
// entries at the configured window interval. Calling Start on an already
// started deduplicator is a no-op.
func (d *ActivityDedup) Start() {
	d.mu.Lock()
	if d.ticker != nil {
		d.mu.Unlock()
		return
	}
	d.ticker = time.NewTicker(d.window)
	d.done = make(chan struct{})
	tickC := d.ticker.C
	d.mu.Unlock()

	go func() {
		for {
			select {
			case <-tickC:
				d.Flush()
			case <-d.done:
				return
			}
		}
	}()
}

// Stop terminates the background flush goroutine and performs a final flush
// of any remaining entries. It is safe to call Stop on a deduplicator that
// was never started; in that case it is a no-op.
func (d *ActivityDedup) Stop() {
	d.mu.Lock()
	if d.ticker == nil {
		d.mu.Unlock()
		return
	}
	d.ticker.Stop()
	d.ticker = nil
	close(d.done)
	d.mu.Unlock()

	// Perform a final flush to send any remaining entries.
	d.Flush()
}
