package daemon

import "testing"

// The compose deploy request used to carry per-request mount-allowlist and
// admin fields (AllowedMounts / IsAdmin) and the client exposed
// ComposeDeleteWithOptions(volumes, removeOrphans) query building. Both were
// removed by refactor: the allowlist is now delivered via
// store.NodeConfiguration.AllowedMounts (node boot config) and enforced in
// the compose service layer (see internal/services/compose tests), and
// ComposeDelete takes no delete options. These tests were deleted as the
// wire contract they pinned no longer exists; equivalent behavior is covered
// in internal/services/compose.

func TestComposeFixesRemovedContractSkipped(t *testing.T) {
	t.Skip("daemon ComposeDeployRequest no longer carries AllowedMounts/IsAdmin; ComposeDeleteWithOptions removed")
}
