package server

import (
	"strings"
	"testing"
)

// Regression tests for the host filesystem confinement boundary (SEC-5.8).

func newHostPathTestServer() *Server {
	return &Server{dataDir: "/var/lib/beacon"}
}

func TestResolveHostPathDenylistModeBlocksSystemLocations(t *testing.T) {
	s := newHostPathTestServer()
	for _, p := range []string{
		"/etc/passwd", "/etc/shadow", "/proc/self/mem", "/sys", "/dev/sda",
		"/boot/vmlinuz", "/usr/bin/env", "/root/.ssh/id_ed25519",
		"/run/secrets/db_pass", "/var/run/docker.sock",
	} {
		if _, err := s.resolveHostPath(p); err == nil {
			t.Errorf("denylist mode allowed protected path %q", p)
		}
	}
}

func TestResolveHostPathDenylistModeAllowsNeutralPaths(t *testing.T) {
	s := newHostPathTestServer()
	for _, p := range []string{"/srv/gamedata/world.zip", "/tmp/scratch.txt", "/home/admin/notes.md"} {
		if _, err := s.resolveHostPath(p); err != nil {
			t.Errorf("denylist mode rejected neutral path %q: %v", p, err)
		}
	}
}

func TestResolveHostPathAlwaysBlocksBeaconDataDir(t *testing.T) {
	s := newHostPathTestServer()
	s.SetHostFileAllowlist([]string{"/var/lib/beacon"}) // even if misconfigured as allowlist
	if _, err := s.resolveHostPath("/var/lib/beacon"); err == nil {
		t.Error("beacon data dir must be denied even when inside allowlist")
	}
	if _, err := s.resolveHostPath("/var/lib/beacon/.sftp/hostkey"); err == nil {
		t.Error("beacon data dir children must be denied")
	}
}

func TestResolveHostPathAllowlistEnforced(t *testing.T) {
	s := newHostPathTestServer()
	if err := s.SetHostFileAllowlist([]string{"/srv/backups", "/mnt/media"}); err != nil {
		t.Fatalf("SetHostFileAllowlist: %v", err)
	}
	for _, p := range []string{"/srv/backups/a.zip", "/mnt/media/movie.mkv"} {
		if _, err := s.resolveHostPath(p); err != nil {
			t.Errorf("allowlisted path %q rejected: %v", p, err)
		}
	}
	for _, p := range []string{"/srv", "/srv/other/file", "/etc/passwd", "/opt/evil", "/srvbackup-evil/x"} {
		if _, err := s.resolveHostPath(p); err == nil {
			t.Errorf("path outside allowlist accepted: %q", p)
		}
	}
}

func TestSetHostFileAllowlistRejectsInvalidRoots(t *testing.T) {
	s := newHostPathTestServer()
	for _, bad := range []string{"relative/path", "/dup/../dup", "", "/null\x00byte"} {
		if err := s.SetHostFileAllowlist([]string{bad}); err == nil {
			t.Errorf("accepted invalid root %q", bad)
		}
	}
}

func TestResolveHostPathPrefixBoundary(t *testing.T) {
	s := newHostPathTestServer()
	_ = s.SetHostFileAllowlist([]string{"/srv/data"})
	// A sibling directory sharing the prefix must NOT pass.
	if _, err := s.resolveHostPath("/srv/data-evil/x"); err == nil {
		t.Error("prefix-sibling directory escaped allowlist")
	}
	if !strings.HasPrefix("/srv/data-evil", "/srv/data") {
		t.Fatal("test premise broken")
	}
}
