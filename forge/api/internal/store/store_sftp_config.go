package store

import (
	"context"
	"time"
)

type SFTPNodeConfig struct {
	NodeID          string    `json:"nodeId"`
	Enabled         bool      `json:"enabled"`
	ListenPort      int       `json:"listenPort"`
	ListenIP        string    `json:"listenIP"`
	MaxConnections  int       `json:"maxConnections"`
	MaxAuthAttempts int       `json:"maxAuthAttempts"`
	IdleTimeout     int       `json:"idleTimeout"` // seconds
	RateLimit       int       `json:"rateLimit"`   // KB/s
	ReadOnly        bool      `json:"readOnly"`
	AllowedIPs      []string  `json:"allowedIps,omitempty"`
	Banner          string    `json:"banner,omitempty"`
	LogLevel        string    `json:"logLevel"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type SFTPGlobalConfig struct {
	Enabled               bool     `json:"enabled"`
	DefaultPort           int      `json:"defaultPort"`
	DefaultMaxConnections int      `json:"defaultMaxConnections"`
	DefaultIdleTimeout    int      `json:"defaultIdleTimeout"`
	DefaultRateLimit      int      `json:"defaultRateLimit"`
	LogLevel              string   `json:"logLevel"`
	AllowedCiphers        []string `json:"allowedCiphers,omitempty"`
	AllowedMACs           []string `json:"allowedMacs,omitempty"`
	AllowedKEXAlgos       []string `json:"allowedKexAlgos,omitempty"`
	HostKeyAlgorithms     []string `json:"hostKeyAlgorithms,omitempty"`
}

func (s *Store) GetSFTPNodeConfig(ctx context.Context, nodeID string) (*SFTPNodeConfig, error) {
	row := s.db.QueryRow(ctx, `
		SELECT node_id, enabled, listen_port, listen_ip, max_connections, max_auth_attempts,
		       idle_timeout, rate_limit, read_only, allowed_ips, banner, log_level, updated_at
		FROM sftp_node_configs
		WHERE node_id = $1
	`, nodeID)

	var cfg SFTPNodeConfig
	err := row.Scan(&cfg.NodeID, &cfg.Enabled, &cfg.ListenPort, &cfg.ListenIP,
		&cfg.MaxConnections, &cfg.MaxAuthAttempts, &cfg.IdleTimeout, &cfg.RateLimit,
		&cfg.ReadOnly, &cfg.AllowedIPs, &cfg.Banner, &cfg.LogLevel, &cfg.UpdatedAt)
	if err != nil {
		return &SFTPNodeConfig{
			NodeID:          nodeID,
			Enabled:         true,
			ListenPort:      2022,
			MaxConnections:  10,
			MaxAuthAttempts: 3,
			IdleTimeout:     300,
			RateLimit:       0,
			ReadOnly:        false,
			LogLevel:        "info",
		}, nil
	}
	return &cfg, nil
}

func (s *Store) UpdateSFTPNodeConfig(ctx context.Context, cfg SFTPNodeConfig) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO sftp_node_configs (node_id, enabled, listen_port, listen_ip, max_connections,
			max_auth_attempts, idle_timeout, rate_limit, read_only, allowed_ips, banner, log_level, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (node_id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			listen_port = EXCLUDED.listen_port,
			listen_ip = EXCLUDED.listen_ip,
			max_connections = EXCLUDED.max_connections,
			max_auth_attempts = EXCLUDED.max_auth_attempts,
			idle_timeout = EXCLUDED.idle_timeout,
			rate_limit = EXCLUDED.rate_limit,
			read_only = EXCLUDED.read_only,
			allowed_ips = EXCLUDED.allowed_ips,
			banner = EXCLUDED.banner,
			log_level = EXCLUDED.log_level,
			updated_at = EXCLUDED.updated_at
	`, cfg.NodeID, cfg.Enabled, cfg.ListenPort, cfg.ListenIP, cfg.MaxConnections,
		cfg.MaxAuthAttempts, cfg.IdleTimeout, cfg.RateLimit, cfg.ReadOnly,
		cfg.AllowedIPs, cfg.Banner, cfg.LogLevel, time.Now().UTC())
	return err
}

func (s *Store) ListSFTPNodeConfigs(ctx context.Context) ([]SFTPNodeConfig, error) {
	rows, err := s.db.Query(ctx, `
		SELECT node_id, enabled, listen_port, listen_ip, max_connections, max_auth_attempts,
		       idle_timeout, rate_limit, read_only, allowed_ips, banner, log_level, updated_at
		FROM sftp_node_configs
		ORDER BY node_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []SFTPNodeConfig
	for rows.Next() {
		var cfg SFTPNodeConfig
		if err := rows.Scan(&cfg.NodeID, &cfg.Enabled, &cfg.ListenPort, &cfg.ListenIP,
			&cfg.MaxConnections, &cfg.MaxAuthAttempts, &cfg.IdleTimeout, &cfg.RateLimit,
			&cfg.ReadOnly, &cfg.AllowedIPs, &cfg.Banner, &cfg.LogLevel, &cfg.UpdatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}

func (s *Store) GetSFTPGlobalConfig(ctx context.Context) (*SFTPGlobalConfig, error) {
	row := s.db.QueryRow(ctx, `SELECT enabled, default_port, default_max_connections,
		default_idle_timeout, default_rate_limit, log_level, allowed_ciphers, allowed_macs,
		allowed_kex_algos, host_key_algorithms FROM sftp_global_config LIMIT 1`)

	var cfg SFTPGlobalConfig
	err := row.Scan(&cfg.Enabled, &cfg.DefaultPort, &cfg.DefaultMaxConnections,
		&cfg.DefaultIdleTimeout, &cfg.DefaultRateLimit, &cfg.LogLevel,
		&cfg.AllowedCiphers, &cfg.AllowedMACs, &cfg.AllowedKEXAlgos, &cfg.HostKeyAlgorithms)
	if err != nil {
		return &SFTPGlobalConfig{
			Enabled:               true,
			DefaultPort:           2022,
			DefaultMaxConnections: 10,
			DefaultIdleTimeout:    300,
			DefaultRateLimit:      0,
			LogLevel:              "info",
		}, nil
	}
	return &cfg, nil
}

func (s *Store) UpdateSFTPGlobalConfig(ctx context.Context, cfg SFTPGlobalConfig) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO sftp_global_config (id, enabled, default_port, default_max_connections,
			default_idle_timeout, default_rate_limit, log_level, allowed_ciphers, allowed_macs,
			allowed_kex_algos, host_key_algorithms)
		VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			default_port = EXCLUDED.default_port,
			default_max_connections = EXCLUDED.default_max_connections,
			default_idle_timeout = EXCLUDED.default_idle_timeout,
			default_rate_limit = EXCLUDED.default_rate_limit,
			log_level = EXCLUDED.log_level,
			allowed_ciphers = EXCLUDED.allowed_ciphers,
			allowed_macs = EXCLUDED.allowed_macs,
			allowed_kex_algos = EXCLUDED.allowed_kex_algos,
			host_key_algorithms = EXCLUDED.host_key_algorithms
	`, cfg.Enabled, cfg.DefaultPort, cfg.DefaultMaxConnections,
		cfg.DefaultIdleTimeout, cfg.DefaultRateLimit, cfg.LogLevel,
		cfg.AllowedCiphers, cfg.AllowedMACs, cfg.AllowedKEXAlgos, cfg.HostKeyAlgorithms)
	return err
}
