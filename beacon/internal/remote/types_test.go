package remote

import (
	"encoding/json"
	"testing"
)

func TestServerConfigurationResponseDecodesMounts(t *testing.T) {
	var response ServerConfigurationResponse
	if err := json.Unmarshal([]byte(`{"settings":{},"mounts":[{"source":"/srv/shared","target":"/mnt/shared","read_only":true}]}`), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Mounts) != 1 {
		t.Fatalf("mount count = %d, want 1", len(response.Mounts))
	}
	mount := response.Mounts[0]
	if mount.Source != "/srv/shared" || mount.Target != "/mnt/shared" || !mount.ReadOnly {
		t.Fatalf("unexpected mount: %#v", mount)
	}
}
