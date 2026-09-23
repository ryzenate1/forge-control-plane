package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

// TenantID/OrgID/ProjectID were removed from PlacementRequest and TenantID
// from PlacementDecision by the placement refactor (tenancy is resolved at
// the HTTP/store layer, not carried in placement DTOs). These tests pin the
// post-refactor contract: placement types no longer serialize tenant fields.

func TestPlacementRequest_HasNoTenantFields(t *testing.T) {
	req := PlacementRequest{
		ServerID:   "srv-1",
		RegionID:   "region-1",
		CPU:        1024,
		MemoryMB:   2048,
		DiskMB:     10240,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	jsonText := string(data)
	for _, key := range []string{"tenantId", "orgId", "projectId"} {
		if strings.Contains(jsonText, `"`+key+`"`) {
			t.Fatalf("placement request must not carry %q after refactor, got %s", key, jsonText)
		}
	}
	var decoded PlacementRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.RegionID != "region-1" || decoded.CPU != 1024 || decoded.MemoryMB != 2048 || decoded.DiskMB != 10240 {
		t.Fatalf("json round trip lost fields: %+v", decoded)
	}
}

func TestPlacementDecision_HasNoTenantField(t *testing.T) {
	dec := PlacementDecision{
		NodeID:   "node-1",
		RegionID: "region-1",
		Score:    0.5,
		Manual:   false,
	}
	data, err := json.Marshal(dec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "tenantId") {
		t.Fatalf("placement decision must not carry tenantId, got %s", data)
	}
	var decoded PlacementDecision
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.NodeID != "node-1" || decoded.RegionID != "region-1" {
		t.Fatalf("round trip mismatch: %+v", decoded)
	}
}
