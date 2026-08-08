package dbbackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gamepanel/forge/internal/store"
)

var backupDir = filepath.Join(os.TempDir(), "forge-managed-db-backups")

type Service struct {
	store     *store.Store
	backupSvc BackupStorage
	logger    *log.Logger
}

type BackupStorage interface {
	Upload(ctx context.Context, path string, data []byte) error
	Download(ctx context.Context, path string) ([]byte, error)
	Delete(ctx context.Context, path string) error
}

func New(s *store.Store, storage BackupStorage) *Service {
	return &Service{
		store:     s,
		backupSvc: storage,
		logger:    log.Default(),
	}
}

func (s *Service) SetLogger(l *log.Logger) {
	if l != nil {
		s.logger = l
	}
}

func (s *Service) Backup(ctx context.Context, dbID string) (*store.ManagedDatabaseBackup, error) {
	db, err := s.store.GetManagedDatabase(ctx, dbID)
	if err != nil {
		return nil, fmt.Errorf("get managed database: %w", err)
	}
	if db.Status != store.ManagedDBStatusRunning {
		return nil, errors.New("database is not running")
	}
	if db.ContainerID == "" {
		return nil, errors.New("database container not provisioned")
	}

	name := fmt.Sprintf("%s-%s-%s", db.Name, db.DatabaseName, time.Now().UTC().Format("20060102T150405"))
	backup, err := s.store.CreateManagedDatabaseBackup(ctx, dbID, name, db.Engine)
	if err != nil {
		return nil, fmt.Errorf("create backup record: %w", err)
	}

	go func() {
		bCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := s.runBackup(bCtx, db, backup); err != nil {
			s.logger.Printf("backup %s failed: %v", backup.ID, err)
			_ = s.store.UpdateManagedDatabaseBackupStatus(bCtx, backup.ID, store.ManagedDBBackupFailed, 0, "", "")
		}
	}()

	return &backup, nil
}

func (s *Service) runBackup(ctx context.Context, d store.ManagedDatabase, backup store.ManagedDatabaseBackup) error {
	if err := s.store.UpdateManagedDatabaseBackupStatus(ctx, backup.ID, store.ManagedDBBackupRunning, 0, "", ""); err != nil {
		return err
	}

	password := extractPassword(d.Credentials)
	outputFile := filepath.Join(backupDir, backup.ID+".dump")
	remoteOutput := "/tmp/forge-db-backup-" + backup.ID + ".dump"
	tool, args := backupCommandForEngine(d.Engine, d.Host, d.Port, d.Username, password, d.DatabaseName, remoteOutput)
	if tool == "" {
		return fmt.Errorf("unsupported engine for backup: %s", d.Engine)
	}
	defer func() {
		_ = exec.Command("docker", "exec", d.ContainerID, "rm", "-f", remoteOutput).Run()
	}()

	var dockerArgs []string
	var cleanupCredential func()
	switch strings.ToLower(d.Engine) {
	case "postgresql", "postgres":
		remotePath, cleanup, err := s.stageCredentialFile(ctx, d.ContainerID, "pgpass", pgPassContents(d.Host, d.Port, d.DatabaseName, d.Username, password))
		if err != nil {
			return fmt.Errorf("stage pgpass file: %w", err)
		}
		cleanupCredential = cleanup
		dockerArgs = append([]string{"exec", "-i", "-e", "PGPASSFILE=" + remotePath}, append([]string{d.ContainerID, tool}, args...)...)
	case "mysql", "mariadb":
		remotePath, cleanup, err := s.stageCredentialFile(ctx, d.ContainerID, "mysql", mysqlConfigContents(d.Host, d.Port, d.Username, password))
		if err != nil {
			return fmt.Errorf("stage mysql credential file: %w", err)
		}
		cleanupCredential = cleanup
		dockerArgs = append([]string{"exec", "-i", d.ContainerID, tool, "--defaults-extra-file=" + remotePath}, args...)
	case "mongodb":
		remotePath, cleanup, err := s.stageCredentialFile(ctx, d.ContainerID, "mongo", mongoConfigContents(d.Host, d.Port, d.Username, password))
		if err != nil {
			return fmt.Errorf("stage mongo credential file: %w", err)
		}
		cleanupCredential = cleanup
		dockerArgs = append([]string{"exec", "-i", d.ContainerID, tool, "--config=" + remotePath}, args...)
	case "redis":
		dockerArgs = append([]string{"exec", "-i", "-e", "REDISCLI_AUTH=" + password, d.ContainerID, tool}, args...)
	default:
		dockerArgs = []string{"exec", "-i", d.ContainerID, tool}
		dockerArgs = append(dockerArgs, args...)
	}

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if cleanupCredential != nil {
		cleanupCredential()
	}
	if runErr != nil {
		return fmt.Errorf("backup command failed: %w, stderr: %s", runErr, stderr.String())
	}
	if err := exec.CommandContext(ctx, "docker", "cp", "--", d.ContainerID+":"+remoteOutput, outputFile).Run(); err != nil {
		return fmt.Errorf("copy backup from container: %w", err)
	}

	data, err := s.readFile(ctx, outputFile)
	if err != nil {
		return fmt.Errorf("read backup file: %w", err)
	}

	checksum := sha256Checksum(data)

	if s.backupSvc != nil {
		defer func() { _ = os.Remove(outputFile) }()
		storagePath := fmt.Sprintf("managed-db-backups/%s/%s.dump", d.ID, backup.ID)
		if err := s.backupSvc.Upload(ctx, storagePath, data); err != nil {
			return fmt.Errorf("upload backup: %w", err)
		}
		if err := s.store.UpdateManagedDatabaseBackupStatus(ctx, backup.ID, store.ManagedDBBackupCompleted, int64(len(data)), checksum, storagePath); err != nil {
			return fmt.Errorf("update backup status: %w", err)
		}
	} else {
		if err := s.store.UpdateManagedDatabaseBackupStatus(ctx, backup.ID, store.ManagedDBBackupCompleted, int64(len(data)), checksum, outputFile); err != nil {
			return fmt.Errorf("update backup status: %w", err)
		}
	}

	return nil
}

