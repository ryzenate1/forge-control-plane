package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func adminBackupTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "admin_backup_" + uuid.NewString()[:8]
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
	})
	store := &Store{db: pool, secrets: newTestKeyring()}
	if err := store.RunMigrations(ctx, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return store, ctx
}

func TestAdminBackupPersistenceLifecycle(t *testing.T) {
	store, ctx := adminBackupTestStore(t)
	serverID := uuid.NewString()
	nodeID := uuid.NewString()
	userID := uuid.NewString()
	var templateID string
	if _, err := store.db.Exec(ctx, `INSERT INTO users(id,email,password_hash,role) VALUES($1,$2,'hash','admin')`, userID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(ctx, `INSERT INTO nodes(id,name,region,base_url,status,token_hash) VALUES($1,'backup-node','test','http://node','online','hash')`, nodeID); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(ctx, `SELECT id::text FROM eggs ORDER BY created_at LIMIT 1`).Scan(&templateID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(ctx, `
		INSERT INTO servers(id,node_id,owner_id,template_id,name,status,memory_mb,cpu_shares,disk_mb)
		VALUES($1,$2,$3,$4,'backup-server','offline',128,128,1024)
	`, serverID, nodeID, userID, templateID); err != nil {
		t.Fatal(err)
	}

	config := BackupConfigurationRecord{
		Name: "nightly", ServerID: &serverID, BackupType: "server",
		StorageProvider: "local", StorageConfig: json.RawMessage(`{"path":"/backups"}`),
		MaxBackups: 7, RetentionDays: 30, CompressionEnabled: true,
		Enabled: true, Data: json.RawMessage(`{"marker":"config"}`),
	}
	if err := store.CreateBackupConfiguration(ctx, &config); err != nil {
		t.Fatal(err)
	}
	loadedConfig, err := store.GetBackupConfiguration(ctx, config.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedConfig.Name != "nightly" || string(loadedConfig.Data) != `{"marker": "config"}` && string(loadedConfig.Data) != `{"marker":"config"}` {
		t.Fatalf("unexpected config: %+v data=%s", loadedConfig, loadedConfig.Data)
	}

	job := BackupJob{
		ConfigurationID: &config.ID, JobType: "server", ServerID: &serverID,
		Name: "nightly-run", Status: "pending", MaxRetries: 3,
		TriggeredBy: "manual", Data: json.RawMessage(`{"progressPercentage":0}`),
	}
	if err := store.CreateBackupJob(ctx, &job); err != nil {
		t.Fatal(err)
	}
	job.Status = "completed"
	if err := store.UpdateBackupJob(ctx, &job); err != nil {
		t.Fatal(err)
	}
	loadedJob, err := store.GetBackupJob(ctx, job.ID)
	if err != nil || loadedJob.Status != "completed" {
		t.Fatalf("unexpected job: %+v err=%v", loadedJob, err)
	}

	artifact := BackupArtifactRecord{
		JobID: &job.ID, ConfigurationID: &config.ID, ArtifactType: "server",
		Name: "nightly.tar.gz", DisplayName: "Nightly", StorageProvider: "local",
		StoragePath: "server/nightly.tar.gz", Status: "verified", IsVerified: true,
		Data: json.RawMessage(`{"hashAlgorithm":"sha256"}`),
	}
	if err := store.CreateBackupArtifactRecord(ctx, &artifact); err != nil {
		t.Fatal(err)
	}
	artifact.IsLocked = true
	reason := "legal hold"
	artifact.LockReason = &reason
	if err := store.UpdateBackupArtifactRecord(ctx, &artifact); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteBackupArtifactRecord(ctx, artifact.ID); err == nil {
		t.Fatal("locked artifact deletion unexpectedly succeeded")
	}
	artifact.IsLocked = false
	artifact.LockReason = nil
	if err := store.UpdateBackupArtifactRecord(ctx, &artifact); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteBackupArtifactRecord(ctx, artifact.ID); err != nil {
		t.Fatal(err)
	}

	policy := BackupRetentionPolicyRecord{
		Name: "global-default", Scope: "global", MaxBackups: 10,
		RetentionDays: 30, Priority: 100, Enabled: true,
	}
	if err := store.CreateBackupRetentionPolicy(ctx, &policy); err != nil {
		t.Fatal(err)
	}
	policy.RetentionDays = 45
	if err := store.UpdateBackupRetentionPolicy(ctx, &policy); err != nil {
		t.Fatal(err)
	}
	loadedPolicy, err := store.GetBackupRetentionPolicy(ctx, policy.ID)
	if err != nil || loadedPolicy.RetentionDays != 45 {
		t.Fatalf("unexpected retention policy: %+v err=%v", loadedPolicy, err)
	}
}

func TestBackupProviderConfigEncryptedAtRest(t *testing.T) {
	store, ctx := adminBackupTestStore(t)
	secretConfig := json.RawMessage(`{"accessKeyId":"key","secretAccessKey":"top-secret"}`)
	provider := BackupStorageProviderRecord{
		Name: "primary-s3", Type: "s3", Config: secretConfig,
		Enabled: true, IsDefault: true,
	}
	if err := store.CreateBackupStorageProvider(ctx, &provider); err != nil {
		t.Fatal(err)
	}
	var plaintext, encrypted string
	if err := store.db.QueryRow(ctx, `
		SELECT config::text, config_encrypted FROM backup_storage_providers WHERE id=$1
	`, provider.ID).Scan(&plaintext, &encrypted); err != nil {
		t.Fatal(err)
	}
	if plaintext != "{}" || encrypted == "" || encrypted == string(secretConfig) {
		t.Fatalf("provider config not encrypted: plaintext=%q encrypted=%q", plaintext, encrypted)
	}
	loaded, err := store.GetBackupStorageProvider(ctx, provider.Name)
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded.Config) != string(secretConfig) {
		t.Fatalf("decrypted provider config mismatch: %s", loaded.Config)
	}
}
