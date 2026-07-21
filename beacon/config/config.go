package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const EnvPrefix = "DAEMON"

type ConfigEntry[T any] struct {
	Key         string
	Default     T
	Description string
}

func (e ConfigEntry[T]) ApplyDefault(v *viper.Viper) {
	v.SetDefault(e.Key, e.Default)
}

func (e ConfigEntry[T]) Value(v *viper.Viper) (T, error) {
	var out T
	if err := mapstructure.WeakDecode(v.Get(e.Key), &out); err != nil {
		return out, fmt.Errorf("decode %s: %w", e.Key, err)
	}
	return out, nil
}

var (
	DebugEntry                       = ConfigEntry[bool]{Key: "debug", Default: false, Description: "enable debug logging and development behaviour"}
	SystemDataDirectoryEntry         = ConfigEntry[string]{Key: "system.data_directory", Default: "/srv/game-panel/servers", Description: "server data root directory"}
	SystemTempDirectoryEntry         = ConfigEntry[string]{Key: "system.temp_directory", Default: "/tmp", Description: "temporary file directory"}
	SystemSFTPBindAddressEntry       = ConfigEntry[string]{Key: "system.sftp.bind_address", Default: "0.0.0.0", Description: "SFTP bind address"}
	SystemSFTPBindPortEntry          = ConfigEntry[int]{Key: "system.sftp.bind_port", Default: 2022, Description: "SFTP bind port"}
	SystemSFTPReadOnlyEntry          = ConfigEntry[bool]{Key: "system.sftp.read_only", Default: false, Description: "run SFTP in read-only mode"}
	SystemAPIHostEntry               = ConfigEntry[string]{Key: "system.api.host", Default: "0.0.0.0", Description: "API bind address"}
	SystemAPIPortEntry               = ConfigEntry[int]{Key: "system.api.port", Default: 9090, Description: "API bind port"}
	SystemAPITLSEnabledEntry         = ConfigEntry[bool]{Key: "system.api.tls.enabled", Default: false, Description: "enable TLS for the API listener"}
	DockerTimezoneEntry              = ConfigEntry[string]{Key: "docker.timezone", Default: "UTC", Description: "timezone used for runtime containers"}
	DockerNetworkInterfaceEntry      = ConfigEntry[string]{Key: "docker.network.interface", Default: "eth0", Description: "network interface used for runtime networking"}
	CrashDetectCleanExitAsCrashEntry = ConfigEntry[bool]{Key: "crash_detection.detect_clean_exit_as_crash", Default: false, Description: "treat clean server exits as crashes"}

	DockerMemoryOverheadEntry = ConfigEntry[float64]{Key: "docker.memory_overhead", Default: 10.0, Description: "default memory overhead percentage added to server memory limits"}
	DockerRootlessEnabledEntry = ConfigEntry[bool]{Key: "docker.rootless_enabled", Default: false, Description: "enable rootless Docker mode (userns=host)"}
	BackupWriteLimitEntry = ConfigEntry[int64]{Key: "backup.write_limit", Default: 0, Description: "backup I/O write limit in bytes/sec (0 = unlimited)"}
	LogMaxSizeEntry = ConfigEntry[string]{Key: "log.max_size", Default: "10m", Description: "Docker log driver max-size"}
	LogMaxFileEntry = ConfigEntry[int]{Key: "log.max_file", Default: 3, Description: "Docker log driver max-file"}
)

var typedEntries = []interface{ ApplyDefault(*viper.Viper) }{
	DebugEntry,
	SystemDataDirectoryEntry,
	SystemTempDirectoryEntry,
	SystemSFTPBindAddressEntry,
	SystemSFTPBindPortEntry,
	SystemSFTPReadOnlyEntry,
	SystemAPIHostEntry,
	SystemAPIPortEntry,
	SystemAPITLSEnabledEntry,
	DockerTimezoneEntry,
	DockerNetworkInterfaceEntry,
	CrashDetectCleanExitAsCrashEntry,
	DockerMemoryOverheadEntry,
	DockerRootlessEnabledEntry,
	BackupWriteLimitEntry,
	LogMaxSizeEntry,
	LogMaxFileEntry,
}

type SftpConfiguration struct {
	Address  string `default:"0.0.0.0" yaml:"bind_address"`
	Port     int    `default:"2022" yaml:"bind_port"`
	ReadOnly bool   `default:"false" yaml:"read_only"`
}

