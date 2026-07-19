package store

type PanelSettings struct {
	CompanyName                   string `json:"companyName"`
	ShortName                     string `json:"shortName"`
	ProductName                   string `json:"productName"`
	BrowserTitle                  string `json:"browserTitle"`
	FooterText                    string `json:"footerText"`
	LogoURL                       string `json:"logoUrl"`
	FaviconURL                    string `json:"faviconUrl"`
	LoginBackgroundURL            string `json:"loginBackgroundUrl"`
	ThemePreset                   string `json:"themePreset"`
	Require2FA                    string `json:"require2FA"`
	DefaultLocale                 string `json:"defaultLocale"`
	DefaultTimezone               string `json:"defaultTimezone"`
	DateFormat                    string `json:"dateFormat"`
	NumberFormat                  string `json:"numberFormat"`
	CurrencyFormat                string `json:"currencyFormat"`
	DefaultDashboard              string `json:"defaultDashboard"`
	LandingPage                   string `json:"landingPage"`
	SidebarLayout                 string `json:"sidebarLayout"`
	CompactMode                   bool   `json:"compactMode"`
	AdvancedMode                  bool   `json:"advancedMode"`
	SMTPHost                      string `json:"smtpHost"`
	SMTPPort                      int    `json:"smtpPort"`
	SMTPEncryption                string `json:"smtpEncryption"`
	SMTPUsername                  string `json:"smtpUsername"`
	SMTPPassword                  string `json:"smtpPassword"`
	MailFromAddress               string `json:"mailFromAddress"`
	MailFromName                  string `json:"mailFromName"`
	RecaptchaEnabled              bool   `json:"recaptchaEnabled"`
	RecaptchaSiteKey              string `json:"recaptchaSiteKey"`
	RecaptchaSecretKey            string `json:"recaptchaSecretKey"`
	ConnectionTimeout             int    `json:"connectionTimeout"`
	RequestTimeout                int    `json:"requestTimeout"`
	AutoAllocEnabled              bool   `json:"autoAllocEnabled"`
	AutoAllocStartPort            int    `json:"autoAllocStartPort"`
	AutoAllocEndPort              int    `json:"autoAllocEndPort"`
	RequireEmailVerification      bool   `json:"requireEmailVerification"`
	PasswordComplexity            string `json:"passwordComplexity"`
	PasswordExpirationDays        int    `json:"passwordExpirationDays"`
	SessionDurationMinutes        int    `json:"sessionDurationMinutes"`
	LoginRateLimitEnabled         bool   `json:"loginRateLimitEnabled"`
	LoginAttemptThreshold         int    `json:"loginAttemptThreshold"`
	AccountLockoutMinutes         int    `json:"accountLockoutMinutes"`
	GeoRestrictions               string `json:"geoRestrictions"`
	APITokenTTLDays               int    `json:"apiTokenTtlDays"`
	APIRotationDays               int    `json:"apiRotationDays"`
	AllowedOrigins                string `json:"allowedOrigins"`
	TrustedNetworks               string `json:"trustedNetworks"`
	MetricsRetentionDays          int    `json:"metricsRetentionDays"`
	LogsRetentionDays             int    `json:"logsRetentionDays"`
	AuditRetentionDays            int    `json:"auditRetentionDays"`
	MetricsSamplingRate           int    `json:"metricsSamplingRate"`
	MonitoringPollIntervalSeconds int    `json:"monitoringPollIntervalSeconds"`
	EmailAlertsEnabled            bool   `json:"emailAlertsEnabled"`
	WebhookAlertsEnabled          bool   `json:"webhookAlertsEnabled"`
	DiscordWebhookURL             string `json:"discordWebhookUrl"`
	SlackWebhookURL               string `json:"slackWebhookUrl"`
	TelegramBotToken              string `json:"telegramBotToken"`
	PlacementStrategy             string `json:"placementStrategy"`
	AntiAffinityRules             string `json:"antiAffinityRules"`
	ResourceReservationsEnabled   bool   `json:"resourceReservationsEnabled"`
	NodePrioritization            string `json:"nodePrioritization"`
	RecoveryStrategy              string `json:"recoveryStrategy"`
	FailoverThresholdSeconds      int    `json:"failoverThresholdSeconds"`
	HeartbeatThresholdSeconds     int    `json:"heartbeatThresholdSeconds"`
	ReservationDurationMinutes    int    `json:"reservationDurationMinutes"`
	ReservationCleanupMinutes     int    `json:"reservationCleanupMinutes"`
	CapacityBufferPercent         int    `json:"capacityBufferPercent"`
	BackupProvider                string `json:"backupProvider"`
	BackupRetentionDays           int    `json:"backupRetentionDays"`
	BackupLimit                   int    `json:"backupLimit"`
	BackupAutoCleanup             bool   `json:"backupAutoCleanup"`
	BackupEncryptionEnabled       bool   `json:"backupEncryptionEnabled"`
	BackupKeyRotationDays         int    `json:"backupKeyRotationDays"`
	BackupRateLimitEnabled        bool   `json:"backupRateLimitEnabled"`
	BackupRateLimitCount          int    `json:"backupRateLimitCount"`
	BackupRateLimitWindowMinutes  int    `json:"backupRateLimitWindowMinutes"`
	BackupPruneAgeMinutes         int    `json:"backupPruneAgeMinutes"`
	// S3 Backup Configuration
	S3BackupEnabled   bool   `json:"s3BackupEnabled"`
	S3Endpoint        string `json:"s3Endpoint"`
	S3Region          string `json:"s3Region"`
	S3Bucket          string `json:"s3Bucket"`
	S3AccessKeyID     string `json:"s3AccessKeyID"`
	S3SecretAccessKey string `json:"s3SecretAccessKey"`
	S3Prefix          string `json:"s3Prefix"`
	S3UsePathStyle    bool   `json:"s3UsePathStyle"`
}