func (s *Service) Restore(ctx context.Context, dbID, backupID string) (*store.ManagedDatabaseRestore, error) {
	db, err := s.store.GetManagedDatabase(ctx, dbID)
	if err != nil {
		return nil, fmt.Errorf("get managed database: %w", err)
	}
	if db.ContainerID == "" {
		return nil, errors.New("database container not provisioned")
	}

	backup, err := s.store.GetManagedDatabaseBackup(ctx, backupID)
	if err != nil {
		return nil, fmt.Errorf("get backup: %w", err)
	}
	if backup.Status != store.ManagedDBBackupCompleted {
		return nil, errors.New("backup is not in completed state")
	}

	restore, err := s.store.CreateManagedDatabaseRestore(ctx, dbID, backupID)
	if err != nil {
		return nil, fmt.Errorf("create restore record: %w", err)
	}

	go func() {
		bCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := s.runRestore(bCtx, db, backup, restore); err != nil {
			s.logger.Printf("restore %s failed: %v", restore.ID, err)
			_ = s.store.UpdateManagedDatabaseRestoreStatus(bCtx, restore.ID, store.ManagedDBRestoreFailed, err.Error())
		}
	}()

	return &restore, nil
}

func (s *Service) runRestore(ctx context.Context, db store.ManagedDatabase, backup store.ManagedDatabaseBackup, restore store.ManagedDatabaseRestore) error {
	if err := s.store.UpdateManagedDatabaseRestoreStatus(ctx, restore.ID, store.ManagedDBRestoreRunning, ""); err != nil {
		return err
	}

	var data []byte
	if s.backupSvc != nil && backup.StoragePath != "" {
		var err error
		data, err = s.backupSvc.Download(ctx, backup.StoragePath)
		if err != nil {
			return fmt.Errorf("download backup: %w", err)
		}
	} else {
		var err error
		data, err = s.readFile(ctx, backup.StoragePath)
		if err != nil {
			return fmt.Errorf("read local backup: %w", err)
		}
	}

	inputFile := filepath.Join(backupDir, "restore-"+restore.ID+".dump")
	if err := s.writeFile(ctx, inputFile, data); err != nil {
		return fmt.Errorf("write restore file: %w", err)
	}
	defer func() { _ = s.removeFile(ctx, inputFile) }()

	password := extractPassword(db.Credentials)
	remoteInput := "/tmp/forge-db-restore-" + restore.ID + ".dump"
	tool, args := restoreCommandForEngine(db.Engine, db.Host, db.Port, db.Username, password, db.DatabaseName, remoteInput)
	if tool == "" {
		return fmt.Errorf("unsupported engine for restore: %s", db.Engine)
	}
	if strings.ToLower(db.Engine) != "redis" {
		if err := exec.CommandContext(ctx, "docker", "cp", "--", inputFile, db.ContainerID+":"+remoteInput).Run(); err != nil {
			return fmt.Errorf("copy restore into container: %w", err)
		}
		defer func() {
			_ = exec.Command("docker", "exec", db.ContainerID, "rm", "-f", remoteInput).Run()
		}()
	}

	var dockerArgs []string
	var cleanupCredential func()
	switch strings.ToLower(db.Engine) {
	case "postgresql", "postgres":
		remotePath, cleanup, err := s.stageCredentialFile(ctx, db.ContainerID, "pgpass", pgPassContents(db.Host, db.Port, db.DatabaseName, db.Username, password))
		if err != nil {
			return fmt.Errorf("stage pgpass file: %w", err)
		}
		cleanupCredential = cleanup
		dockerArgs = append([]string{"exec", "-i", "-e", "PGPASSFILE=" + remotePath}, append([]string{db.ContainerID, tool}, args...)...)
	case "mysql", "mariadb":
		remotePath, cleanup, err := s.stageCredentialFile(ctx, db.ContainerID, "mysql", mysqlConfigContents(db.Host, db.Port, db.Username, password))
		if err != nil {
			return fmt.Errorf("stage mysql credential file: %w", err)
		}
		cleanupCredential = cleanup
		dockerArgs = append([]string{"exec", "-i", db.ContainerID, tool, "--defaults-extra-file=" + remotePath}, args...)
	case "mongodb":
		remotePath, cleanup, err := s.stageCredentialFile(ctx, db.ContainerID, "mongo", mongoConfigContents(db.Host, db.Port, db.Username, password))
		if err != nil {
			return fmt.Errorf("stage mongo credential file: %w", err)
		}
		cleanupCredential = cleanup
		dockerArgs = append([]string{"exec", "-i", db.ContainerID, tool, "--config=" + remotePath}, args...)
	case "redis":
		dockerArgs = append([]string{"exec", "-i", "-e", "REDISCLI_AUTH=" + password, db.ContainerID, tool}, args...)
	default:
		dockerArgs = []string{"exec", "-i", db.ContainerID, tool}
		dockerArgs = append(dockerArgs, args...)
	}

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	if strings.ToLower(db.Engine) == "redis" {
		input, err := os.Open(inputFile)
		if err != nil {
			return fmt.Errorf("open redis restore input: %w", err)
		}
		defer input.Close()
		cmd.Stdin = input
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if cleanupCredential != nil {
		cleanupCredential()
	}
	if runErr != nil {
		return fmt.Errorf("restore command failed: %w, stderr: %s", runErr, stderr.String())
	}

	if err := s.store.UpdateManagedDatabaseRestoreStatus(ctx, restore.ID, store.ManagedDBRestoreCompleted, ""); err != nil {
		return fmt.Errorf("update restore status: %w", err)
	}

	return nil
}

func (s *Service) RotatePassword(ctx context.Context, dbID string) (*store.ManagedDatabase, error) {
	_ = ctx
	_ = dbID
	return nil, errors.New("password rotation via Docker exec not yet implemented")
}

func (s *Service) ListBackups(ctx context.Context, dbID string) ([]store.ManagedDatabaseBackup, error) {
	return s.store.ListManagedDatabaseBackups(ctx, dbID)
}

func (s *Service) ListRestores(ctx context.Context, dbID string) ([]store.ManagedDatabaseRestore, error) {
	return s.store.ListManagedDatabaseRestores(ctx, dbID)
}

func (s *Service) DeleteBackup(ctx context.Context, backupID string) error {
	backup, err := s.store.GetManagedDatabaseBackup(ctx, backupID)
	if err != nil {
		return err
	}
	if s.backupSvc != nil && backup.StoragePath != "" {
		_ = s.backupSvc.Delete(ctx, backup.StoragePath)
	}
	return s.store.DeleteManagedDatabaseBackup(ctx, backupID)
}

func (s *Service) stageCredentialFile(ctx context.Context, containerID, kind, contents string) (string, func(), error) {
	if strings.ContainsAny(containerID, "\x00\r\n") {
		return "", nil, errors.New("invalid container id")
	}
	localFile, err := os.CreateTemp(backupDir, kind+"-*")
	if err != nil {
		return "", nil, fmt.Errorf("create local credential file: %w", err)
	}
	localPath := localFile.Name()
	cleanupLocal := func() { _ = os.Remove(localPath) }

	if _, err := localFile.WriteString(contents); err != nil {
		_ = localFile.Close()
		cleanupLocal()
		return "", nil, fmt.Errorf("write local credential file: %w", err)
	}
	if err := localFile.Close(); err != nil {
		cleanupLocal()
		return "", nil, fmt.Errorf("close local pgpass file: %w", err)
	}
	if err := os.Chmod(localPath, 0o600); err != nil {
		cleanupLocal()
		return "", nil, fmt.Errorf("chmod local pgpass file: %w", err)
	}

	remotePath := "/tmp/." + kind + "-" + filepath.Base(localPath)
	if err := exec.CommandContext(ctx, "docker", "cp", localPath, "--", containerID+":"+remotePath).Run(); err != nil {
		cleanupLocal()
		return "", nil, fmt.Errorf("copy credential into container: %w", err)
	}
	if err := exec.CommandContext(ctx, "docker", "exec", containerID, "chmod", "0600", remotePath).Run(); err != nil {
		cleanupLocal()
		_ = exec.Command("docker", "exec", containerID, "rm", "-f", remotePath).Run()
		return "", nil, fmt.Errorf("chmod credential in container: %w", err)
	}

	cleanup := func() {
		cleanupLocal()
		_ = exec.Command("docker", "exec", containerID, "rm", "-f", remotePath).Run()
	}
	return remotePath, cleanup, nil
}

func pgPassContents(host string, port int, database, username, password string) string {
	return fmt.Sprintf("%s:%d:%s:%s:%s\n", escapePgPassField(host), port, escapePgPassField(database), escapePgPassField(username), escapePgPassField(password))
}

func escapePgPassField(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ":", `\:`)
	return s
}

