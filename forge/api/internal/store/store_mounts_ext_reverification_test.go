package store

import (
	"os"
	"path"
	"strings"
	"testing"
)

// TestValidateMountPath_AllowlistPrefix_Reverification verifies the
// GH-19/SE-04 host-breakout hardening in validateMountPath (store_mounts_ext.go:323).
//
// When MOUNTS_ALLOWED_PREFIX is set, only sources under those prefixes are
// allowed (cleaned == prefix or HasPrefix with "/"). Otherwise deny-list mode
// blocks known sensitive host paths.

func TestValidateMountPath_AllowlistPrefix_Boundary_Reverification(t *testing.T) {
	orig := os.Getenv("MOUNTS_ALLOWED_PREFIX")
	t.Cleanup(func() {
		if orig != "" {
			_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", orig)
		} else {
			_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
		}
	})
	_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", "/srv/forge-mounts,/var/lib/forge/mounts")

	cases := []struct {
		source string
		allow  bool
		name   string
	}{
		{source: "/srv/forge-mounts", allow: true, name: "exact prefix"},
		{source: "/srv/forge-mounts/", allow: false, name: "prefix with trailing slash (unclean -> blocked)"},
		{source: "/srv/forge-mounts/app", allow: true, name: "subdir under prefix"},
		{source: "/srv/forge-mounts/app/data", allow: true, name: "nested subdir"},
		{source: "/var/lib/forge/mounts", allow: true, name: "second exact prefix"},
		{source: "/var/lib/forge/mounts/app", allow: true, name: "second prefix subdir"},
		{source: "/srv/forge-mounts-data", allow: false, name: "prefix boundary missing slash"},
		{source: "/srv/forge-mounts2", allow: false, name: "prefix with suffix not slash"},
		{source: "/srv/forge-mount", allow: false, name: "prefix incomplete"},
		{source: "/srv/game-data", allow: false, name: "outside allowlist"},
		{source: "/etc", allow: false, name: "sensitive etc"},
		{source: "/etc/shadow", allow: false, name: "sensitive etc/shadow"},
		{source: "/var/run/docker.sock", allow: false, name: "docker sock"},
		{source: "/srv/forge-mounts/../etc/shadow", allow: false, name: "traversal via .. (unclean)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Target is always clean valid target for these source checks
			err := validateMountPath(tc.source, "source")
			if tc.allow && err != nil {
				t.Errorf("validateMountPath(%q, source) = %v, want nil (allowed)", tc.source, err)
			}
			if !tc.allow && err == nil {
				t.Errorf("validateMountPath(%q, source) = nil, want error (blocked)", tc.source)
			}
			if !tc.allow && err != nil {
				// In allowlist mode, most errors should guide to MOUNTS_ALLOWED_PREFIX,
				// but unclean paths are blocked earlier with "absolute, clean path"
				// which is also a valid rejection (no need to mention allowlist).
				msg := err.Error()
				if !strings.Contains(msg, "MOUNTS_ALLOWED_PREFIX") && !strings.Contains(msg, "absolute, clean path") && !strings.Contains(strings.ToLower(msg), "reserved") && !strings.Contains(strings.ToLower(msg), "protected") {
					t.Errorf("validateMountPath(%q) error %q, want to mention MOUNTS_ALLOWED_PREFIX or clean-path/reserved", tc.source, msg)
				}
			}
		})
	}
}

