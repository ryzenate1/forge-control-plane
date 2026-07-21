package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg == nil {
		t.Fatal("Default() returned nil")
	}
	if cfg.Debug != false {
		t.Error("Debug should default to false")
	}
	if cfg.System.API.Host != "0.0.0.0" {
		t.Errorf("API.Host = %q, want %q", cfg.System.API.Host, "0.0.0.0")
	}
	if cfg.System.API.Port != 9090 {
		t.Errorf("API.Port = %d, want %d", cfg.System.API.Port, 9090)
	}
	if cfg.System.Sftp.Address != "0.0.0.0" {
		t.Errorf("SFTP.Address = %q, want %q", cfg.System.Sftp.Address, "0.0.0.0")
	}
	if cfg.System.Sftp.Port != 2022 {
		t.Errorf("SFTP.Port = %d, want %d", cfg.System.Sftp.Port, 2022)
	}
	if cfg.System.DataDirectory != "/srv/game-panel/servers" {
		t.Errorf("DataDirectory = %q, want %q", cfg.System.DataDirectory, "/srv/game-panel/servers")
	}
	if cfg.AllowedMounts == nil {
		t.Error("AllowedMounts should be non-nil")
	}
	if cfg.AllowedOrigins == nil {
		t.Error("AllowedOrigins should be non-nil")
	}
	if cfg.RemoteQuery == nil {
		t.Error("RemoteQuery should be non-nil")
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() failed for valid config: %v", err)
	}
}

func TestValidate_EmptyAPIHost(t *testing.T) {
	cfg := Default()
	cfg.System.API.Host = ""
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject empty API host")
	}
}

func TestValidate_InvalidPort_Zero(t *testing.T) {
	cfg := Default()
	cfg.System.API.Port = 0
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject port 0")
	}
}

func TestValidate_InvalidPort_TooHigh(t *testing.T) {
	cfg := Default()
	cfg.System.API.Port = 70000
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject port 70000")
	}
}

func TestValidate_EmptySFTPAddress(t *testing.T) {
	cfg := Default()
	cfg.System.Sftp.Address = ""
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject empty SFTP address")
	}
}

func TestValidate_EmptyDataDirectory(t *testing.T) {
	cfg := Default()
	cfg.System.DataDirectory = ""
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject empty data directory")
	}
}

func TestValidate_TLSWithoutCert(t *testing.T) {
	cfg := Default()
	cfg.System.API.TLS.Enabled = true
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject TLS enabled without cert/key")
	}
}

func TestValidate_TLSWithNonExistentCert(t *testing.T) {
	cfg := Default()
	cfg.System.API.TLS.Enabled = true
	cfg.System.API.TLS.CertFile = "/nonexistent/cert.pem"
	cfg.System.API.TLS.KeyFile = "/nonexistent/key.pem"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject TLS with non-existent cert file")
	}
}

func TestValidate_TLSWithValidCert(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	if err := os.WriteFile(certFile, []byte("fake cert"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte("fake key"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := Default()
	cfg.System.API.TLS.Enabled = true
	cfg.System.API.TLS.CertFile = certFile
	cfg.System.API.TLS.KeyFile = keyFile

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should accept valid TLS config: %v", err)
	}
}

func TestValidate_InvalidUUID(t *testing.T) {
	cfg := Default()
	cfg.UUID = "not-a-uuid"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject invalid UUID format")
	}
}

func TestValidate_ValidUUID(t *testing.T) {
	cfg := Default()
	cfg.UUID = "550e8400-e29b-41d4-a716-446655440000"
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should accept valid UUID: %v", err)
	}
}

func TestValidate_InvalidPanelURL(t *testing.T) {
	cfg := Default()
	cfg.PanelURL = "ftp://panel.example.com"
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject unsupported panel_url schemes")
	}
}

func TestValidate_InvalidAllowedOrigin(t *testing.T) {
	cfg := Default()
	cfg.AllowedOrigins = []string{"not-a-url"}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject invalid allowed origins")
	}
}