func mysqlConfigContents(host string, port int, username, password string) string {
	quote := func(value string) string {
		value = strings.ReplaceAll(value, `\`, `\\`)
		value = strings.ReplaceAll(value, `"`, `\"`)
		value = strings.ReplaceAll(value, "\r", "")
		value = strings.ReplaceAll(value, "\n", "")
		return `"` + value + `"`
	}
	return fmt.Sprintf("[client]\nhost=%s\nport=%d\nuser=%s\npassword=%s\n", quote(host), port, quote(username), quote(password))
}

func mongoConfigContents(host string, port int, username, password string) string {
	return fmt.Sprintf("host: %s\nport: %d\nusername: %s\npassword: %s\n", strconv.Quote(host), port, strconv.Quote(username), strconv.Quote(password))
}

func extractPassword(creds json.RawMessage) string {
	if len(creds) == 0 {
		return ""
	}
	var m map[string]string
	if err := json.Unmarshal(creds, &m); err != nil {
		return ""
	}
	return m["password"]
}

func sha256Checksum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (s *Service) readFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func (s *Service) writeFile(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (s *Service) removeFile(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Service) EngineDumpCommands(engine string) json.RawMessage {
	cmds := map[string]string{}
	switch strings.ToLower(engine) {
	case "postgresql", "postgres":
		cmds["backup"] = "pg_dump"
		cmds["restore"] = "pg_restore"
	case "mysql":
		cmds["backup"] = "mysqldump"
		cmds["restore"] = "mysql"
	case "mariadb":
		cmds["backup"] = "mysqldump"
		cmds["restore"] = "mysql"
	case "mongodb":
		cmds["backup"] = "mongodump"
		cmds["restore"] = "mongorestore"
	case "redis":
		cmds["backup"] = "redis-cli --rdb"
		cmds["restore"] = "redis-cli --pipe"
	}
	raw, _ := json.Marshal(cmds)
	return raw
}

func init() {
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		log.Printf("failed to create backup dir %s: %v", backupDir, err)
	} else if err := os.Chmod(backupDir, 0700); err != nil {
		log.Printf("failed to secure backup dir %s: %v", backupDir, err)
	}
}

type noopStorage struct{}

func (n noopStorage) Upload(_ context.Context, _ string, _ []byte) error { return nil }
func (n noopStorage) Download(_ context.Context, _ string) ([]byte, error) {
	return nil, errors.New("noop storage cannot download")
}
func (n noopStorage) Delete(_ context.Context, _ string) error { return nil }

func NewNoopStorage() BackupStorage { return noopStorage{} }

var _ io.Reader = (*bytes.Buffer)(nil)
