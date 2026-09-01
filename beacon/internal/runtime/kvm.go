package runtime

import (
	"context"
	"errors"
	"io"
	"time"
)

// KVMProvider is the provider name operators use to select the KVM/QEMU virtual
// machine engine. It matches the literal used by the experimental-provider gate
// in providers.go so both stay in sync.
const KVMProvider = "kvm"

// errKVMNotImplemented is the single honest message every KVM operation
// returns. Returning success from an unimplemented adapter would make the panel
// believe a VM is running on this node, so every method refuses instead.
var errKVMNotImplemented = errors.New(
	"KVM runtime not implemented: requires libvirt and qemu-system on host; see docs",
)

// KVMRuntime is a placeholder adapter for the KVM/QEMU (libvirt) engine. It
// satisfies Runtime so a Beacon build configured with provider "kvm" can start
// and report a clear, actionable error on every workload operation rather than
// panicking with "unsupported runtime provider".
type KVMRuntime struct{}

// NewKVMRuntime returns the KVM stub. Construction never fails: the missing
// hypervisor tooling is reported by the operations that would need it, which
// keeps daemon startup and provider validation independent of the node.
func NewKVMRuntime() (*KVMRuntime, error) {
	return &KVMRuntime{}, nil
}

func (r *KVMRuntime) Provider() string { return KVMProvider }

// Available reports false: there is no live hypervisor behind this adapter, so
// health, readiness, and metrics must not claim otherwise.
func (r *KVMRuntime) Available() bool { return false }

func (r *KVMRuntime) unavailable() error { return errKVMNotImplemented }

func (r *KVMRuntime) Close() error { return nil }
func (r *KVMRuntime) Create(context.Context, CreateRequest) error {
	return r.unavailable()
}
func (r *KVMRuntime) Install(context.Context, InstallRequest) (InstallResult, error) {
	return InstallResult{}, r.unavailable()
}
func (r *KVMRuntime) Inspect(context.Context, string) (ContainerState, error) {
	return ContainerState{}, r.unavailable()
}
func (r *KVMRuntime) List(context.Context) ([]ContainerState, error) {
	return nil, r.unavailable()
}
func (r *KVMRuntime) Start(context.Context, string) error { return r.unavailable() }
func (r *KVMRuntime) SendCommand(context.Context, string, string) error {
	return r.unavailable()
}
func (r *KVMRuntime) Stop(context.Context, string) error { return r.unavailable() }
func (r *KVMRuntime) WaitForStop(context.Context, string, time.Duration, bool) error {
	return r.unavailable()
}
func (r *KVMRuntime) Kill(context.Context, string) error { return r.unavailable() }
func (r *KVMRuntime) Signal(context.Context, string, string) error {
	return r.unavailable()
}
func (r *KVMRuntime) Restart(context.Context, string) error { return r.unavailable() }
func (r *KVMRuntime) Stats(context.Context, string) (Stats, error) {
	return Stats{}, r.unavailable()
}
func (r *KVMRuntime) Logs(context.Context, string) (io.ReadCloser, error) {
	return nil, r.unavailable()
}
func (r *KVMRuntime) LogsStream(context.Context, string, string) (io.ReadCloser, error) {
	return nil, r.unavailable()
}
func (r *KVMRuntime) StatsStream(context.Context, string) (io.ReadCloser, error) {
	return nil, r.unavailable()
}
func (r *KVMRuntime) AttachConsole(context.Context, string) (ConsoleSession, error) {
	return nil, r.unavailable()
}
func (r *KVMRuntime) Delete(context.Context, string) error { return r.unavailable() }
