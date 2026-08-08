package runtime

import (
	"context"
	"errors"
	"io"
	"time"
)

// UnavailableRuntime is used only when an operator explicitly enables daemon
// mock mode. It fails every workload operation predictably instead of passing
// a nil Runtime through the HTTP server and panicking on the first request.
type UnavailableRuntime struct {
	err error
}

func NewUnavailableRuntime(cause error) Runtime {
	if cause == nil {
		cause = errors.New("container runtime is unavailable")
	}
	return &UnavailableRuntime{err: cause}
}

func (r *UnavailableRuntime) unavailable() error {
	return errors.Join(errors.New("container runtime is unavailable"), r.err)
}

func (r *UnavailableRuntime) Close() error { return nil }
func (r *UnavailableRuntime) Create(context.Context, CreateRequest) error {
	return r.unavailable()
}
func (r *UnavailableRuntime) Install(context.Context, InstallRequest) (InstallResult, error) {
	return InstallResult{}, r.unavailable()
}
func (r *UnavailableRuntime) Inspect(context.Context, string) (ContainerState, error) {
	return ContainerState{}, r.unavailable()
}
func (r *UnavailableRuntime) List(context.Context) ([]ContainerState, error) {
	return nil, r.unavailable()
}
func (r *UnavailableRuntime) Start(context.Context, string) error { return r.unavailable() }
func (r *UnavailableRuntime) SendCommand(context.Context, string, string) error {
	return r.unavailable()
}
func (r *UnavailableRuntime) Stop(context.Context, string) error { return r.unavailable() }
func (r *UnavailableRuntime) WaitForStop(context.Context, string, time.Duration, bool) error {
	return r.unavailable()
}
func (r *UnavailableRuntime) Kill(context.Context, string) error { return r.unavailable() }
func (r *UnavailableRuntime) Signal(context.Context, string, string) error {
	return r.unavailable()
}
func (r *UnavailableRuntime) Restart(context.Context, string) error { return r.unavailable() }
func (r *UnavailableRuntime) Stats(context.Context, string) (Stats, error) {
	return Stats{}, r.unavailable()
}
func (r *UnavailableRuntime) Logs(context.Context, string) (io.ReadCloser, error) {
	return nil, r.unavailable()
}
func (r *UnavailableRuntime) LogsStream(context.Context, string, string) (io.ReadCloser, error) {
	return nil, r.unavailable()
}
func (r *UnavailableRuntime) StatsStream(context.Context, string) (io.ReadCloser, error) {
	return nil, r.unavailable()
}
func (r *UnavailableRuntime) AttachConsole(context.Context, string) (ConsoleSession, error) {
	return nil, r.unavailable()
}
func (r *UnavailableRuntime) Delete(context.Context, string) error { return r.unavailable() }
