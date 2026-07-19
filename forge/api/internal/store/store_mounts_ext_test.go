package store

import "testing"

func TestValidateMountPaths(t *testing.T) {
	valid := []struct {
		source string
		target string
	}{
		{source: "/srv/game-data", target: "/data"},
		{source: "/mnt/shared/maps", target: "/home/container/maps"},
	}
	for _, tc := range valid {
		if err := validateMountPaths(tc.source, tc.target); err != nil {
			t.Errorf("validateMountPaths(%q, %q) returned %v", tc.source, tc.target, err)
		}
	}

	invalid := []struct {
		name   string
		source string
		target string
	}{
		{name: "relative source", source: "srv/game-data", target: "/data"},
		{name: "relative target", source: "/srv/game-data", target: "data"},
		{name: "unclean source", source: "/srv/../etc", target: "/data"},
		{name: "unclean target", source: "/srv/game-data", target: "/data/../config"},
		{name: "backslash", source: "/srv\\game-data", target: "/data"},
		{name: "forge configuration source", source: "/etc/forge", target: "/data"},
		{name: "forge volumes source", source: "/var/lib/forge/volumes", target: "/data"},
		{name: "container root target", source: "/srv/game-data", target: "/home/container"},
		{name: "filesystem root target", source: "/srv/game-data", target: "/"},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateMountPaths(tc.source, tc.target); err == nil {
				t.Errorf("validateMountPaths(%q, %q) succeeded", tc.source, tc.target)
			}
		})
	}
}
