package http

import (
	"encoding/json"
	"reflect"
	"testing"

	"gamepanel/forge/internal/store"
)

func TestUpdateNodeRequest_OmittedFieldsRemainNil(t *testing.T) {
	var request UpdateNodeRequest
	if err := json.Unmarshal([]byte(`{"name":"updated-node"}`), &request); err != nil {
		t.Fatalf("unmarshal PATCH request: %v", err)
	}

	if request.Name == nil || *request.Name != "updated-node" {
		t.Fatalf("expected name pointer to updated-node, got %#v", request.Name)
	}

	for _, field := range []struct {
		name  string
		value any
	}{
		{"description", request.Description},
		{"locationId", request.LocationID},
		{"baseUrl", request.BaseURL},
		{"fqdn", request.FQDN},
		{"scheme", request.Scheme},
		{"behindProxy", request.BehindProxy},
		{"maintenanceMode", request.Maintenance},
		{"desiredState", request.DesiredState},
		{"draining", request.Draining},
		{"memoryMb", request.MemoryMB},
		{"diskMb", request.DiskMB},
		{"uploadSizeMb", request.UploadSizeMB},
		{"daemonBase", request.DaemonBase},
		{"daemonListen", request.DaemonListen},
		{"daemonSftp", request.DaemonSFTP},
		{"status", request.Status},
	} {
		if !reflect.ValueOf(field.value).IsNil() {
			t.Errorf("expected omitted %s to remain nil, got %#v", field.name, field.value)
		}
	}
}

func TestUpdateNodeRequest_ExplicitZeroValuesRemainPresent(t *testing.T) {
	var request UpdateNodeRequest
	body := []byte(`{
		"name":"",
		"description":"",
		"locationId":"",
		"baseUrl":"",
		"fqdn":"",
		"scheme":"",
		"behindProxy":false,
		"maintenanceMode":false,
		"desiredState":"maintenance",
		"draining":false,
		"memoryMb":0,
		"diskMb":0,
		"uploadSizeMb":0,
		"daemonBase":"",
		"daemonListen":0,
		"daemonSftp":0,
		"status":""
	}`)
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatalf("unmarshal PATCH request: %v", err)
	}

	if request.Name == nil || *request.Name != "" ||
		request.Description == nil || *request.Description != "" ||
		request.LocationID == nil || *request.LocationID != "" ||
		request.BaseURL == nil || *request.BaseURL != "" ||
		request.FQDN == nil || *request.FQDN != "" ||
		request.Scheme == nil || *request.Scheme != "" ||
		request.DaemonBase == nil || *request.DaemonBase != "" ||
		request.Status == nil || *request.Status != "" {
		t.Fatal("expected explicit empty string fields to remain present")
	}

	if request.BehindProxy == nil || *request.BehindProxy ||
		request.Maintenance == nil || *request.Maintenance ||
		request.Draining == nil || *request.Draining {
		t.Fatal("expected explicit false boolean fields to remain present")
	}

	if request.MemoryMB == nil || *request.MemoryMB != 0 ||
		request.DiskMB == nil || *request.DiskMB != 0 ||
		request.UploadSizeMB == nil || *request.UploadSizeMB != 0 ||
		request.DaemonListen == nil || *request.DaemonListen != 0 ||
		request.DaemonSFTP == nil || *request.DaemonSFTP != 0 {
		t.Fatal("expected explicit zero numeric fields to remain present")
	}

	if request.DesiredState == nil || *request.DesiredState != store.NodeDesiredStateMaintenance {
		t.Fatalf("expected desired state %q, got %#v", store.NodeDesiredStateMaintenance, request.DesiredState)
	}
}
