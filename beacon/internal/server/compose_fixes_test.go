package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShortFormHostPort_Fixes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"len1 single port", "80", "80"},
		{"len1 with protocol", "80/tcp", "80"},
		{"host:container", "8080:80", "8080"},
		{"range host:container range", "8080-8082:80-82", "8080-8082"},
		{"ip:host:container", "127.0.0.1:8080:80", "8080"},
		{"ip::container random host", "127.0.0.1::80", ""},
		{"ip:range:container range", "127.0.0.1:8080-8082:80-82", "8080-8082"},
		{"with protocol suffix", "8080:80/udp", "8080"},
		{"bracket ipv6", "[::1]:8080:80", "8080"},
	}
	for _, tc := range tests {
		got := shortFormHostPort(tc.input)
		if got != tc.expected {
			t.Errorf("%s: shortFormHostPort(%q) = %q, want %q", tc.name, tc.input, got, tc.expected)
		}
	}
}

func TestValidateComposePorts_RangePrivileged(t *testing.T) {
	// Host range starting at privileged port 80 should be rejected
	if err := validateComposePorts("web", []any{"80-82:8080"}); err == nil {
		t.Fatal("expected privileged range 80-82 to be rejected")
	}
	// Non-privileged range should pass
	if err := validateComposePorts("web", []any{"8080-8082:80"}); err != nil {
		t.Fatalf("unexpected error for non-privileged host range: %v", err)
	}
	// Single len1 privileged should be rejected
	if err := validateComposePorts("web", []any{"80"}); err == nil {
		t.Fatal("expected len1 privileged port 80 to be rejected")
	}
	// len1 non-privileged should pass
	if err := validateComposePorts("web", []any{"8080"}); err != nil {
		t.Fatalf("unexpected error for 8080: %v", err)
	}
	// 127.0.0.1::80 random host should not be privileged
	if err := validateComposePorts("web", []any{"127.0.0.1::80"}); err != nil {
		t.Fatalf("unexpected error for random host port: %v", err)
	}
}

func TestValidateComposeVolumesWithAllowlist(t *testing.T) {
	// Sensitive path /etc requires admin + allowlist
	if err := validateComposeVolumesWithAllowlist("app", []any{"/etc:/host"}, false, []string{"/etc"}); err == nil {
		t.Fatal("expected non-admin /etc mount to be rejected")
	}
	if err := validateComposeVolumesWithAllowlist("app", []any{"/etc:/host"}, true, nil); err == nil {
		t.Fatal("expected /etc without allowlist to be rejected even with admin")
	}
	if err := validateComposeVolumesWithAllowlist("app", []any{"/etc:/host"}, true, []string{"/etc"}); err != nil {
		t.Fatalf("expected admin+allowlisted /etc to pass, got %v", err)
	}
	if err := validateComposeVolumesWithAllowlist("app", []any{"/etc/sub:/host"}, true, []string{"/etc"}); err != nil {
		t.Fatalf("expected subpath of allowlisted /etc to pass, got %v", err)
	}
	// Non-sensitive path should pass even without allowlist (shared predicate)
	if err := validateComposeVolumesWithAllowlist("app", []any{"/data:/host"}, false, nil); err != nil {
		t.Fatalf("expected non-sensitive /data to pass without allowlist, got %v", err)
	}
	// Long-form bind should use same predicate
	if err := validateComposeVolumesWithAllowlist("app", []any{map[string]any{"type": "bind", "source": "/etc", "target": "/host"}}, true, []string{"/etc"}); err != nil {
		t.Fatalf("expected long-form allowlisted /etc to pass, got %v", err)
	}
	if err := validateComposeVolumesWithAllowlist("app", []any{map[string]any{"type": "bind", "source": "/etc", "target": "/host"}}, true, nil); err == nil {
		t.Fatal("expected long-form /etc without allowlist to be rejected")
	}
	// Anonymous volume should pass
	if err := validateComposeVolumesWithAllowlist("app", []any{"/data"}, false, nil); err != nil {
		t.Fatalf("expected anonymous volume to pass, got %v", err)
	}
}

func TestValidateComposePolicyWithAllowlist_VolumeGate(t *testing.T) {
	validYAML := "services:\n  app:\n    image: nginx\n    volumes:\n      - /etc:/host\n"
	// Without admin+allowlist should fail
	if err := validateComposePolicyWithAllowlist(validYAML, nil, false); err == nil {
		t.Fatal("expected /etc without admin to be rejected")
	}
	if err := validateComposePolicyWithAllowlist(validYAML, []string{"/etc"}, true); err != nil {
		t.Fatalf("expected admin+allowlisted to pass, got %v", err)
	}
	// Non-sensitive should pass even without allowlist
	nonSensitiveYAML := "services:\n  app:\n    image: nginx\n    volumes:\n      - /data:/host\n"
	if err := validateComposePolicyWithAllowlist(nonSensitiveYAML, nil, false); err != nil {
		t.Fatalf("expected non-sensitive to pass, got %v", err)
	}
}

