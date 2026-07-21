package config

type MTLS struct {
	Enabled      bool
	CACertPath   string
	CertPath     string
	KeyPath      string
	DevBypass    bool
	AutoMigrate  bool
}

func MTLSConfig() MTLS {
	return MTLS{
		Enabled:     envBool("MTLS_ENABLED", false),
		CACertPath:  env("MTLS_CA_CERT", ""),
		CertPath:    env("MTLS_CERT", ""),
		KeyPath:     env("MTLS_KEY", ""),
		DevBypass:   envBool("MTLS_DEV_BYPASS", false),
		AutoMigrate: envBool("MTLS_AUTO_MIGRATE", false),
	}
}