type TLSConfiguration struct {
	Enabled  bool   `default:"false" yaml:"enabled"`
	CertFile string `default:"" yaml:"cert_file"`
	KeyFile  string `default:"" yaml:"key_file"`
}

type ApiConfiguration struct {
	Host string           `default:"0.0.0.0" yaml:"host"`
	Port int              `default:"9090" yaml:"port"`
	TLS  TLSConfiguration `yaml:"tls"`
}

type SystemConfiguration struct {
	DataDirectory string            `default:"/srv/game-panel/servers" yaml:"data_directory"`
	TempDirectory string            `default:"/tmp" yaml:"temp_directory"`
	Sftp          SftpConfiguration `yaml:"sftp"`
	API           ApiConfiguration  `yaml:"api"`
}

type DockerConfiguration struct {
	Network struct {
		Interface string `default:"eth0" yaml:"interface"`
	} `yaml:"network"`
	Timezone        string  `default:"UTC" yaml:"timezone"`
	MemoryOverhead  float64 `default:"10" yaml:"memory_overhead"`
	RootlessEnabled bool    `default:"false" yaml:"rootless_enabled"`
}

type CrashDetectionConfiguration struct {
	DetectCleanExitAsCrash bool `default:"false" yaml:"detect_clean_exit_as_crash"`
}

type BackupConfiguration struct {
	WriteLimit int64 `default:"0" yaml:"write_limit"`
}

type LogConfiguration struct {
	MaxSize string `default:"10m" yaml:"max_size"`
	MaxFile int    `default:"3" yaml:"max_file"`
}

type Configuration struct {
	Debug          bool                        `default:"false" yaml:"debug"`
	UUID           string                      `yaml:"uuid"`
	TokenID        string                      `yaml:"token_id"`
	Token          string                      `yaml:"token"`
	PanelURL       string                      `yaml:"panel_url"`
	Remote         string                      `yaml:"remote"`
	System         SystemConfiguration         `yaml:"system"`
	AllowedMounts  []string                    `yaml:"allowed_mounts"`
	AllowedOrigins []string                    `yaml:"allowed_origins"`
	RemoteQuery    map[string]int              `yaml:"remote_query"`
	Docker         DockerConfiguration         `yaml:"docker"`
	CrashDetection CrashDetectionConfiguration `yaml:"crash_detection"`
	Backup         BackupConfiguration         `yaml:"backup"`
	Log            LogConfiguration            `yaml:"log"`
}

var (
	mu     sync.RWMutex
	config *Configuration
)

func setGlobal(cfg *Configuration) {
	mu.Lock()
	defer mu.Unlock()
	config = cfg
}

func Default() *Configuration {
	return &Configuration{
		Debug: false,
		System: SystemConfiguration{
			DataDirectory: "/srv/game-panel/servers",
			TempDirectory: "/tmp",
			Sftp: SftpConfiguration{
				Address:  "0.0.0.0",
				Port:     2022,
				ReadOnly: false,
			},
			API: ApiConfiguration{
				Host: "0.0.0.0",
				Port: 9090,
				TLS:  TLSConfiguration{Enabled: false},
			},
		},
		AllowedMounts:  []string{},
		AllowedOrigins: []string{},
		RemoteQuery:    map[string]int{},
		Docker: DockerConfiguration{
			Timezone:        "UTC",
			MemoryOverhead:  10.0,
			RootlessEnabled: false,
		},
		CrashDetection: CrashDetectionConfiguration{DetectCleanExitAsCrash: false},
		Backup:         BackupConfiguration{WriteLimit: 0},
		Log:            LogConfiguration{MaxSize: "10m", MaxFile: 3},
	}
}

type LoadOptions struct {
	Path      string
	EnvPrefix string
	Flags     *pflag.FlagSet
}

func LoadWithOptions(opts LoadOptions) (*Configuration, error) {
	if opts.EnvPrefix == "" {
		opts.EnvPrefix = EnvPrefix
	}

	v := viper.New()
	applyDefaults(v)
	v.SetEnvPrefix(opts.EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if opts.Path != "" {
		v.SetConfigFile(opts.Path)
		if err := v.ReadInConfig(); err != nil {
			if !errors.Is(err, os.ErrNotExist) && !errors.As(err, &viper.ConfigFileNotFoundError{}) {
				return nil, fmt.Errorf("read config file %q: %w", opts.Path, err)
			}
		}
	}

	if opts.Flags != nil {
		if err := v.BindPFlags(opts.Flags); err != nil {
			return nil, fmt.Errorf("bind flags: %w", err)
		}
	}

	cfg, err := decodeIntoConfiguration(v)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}
	setGlobal(cfg)
	return cfg, nil
}

