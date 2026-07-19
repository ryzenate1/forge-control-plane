package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gamepanel/forge/internal/daemon"
)

func TestDockerAdapterPropagatesResourceContractWithoutAliasingShares(t *testing.T) {
	var captured daemon.CreateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(daemon.CreateResponse{ServerID: captured.ServerID, Accepted: true})
	}))
	defer server.Close()
	adapter := NewDockerAdapter(daemon.NewClient())
	_, err := adapter.CreateServer(context.Background(), Target{NodeURL: server.URL, NodeToken: "token", ServerID: "server"}, CreateServerRequest{
		Image: "image", CPUShares: 512, CPULimit: 250, SwapMB: 256, IOWeight: 600, Threads: "0-1", OOMDisabled: true,
		PIDLimit: 64, StopSignal: "SIGINT", StopTimeout: 45, UID: 1000, GID: 1001, DNS: []string{"1.1.1.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.CPUShares != 512 || captured.CPUPercent != 250 {
		t.Fatalf("shares/percentage were aliased: %+v", captured)
	}
	if captured.NetworkName != "gamepanel" || captured.SwapMB != 256 || captured.IOWeight != 600 || captured.PIDLimit != 64 {
		t.Fatalf("resource contract was not propagated: %+v", captured)
	}
	if captured.StopSignal != "SIGINT" || captured.StopTimeout != 45 || captured.UID != 1000 || captured.GID != 1001 {
		t.Fatalf("lifecycle/identity contract was not propagated: %+v", captured)
	}
}