func TestValidate_WildcardAllowedOrigin(t *testing.T) {
	cfg := Default()
	cfg.AllowedOrigins = []string{"*"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should accept wildcard allowed origin: %v", err)
	}
}

func TestValidate_InvalidRemoteQueryPort(t *testing.T) {
	cfg := Default()
	cfg.RemoteQuery = map[string]int{"server1": 70000}
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject invalid remote_query ports")
	}
}

func TestValidate_DataAndTempDirectorySamePath(t *testing.T) {
	cfg := Default()
	cfg.System.TempDirectory = cfg.System.DataDirectory
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should reject matching data and temp directories")
	}
}

func TestLoad_YAMLFile(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "config.yml")

	yamlContent := `debug: true
uuid: 550e8400-e29b-41d4-a716-446655440000
token_id: test-token-id
token: test-token
panel_url: https://panel.example.com
remote: https://remote.example.com
system:
  data_directory: /custom/data
  temp_directory: /custom/tmp
  sftp:
    bind_address: 127.0.0.1
    bind_port: 2023
    read_only: true
  api:
    host: 127.0.0.1
    port: 8080
    tls:
      enabled: false
docker:
  timezone: America/New_York
  network:
    interface: eth1
allowed_mounts:
  - /mnt/game1
  - /mnt/game2
allowed_origins:
  - https://example.com
remote_query:
  server1: 25565
  server2: 25566
crash_detection:
  detect_clean_exit_as_crash: true
`
	if err := os.WriteFile(yamlFile, []byte(yamlContent), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWithOptions(LoadOptions{Path: yamlFile})
	if err != nil {
		t.Fatalf("LoadWithOptions() failed: %v", err)
	}

	if !cfg.Debug {
		t.Error("Debug should be true")
	}
	if cfg.UUID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("UUID = %q, want %q", cfg.UUID, "550e8400-e29b-41d4-a716-446655440000")
	}
	if cfg.System.DataDirectory != "/custom/data" {
		t.Errorf("DataDirectory = %q, want %q", cfg.System.DataDirectory, "/custom/data")
	}
	if cfg.System.API.Port != 8080 {
		t.Errorf("API.Port = %d, want %d", cfg.System.API.Port, 8080)
	}
	if cfg.System.Sftp.Port != 2023 {
		t.Errorf("SFTP.Port = %d, want %d", cfg.System.Sftp.Port, 2023)
	}
	if len(cfg.AllowedMounts) != 2 {
		t.Errorf("AllowedMounts length = %d, want %d", len(cfg.AllowedMounts), 2)
	}
	if cfg.Docker.Timezone != "America/New_York" {
		t.Errorf("Docker.Timezone = %q, want %q", cfg.Docker.Timezone, "America/New_York")
	}
	if !cfg.CrashDetection.DetectCleanExitAsCrash {
		t.Error("CrashDetection.DetectCleanExitAsCrash should be true")
	}
}

func TestLoadFromSources_NoFileNoEnv(t *testing.T) {
	os.Unsetenv("DAEMON_SYSTEM_API_PORT")
	os.Unsetenv("DAEMON_SYSTEM_API_HOST")

	cfg, err := LoadWithOptions(LoadOptions{EnvPrefix: EnvPrefix})
	if err != nil {
		t.Fatalf("LoadWithOptions() failed: %v", err)
	}

	if cfg.System.API.Port != 9090 {
		t.Errorf("API.Port = %d, want default %d", cfg.System.API.Port, 9090)
	}
	if cfg.System.API.Host != "0.0.0.0" {
		t.Errorf("API.Host = %q, want default %q", cfg.System.API.Host, "0.0.0.0")
	}
}

