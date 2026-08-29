package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/google/uuid"
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
	SystemTempDirectoryEntry         = ConfigEntry[string]{Key: "system.temp_directory", Default: "/srv/game-panel/tmp", Description: "private temporary file directory"}
	SystemSFTPBindAddressEntry       = ConfigEntry[string]{Key: "system.sftp.bind_address", Default: "127.0.0.1", Description: "SFTP bind address"}
	SystemSFTPBindPortEntry          = ConfigEntry[int]{Key: "system.sftp.bind_port", Default: 2022, Description: "SFTP bind port"}
	SystemSFTPReadOnlyEntry          = ConfigEntry[bool]{Key: "system.sftp.read_only", Default: false, Description: "run SFTP in read-only mode"}
	SystemAPIHostEntry               = ConfigEntry[string]{Key: "system.api.host", Default: "127.0.0.1", Description: "API bind address"}
	SystemAPIPortEntry               = ConfigEntry[int]{Key: "system.api.port", Default: 9090, Description: "API bind port"}
	SystemAPITLSEnabledEntry         = ConfigEntry[bool]{Key: "system.api.tls.enabled", Default: false, Description: "enable TLS for the API listener"}
	DockerTimezoneEntry              = ConfigEntry[string]{Key: "docker.timezone", Default: "UTC", Description: "timezone used for runtime containers"}
	DockerNetworkInterfaceEntry      = ConfigEntry[string]{Key: "docker.network.interface", Default: "", Description: "network interface used for runtime networking"}
	CrashDetectCleanExitAsCrashEntry = ConfigEntry[bool]{Key: "crash_detection.detect_clean_exit_as_crash", Default: false, Description: "treat clean server exits as crashes"}

	DockerMemoryOverheadEntry  = ConfigEntry[float64]{Key: "docker.memory_overhead", Default: 10.0, Description: "default memory overhead percentage added to server memory limits"}
	DockerRootlessEnabledEntry = ConfigEntry[bool]{Key: "docker.rootless_enabled", Default: false, Description: "enable rootless Docker mode (userns=host)"}
	BackupWriteLimitEntry      = ConfigEntry[int64]{Key: "backup.write_limit", Default: 0, Description: "backup I/O write limit in bytes/sec (0 = unlimited)"}
	LogMaxSizeEntry            = ConfigEntry[string]{Key: "log.max_size", Default: "10m", Description: "Docker log driver max-size"}
	LogMaxFileEntry            = ConfigEntry[int]{Key: "log.max_file", Default: 3, Description: "Docker log driver max-file"}
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
	Address  string `default:"127.0.0.1" yaml:"bind_address"`
	Port     int    `default:"2022" yaml:"bind_port"`
	ReadOnly bool   `default:"false" yaml:"read_only"`
}

type TLSConfiguration struct {
	Enabled  bool   `default:"false" yaml:"enabled"`
	CertFile string `default:"" yaml:"cert_file"`
	KeyFile  string `default:"" yaml:"key_file"`
}

type ApiConfiguration struct {
	Host string           `default:"127.0.0.1" yaml:"host"`
	Port int              `default:"9090" yaml:"port"`
	TLS  TLSConfiguration `yaml:"tls"`
}

type SystemConfiguration struct {
	DataDirectory    string            `default:"/srv/game-panel/servers" yaml:"data_directory"`
	TempDirectory    string            `default:"/srv/game-panel/tmp" yaml:"temp_directory"`
	RootDirectory    string            `yaml:"root_directory"`
	LogDirectory     string            `yaml:"log_directory"`
	ArchiveDirectory string            `yaml:"archive_directory"`
	BackupDirectory  string            `yaml:"backup_directory"`
	Username         string            `yaml:"username"`
	Sftp             SftpConfiguration `yaml:"sftp"`
	API              ApiConfiguration  `yaml:"api"`
}

