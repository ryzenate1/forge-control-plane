package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gamepanel/beacon/internal/runtime"
)

// TestCreatePhantomProvider_Rejected verifies honesty fix 110-03-17:
// beacon POST /servers with phantom providers LXC/KVM must be rejected with
// 400 instead of silently returning mode:docker.
func TestCreatePhantomProvider_Rejected(t *testing.T) {
	t.Setenv("ENABLE_EXPERIMENTAL_RUNTIMES", "")
	cases := []struct {
		provider string
		want400  bool
	}{
		{"lxc", true},
		{"kvm", true},
		{"LXC", true},
		{" KVM ", true},
		{"unknown-phantom", true},
		{"docker", false},
		// containerd/firecracker are only servable in builds compiled with
		// their tags; without the tag IsSupportedProvider rejects them and
		// the handler must refuse rather than serve Docker behind their back.
		{"containerd", !runtime.IsSupportedProvider("containerd")},
		{"podman", false},
		{"firecracker", !runtime.IsSupportedProvider("firecracker")},
		{"kubernetes", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			rt := &stubRuntime{}
			_, handler := NewServer(rt, t.TempDir())
			body := `{"serverId":"` + testServerID + `","image":"busybox"`
			if tc.provider != "" {
				body += `,"provider":"` + tc.provider + `"`
			}
			body += `}`
			req := httptest.NewRequest(http.MethodPost, "/servers", strings.NewReader(body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if tc.want400 {
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("provider %q expected 400, got %d: %s", tc.provider, rec.Code, rec.Body.String())
				}
				if !strings.Contains(strings.ToLower(rec.Body.String()), "unsupported provider") {
					t.Fatalf("provider %q body should contain 'unsupported provider', got %s", tc.provider, rec.Body.String())
				}
				if rt.createCalled {
					t.Fatalf("provider %q should not call runtime Create", tc.provider)
				}
			} else {
				if rec.Code != http.StatusAccepted {
					t.Fatalf("provider %q expected 202, got %d: %s", tc.provider, rec.Code, rec.Body.String())
				}
				if !rt.createCalled {
					t.Fatalf("provider %q should have called runtime Create", tc.provider)
				}
			}
		})
	}
}

func TestCreatePhantomProvider_AllowedWithExperimentalFlag(t *testing.T) {
	t.Setenv("ENABLE_EXPERIMENTAL_RUNTIMES", "true")
	for _, p := range []string{"lxc", "kvm", "LXC", "KVM"} {
		t.Run(p, func(t *testing.T) {
			rt := &stubRuntime{}
			_, handler := NewServer(rt, t.TempDir())
			body := `{"serverId":"` + testServerID + `","image":"busybox","provider":"` + p + `"}`
			req := httptest.NewRequest(http.MethodPost, "/servers", strings.NewReader(body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusAccepted {
				t.Fatalf("provider %q with flag on expected 202, got %d: %s", p, rec.Code, rec.Body.String())
			}
			if !rt.createCalled {
				t.Fatalf("provider %q with flag on should have called runtime Create", p)
			}
		})
	}
}

func TestCreatePhantomProvider_ModeReflectsProvider(t *testing.T) {
	t.Setenv("ENABLE_EXPERIMENTAL_RUNTIMES", "")
	rt := &stubRuntime{}
	_, handler := NewServer(rt, t.TempDir())
	body := `{"serverId":"` + testServerID + `","image":"busybox","provider":"docker"}`
	req := httptest.NewRequest(http.MethodPost, "/servers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"mode":"docker"`) {
		t.Fatalf("expected mode docker, got %s", rec.Body.String())
	}
	// Without provider, mode should still be docker
	rt2 := &stubRuntime{}
	_, handler2 := NewServer(rt2, t.TempDir())
	body2 := `{"serverId":"` + testServerID + `","image":"busybox"}`
	req2 := httptest.NewRequest(http.MethodPost, "/servers", strings.NewReader(body2))
	rec2 := httptest.NewRecorder()
	handler2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), `"mode":"docker"`) {
		t.Fatalf("expected default mode docker, got %s", rec2.Body.String())
	}
}
