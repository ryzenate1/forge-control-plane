package store

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestOperationalSecretPlaintextMigrationIsIdempotent(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()
	if err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	userID, nodeID, hostID, databaseID, deliveryID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users (id,email,password_hash,role,totp_secret) VALUES ($1,$2,$3,'admin','TOTPPLAINTEXT')`, []any{userID, userID + "@example.test", string(passwordHash)}},
		{`INSERT INTO nodes (id,name,region,base_url,token_hash,daemon_token_id,daemon_token) VALUES ($1,'legacy','test','http://node.test','node-plaintext','legacy-id','node-plaintext')`, []any{nodeID}},
		{`INSERT INTO database_hosts (id,engine,name,host,port,username,password,tls_mode) VALUES ($1,'postgresql','legacy','db.test',5432,'admin','host-plaintext','disable')`, []any{hostID}},
		{`INSERT INTO server_databases (id,server_id,database_host_id,database_name,username,password) SELECT $1,id,$2,'legacy_db','legacy_user','database-plaintext' FROM servers LIMIT 1`, []any{databaseID, hostID}},
		{`INSERT INTO webhooks (id,name,url,webhook_type,secret) VALUES ('legacy-hook','legacy','https://8.8.8.8/hook','regular','webhook-plaintext')`, nil},
		{`INSERT INTO webhook_deliveries (id,webhook_id,event_name,target_url,webhook_type,secret,payload,request_body) VALUES ($1,'legacy-hook','test','https://8.8.8.8/hook','regular','snapshot-plaintext','{}','{}')`, []any{deliveryID}},
		{`INSERT INTO recovery_tokens (id,user_id,token) VALUES ($1,$2,'recovery-plaintext')`, []any{uuid.NewString(), userID}},
		{`UPDATE panel_settings SET smtp_password='smtp-plaintext', recaptcha_secret_key='captcha-plaintext', discord_webhook_url='https://legacy-discord.example/secret', slack_webhook_url='https://legacy-slack.example/secret', telegram_bot_token='legacy-telegram' WHERE id=TRUE`, nil},
		{`UPDATE panel_mail_settings SET smtp_password='mail-plaintext' WHERE id=TRUE`, nil},
		{`UPDATE panel_advanced_settings SET recaptcha_secret_key='advanced-plaintext' WHERE id=TRUE`, nil},
		{`UPDATE panel_settings_expanded SET settings='{"discordWebhookUrl":"https://discord.example/secret","slackWebhookUrl":"https://slack.example/secret","telegramBotToken":"telegram-plaintext"}' WHERE id=TRUE`, nil},
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MigrateOperationalSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.MigrateOperationalSecrets(ctx); err != nil {
		t.Fatalf("idempotent rerun: %v", err)
	}
	checks := []string{
		`SELECT count(*) FROM nodes WHERE daemon_token<>'' OR daemon_token_encrypted NOT LIKE 'forge:v1:%'`,
		`SELECT count(*) FROM database_hosts WHERE password<>'' OR password_encrypted NOT LIKE 'forge:v1:%'`,
		`SELECT count(*) FROM server_databases WHERE password<>'' OR password_encrypted NOT LIKE 'forge:v1:%'`,
		`SELECT count(*) FROM users WHERE id='` + userID + `' AND (totp_secret<>'' OR totp_secret_encrypted NOT LIKE 'forge:v1:%')`,
		`SELECT count(*) FROM webhooks WHERE secret<>'' OR secret_encrypted NOT LIKE 'forge:v1:%'`,
		`SELECT count(*) FROM webhook_deliveries WHERE secret<>'' OR secret_encrypted NOT LIKE 'forge:v1:%'`,
		`SELECT count(*) FROM recovery_tokens WHERE token<>'' OR token_hash NOT LIKE '$2%'`,
		`SELECT count(*) FROM panel_settings_expanded WHERE settings ?| ARRAY['discordWebhookUrl','slackWebhookUrl','telegramBotToken']`,
		`SELECT count(*) FROM panel_settings WHERE discord_webhook_url<>'' OR slack_webhook_url<>'' OR telegram_bot_token<>''`,
	}
	for _, query := range checks {
		var count int
		if err := s.db.QueryRow(ctx, query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("plaintext check %q = %d, %v", query, count, err)
		}
	}
	token, err := s.GetNodeDaemonToken(ctx, nodeID)
	if err != nil || token != "node-plaintext" {
		t.Fatalf("migrated node token = %q, %v", token, err)
	}
}

func TestEncryptedOperationalSecretUseAndRecoveryConsume(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()
	if err := s.MigrateOperationalSecrets(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}

	nodeID := "22222222-2222-2222-2222-222222222222"
	credential, err := s.RotateNodeToken(ctx, nodeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := s.VerifyNodeToken(ctx, nodeID, credential)
	if err != nil || !valid {
		t.Fatalf("rotated node credential valid=%v err=%v", valid, err)
	}

	host, err := s.CreateDatabaseHost(ctx, CreateDatabaseHostRequest{NodeID: nodeID, Engine: "postgresql", Name: "encrypted", Host: "db.example.test", Port: 5432, Username: "admin", Password: "host-password", TLSMode: "disable"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	database, err := s.CreateServerDatabase(ctx, "44444444-4444-4444-4444-444444444444", CreateServerDatabaseRequest{Database: "encrypted", Remote: "%"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, hostPassword, err := s.GetDatabaseHostForServerDatabase(ctx, database.ID)
	if err != nil || hostPassword != "host-password" || host.ID == "" || database.Password == nil {
		t.Fatalf("database secrets host=%q database=%v err=%v", hostPassword, database.Password, err)
	}
	oldPassword := *database.Password
	if err := s.CommitServerDatabasePassword(ctx, "44444444-4444-4444-4444-444444444444", database.ID, oldPassword, "rotated-password", nil); err != nil {
		t.Fatal(err)
	}
	rotated, err := s.GetServerDatabaseForProvisioning(ctx, "44444444-4444-4444-4444-444444444444", database.ID)
	if err != nil || rotated.Password == nil || *rotated.Password != "rotated-password" {
		t.Fatalf("rotated database secret = %v, %v", rotated.Password, err)
	}

	mailSettings := PanelMailSettings{SMTPHost: "smtp.example.test", SMTPPort: 587, SMTPEncryption: "tls", SMTPUsername: "mailer", SMTPPassword: "smtp-password", MailFromAddress: "noreply@example.test"}
	if err := s.UpdatePanelMailSettings(ctx, mailSettings); err != nil {
		t.Fatal(err)
	}
	gotMail, err := s.GetPanelMailSettings(ctx)
	if err != nil || gotMail.SMTPPassword != "smtp-password" {
		t.Fatalf("SMTP decrypt = %q, %v", gotMail.SMTPPassword, err)
	}

	webhook, err := s.CreateWebhook(ctx, CreateWebhookRequest{Name: "encrypted", URL: "https://8.8.8.8/hook", WebhookType: WebhookTypeRegular, Events: []string{"test"}, Enabled: true, Secret: "webhook-password"})
	if err != nil || webhook.Secret != maskedStoreSecret {
		t.Fatalf("webhook create = %#v, %v", webhook, err)
	}
	if err := s.EnqueueWebhookEvent(ctx, "test", map[string]any{"value": true}); err != nil {
		t.Fatal(err)
	}
	delivery, err := s.ClaimWebhookDelivery(ctx, "test-worker", time.Minute)
	if err != nil || delivery == nil || delivery.Secret != "webhook-password" {
		t.Fatalf("webhook delivery secret = %#v, %v", delivery, err)
	}

	userID := uuid.NewString()
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if _, err := s.db.Exec(ctx, `INSERT INTO users (id,email,password_hash,role) VALUES ($1,$2,$3,'admin')`, userID, userID+"@example.test", string(passwordHash)); err != nil {
		t.Fatal(err)
	}
	setup, err := s.SetupTwoFactor(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	code, err := computeTOTP(setup.Secret, uint64(time.Now().Unix()/30))
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := s.EnableTwoFactor(ctx, userID, code, "correct-password")
	if err != nil || len(recovery) != 10 {
		t.Fatalf("enable two-factor = %d codes, %v", len(recovery), err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.VerifyTwoFactorCheckpoint(ctx, userID, "", recovery[0])
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for result := range results {
		if result == nil {
			successes++
		} else if !strings.Contains(result.Error(), "invalid") {
			t.Fatalf("unexpected recovery consume error: %v", result)
		}
	}
	if successes != 1 {
		t.Fatalf("atomic recovery consume successes=%d, want 1", successes)
	}
}
