package http

import (
	"context"

	"gamepanel/forge/internal/store"
)

// serverOwner returns the owner_id of a server (best-effort). Returns ok=false
// if the server doesn't exist or the store isn't configured.
func serverOwner(ctx context.Context, cfg Config, serverID string) (string, bool) {
	if cfg.Store == nil {
		return "", false
	}
	srv, err := cfg.Store.GetServer(ctx, serverID)
	if err != nil {
		return "", false
	}
	return srv.Owner, true
}

// IsUserLimitError is a convenience re-export so handlers in this package
// don't need to import the store package just for this check.
func IsUserLimitError(err error) bool {
	return store.IsUserLimitError(err)
}
