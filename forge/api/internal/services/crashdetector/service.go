package crashdetector

import (
	"context"
	"sync"
	"time"

	"gamepanel/forge/internal/store"
)

type Config struct {
	Threshold   int           // number of crashes within window to trigger action
	Window      time.Duration // time window for counting crashes
	Cooldown    time.Duration // cooldown after action is taken
	AutoRestart bool          // automatically restart on crash
	MaxRestarts int           // max auto-restarts before suspending
	NotifyAdmin bool          // notify admin on crash threshold
}

type ServerCrashState struct {
	ServerID   string
	Crashes    []time.Time
	Restarts   int
	LastAction time.Time
	Suspended  bool
	mu         sync.Mutex
}

type Detector struct {
	config    Config
	store     *store.Store
	states    map[string]*ServerCrashState
	mu        sync.RWMutex
	onCrash   func(ctx context.Context, serverID string, crashCount int)
	onSuspend func(ctx context.Context, serverID string)
}

func New(cfg Config, store *store.Store) *Detector {
	return &Detector{
		config: cfg,
		store:  store,
		states: make(map[string]*ServerCrashState),
	}
}

func (d *Detector) OnCrash(handler func(ctx context.Context, serverID string, crashCount int)) {
	d.onCrash = handler
}

func (d *Detector) OnSuspend(handler func(ctx context.Context, serverID string)) {
	d.onSuspend = handler
}

func (d *Detector) ReportCrash(ctx context.Context, serverID, nodeID string, exitCode int, oomKilled, cleanExit bool) {
	d.mu.Lock()
	state, ok := d.states[serverID]
	if !ok {
		state = &ServerCrashState{ServerID: serverID}
		d.states[serverID] = state
	}
	d.mu.Unlock()

	state.mu.Lock()
	defer state.mu.Unlock()

	now := time.Now()
	state.Crashes = append(state.Crashes, now)

	cutoff := now.Add(-d.config.Window)
	var recent []time.Time
	for _, t := range state.Crashes {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	state.Crashes = recent

	crashCount := len(recent)

	if d.store != nil {
		_, _ = d.store.CreateCrashEvent(ctx, store.CreateCrashEventRequest{
			ServerID:      serverID,
			NodeID:        nodeID,
			ExitCode:      exitCode,
			OOMKilled:     oomKilled,
			CleanExit:     cleanExit,
			AutoRestarted: d.config.AutoRestart && crashCount >= d.config.Threshold,
			CrashCount:    crashCount,
		})
	}

	if !state.LastAction.IsZero() && now.Sub(state.LastAction) < d.config.Cooldown {
		return
	}

	if crashCount >= d.config.Threshold {
		state.LastAction = now

		if d.config.AutoRestart && state.Restarts < d.config.MaxRestarts {
			state.Restarts++
			if d.onCrash != nil {
				d.onCrash(ctx, serverID, crashCount)
			}
		}

		if state.Restarts >= d.config.MaxRestarts {
			state.Suspended = true
			if d.onSuspend != nil {
				d.onSuspend(ctx, serverID)
			}
		}
	}
}

func (d *Detector) GetState(ctx context.Context, serverID string) *ServerCrashState {
	d.mu.RLock()
	state, ok := d.states[serverID]
	d.mu.RUnlock()
	if ok {
		return state
	}

	if d.store != nil {
		count, err := d.store.CountRecentCrashes(ctx, serverID, d.config.Window)
		if err == nil && count > 0 {
			state = &ServerCrashState{ServerID: serverID}
			now := time.Now()
			for i := 0; i < count; i++ {
				state.Crashes = append(state.Crashes, now)
			}
			d.mu.Lock()
			d.states[serverID] = state
			d.mu.Unlock()
			return state
		}
	}

	return nil
}

func (d *Detector) Reset(serverID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.states, serverID)
}

func DefaultConfig() Config {
	return Config{
		Threshold:   3,
		Window:      5 * time.Minute,
		Cooldown:    1 * time.Minute,
		AutoRestart: true,
		MaxRestarts: 3,
		NotifyAdmin: true,
	}
}
