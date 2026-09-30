package appstore

import (
	"testing"

	"gamepanel/forge/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: this file previously drove the service through mock store/compose
// doubles injected via NewWithStore, covering per-user install ownership
// (store.AppStoreInstall.UserID + ErrInstallForbidden), uninstall force flags,
// ignore-upgrade gating, cross-version confirmation (isCrossVersion/parseMajor/
// ErrCrossVersionRequiresConfirm), and template-resolution fail-fast errors.
// The refactor removed all of that: Service now depends on the concrete
// *store.Store and *compose.Service (no mock seam), installs are scoped by
// project/environment instead of a user column, UninstallApp/UpgradeApp take
// only (ctx, installID), and resolveTemplate is a best-effort string
// substitution that never errors. InstallRequest.UserID survives only as a
// pass-through to compose.DeployRequest. What still exists is pinned below.

func TestNewRequiresConcreteDependencies(t *testing.T) {
	t.Run("nil store rejected", func(t *testing.T) {
		svc, err := New(nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "store required")
		assert.Nil(t, svc)
	})

	t.Run("nil compose service rejected", func(t *testing.T) {
		svc, err := New(&store.Store{}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "compose service required")
		assert.Nil(t, svc)
	})
}

func TestResolveTemplateIsBestEffortSubstitution(t *testing.T) {
	// Only the exact ${VAR} and ${VAR:-} forms are replaced; anything else
	// (default-value forms like ${TAG:-alpine}, fail-fast forms like
	// ${MISSING:?msg}) is left verbatim — the old ":?required" fail-fast
	// error contract is gone, the current signature has no error path.
	got := resolveTemplate("image: nginx:${TAG}\nshares: ${SHARES:-}\ndef: ${TAG:-alpine}\nmissing: ${MISSING:?need it}", map[string]string{
		"TAG":    "1.25",
		"SHARES": "512",
	})
	assert.Contains(t, got, "nginx:1.25")
	assert.Contains(t, got, "shares: 512")
	assert.Contains(t, got, "def: ${TAG:-alpine}")
	assert.Contains(t, got, "missing: ${MISSING:?need it}")

	// nil params returns the template untouched.
	assert.Equal(t, "a${TAG}b", resolveTemplate("a${TAG}b", nil))
}
