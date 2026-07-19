package models

import (
	"testing"
)

func TestAuditLogValidate(t *testing.T) {
	tests := []struct {
		name    string
		log     AuditLog
		wantErr bool
	}{
		{
			name:    "valid audit log",
			log:     AuditLog{UserID: "user-1", Action: "create", ResourceType: "server"},
			wantErr: false,
		},
		{
			name:    "missing user_id",
			log:     AuditLog{Action: "create", ResourceType: "server"},
			wantErr: true,
		},
		{
			name:    "missing action",
			log:     AuditLog{UserID: "user-1", ResourceType: "server"},
			wantErr: true,
		},
		{
			name:    "missing resource_type",
			log:     AuditLog{UserID: "user-1", Action: "create"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.log.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAuditLogTableName(t *testing.T) {
	log := AuditLog{}
	if got := log.TableName(); got != "audit_logs" {
		t.Errorf("TableName() = %v, want audit_logs", got)
	}
}

func TestJSONMapScanNil(t *testing.T) {
	var j JSONMap
	if err := j.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error = %v", err)
	}
	if j != nil {
		t.Errorf("expected nil, got %v", j)
	}
}

func TestJSONMapScanBytes(t *testing.T) {
	var j JSONMap
	if err := j.Scan([]byte(`{"key":"value"}`)); err != nil {
		t.Fatalf("Scan error = %v", err)
	}
	if j["key"] != "value" {
		t.Errorf("expected key=value, got %v", j)
	}
}

func TestJSONMapValue(t *testing.T) {
	j := JSONMap{"a": float64(1)}
	v, err := j.Value()
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	if v == nil {
		t.Fatal("expected non-nil value")
	}
}

func TestJSONMapValueNil(t *testing.T) {
	var j JSONMap
	v, err := j.Value()
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}
	if v != nil {
		t.Errorf("expected nil, got %v", v)
	}
}
