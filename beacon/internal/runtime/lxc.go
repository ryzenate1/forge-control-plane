package runtime

import (
	"context"
	"errors"
	"io"
	"time"
)

// LXCProvider is the provider name operators use to select the LXC container
// engine. It matches the literal used by the experimental-provider gate in
// providers.go so both stay in sync.
const LXCProvider = "lxc"

// errLXCNotImplemented is the single honest message every LXC operation
// returns. A stub that pretended to succeed would let the panel believe a
// workload exists on the node, so every method refuses instead.
var errLXCNotImplemented = errors.New(
	"LXC runtime not implemented: requires lxc-tools or Incus on host; see docs",
)

// LXCRuntime is a placeholder adapter for the LXC/lxcfs (or Incus) engine. It
// satisfies Runtime so a Beacon build configured with provider "lxc" can start
// and report a clear, actionable error on every workload operation rather than
// panicking with "unsupported runtime provider".
type LXCRuntime struct{}

// NewLXCRuntime returns the LXC stub. Construction never fails: the missing
// host tooling is reported by the operations that would need it, which keeps
// daemon startup and provider validation independent of the node's engine.
func NewLXCRuntime() (*LXCRuntime, error) {
	return &LXCRuntime{}, nil
}

func (r *LXCRuntime) Provider() string { return LXCProvider }

// Available reports false: there is no live LXC engine behind this adapter, so
// health, readiness, and metrics must not claim otherwise.
func (r *LXCRuntime) Available() bool { return false }

func (r *LXCRuntime) unavailable() error { return errLXCNotImplemented }

func (r *LXCRuntime) Close() error { return nil }
func (r *LXCRuntime) Create(context.Context, CreateRequest) error {
	return r.unavailable()
}
func (r *LXCRuntime) Install(context.Context, InstallRequest) (InstallResult, error) {
	return InstallResult{}, r.unavailable()
}
func (r *LXCRuntime) Inspect(context.Context, string) (ContainerState, error) {
	return ContainerState{}, r.unavailable()
}
func (r *LXCRuntime) List(context.Context) ([]ContainerState, error) {
	return nil, r.unavailable()
}
func (r *LXCRuntime) Start(context.Context, string) error { return r.unavailable() }
func (r *LXCRuntime) SendCommand(context.Context, string, string) error {
	return r.unavailable()
}
func (r *LXCRuntime) Stop(context.Context, string) error { return r.unavailable() }
func (r *LXCRuntime) WaitForStop(context.Context, string, time.Duration, bool) error {
	return r.unavailable()
}
func (r *LXCRuntime) Kill(context.Context, string) error { return r.unavailable() }
func (r *LXCRuntime) Signal(context.Context, string, string) error {
	return r.unavailable()
}
func (r *LXCRuntime) Restart(context.Context, string) error { return r.unavailable() }
func (r *LXCRuntime) Stats(context.Context, string) (Stats, error) {
	return Stats{}, r.unavailable()
}
func (r *LXCRuntime) Logs(context.Context, string) (io.ReadCloser, error) {
	return nil, r.unavailable()
}
func (r *LXCRuntime) LogsStream(context.Context, string, string) (io.ReadCloser, error) {
	return nil, r.unavailable()
}
func (r *LXCRuntime) StatsStream(context.Context, string) (io.ReadCloser, error) {
	return nil, r.unavailable()
}
func (r *LXCRuntime) AttachConsole(context.Context, string) (ConsoleSession, error) {
	return nil, r.unavailable()
}
func (r *LXCRuntime) Delete(context.Context, string) error { return r.unavailable() }
