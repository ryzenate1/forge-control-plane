package config

type Mail struct {
	Default string
	Mailers map[string]MailerConfig
	From    MailFromConfig
}

type MailerConfig struct {
	Transport  string
	Host       string
	Port       int
	Encryption string
	Username   string
	Password   string
	Timeout    int
}

type MailFromConfig struct {
	Address string
	Name    string
}

func MailConfig() Mail {
	return Mail{
		Default: env("MAIL_MAILER", "smtp"),
		Mailers: map[string]MailerConfig{
			"smtp": {
				Transport:  "smtp",
				Host:       env("MAIL_HOST", "127.0.0.1"),
				Port:       envInt("MAIL_PORT", 587),
				Encryption: env("MAIL_ENCRYPTION", "tls"),
				Username:   env("MAIL_USERNAME", ""),
				Password:   env("MAIL_PASSWORD", ""),
				Timeout:    envInt("MAIL_TIMEOUT", 30),
			},
			"log": {
				Transport: "log",
			},
		},
		From: MailFromConfig{
			Address: env("MAIL_FROM_ADDRESS", "noreply@gamepanel.local"),
			Name:    env("MAIL_FROM_NAME", "GamePanel"),
		},
	}
}