func DefaultPanelSettings() PanelSettings {
	return PanelSettings{
		CompanyName:                   "Forge Control Plane",
		ShortName:                     "Forge",
		ProductName:                   "GamePanel",
		BrowserTitle:                  "GamePanel",
		ThemePreset:                   "default",
		Require2FA:                    "none",
		DefaultLocale:                 "en",
		DefaultTimezone:               "UTC",
		DateFormat:                    "yyyy-MM-dd",
		NumberFormat:                  "en-US",
		CurrencyFormat:                "USD",
		DefaultDashboard:              "overview",
		LandingPage:                   "servers",
		SidebarLayout:                 "expanded",
		SMTPPort:                      587,
		SMTPEncryption:                "tls",
		ConnectionTimeout:             30,
		RequestTimeout:                30,
		AutoAllocStartPort:            25565,
		AutoAllocEndPort:              25600,
		PasswordComplexity:            "standard",
		SessionDurationMinutes:        1440,
		LoginRateLimitEnabled:         true,
		LoginAttemptThreshold:         5,
		AccountLockoutMinutes:         15,
		MetricsRetentionDays:          30,
		LogsRetentionDays:             30,
		AuditRetentionDays:            365,
		MetricsSamplingRate:           100,
		MonitoringPollIntervalSeconds: 30,
		PlacementStrategy:             "balanced",
		ResourceReservationsEnabled:   true,
		NodePrioritization:            "capacity",
		RecoveryStrategy:              "manual",
		FailoverThresholdSeconds:      300,
		HeartbeatThresholdSeconds:     60,
		ReservationDurationMinutes:    30,
		ReservationCleanupMinutes:     60,
		CapacityBufferPercent:         10,
		BackupProvider:                "local",
		BackupRetentionDays:           7,
		BackupAutoCleanup:             true,
		BackupKeyRotationDays:         90,
		BackupRateLimitEnabled:        true,
		BackupRateLimitCount:          2,
		BackupRateLimitWindowMinutes:  10,
		BackupPruneAgeMinutes:         360,
		// S3 Backup Configuration
		S3BackupEnabled:   false,
		S3Endpoint:        "",
		S3Region:          "",
		S3Bucket:          "",
		S3AccessKeyID:     "",
		S3SecretAccessKey: "",
		S3Prefix:          "",
		S3UsePathStyle:    true,
	}
}
