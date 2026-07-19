package store

import (
	"context"
	"encoding/json"
	"errors"
)

type RateLimitSettings struct {
	AuthRequestsPerMinute     int  `json:"authRequestsPerMinute"`
	MutationRequestsPerMinute int  `json:"mutationRequestsPerMinute"`
	ReadRequestsPerMinute     int  `json:"readRequestsPerMinute"`
	LoginRateLimitEnabled     bool `json:"loginRateLimitEnabled"`
	LoginAttemptThreshold     int  `json:"loginAttemptThreshold"`
	AccountLockoutMinutes     int  `json:"accountLockoutMinutes"`
	SignedURLExpiryMinutes    int  `json:"signedUrlExpiryMinutes"`
	MaxWebSocketsPerServer    int  `json:"maxWebSocketsPerServer"`
	ConsoleThrottleEnabled    bool `json:"consoleThrottleEnabled"`
	ConsoleThrottleLines      int  `json:"consoleThrottleLines"`
	ConsoleThrottlePeriodMs   int  `json:"consoleThrottlePeriodMs"`
}

func DefaultRateLimitSettings() RateLimitSettings {
	return RateLimitSettings{
		AuthRequestsPerMinute:     5,
		MutationRequestsPerMinute: 30,
		ReadRequestsPerMinute:     120,
		LoginRateLimitEnabled:     true,
		LoginAttemptThreshold:     5,
		AccountLockoutMinutes:     15,
		SignedURLExpiryMinutes:    5,
		MaxWebSocketsPerServer:    30,
		ConsoleThrottleEnabled:    false,
		ConsoleThrottleLines:      2000,
		ConsoleThrottlePeriodMs:   100,
	}
}

func (s *Store) GetRateLimitSettings(ctx context.Context) (RateLimitSettings, error) {
	if s.db == nil {
		return DefaultRateLimitSettings(), errors.New("no database connection")
	}
	var raw []byte
	err := s.db.QueryRow(ctx, `SELECT settings FROM panel_rate_limit_settings WHERE id = TRUE`).Scan(&raw)
	if err != nil {
		return DefaultRateLimitSettings(), err
	}
	if len(raw) == 0 {
		return DefaultRateLimitSettings(), nil
	}
	rl := DefaultRateLimitSettings()
	_ = json.Unmarshal(raw, &rl)
	return rl, nil
}

func (s *Store) UpdateRateLimitSettings(ctx context.Context, rl RateLimitSettings) error {
	if s.db == nil {
		return errors.New("no database connection")
	}
	body, err := json.Marshal(rl)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO panel_rate_limit_settings (id, settings, updated_at)
		VALUES (TRUE, $1::jsonb, now())
		ON CONFLICT (id) DO UPDATE SET
			settings = EXCLUDED.settings,
			updated_at = now()
	`, string(body))
	return err
}