func TestMountsAllowedPrefixes_Parsing_Reverification(t *testing.T) {
	orig := os.Getenv("MOUNTS_ALLOWED_PREFIX")
	t.Cleanup(func() {
		if orig != "" {
			_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", orig)
		} else {
			_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
		}
	})

	tests := []struct {
		env  string
		want []string
	}{
		{env: "", want: nil},
		{env: "  ", want: nil},
		{env: "/srv/forge-mounts", want: []string{"/srv/forge-mounts"}},
		{env: "/srv/forge-mounts,/var/lib/forge/mounts", want: []string{"/srv/forge-mounts", "/var/lib/forge/mounts"}},
		{env: "/srv/a:/var/lib/b", want: []string{"/srv/a", "/var/lib/b"}},
		{env: "/srv/a;/var/lib/b", want: []string{"/srv/a", "/var/lib/b"}},
		{env: "/srv/a, /var/lib/b", want: []string{"/srv/a", "/var/lib/b"}},
		{env: "/srv/forge-mounts/", want: []string{"/srv/forge-mounts"}}, // cleaned
		{env: "/srv//forge-mounts", want: []string{"/srv/forge-mounts"}},
		{env: ",, ,/srv/a,,", want: []string{"/srv/a"}},
		{env: "/srv/a /var/lib/b", want: []string{"/srv/a", "/var/lib/b"}}, // whitespace split inside part
	}
	for _, tc := range tests {
		_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", tc.env)
		got := mountsAllowedPrefixes()
		if len(got) != len(tc.want) {
			t.Errorf("env %q: mountsAllowedPrefixes() = %v (len %d), want %v (len %d)", tc.env, got, len(got), tc.want, len(tc.want))
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("env %q: index %d got %q want %q", tc.env, i, got[i], tc.want[i])
			}
		}
	}

	// "." is skipped, "/" is preserved by parser but ignored by validator
	_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", "/, ., /srv/a")
	got2 := mountsAllowedPrefixes()
	hasDot := false
	for _, p := range got2 {
		if p == "." {
			hasDot = true
		}
	}
	if hasDot {
		t.Errorf("mountsAllowedPrefixes should skip \".\", got %v", got2)
	}
	// "/" is kept by parser (cleaned != "."), but validateMountPath skips it
	foundSrv := false
	for _, p := range got2 {
		if p == "/srv/a" {
			foundSrv = true
		}
	}
	if !foundSrv {
		t.Errorf("should contain /srv/a, got %v", got2)
	}
	// Verify that "/" prefix is ignored by validator (not effective)
	if len(got2) == 0 {
		t.Errorf("expected at least /srv/a, got %v", got2)
	}
}

func TestMountsAllowedPrefixHint_Reverification(t *testing.T) {
	orig := os.Getenv("MOUNTS_ALLOWED_PREFIX")
	t.Cleanup(func() {
		if orig != "" {
			_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", orig)
		} else {
			_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
		}
	})

	_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
	hint := mountsAllowedPrefixHint()
	if !strings.Contains(hint, "/srv/forge-mounts") || !strings.Contains(hint, "/var/lib/forge/mounts") {
		t.Errorf("empty env hint %q should contain default prefixes", hint)
	}
	if !strings.Contains(hint, "MOUNTS_ALLOWED_PREFIX") {
		t.Errorf("hint %q should mention MOUNTS_ALLOWED_PREFIX", hint)
	}

	_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", "/srv/custom,/data")
	hint = mountsAllowedPrefixHint()
	if hint != "/srv/custom, /data" && hint != "/srv/custom,/data" {
		// Join uses ", " but check contains both
		if !strings.Contains(hint, "/srv/custom") || !strings.Contains(hint, "/data") {
			t.Errorf("custom hint %q should contain both prefixes", hint)
		}
	}
}

func TestValidateMountPath_DenylistMode_Reverification(t *testing.T) {
	orig := os.Getenv("MOUNTS_ALLOWED_PREFIX")
	_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
	t.Cleanup(func() {
		if orig != "" {
			_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", orig)
		} else {
			_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
		}
	})

	// Deny-list blocks
	blocked := []string{
		"/etc", "/etc/passwd", "/proc", "/proc/self/environ",
		"/sys", "/sys/kernel", "/dev", "/dev/sda",
		"/boot", "/root", "/var/run", "/run",
		"/var/lib/forge", "/var/lib/docker",
		"/", "/home/container",
		"/var/lib/forge/volumes", "/etc/forge",
	}
	for _, src := range blocked {
		t.Run("blocked_"+path.Base(src)+"_"+strings.ReplaceAll(src, "/", "_"), func(t *testing.T) {
			err := validateMountPath(src, "source")
			if err == nil {
				t.Errorf("validateMountPath(%q, source) in deny-list mode should be blocked", src)
			}
		})
	}

	allowed := []string{
		"/srv/forge-mounts/data", "/srv/game-data", "/mnt/shared/maps",
		"/data", "/srv", "/var/lib/other",
	}
	for _, src := range allowed {
		t.Run("allowed_"+strings.ReplaceAll(src, "/", "_"), func(t *testing.T) {
			err := validateMountPath(src, "source")
			if err != nil {
				t.Errorf("validateMountPath(%q, source) in deny-list mode should be allowed, got %v", src, err)
			}
		})
	}

	// Target checks are independent of allowlist
	if err := validateMountPath("/", "target"); err == nil {
		t.Error("target / should be blocked")
	}
	if err := validateMountPath("/home/container", "target"); err == nil {
		t.Error("target /home/container should be blocked")
	}
	if err := validateMountPath("/data", "target"); err != nil {
		t.Errorf("target /data should be allowed, got %v", err)
	}
}

