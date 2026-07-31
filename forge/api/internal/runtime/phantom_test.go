package runtime

import (
	"context"
	"errors"
	"testing"
)

// TestCreatePhantomProvider_Rejected verifies honesty fix 110-03-17:
// phantom providers LXC/KVM silently fell back to docker. They must now be
// rejected with ErrUnsupportedProvider (mapped to HTTP 400) unless the
// experimental flag is explicitly enabled.
func TestCreatePhantomProvider_Rejected(t *testing.T) {
	// Ensure experimental flag is off (default).
	t.Setenv("ENABLE_EXPERIMENTAL_RUNTIMES", "")
	if IsSupportedProvider(LXCProvider) {
		t.Fatalf("IsSupportedProvider(%q) should be false when flag is off", LXCProvider)
	}
	if IsSupportedProvider(KVMProvider) {
		t.Fatalf("IsSupportedProvider(%q) should be false when flag is off", KVMProvider)
	}
	// Direct validation should return wrapped ErrUnsupportedProvider.
	if err := ValidateProvider(LXCProvider); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("ValidateProvider(lxc) err=%v, want ErrUnsupportedProvider", err)
	}
	if err := ValidateProvider(KVMProvider); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("ValidateProvider(kvm) err=%v, want ErrUnsupportedProvider", err)
	}
	if err := ValidateProvider("unknown-phantom"); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("ValidateProvider(unknown) err=%v, want ErrUnsupportedProvider", err)
	}
	// Supported providers must still pass.
	for _, p := range []string{DockerProvider, ContainerdProvider, PodmanProvider, FirecrackerProvider, KubernetesProvider, ""} {
		if err := ValidateProvider(p); err != nil {
			t.Errorf("ValidateProvider(%q) should not error, got %v", p, err)
		}
		if !IsSupportedProvider(p) {
			t.Errorf("IsSupportedProvider(%q) should be true", p)
		}
	}

	// MultiRuntimeAdapter must reject phantom via CreateServer (fallback guard at :45).
	m := NewMultiRuntimeAdapter(&mockRuntime{name: "default"})
	m.Register(DockerProvider, &mockRuntime{name: DockerProvider})
	// lxc should be rejected, not silently fall back to docker.
	if _, err := m.CreateServer(context.Background(), Target{Provider: LXCProvider}, CreateServerRequest{}); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("CreateServer with lxc err=%v, want ErrUnsupportedProvider", err)
	}
	if _, err := m.CreateServer(context.Background(), Target{Provider: KVMProvider}, CreateServerRequest{}); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("CreateServer with kvm err=%v, want ErrUnsupportedProvider", err)
	}
	if _, err := m.CreateServer(context.Background(), Target{Provider: "phantom-xyz"}, CreateServerRequest{}); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("CreateServer with unknown err=%v, want ErrUnsupportedProvider", err)
	}
	// Other operations must also reject.
	if err := m.SyncServerConfiguration(context.Background(), Target{Provider: LXCProvider}, ServerConfiguration{}); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("SyncServerConfiguration with lxc err=%v, want ErrUnsupportedProvider", err)
	}
	if _, err := m.Stats(context.Background(), Target{Provider: KVMProvider}); !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("Stats with kvm err=%v, want ErrUnsupportedProvider", err)
	}
	// Supported provider should delegate (fallback to default if not registered is allowed).
	if _, err := m.CreateServer(context.Background(), Target{Provider: DockerProvider}, CreateServerRequest{}); err != nil {
		t.Errorf("CreateServer with docker should succeed, got %v", err)
	}
	if _, err := m.CreateServer(context.Background(), Target{Provider: ""}, CreateServerRequest{}); err != nil {
		t.Errorf("CreateServer with empty provider should succeed via default, got %v", err)
	}
	if _, err := m.CreateServer(context.Background(), Target{Provider: "CONTAINERD"}, CreateServerRequest{}); err != nil && errors.Is(err, ErrUnsupportedProvider) {
		t.Errorf("Provider matching should be case-insensitive, got %v", err)
	}
}

func TestCreatePhantomProvider_AllowedWithExperimentalFlag(t *testing.T) {
	t.Setenv("ENABLE_EXPERIMENTAL_RUNTIMES", "true")
	if !IsSupportedProvider(LXCProvider) {
		t.Fatalf("IsSupportedProvider(lxc) should be true when flag is on")
	}
	if !IsSupportedProvider(KVMProvider) {
		t.Fatalf("IsSupportedProvider(kvm) should be true when flag is on")
	}
	if err := ValidateProvider(LXCProvider); err != nil {
		t.Fatalf("ValidateProvider(lxc) with flag on should not error, got %v", err)
	}
	// With flag on, adapter should not return ErrUnsupportedProvider;
	// if lxc runtime is not registered it falls back to default (honest but not phantom rejection).
	m := NewMultiRuntimeAdapter(&mockRuntime{name: "default"})
	m.Register(DockerProvider, &mockRuntime{name: DockerProvider})
	// No lxc runtime registered, but provider is now considered supported, so it falls back to default.
	if _, err := m.CreateServer(context.Background(), Target{Provider: LXCProvider}, CreateServerRequest{}); errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("CreateServer with lxc and flag on should not be ErrUnsupportedProvider, got %v", err)
	}
	// If experimental runtime is explicitly registered, it should be used.
	lxcRt := &mockRuntime{name: LXCProvider}
	m.Register(LXCProvider, lxcRt)
	if _, err := m.CreateServer(context.Background(), Target{Provider: "lxc"}, CreateServerRequest{}); err != nil {
		t.Fatalf("CreateServer with registered lxc and flag on should succeed, got %v", err)
	}
	if _, err := m.CreateServer(context.Background(), Target{Provider: "LXC"}, CreateServerRequest{}); err != nil {
		t.Fatalf("Provider lookup should be case-insensitive, got %v", err)
	}
}

func TestValidateProvider_CaseAndWhitespace(t *testing.T) {
	t.Setenv("ENABLE_EXPERIMENTAL_RUNTIMES", "")
	cases := []struct {
		input string
		want  bool
	}{
		{"docker", true},
		{" Docker ", true},
		{"DOCKER", true},
		{"lxc", false},
		{" KVM ", false},
		{"", true},
		{"auto", false},
	}
	for _, tc := range cases {
		if got := IsSupportedProvider(tc.input); got != tc.want {
			t.Errorf("IsSupportedProvider(%q)=%v want %v", tc.input, got, tc.want)
		}
	}
}
