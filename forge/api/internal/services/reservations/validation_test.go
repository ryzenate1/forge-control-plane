package reservations

import (
	"context"
	"testing"

	"gamepanel/forge/internal/store"
)

func TestValidateReservationRequest(t *testing.T) {
	cases := []struct {
		name    string
		req     store.CreatePlacementReservationRequest
		wantErr bool
	}{
		{"valid", store.CreatePlacementReservationRequest{NodeID: "n-1", CPU: 100, Memory: 256, Disk: 512}, false},
		{"partial resources allowed", store.CreatePlacementReservationRequest{NodeID: "n-1", CPU: 100}, false},
		{"missing node", store.CreatePlacementReservationRequest{CPU: 100}, true},
		{"negative cpu", store.CreatePlacementReservationRequest{NodeID: "n-1", CPU: -1, Memory: 256, Disk: 512}, true},
		{"negative memory", store.CreatePlacementReservationRequest{NodeID: "n-1", CPU: 100, Memory: -5, Disk: 512}, true},
		{"claims nothing", store.CreatePlacementReservationRequest{NodeID: "n-1"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateReservationRequest(tc.req); (err != nil) != tc.wantErr {
				t.Fatalf("ValidateReservationRequest() = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestManagerCreateReservationRejectsEmpty(t *testing.T) {
	m := New(nil)
	_, err := m.CreateReservation(context.Background(), store.CreatePlacementReservationRequest{NodeID: "n-1"})
	if err == nil {
		t.Fatal("manager must reject a reservation that claims no capacity")
	}
}
