package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestMasterKeyringFromEnvironment(t *testing.T) {
	t.Run("production requires key", func(t *testing.T) {
		t.Setenv("FORGE_MASTER_KEY", "")
		t.Setenv("FORGE_ALLOW_EPHEMERAL_MASTER_KEY", "false")
		if _, _, err := masterKeyringFromEnvironment(true); err == nil {
			t.Fatal("production accepted missing master key")
		}
	})
	t.Run("invalid key rejected before database use", func(t *testing.T) {
		t.Setenv("FORGE_MASTER_KEY", "weak-auth-secret")
		if _, _, err := masterKeyringFromEnvironment(false); err == nil {
			t.Fatal("invalid master key accepted")
		}
	})
	t.Run("active and previous keys", func(t *testing.T) {
		key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
		old := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("b", 32)))
		t.Setenv("FORGE_MASTER_KEY", key)
		t.Setenv("FORGE_MASTER_KEY_ID", "new")
		t.Setenv("FORGE_PREVIOUS_MASTER_KEYS", "old="+old)
		ring, ephemeral, err := masterKeyringFromEnvironment(true)
		if err != nil || ephemeral || ring.ActiveKeyID() != "new" {
			t.Fatalf("keyring = %v, ephemeral=%v, err=%v", ring, ephemeral, err)
		}
	})
}

func TestHealthcheckUsesReadinessEndpoint(t *testing.T) {
	if readinessHealthPath != "/api/v1/health/ready" {
		t.Fatalf("healthcheck path = %q, want readiness endpoint", readinessHealthPath)
	}
}

func TestDemoSeedEnabled(t *testing.T) {
	tests := []struct {
		name    string
		appEnv  string
		value   string
		want    bool
		wantErr bool
	}{
		{name: "default disabled", appEnv: "development", value: "", want: false},
		{name: "explicitly disabled", appEnv: "development", value: "false", want: false},
		{name: "enabled outside production", appEnv: "development", value: "true", want: true},
		{name: "production remains disabled", appEnv: "production", value: "false", want: false},
		{name: "production enable is rejected", appEnv: "production", value: "true", wantErr: true},
		{name: "production check is case insensitive", appEnv: " Production ", value: "TRUE", wantErr: true},
		{name: "invalid value is rejected", appEnv: "development", value: "sometimes", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := demoSeedEnabled(tt.appEnv, tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("demoSeedEnabled() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("demoSeedEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
