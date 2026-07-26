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
	"strings"
	"time"

	"github.com/google/uuid"

	"gamepanel/forge/internal/store"
)

const backupDir = "/tmp/managed-db-backups"

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
	tool, args := backupCommandForEngine(d.Engine, d.Host, d.Port, d.Username, password, d.DatabaseName, outputFile)
	if tool == "" {
		return fmt.Errorf("unsupported engine for backup: %s", d.Engine)
	}

	// Security: avoid ever passing the database password via a PGPASSWORD
	// (or similar) environment variable. Environment variables of a running
	// process are readable by anyone with access to /proc/<pid>/environ on
	// Linux (or by anyone who can inspect the process table via other
	// means), so they are a poor place to carry secrets. Instead, stage a
	// short-lived, mode-0600 .pgpass file inside the target container and
	// point pg_dump at it via PGPASSFILE; the file is removed immediately
	// after the command completes.
	//
	// redis-cli has no equivalent "password file" option in the version
	// range this project supports, so its password is passed via the -a
	// flag in backupCommandForEngine/engines.go rather than an env var. That
	// is a narrower exposure than a process-wide env var (visible only via
	// the container's own process listing/cmdline, not via a host-wide
	// environ dump), but it is still a known residual risk; consider moving
	// to a Redis ACL user with a config-file-based credential if this needs
	// to be hardened further.
	var dockerArgs []string
	var cleanupPgPass func()
	if d.Engine == "postgresql" || d.Engine == "postgres" {
		remotePath, cleanup, err := s.stagePgPassFile(ctx, d.ContainerID, d.Host, d.Port, d.DatabaseName, d.Username, password)
		if err != nil {
			return fmt.Errorf("stage pgpass file: %w", err)
		}
		cleanupPgPass = cleanup
		dockerArgs = append([]string{"exec", "-i", "-e", "PGPASSFILE=" + remotePath}, append([]string{d.ContainerID, tool}, args...)...)
	} else {
		dockerArgs = []string{"exec", "-i", d.ContainerID, tool}
		dockerArgs = append(dockerArgs, args...)
	}

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if cleanupPgPass != nil {
		cleanupPgPass()
	}
	if runErr != nil {
		return fmt.Errorf("backup command failed: %w, stderr: %s", runErr, stderr.String())
	}

	data, err := s.readFile(ctx, outputFile)
	if err != nil {
		return fmt.Errorf("read backup file: %w", err)
	}

	checksum := sha256Checksum(data)

	if s.backupSvc != nil {
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
	tool, args := restoreCommandForEngine(db.Engine, db.Host, db.Port, db.Username, password, db.DatabaseName, inputFile)
	if tool == "" {
		return fmt.Errorf("unsupported engine for restore: %s", db.Engine)
	}

	// See the matching comment in runBackup: avoid PGPASSWORD-style env vars
	// for the password and instead stage a short-lived, mode-0600 .pgpass
	// file inside the container for PostgreSQL restores.
	var dockerArgs []string
	var cleanupPgPass func()
	if db.Engine == "postgresql" || db.Engine == "postgres" {
		remotePath, cleanup, err := s.stagePgPassFile(ctx, db.ContainerID, db.Host, db.Port, db.DatabaseName, db.Username, password)
		if err != nil {
			return fmt.Errorf("stage pgpass file: %w", err)
		}
		cleanupPgPass = cleanup
		dockerArgs = append([]string{"exec", "-i", "-e", "PGPASSFILE=" + remotePath}, append([]string{db.ContainerID, tool}, args...)...)
	} else {
		dockerArgs = []string{"exec", "-i", db.ContainerID, tool}
		dockerArgs = append(dockerArgs, args...)
	}

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if cleanupPgPass != nil {
		cleanupPgPass()
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
	db, err := s.store.GetManagedDatabase(ctx, dbID)
	if err != nil {
		return nil, fmt.Errorf("get managed database: %w", err)
	}
	raw := uuid.NewString() + time.Now().String()
	h := sha256.Sum256([]byte(raw))
	newPassword := hex.EncodeToString(h[:])[:32]

	newEncrypted, err := s.encryptPassword(newPassword)
	if err != nil {
		return nil, fmt.Errorf("encrypt password: %w", err)
	}

	_ = newEncrypted

	_ = db

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

func (s *Service) encryptPassword(password string) (string, error) {
	return password, nil
}

// stagePgPassFile writes a short-lived, mode-0600 pgpass file describing how
// to authenticate to a PostgreSQL database, copies it into the target
// container, and returns the in-container path plus a cleanup function that
// removes both the local and in-container copies. Callers should always
// invoke the returned cleanup function (e.g. via a plain call once the
// backup/restore command has finished) so the credential does not linger on
// disk any longer than necessary.
//
// This avoids passing the password via a PGPASSWORD environment variable,
// which would otherwise be readable by anyone able to inspect the process's
// environment (e.g. /proc/<pid>/environ on Linux).
func (s *Service) stagePgPassFile(ctx context.Context, containerID, host string, port int, database, username, password string) (string, func(), error) {
	localFile, err := os.CreateTemp(backupDir, "pgpass-*")
	if err != nil {
		return "", nil, fmt.Errorf("create local pgpass file: %w", err)
	}
	localPath := localFile.Name()
	cleanupLocal := func() { _ = os.Remove(localPath) }

	line := fmt.Sprintf("%s:%d:%s:%s:%s\n",
		escapePgPassField(host),
		port,
		escapePgPassField(database),
		escapePgPassField(username),
		escapePgPassField(password),
	)
	if _, err := localFile.WriteString(line); err != nil {
		_ = localFile.Close()
		cleanupLocal()
		return "", nil, fmt.Errorf("write local pgpass file: %w", err)
	}
	if err := localFile.Close(); err != nil {
		cleanupLocal()
		return "", nil, fmt.Errorf("close local pgpass file: %w", err)
	}
	if err := os.Chmod(localPath, 0o600); err != nil {
		cleanupLocal()
		return "", nil, fmt.Errorf("chmod local pgpass file: %w", err)
	}

	remotePath := "/tmp/.pgpass-" + filepath.Base(localPath)
	if err := exec.CommandContext(ctx, "docker", "cp", localPath, containerID+":"+remotePath).Run(); err != nil {
		cleanupLocal()
		return "", nil, fmt.Errorf("copy pgpass into container: %w", err)
	}
	if err := exec.CommandContext(ctx, "docker", "exec", containerID, "chmod", "0600", remotePath).Run(); err != nil {
		cleanupLocal()
		_ = exec.Command("docker", "exec", containerID, "rm", "-f", remotePath).Run()
		return "", nil, fmt.Errorf("chmod pgpass in container: %w", err)
	}

	cleanup := func() {
		cleanupLocal()
		_ = exec.Command("docker", "exec", containerID, "rm", "-f", remotePath).Run()
	}
	return remotePath, cleanup, nil
}

// escapePgPassField escapes ':' and '\\' per the pgpass file format so that
// values containing those characters (e.g. a generated password) don't
// corrupt the field layout.
func escapePgPassField(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ":", `\:`)
	return s
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
	cmd := exec.CommandContext(ctx, "cat", path)
	return cmd.Output()
}

func (s *Service) writeFile(ctx context.Context, path string, data []byte) error {
	cmd := exec.CommandContext(ctx, "mkdir", "-p", filepath.Dir(path))
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("cat > %s", path))
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}

func (s *Service) removeFile(ctx context.Context, path string) error {
	return exec.CommandContext(ctx, "rm", "-f", path).Run()
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
	if err := exec.Command("mkdir", "-p", backupDir).Run(); err != nil {
		log.Printf("failed to create backup dir %s: %v", backupDir, err)
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
