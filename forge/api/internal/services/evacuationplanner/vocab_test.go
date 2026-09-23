package evacuationplanner

import (
	"context"
	"testing"

	"gamepanel/forge/internal/store"
)

// NOTE: the locality-normalisation layer is gone. canonicalStorageLocality,
// isLocalOnlyLocality and the StorageLocalOnlyLegacy alias were deleted, so
// "local"/"LOCAL_ONLY" style inputs are no longer folded onto the canonical
// value: the enum is exactly three wire strings and every comparison is a
// direct, case-sensitive equality check. The surviving vocabulary is pinned
// below.

func TestStorageLocalityConstants(t *testing.T) {
	if StorageLocalOnly != "local_only" {
		t.Errorf("StorageLocalOnly = %q, want local_only", StorageLocalOnly)
	}
	if StorageShared != "shared" {
		t.Errorf("StorageShared = %q, want shared", StorageShared)
	}
	if StorageReplicated != "replicated" {
		t.Errorf("StorageReplicated = %q, want replicated", StorageReplicated)
	}
}

func TestIsNetworkStorage(t *testing.T) {
	network := []string{"nfs://server/export", "smb://host/share", "sshfs://user@host:/data", "user@host:/data"}
	for _, src := range network {
		if !isNetworkStorage(src) {
			t.Errorf("isNetworkStorage(%q) = false, want true", src)
		}
	}
	local := []string{"/var/lib/forge/volumes/db", "bind-src", "host:path", "", "/"}
	for _, src := range local {
		if isNetworkStorage(src) {
			t.Errorf("isNetworkStorage(%q) = true, want false", src)
		}
	}
}

type stubMountStore struct {
	mounts []store.ServerMount
	err    error
}

func (s *stubMountStore) ServerMounts(context.Context, string) ([]store.ServerMount, error) {
	return s.mounts, s.err
}

func TestServiceStorageLocality(t *testing.T) {
	ctx := context.Background()

	// Without a mount store every workload is assumed migratable.
	if got, err := New(nil, nil).StorageLocality(ctx, "srv"); err != nil || got != StorageReplicated {
		t.Fatalf("no mount store: got %q (err=%v), want replicated", got, err)
	}

	svc := New(nil, nil)
	svc.SetServerMountStore(&stubMountStore{})
	if got, err := svc.StorageLocality(ctx, "srv"); err != nil || got != StorageReplicated {
		t.Fatalf("no mounts: got %q (err=%v), want replicated", got, err)
	}

	svc.SetServerMountStore(&stubMountStore{mounts: []store.ServerMount{{Source: "/data/vol1"}}})
	if got, _ := svc.StorageLocality(ctx, "srv"); got != StorageLocalOnly {
		t.Fatalf("writable bind mount: got %q, want local_only", got)
	}

	svc.SetServerMountStore(&stubMountStore{mounts: []store.ServerMount{{Source: "nfs://fileserver/export"}}})
	if got, _ := svc.StorageLocality(ctx, "srv"); got != StorageShared {
		t.Fatalf("network mount: got %q, want shared", got)
	}

	// Read-only mounts are skipped entirely.
	svc.SetServerMountStore(&stubMountStore{mounts: []store.ServerMount{{Source: "/etc/ssl", ReadOnly: true}}})
	if got, _ := svc.StorageLocality(ctx, "srv"); got != StorageReplicated {
		t.Fatalf("read-only mount: got %q, want replicated", got)
	}

	// A mount-store failure degrades to replicated (fail-open for scheduling).
	svc.SetServerMountStore(&stubMountStore{err: context.DeadlineExceeded})
	if _, err := svc.StorageLocality(ctx, "srv"); err == nil {
		t.Fatal("expected mount store error to propagate")
	}
}

func TestReplacementPolicyForServer(t *testing.T) {
	svc := New(nil, nil)
	cases := map[StorageLocality]ReplacementPolicy{
		StorageLocalOnly:  ReplacementPolicyProtect,
		StorageShared:     ReplacementPolicyAutoReplace,
		StorageReplicated: ReplacementPolicyAutoReplace,
		// Unnormalised/unknown values fall through to auto-replace; there is no
		// canonicalisation step left to rescue them.
		StorageLocality("local"): ReplacementPolicyAutoReplace,
	}
	for locality, want := range cases {
		if got := svc.ReplacementPolicyForServer(context.Background(), store.Server{}, locality); got != want {
			t.Errorf("ReplacementPolicyForServer(%q) = %q, want %q", locality, got, want)
		}
	}
}