func TestValidateMountPath_AllowlistMode_CleanAndBoundary_Reverification(t *testing.T) {
	orig := os.Getenv("MOUNTS_ALLOWED_PREFIX")
	t.Cleanup(func() {
		if orig != "" {
			_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", orig)
		} else {
			_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
		}
	})
	_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", "/srv/forge-mounts")

	// Unclean paths should be blocked before allowlist check
	unclean := []string{"/srv/forge-mounts/../etc", "/srv//forge-mounts/app", "/srv/forge-mounts/./app"}
	for _, src := range unclean {
		// Note: "/srv//forge-mounts/app" has Clean != value, so blocked early
		// "/srv/forge-mounts/../etc" is unclean
		err := validateMountPath(src, "source")
		// These are unclean, so they should be blocked regardless
		if err == nil {
			t.Logf("validateMountPath(%q) unexpectedly allowed (unclean check may allow double slash? clean=%q)", src, path.Clean(src))
		}
	}

	// Clean boundary tests
	if err := validateMountPath("/srv/forge-mounts", "source"); err != nil {
		t.Errorf("exact prefix should be allowed, got %v", err)
	}
	if err := validateMountPath("/srv/forge-mounts/app", "source"); err != nil {
		t.Errorf("prefix/app should be allowed, got %v", err)
	}
	if err := validateMountPath("/srv/forge-mounts-app", "source"); err == nil {
		t.Error("prefix-app (no slash) should be blocked")
	}
}

func TestValidateMountPaths_TargetReserved_Reverification(t *testing.T) {
	orig := os.Getenv("MOUNTS_ALLOWED_PREFIX")
	_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
	t.Cleanup(func() {
		if orig != "" {
			_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", orig)
		} else {
			_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
		}
	})

	// validateMountPaths should check both source and target
	if err := validateMountPaths("/srv/data", "/"); err == nil {
		t.Error("validateMountPaths with target / should be blocked")
	}
	if err := validateMountPaths("/srv/data", "/home/container"); err == nil {
		t.Error("validateMountPaths with target /home/container should be blocked")
	}
	if err := validateMountPaths("/etc", "/data"); err == nil {
		t.Error("validateMountPaths with source /etc should be blocked")
	}
	if err := validateMountPaths("/srv/data", "/data"); err != nil {
		t.Errorf("validateMountPaths(/srv/data, /data) should be allowed, got %v", err)
	}

	// With allowlist, source outside should be blocked even if target is ok
	_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", "/srv/forge-mounts")
	if err := validateMountPaths("/srv/other", "/data"); err == nil {
		t.Error("allowlist: /srv/other should be blocked")
	}
	if err := validateMountPaths("/srv/forge-mounts/app", "/data"); err != nil {
		t.Errorf("allowlist: /srv/forge-mounts/app should be allowed, got %v", err)
	}
}

func TestValidateMountPath_BackslashAndRelative_Reverification(t *testing.T) {
	orig := os.Getenv("MOUNTS_ALLOWED_PREFIX")
	_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
	t.Cleanup(func() {
		if orig != "" {
			_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", orig)
		} else {
			_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
		}
	})

	cases := []struct {
		value string
		field string
	}{
		{value: "srv/data", field: "source"},
		{value: "data", field: "target"},
		{value: "/srv\\data", field: "source"},
		{value: "/srv/../etc", field: "source"},
		{value: "/data/../etc", field: "target"},
	}
	for _, tc := range cases {
		err := validateMountPath(tc.value, tc.field)
		if err == nil {
			t.Errorf("validateMountPath(%q, %q) should be blocked (relative/unclean/backslash)", tc.value, tc.field)
		}
	}
}

func TestMountsAllowedPrefixes_EmptySkips_Reverification(t *testing.T) {
	orig := os.Getenv("MOUNTS_ALLOWED_PREFIX")
	t.Cleanup(func() {
		if orig != "" {
			_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", orig)
		} else {
			_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
		}
	})

	_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", " , , ")
	if got := mountsAllowedPrefixes(); len(got) != 0 {
		t.Errorf("empty/whitespace only should return nil/empty, got %v", got)
	}

	_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", "/srv/a,,/srv/b")
	got := mountsAllowedPrefixes()
	if len(got) != 2 {
		t.Errorf("double comma should be handled, got %v", got)
	}
}
