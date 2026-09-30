//go:build firecracker

package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewFirecrackerRuntimeDefaults(t *testing.T) {
	socketDir := t.TempDir()
	cfg := FirecrackerConfig{SocketPath: socketDir}
	r, err := NewFirecrackerRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.config.FirecrackerBin != "firecracker" {
		t.Fatalf("default FirecrackerBin = %q, want %q", r.config.FirecrackerBin, "firecracker")
	}
	if r.config.KernelImage != "/var/lib/gamepanel/firecracker/kernel.bin" {
		t.Fatalf("default KernelImage = %q", r.config.KernelImage)
	}
	if r.config.RootfsImage != "/var/lib/gamepanel/firecracker/rootfs.ext4" {
		t.Fatalf("default RootfsImage = %q", r.config.RootfsImage)
	}
	if r.config.JailerPath != "jailer" {
		t.Fatalf("default JailerPath = %q, want %q", r.config.JailerPath, "jailer")
	}
	if r.config.SocketPath != socketDir {
		t.Fatalf("SocketPath = %q, want %q", r.config.SocketPath, socketDir)
	}
}

func TestNewFirecrackerRuntimeCustomConfig(t *testing.T) {
	socketDir := t.TempDir()
	cfg := FirecrackerConfig{
		SocketPath:     socketDir,
		FirecrackerBin: "/usr/local/bin/firecracker",
		KernelImage:    "/custom/vmlinux.bin",
		RootfsImage:    "/custom/rootfs.ext4",
		CPUTemplate:    "T2",
		JailerPath:     "/usr/local/bin/jailer",
	}
	r, err := NewFirecrackerRuntime(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.config.FirecrackerBin != "/usr/local/bin/firecracker" {
		t.Fatalf("FirecrackerBin = %q", r.config.FirecrackerBin)
	}
	if r.config.KernelImage != "/custom/vmlinux.bin" {
		t.Fatalf("KernelImage = %q", r.config.KernelImage)
	}
	if r.config.RootfsImage != "/custom/rootfs.ext4" {
		t.Fatalf("RootfsImage = %q", r.config.RootfsImage)
	}
	if r.config.CPUTemplate != "T2" {
		t.Fatalf("CPUTemplate = %q", r.config.CPUTemplate)
	}
	if r.config.JailerPath != "/usr/local/bin/jailer" {
		t.Fatalf("JailerPath = %q", r.config.JailerPath)
	}
}

func TestNewFirecrackerRuntimeCreatesSocketDir(t *testing.T) {
	socketDir := filepath.Join(t.TempDir(), "nested", "sockets")
	_, err := NewFirecrackerRuntime(FirecrackerConfig{SocketPath: socketDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(socketDir); os.IsNotExist(err) {
		t.Fatal("socket directory was not created")
	}
}

func TestFirecrackerProvider(t *testing.T) {
	r := &FirecrackerRuntime{}
	if got := r.Provider(); got != ProviderFirecracker {
		t.Fatalf("Provider() = %q, want %q", got, ProviderFirecracker)
	}
}

func TestFirecrackerSendCommandNoInstance(t *testing.T) {
	r := &FirecrackerRuntime{}
	err := r.SendCommand(context.Background(), "server-id", "test-command")
	if err == nil {
		t.Fatal("expected SendCommand to return an error")
	}
	if err.Error() != "send command not supported for firecracker runtime" {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestFirecrackerPingNilReceiver(t *testing.T) {
	var r *FirecrackerRuntime
	err := r.Ping(context.Background())
	if err == nil {
		t.Fatal("expected Ping with nil receiver to fail")
	}
	if err.Error() != "firecracker runtime is not initialized" {
		t.Fatalf("unexpected error: %v", err)
	}
}