func decodeIntoConfiguration(v *viper.Viper) (*Configuration, error) {
	cfg := Default()
	if err := v.Unmarshal(cfg, func(dc *mapstructure.DecoderConfig) {
		dc.TagName = "yaml"
		dc.WeaklyTypedInput = true
	}); err != nil {
		return nil, fmt.Errorf("decode merged settings: %w", err)
	}
	return cfg, nil
}

func applyDefaults(v *viper.Viper) {
	for _, entry := range typedEntries {
		entry.ApplyDefault(v)
	}
}

func (c *Configuration) Validate() error {
	var errs []error

	if c == nil {
		return errors.New("configuration is nil")
	}

	if c.System.API.Host == "" {
		errs = append(errs, errors.New("system.api.host must not be empty"))
	}
	if err := validatePort("system.api.port", c.System.API.Port); err != nil {
		errs = append(errs, err)
	}

	if c.System.Sftp.Address == "" {
		errs = append(errs, errors.New("system.sftp.bind_address must not be empty"))
	}
	if err := validatePort("system.sftp.bind_port", c.System.Sftp.Port); err != nil {
		errs = append(errs, err)
	}

	if c.System.DataDirectory == "" {
		errs = append(errs, errors.New("system.data_directory must not be empty"))
	}
	if c.System.TempDirectory == "" {
		errs = append(errs, errors.New("system.temp_directory must not be empty"))
	}
	if c.System.DataDirectory != "" && c.System.TempDirectory != "" && filepath.Clean(c.System.DataDirectory) == filepath.Clean(c.System.TempDirectory) {
		errs = append(errs, errors.New("system.data_directory and system.temp_directory must be different paths"))
	}

	if c.System.API.TLS.Enabled {
		if c.System.API.TLS.CertFile == "" {
			errs = append(errs, errors.New("system.api.tls.cert_file is required when TLS is enabled"))
		}
		if c.System.API.TLS.KeyFile == "" {
			errs = append(errs, errors.New("system.api.tls.key_file is required when TLS is enabled"))
		}
		if c.System.API.TLS.CertFile != "" {
			if err := fileReadable(c.System.API.TLS.CertFile); err != nil {
				errs = append(errs, fmt.Errorf("system.api.tls.cert_file: %w", err))
			}
		}
		if c.System.API.TLS.KeyFile != "" {
			if err := fileReadable(c.System.API.TLS.KeyFile); err != nil {
				errs = append(errs, fmt.Errorf("system.api.tls.key_file: %w", err))
			}
		}
	}

	if c.PanelURL != "" {
		if err := validateHTTPURL("panel_url", c.PanelURL); err != nil {
			errs = append(errs, err)
		}
	}
	if c.Remote != "" {
		if err := validateHTTPURL("remote", c.Remote); err != nil {
			errs = append(errs, err)
		}
	}
	for i, origin := range c.AllowedOrigins {
		if origin == "*" {
			continue
		}
		if err := validateHTTPURL(fmt.Sprintf("allowed_origins[%d]", i), origin); err != nil {
			errs = append(errs, err)
		}
	}
	for server, port := range c.RemoteQuery {
		if strings.TrimSpace(server) == "" {
			errs = append(errs, errors.New("remote_query contains an empty server key"))
		}
		if err := validatePort(fmt.Sprintf("remote_query[%q]", server), port); err != nil {
			errs = append(errs, err)
		}
	}

	if c.UUID != "" && !looksLikeUUID(c.UUID) {
		errs = append(errs, fmt.Errorf("uuid %q does not look like a UUID", c.UUID))
	}

	return errors.Join(errs...)
}

func validatePort(name string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535, got %d", name, port)
	}
	return nil
}

func validateHTTPURL(name string, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", name, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https scheme", name)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s must include a host", name)
	}
	return nil
}

func fileReadable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	return nil
}

func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
				return false
			}
		}
	}
	return true
}

func (c *Configuration) BackupConfig() BackupConfiguration {
	mu.RLock()
	defer mu.RUnlock()
	return c.Backup
}

func (c *Configuration) AllowedMountsList() []string {
	mu.RLock()
	defer mu.RUnlock()
	if c.AllowedMounts == nil {
		return []string{}
	}
	out := make([]string, len(c.AllowedMounts))
	copy(out, c.AllowedMounts)
	return out
}
