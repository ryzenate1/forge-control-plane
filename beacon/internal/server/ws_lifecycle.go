package server

import (
	"context"
	"time"

	"gamepanel/beacon/internal/runtime"
)

// Console and stats websockets outlive a single workload run. A viewer that
// connects to a stopped workload used to receive one opaque error and a closed
// socket, which the panel could only interpret as a transport failure and
// answer with a reconnect storm. Both streams now report the workload's
// lifecycle explicitly, keep the socket open while it is stopped, and attach on
// their own once it starts.
const (
	// lifecyclePollInterval is how often a waiting stream re-inspects the
	// workload while it is not running.
	lifecyclePollInterval = 2 * time.Second
)

// lifecycleFrame renders a runtime inspection as the wire shape shared by the
// console and stats streams.
//
// Fields that are genuinely unknown are omitted rather than zeroed: a workload
// that has never started has no start time, and "startedAt": "0001-01-01" or
// "uptimeMs": 0 would read as a measurement. Absent means unknown.
func lifecycleFrame(state runtime.ContainerState) map[string]any {
	frame := map[string]any{
		"serverId": state.ServerID,
		"exists":   state.Exists,
		"running":  state.Running,
		"status":   state.Status,
	}
	if !state.StartedAt.IsZero() {
		frame["startedAt"] = state.StartedAt.UTC().Format(time.RFC3339Nano)
		if state.Running {
			if uptime := time.Since(state.StartedAt); uptime > 0 {
				frame["uptimeMs"] = uptime.Milliseconds()
			}
		}
	}
	return frame
}

// lifecycleKey collapses an inspection into the values a viewer reacts to, so
// a waiting stream re-sends a state frame on change instead of every poll.
func lifecycleKey(state runtime.ContainerState) string {
	prefix := "absent"
	switch {
	case state.Running:
		prefix = "running"
	case state.Exists:
		prefix = "stopped"
	}
	return prefix + ":" + state.Status + ":" + state.StartedAt.UTC().Format(time.RFC3339Nano)
}

// waitTick blocks for one poll interval. It reports false when the session is
// over, so callers exit instead of looping on a dead connection.
func waitTick(ctx context.Context) bool {
	timer := time.NewTimer(lifecyclePollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// inspectForStream inspects a workload for a websocket stream.
//
// An inspect failure is a real failure and is reported as one — never as an
// absent or stopped workload, which would be a fabricated reading. It returns
// ok=false when the caller should give up on the session entirely (the write
// failed or the session ended) and retry=true when it should poll again.
func (s *Server) inspectForStream(ctx context.Context, writer *webSocketWriter, serverID string) (state runtime.ContainerState, retry bool, ok bool) {
	inspection, err := s.runtime.Inspect(ctx, serverID)
	if err != nil {
		if ctx.Err() != nil {
			return runtime.ContainerState{}, false, false
		}
		if writeErr := writer.WriteJSON(map[string]any{
			"serverId": serverID,
			"type":     "error",
			"code":     "inspect_failed",
			"data":     err.Error(),
			"error":    err.Error(),
		}); writeErr != nil {
			return runtime.ContainerState{}, false, false
		}
		return runtime.ContainerState{}, true, true
	}
	return inspection, false, true
}
