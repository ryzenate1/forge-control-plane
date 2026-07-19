package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gamepanel/forge/internal/secrets"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrSecretEncryptionUnavailable = errors.New("secret encryption is not configured")

func (s *Store) encryptSecret(plaintext, aad string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if s == nil || s.secrets == nil {
		return "", ErrSecretEncryptionUnavailable
	}
	return s.secrets.Encrypt([]byte(plaintext), aad)
}

func (s *Store) decryptSecret(envelope, plaintext, aad string) (string, error) {
	if envelope == "" {
		if plaintext == "" {
			return "", nil
		}
		return plaintext, nil // only reachable before the startup migration commits
	}
	if s == nil || s.secrets == nil {
		return "", ErrSecretEncryptionUnavailable
	}
	decoded, err := s.secrets.Decrypt(envelope, aad)
	if err != nil {
		return "", fmt.Errorf("decrypt stored secret: %w", err)
	}
	return string(decoded), nil
}

func secretAAD(table, id, field string) string { return table + ":" + id + ":" + field }

// MigrateOperationalSecrets transactionally encrypts legacy plaintext and
// re-encrypts envelopes from configured previous keys with the active key. It is
// idempotent and is the executable master-key rotation path.
func (s *Store) MigrateOperationalSecrets(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("no database connection")
	}
	if s.secrets == nil {
		return ErrSecretEncryptionUnavailable
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	type fieldSpec struct{ table, idColumn, plainColumn, encryptedColumn string }
	fields := []fieldSpec{
		{"nodes", "id::text", "daemon_token", "daemon_token_encrypted"},
		{"database_hosts", "id::text", "password", "password_encrypted"},
		{"server_databases", "id::text", "password", "password_encrypted"},
		{"users", "id::text", "totp_secret", "totp_secret_encrypted"},
		{"webhooks", "id", "secret", "secret_encrypted"},
		{"webhook_deliveries", "id::text", "secret", "secret_encrypted"},
		{"panel_settings", "id::text", "smtp_password", "smtp_password_encrypted"},
		{"panel_settings", "id::text", "recaptcha_secret_key", "recaptcha_secret_key_encrypted"},
		{"panel_mail_settings", "id::text", "smtp_password", "smtp_password_encrypted"},
		{"panel_advanced_settings", "id::text", "recaptcha_secret_key", "recaptcha_secret_key_encrypted"},
	}
	for _, spec := range fields {
		query := fmt.Sprintf("SELECT %s, COALESCE(%s,''), COALESCE(%s,'') FROM %s FOR UPDATE", spec.idColumn, spec.plainColumn, spec.encryptedColumn, spec.table)
		rows, err := tx.Query(ctx, query)
		if err != nil {
			return fmt.Errorf("scan %s secret rows: %w", spec.table, err)
		}
		type pending struct{ id, plain, encrypted string }
		var values []pending
		for rows.Next() {
			var value pending
			if err := rows.Scan(&value.id, &value.plain, &value.encrypted); err != nil {
				rows.Close()
				return err
			}
			values = append(values, value)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, value := range values {
			aad := secretAAD(spec.table, value.id, spec.plainColumn)
			secret := value.plain
			if value.encrypted != "" {
				secret, err = s.decryptSecret(value.encrypted, "", aad)
				if err != nil {
					return fmt.Errorf("migrate %s secret: %w", spec.table, err)
				}
			}
			if secret == "" {
				continue
			}
			if value.encrypted == "" || s.secrets.NeedsRotation(value.encrypted) {
				value.encrypted, err = s.encryptSecret(secret, aad)
				if err != nil {
					return err
				}
			}
			update := fmt.Sprintf("UPDATE %s SET %s=$1, %s='' WHERE %s=$2", spec.table, spec.encryptedColumn, spec.plainColumn, spec.idColumn)
			if _, err := tx.Exec(ctx, update, value.encrypted, value.id); err != nil {
				return fmt.Errorf("persist %s encrypted secret: %w", spec.table, err)
			}
		}
	}

	if err := migrateLegacyNodeVerifier(ctx, tx, s); err != nil {
		return err
	}
	if err := migrateExpandedSettingsSecrets(ctx, tx, s); err != nil {
		return err
	}
	if err := migrateRecoveryCodeHashes(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RestoreOperationalSecrets repopulates reversible legacy columns without
// deleting ciphertext. Run it immediately before rolling back to a pre-046
// binary. Recovery codes cannot be restored because their hashes are one-way.
func (s *Store) RestoreOperationalSecrets(ctx context.Context) error {
	if s == nil || s.db == nil || s.secrets == nil {
		return ErrSecretEncryptionUnavailable
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	type fieldSpec struct{ table, idColumn, plainColumn, encryptedColumn string }
	fields := []fieldSpec{
		{"nodes", "id::text", "daemon_token", "daemon_token_encrypted"},
		{"database_hosts", "id::text", "password", "password_encrypted"},
		{"server_databases", "id::text", "password", "password_encrypted"},
		{"users", "id::text", "totp_secret", "totp_secret_encrypted"},
		{"webhooks", "id", "secret", "secret_encrypted"},
		{"webhook_deliveries", "id::text", "secret", "secret_encrypted"},
		{"panel_settings", "id::text", "smtp_password", "smtp_password_encrypted"},
		{"panel_settings", "id::text", "recaptcha_secret_key", "recaptcha_secret_key_encrypted"},
		{"panel_mail_settings", "id::text", "smtp_password", "smtp_password_encrypted"},
		{"panel_advanced_settings", "id::text", "recaptcha_secret_key", "recaptcha_secret_key_encrypted"},
	}
	for _, spec := range fields {
		rows, err := tx.Query(ctx, fmt.Sprintf("SELECT %s, COALESCE(%s,'') FROM %s WHERE COALESCE(%s,'')<>'' FOR UPDATE", spec.idColumn, spec.encryptedColumn, spec.table, spec.encryptedColumn))
		if err != nil {
			return err
		}
		type rowValue struct{ id, encrypted string }
		var values []rowValue
		for rows.Next() {
			var value rowValue
			if err := rows.Scan(&value.id, &value.encrypted); err != nil {
				rows.Close()
				return err
			}
			values = append(values, value)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, value := range values {
			plaintext, err := s.decryptSecret(value.encrypted, "", secretAAD(spec.table, value.id, spec.plainColumn))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, fmt.Sprintf("UPDATE %s SET %s=$1 WHERE %s=$2", spec.table, spec.plainColumn, spec.idColumn), plaintext, value.id); err != nil {
				return err
			}
		}
	}
	var raw []byte
	var discordEncrypted, slackEncrypted, telegramEncrypted string
	if err := tx.QueryRow(ctx, `SELECT settings, COALESCE(discord_webhook_url_encrypted,''), COALESCE(slack_webhook_url_encrypted,''), COALESCE(telegram_bot_token_encrypted,'') FROM panel_settings_expanded WHERE id=TRUE FOR UPDATE`).Scan(&raw, &discordEncrypted, &slackEncrypted, &telegramEncrypted); err == nil {
		settings := map[string]any{}
		_ = json.Unmarshal(raw, &settings)
		for _, item := range []struct{ key, envelope string }{{"discordWebhookUrl", discordEncrypted}, {"slackWebhookUrl", slackEncrypted}, {"telegramBotToken", telegramEncrypted}} {
			if item.envelope != "" {
				plaintext, err := s.decryptSecret(item.envelope, "", secretAAD("panel_settings_expanded", "true", item.key))
				if err != nil {
					return err
				}
				settings[item.key] = plaintext
			}
		}
		body, _ := json.Marshal(settings)
		if _, err := tx.Exec(ctx, `UPDATE panel_settings_expanded SET settings=$1::jsonb WHERE id=TRUE`, string(body)); err != nil {
			return err
		}
		discord, _ := settings["discordWebhookUrl"].(string)
		slack, _ := settings["slackWebhookUrl"].(string)
		telegram, _ := settings["telegramBotToken"].(string)
		if _, err := tx.Exec(ctx, `UPDATE panel_settings SET discord_webhook_url=$1, slack_webhook_url=$2, telegram_bot_token=$3 WHERE id=TRUE`, discord, slack, telegram); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func migrateLegacyNodeVerifier(ctx context.Context, tx pgx.Tx, s *Store) error {
	rows, err := tx.Query(ctx, `SELECT id::text, token_hash, COALESCE(daemon_token_encrypted,'') FROM nodes FOR UPDATE`)
	if err != nil {
		return err
	}
	type nodeVerifier struct{ id, hash, encrypted string }
	var nodes []nodeVerifier
	for rows.Next() {
		var node nodeVerifier
		if err := rows.Scan(&node.id, &node.hash, &node.encrypted); err != nil {
			rows.Close()
			return err
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, node := range nodes {
		if strings.HasPrefix(node.hash, "$2") || node.encrypted == "" {
			continue
		}
		secret, err := s.decryptSecret(node.encrypted, "", secretAAD("nodes", node.id, "daemon_token"))
		if err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE nodes SET token_hash=$1 WHERE id=$2`, string(hash), node.id); err != nil {
			return err
		}
	}
	return nil
}

func migrateExpandedSettingsSecrets(ctx context.Context, tx pgx.Tx, s *Store) error {
	var raw []byte
	var discordEncrypted, slackEncrypted, telegramEncrypted string
	var discordLegacy, slackLegacy, telegramLegacy string
	err := tx.QueryRow(ctx, `SELECT e.settings, COALESCE(e.discord_webhook_url_encrypted,''), COALESCE(e.slack_webhook_url_encrypted,''), COALESCE(e.telegram_bot_token_encrypted,''), COALESCE(p.discord_webhook_url,''), COALESCE(p.slack_webhook_url,''), COALESCE(p.telegram_bot_token,'') FROM panel_settings_expanded e JOIN panel_settings p ON p.id=e.id WHERE e.id=TRUE FOR UPDATE OF e,p`).Scan(&raw, &discordEncrypted, &slackEncrypted, &telegramEncrypted, &discordLegacy, &slackLegacy, &telegramLegacy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	settings := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return errors.New("panel expanded settings contain invalid JSON")
		}
	}
	type expandedSecret struct {
		jsonKey string
		column  string
		stored  string
	}
	items := []expandedSecret{{"discordWebhookUrl", "discord_webhook_url_encrypted", discordEncrypted}, {"slackWebhookUrl", "slack_webhook_url_encrypted", slackEncrypted}, {"telegramBotToken", "telegram_bot_token_encrypted", telegramEncrypted}}
	legacy := []string{discordLegacy, slackLegacy, telegramLegacy}
	for i := range items {
		plain, _ := settings[items[i].jsonKey].(string)
		if plain == "" {
			plain = legacy[i]
		}
		delete(settings, items[i].jsonKey)
		aad := secretAAD("panel_settings_expanded", "true", items[i].jsonKey)
		if items[i].stored != "" {
			plain, err = s.decryptSecret(items[i].stored, "", aad)
			if err != nil {
				return err
			}
		}
		if plain != "" && (items[i].stored == "" || s.secrets.NeedsRotation(items[i].stored)) {
			items[i].stored, err = s.encryptSecret(plain, aad)
			if err != nil {
				return err
			}
		}
	}
	sanitized, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE panel_settings_expanded SET settings=$1::jsonb, discord_webhook_url_encrypted=$2, slack_webhook_url_encrypted=$3, telegram_bot_token_encrypted=$4 WHERE id=TRUE`, string(sanitized), items[0].stored, items[1].stored, items[2].stored); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE panel_settings SET discord_webhook_url='', slack_webhook_url='', telegram_bot_token='' WHERE id=TRUE`)
	return err
}

func migrateRecoveryCodeHashes(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `SELECT id::text, COALESCE(token,''), COALESCE(token_hash,'') FROM recovery_tokens FOR UPDATE`)
	if err != nil {
		return err
	}
	type tokenRow struct{ id, token, hash string }
	var tokens []tokenRow
	for rows.Next() {
		var token tokenRow
		if err := rows.Scan(&token.id, &token.token, &token.hash); err != nil {
			rows.Close()
			return err
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, token := range tokens {
		if token.hash != "" || token.token == "" {
			continue
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(token.token), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE recovery_tokens SET token_hash=$1, token='' WHERE id=$2`, string(hash), token.id); err != nil {
			return err
		}
	}
	return nil
}

func newTestKeyring() *secrets.Keyring {
	ring, _ := secrets.New("test", "0000000000000000000000000000000000000000000000000000000000000000", nil)
	return ring
}