type DockerConfiguration struct {
	Network struct {
		Interface string `default:"" yaml:"interface"`
		Name      string `yaml:"name"`
		Mode      string `yaml:"mode"`
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

func Default() *Configuration {
	root := "/srv/game-panel"
	logDir := "/var/log/beacon"
	archiveDir := "/srv/game-panel/archives"
	backupDir := "/srv/game-panel/backups"
	dataDir := SystemDataDirectoryEntry.Default
	tempDir := SystemTempDirectoryEntry.Default
	if runtime.GOOS == "windows" {
		sysDrive := os.Getenv("SystemDrive")
		if sysDrive == "" {
			sysDrive = "C:"
		}
		root = sysDrive + `\srv\game-panel`
		logDir = sysDrive + `\srv\game-panel\logs`
		archiveDir = sysDrive + `\srv\game-panel\archives`
		backupDir = sysDrive + `\srv\game-panel\backups`
		dataDir = sysDrive + `\srv\game-panel\servers`
		tempDir = sysDrive + `\srv\game-panel\tmp`
	}
	return &Configuration{
		Debug: DebugEntry.Default,
		System: SystemConfiguration{
			DataDirectory:    dataDir,
			TempDirectory:    tempDir,
			RootDirectory:    root,
			LogDirectory:     logDir,
			ArchiveDirectory: archiveDir,
			BackupDirectory:  backupDir,
			Username:         "beacon",
			Sftp: SftpConfiguration{
				Address:  SystemSFTPBindAddressEntry.Default,
				Port:     SystemSFTPBindPortEntry.Default,
				ReadOnly: SystemSFTPReadOnlyEntry.Default,
			},
			API: ApiConfiguration{
				Host: SystemAPIHostEntry.Default,
				Port: SystemAPIPortEntry.Default,
				TLS:  TLSConfiguration{Enabled: SystemAPITLSEnabledEntry.Default},
			},
		},
		AllowedMounts:  []string{},
		AllowedOrigins: []string{},
		RemoteQuery:    map[string]int{},
		Docker: DockerConfiguration{
			Timezone:        DockerTimezoneEntry.Default,
			MemoryOverhead:  DockerMemoryOverheadEntry.Default,
			RootlessEnabled: DockerRootlessEnabledEntry.Default,
		},
		CrashDetection: CrashDetectionConfiguration{DetectCleanExitAsCrash: CrashDetectCleanExitAsCrashEntry.Default},
		Backup:         BackupConfiguration{WriteLimit: BackupWriteLimitEntry.Default},
		Log:            LogConfiguration{MaxSize: LogMaxSizeEntry.Default, MaxFile: LogMaxFileEntry.Default},
	}
}

type LoadOptions struct {
	Path      string
	Paths     []string
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

	paths := append([]string(nil), opts.Paths...)
	if opts.Path != "" {
		paths = append([]string{opts.Path}, paths...)
	}
	for i, configPath := range paths {
		if strings.TrimSpace(configPath) == "" {
			continue
		}
		if err := validateConfigFile(configPath); err != nil {
			return nil, err
		}
		v.SetConfigFile(configPath)
		var err error
		if i == 0 {
			err = v.ReadInConfig()
		} else {
			err = v.MergeInConfig()
		}
		if err != nil {
			return nil, fmt.Errorf("read config file %q: %w", configPath, err)
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
	if cfg.Token, err = expandSecretReference(cfg.Token); err != nil {
		return nil, fmt.Errorf("load token secret: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}
	return cfg, nil
}

func expandSecretReference(value string) (string, error) {
	const prefix = "file://"
	if !strings.HasPrefix(value, prefix) {
		return value, nil
	}
	path := strings.TrimPrefix(value, prefix)
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 1 || info.Size() > 4096 {
		return "", errors.New("secret file must be a non-empty regular non-symlink file no larger than 4 KiB")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("secret file permissions must be 0600")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	secret := strings.TrimSpace(string(body))
	if strings.ContainsAny(secret, "\r\n\x00") {
		return "", errors.New("secret file must contain exactly one line")
	}
	return secret, nil
}

func decodeIntoConfiguration(v *viper.Viper) (*Configuration, error) {
	cfg := Default()
	if err := v.Unmarshal(cfg, func(dc *mapstructure.DecoderConfig) {
		dc.TagName = "yaml"
		dc.WeaklyTypedInput = false
		dc.DecodeHook = mapstructure.StringToBasicTypeHookFunc()
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
	for name, value := range map[string]string{
		"system.root_directory":    c.System.RootDirectory,
		"system.log_directory":     c.System.LogDirectory,
		"system.archive_directory": c.System.ArchiveDirectory,
		"system.backup_directory":  c.System.BackupDirectory,
	} {
		if value == "" || !filepath.IsAbs(value) || filepath.Clean(value) == string(os.PathSeparator) {
			errs = append(errs, fmt.Errorf("%s must be a non-root absolute path", name))
		}
	}
	if matched, _ := regexp.MatchString(`^[a-z_][a-z0-9_-]{0,31}$`, c.System.Username); !matched {
		errs = append(errs, errors.New("system.username must be a valid local service username"))
	}
	if c.Docker.MemoryOverhead < 0 || c.Docker.MemoryOverhead > 100 {
		errs = append(errs, errors.New("docker.memory_overhead must be between 0 and 100"))
	}
	if c.Backup.WriteLimit < 0 {
		errs = append(errs, errors.New("backup.write_limit must not be negative"))
	}
	if c.Log.MaxFile < 1 || c.Log.MaxFile > 100 {
		errs = append(errs, errors.New("log.max_file must be between 1 and 100"))
	}
	if !validDockerLogSize(c.Log.MaxSize) {
		errs = append(errs, fmt.Errorf("log.max_size %q is invalid", c.Log.MaxSize))
	}
	if _, err := time.LoadLocation(c.Docker.Timezone); err != nil {
		errs = append(errs, fmt.Errorf("docker.timezone: %w", err))
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
	if !c.System.API.TLS.Enabled && !isLoopbackHost(c.System.API.Host) {
		errs = append(errs, errors.New("system.api.tls must be enabled when binding beyond loopback"))
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
			errs = append(errs, errors.New("allowed_origins must not contain wildcard '*'"))
			continue
		}
		if err := validateHTTPURL(fmt.Sprintf("allowed_origins[%d]", i), origin); err != nil {
			errs = append(errs, err)
		}
	}
	for server, port := range c.RemoteQuery {
		if !validRemoteQueryKey(server) {
			errs = append(errs, fmt.Errorf("remote_query contains invalid server key %q", server))
		}
		if err := validatePort(fmt.Sprintf("remote_query[%q]", server), port); err != nil {
			errs = append(errs, err)
		}
	}

	if c.UUID != "" {
		parsed, err := uuid.Parse(c.UUID)
		if err != nil || parsed == uuid.Nil || parsed.Variant() != uuid.RFC4122 || (parsed.Version() != 4 && parsed.Version() != 7) {
			errs = append(errs, fmt.Errorf("uuid %q must be a non-nil RFC 4122 version 4 or 7 UUID", c.UUID))
		}
	}
	if (strings.TrimSpace(c.Token) == "") != (strings.TrimSpace(c.TokenID) == "") {
		errs = append(errs, errors.New("token and token_id must be configured together"))
	}
	if c.Token != "" && len(c.Token) < 32 {
		errs = append(errs, errors.New("token must contain at least 32 characters"))
	}
	for i, mountPath := range c.AllowedMounts {
		if err := validateAllowedMount(mountPath); err != nil {
			errs = append(errs, fmt.Errorf("allowed_mounts[%d]: %w", i, err))
		}
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
	if parsed.User != nil {
		return fmt.Errorf("%s must not include URL credentials", name)
	}
	if parsed.Scheme != "https" && !isLoopbackHost(parsed.Hostname()) {
		return fmt.Errorf("%s must use https unless it targets loopback", name)
	}
	return nil
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func fileReadable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return f.Close()
}

func (c *Configuration) BackupConfig() BackupConfiguration {
	return c.Backup
}

func (c *Configuration) AllowedMountsList() []string {
	if c.AllowedMounts == nil {
		return []string{}
	}
	out := make([]string, len(c.AllowedMounts))
	copy(out, c.AllowedMounts)
	return out
}

var dockerLogSizePattern = regexp.MustCompile(`^[1-9][0-9]*(?:[kKmMgG])?$`)

func validDockerLogSize(value string) bool {
	return dockerLogSizePattern.MatchString(strings.TrimSpace(value))
}

func validRemoteQueryKey(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 255 && !strings.ContainsAny(value, "/\\\x00\r\n")
}

func validateAllowedMount(value string) error {
	if strings.TrimSpace(value) == "" || !filepath.IsAbs(value) {
		return errors.New("path must be absolute")
	}
	cleaned := filepath.Clean(value)
	for _, denied := range []string{"/", "/etc", "/proc", "/sys", "/dev", "/boot", "/root"} {
		if cleaned == denied || strings.HasPrefix(cleaned, denied+string(os.PathSeparator)) {
			return fmt.Errorf("path %q targets a protected host location", cleaned)
		}
	}
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil && resolved != cleaned {
		return fmt.Errorf("path %q contains a symlink (resolves to %q)", cleaned, resolved)
	}
	return nil
}

func validateConfigFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("read config file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("config file %q is not a regular file", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("config file %q permissions %04o expose secrets; use 0600", path, info.Mode().Perm())
	}
	return nil
}