func TestLoadFromSources_WithYAMLFile(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "config.yml")

	yamlContent := `system:
  api:
    port: 8080
  data_directory: /custom/data
`
	if err := os.WriteFile(yamlFile, []byte(yamlContent), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWithOptions(LoadOptions{Path: yamlFile, EnvPrefix: EnvPrefix})
	if err != nil {
		t.Fatalf("LoadWithOptions() failed: %v", err)
	}

	if cfg.System.API.Port != 8080 {
		t.Errorf("API.Port = %d, want %d", cfg.System.API.Port, 8080)
	}
	if cfg.System.DataDirectory != "/custom/data" {
		t.Errorf("DataDirectory = %q, want %q", cfg.System.DataDirectory, "/custom/data")
	}
	if cfg.System.API.Host != "0.0.0.0" {
		t.Errorf("API.Host = %q, want default %q", cfg.System.API.Host, "0.0.0.0")
	}
}

func TestLoadFromSources_EnvOverridesFile(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "config.yml")

	yamlContent := `system:
  api:
    port: 8080
`
	if err := os.WriteFile(yamlFile, []byte(yamlContent), 0600); err != nil {
		t.Fatal(err)
	}

	os.Setenv("DAEMON_SYSTEM_API_PORT", "9999")
	defer os.Unsetenv("DAEMON_SYSTEM_API_PORT")

	cfg, err := LoadWithOptions(LoadOptions{Path: yamlFile, EnvPrefix: EnvPrefix})
	if err != nil {
		t.Fatalf("LoadWithOptions() failed: %v", err)
	}

	if cfg.System.API.Port != 9999 {
		t.Errorf("API.Port = %d, want env override %d", cfg.System.API.Port, 9999)
	}
}

func TestLoadFromSources_InvalidConfig(t *testing.T) {
	os.Setenv("DAEMON_SYSTEM_API_PORT", "70000")
	defer os.Unsetenv("DAEMON_SYSTEM_API_PORT")

	_, err := LoadWithOptions(LoadOptions{EnvPrefix: EnvPrefix})
	if err == nil {
		t.Error("LoadWithOptions() should reject invalid config via env var")
	}
}

func TestLoadWithOptions_FlagsOverrideEnvAndFile(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "config.yml")

	yamlContent := `system:
  api:
    port: 8080
`
	if err := os.WriteFile(yamlFile, []byte(yamlContent), 0600); err != nil {
		t.Fatal(err)
	}

	os.Setenv("DAEMON_SYSTEM_API_PORT", "9091")
	defer os.Unsetenv("DAEMON_SYSTEM_API_PORT")

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Int("system.api.port", 9999, "api port")
	if err := flags.Set("system.api.port", "7070"); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWithOptions(LoadOptions{Path: yamlFile, EnvPrefix: EnvPrefix, Flags: flags})
	if err != nil {
		t.Fatalf("LoadWithOptions() failed: %v", err)
	}
	if cfg.System.API.Port != 7070 {
		t.Errorf("API.Port = %d, want flag override %d", cfg.System.API.Port, 7070)
	}
}

func TestConfigEntryValue(t *testing.T) {
	v := viperWithDefaultsForTest()
	v.Set(SystemAPIPortEntry.Key, "9095")

	port, err := SystemAPIPortEntry.Value(v)
	if err != nil {
		t.Fatalf("ConfigEntry.Value() failed: %v", err)
	}
	if port != 9095 {
		t.Errorf("ConfigEntry.Value() = %d, want %d", port, 9095)
	}
}

func viperWithDefaultsForTest() *viper.Viper {
	v := viper.New()
	applyDefaults(v)
	return v
}

func TestAccessors_AllowedMountsList(t *testing.T) {
	cfg := Default()
	cfg.AllowedMounts = []string{"/mnt/a", "/mnt/b"}
	setGlobal(cfg)

	mounts := cfg.AllowedMountsList()
	if len(mounts) != 2 {
		t.Errorf("AllowedMountsList() length = %d, want %d", len(mounts), 2)
	}

	mounts = append(mounts, "/mnt/c")
	if len(cfg.AllowedMountsList()) != 2 {
		t.Error("AllowedMountsList() should return defensive copy")
	}
}

func TestAccessors_BackupConfig(t *testing.T) {
	cfg := Default()
	setGlobal(cfg)

	bc := cfg.BackupConfig()
	if bc.WriteLimit != 0 {
		t.Errorf("BackupConfig().WriteLimit = %d, want %d", bc.WriteLimit, 0)
	}
}