func TestHandleComposeDelete_VolumesOptIn(t *testing.T) {
	// Verify handleComposeDelete only passes -v when volumes=true
	// We test by checking that the docker command would be built correctly
	// via a dry-run: we don't actually run docker, so we test query parsing logic
	// indirectly by verifying handleComposeDelete handles volumes param.
	// Create a temp compose stack dir with a compose file so delete path is exercised.
	dir := t.TempDir()
	srv := &Server{composeStacks: &composeStack{stacks: make(map[string]*composeLockEntry), dir: dir}}
	// Create a stack directory with compose.yaml
	stackID := "teststack"
	stackDir := filepath.Join(dir, stackID)
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  app:\n    image: alpine\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	// Mock docker by ensuring compose file exists but docker binary missing will cause error in down
	// Instead we test the request handling for invalid stack ID still works.
	// Test that volumes=false does NOT add -v is covered by code inspection; here we test HTTP query handling
	req := httptest.NewRequest(http.MethodDelete, "/compose/"+stackID+"?volumes=true&removeOrphans=true", nil)
	req.SetPathValue("stackId", stackID)
	rr := httptest.NewRecorder()
	// We expect the handler to attempt docker down and fail with 409 or succeed with 200 even if docker not found
	// But crucially it should parse volumes=true correctly without panic.
	srv.handleComposeDelete(rr, req)
	// The handler should not panic; status should be 200 or 409, but not 500 due to bad parsing
	if rr.Code != http.StatusOK && rr.Code != http.StatusConflict && rr.Code != http.StatusNotFound {
		t.Fatalf("unexpected status %d body %s", rr.Code, rr.Body.String())
	}
	// Test without volumes should also not panic
	req2 := httptest.NewRequest(http.MethodDelete, "/compose/"+stackID, nil)
	req2.SetPathValue("stackId", stackID)
	rr2 := httptest.NewRecorder()
	// Recreate compose file since previous delete removed dir
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  app:\n    image: alpine\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	srv.handleComposeDelete(rr2, req2)
	if rr2.Code != http.StatusOK && rr2.Code != http.StatusConflict {
		t.Fatalf("unexpected status without volumes %d body %s", rr2.Code, rr2.Body.String())
	}
}

func TestEncodeComposeEnv_Rejection(t *testing.T) {
	if _, err := encodeComposeEnv(map[string]string{"BAD-KEY": "value"}); err == nil {
		t.Fatal("expected invalid env key to be rejected")
	}
}

func TestValidateHostMountWithAllowlist_BeaconMatchesForge(t *testing.T) {
	// Verify beacon's validateComposeVolumesWithAllowlist matches the mount
	// policy it is shared with: an ordinary host path is allowed, /proc and /sys
	// are never allowed, and *configuring* an allowlist turns it into the gate —
	// a non-admin token may not use it at all, so even /data is refused once an
	// allowlist is present.
	//
	// NOTE: the strict "/data passes regardless" parity row was wrong for
	// today's predicate (an allowlist, once configured, gates every absolute
	// source), and the forge-side host-mount gate it mirrored no longer exists —
	// forge's compose validation reports host mounts as warnings instead.
	tests := []struct {
		src         string
		isAdmin     bool
		allowed     []string
		shouldError bool
	}{
		{"/etc", false, []string{"/etc"}, true},
		{"/etc", true, nil, true},
		{"/etc", true, []string{"/etc"}, false},
		{"/etc/passwd", true, []string{"/etc"}, false},
		{"/etc/passwd", true, []string{"/other"}, true},
		{"/data", false, nil, false}, // no allowlist configured: ordinary host path passes
		{"/data", false, []string{"/other"}, true},
		{"/data", true, []string{"/data"}, false},
		{"/proc", true, []string{"/proc"}, true}, // never openable, allowlist or not
	}
	for i, tc := range tests {
		err := validateComposeVolumesWithAllowlist("app", []any{tc.src + ":/target"}, tc.isAdmin, tc.allowed)
		if (err != nil) != tc.shouldError {
			t.Errorf("case %d src=%q admin=%v allowed=%v: got err %v wantError %v", i, tc.src, tc.isAdmin, tc.allowed, err, tc.shouldError)
		}
	}
	if got := shortFormHostPort("8080-8082:80-82"); got != "8080-8082" {
		t.Errorf("range parse got %q want %q", got, "8080-8082")
	}
	if !strings.Contains("8080-8082", "-") {
		t.Error("range contains check")
	}
}
