package transfer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gamepanel/beacon/internal/ignore"
	"gamepanel/beacon/internal/rootfs"
)

const ProtocolVersion = "forge-beacon-transfer/v1"

const (
	DirectionSourceControl     = "source-control"
	DirectionDestinationUpload = "destination-upload"
)

var (
	ErrUnauthorized     = errors.New("invalid transfer credential")
	ErrExpired          = errors.New("transfer credential expired")
	ErrReplayed         = errors.New("transfer credential has already been consumed")
	ErrOffsetMismatch   = errors.New("transfer offset does not match destination offset")
	ErrChecksumMismatch = errors.New("transfer checksum mismatch")
	ErrTransferBounds   = errors.New("transfer exceeds declared total size")
	ErrInvalidBinding   = errors.New("transfer credential binding mismatch")
)

type CredentialClaims struct {
	Version      string    `json:"version"`
	MigrationID  string    `json:"migrationId"`
	ServerID     string    `json:"serverId"`
	SourceNodeID string    `json:"sourceNodeId"`
	TargetNodeID string    `json:"targetNodeId"`
	Direction    string    `json:"direction"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type CredentialRegistration struct {
	Claims         CredentialClaims `json:"claims"`
	CredentialHash string           `json:"credentialHash"`
}

type Metadata struct {
	Version        string     `json:"version"`
	MigrationID    string     `json:"migrationId"`
	ServerID       string     `json:"serverId"`
	SourceNodeID   string     `json:"sourceNodeId"`
	TargetNodeID   string     `json:"targetNodeId"`
	Direction      string     `json:"direction"`
	Phase          string     `json:"phase"`
	CredentialHash string     `json:"credentialHash,omitempty"`
	ExpiresAt      time.Time  `json:"expiresAt"`
	ConsumedAt     *time.Time `json:"consumedAt,omitempty"`
	ArchiveSize    int64      `json:"archiveSize,omitempty"`
	Offset         int64      `json:"offset,omitempty"`
	Checksum       string     `json:"checksum,omitempty"`
	Error          string     `json:"error,omitempty"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type Engine struct {
	dataDir string
	now     func() time.Time
	mu      sync.Mutex
	active  map[string]context.CancelFunc
}

func NewProtocolEngine(dataDir string) (*Engine, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, errors.New("transfer data directory is required")
	}
	root, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	return &Engine{dataDir: root, now: time.Now, active: make(map[string]context.CancelFunc)}, nil
}

func HashCredential(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(sum[:])
}

func NewCredential() (string, error) {
	body := make([]byte, 32)
	if _, err := rand.Read(body); err != nil {
		return "", err
	}
	return hex.EncodeToString(body), nil
}

func (e *Engine) Register(reg CredentialRegistration) error {
	if err := validateClaims(reg.Claims); err != nil {
		return err
	}
	if reg.Claims.ExpiresAt.Before(e.now().UTC()) {
		return ErrExpired
	}
	if len(reg.CredentialHash) != sha256.Size*2 {
		return errors.New("credentialHash must be a SHA-256 hex digest")
	}
	if _, err := hex.DecodeString(reg.CredentialHash); err != nil {
		return errors.New("credentialHash must be a SHA-256 hex digest")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	meta, err := e.load(reg.Claims.MigrationID, reg.Claims.Direction)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if meta.Version != reg.Claims.Version || meta.ServerID != reg.Claims.ServerID || meta.SourceNodeID != reg.Claims.SourceNodeID || meta.TargetNodeID != reg.Claims.TargetNodeID || meta.Direction != reg.Claims.Direction {
			return ErrInvalidBinding
		}
		meta.CredentialHash = strings.ToLower(reg.CredentialHash)
		meta.ExpiresAt = reg.Claims.ExpiresAt.UTC()
		meta.ConsumedAt = nil
		meta.UpdatedAt = e.now().UTC()
		return e.save(meta)
	}
	meta = Metadata{
		Version: ProtocolVersion, MigrationID: reg.Claims.MigrationID, ServerID: reg.Claims.ServerID,
		SourceNodeID: reg.Claims.SourceNodeID, TargetNodeID: reg.Claims.TargetNodeID,
		Direction: reg.Claims.Direction, Phase: "authorized", CredentialHash: strings.ToLower(reg.CredentialHash),
		ExpiresAt: reg.Claims.ExpiresAt.UTC(), UpdatedAt: e.now().UTC(),
	}
	return e.save(meta)
}

func (e *Engine) Authorize(migrationID, direction, credential string) (Metadata, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.authorizeLocked(migrationID, direction, credential)
}

func (e *Engine) authorizeLocked(migrationID, direction, credential string) (Metadata, error) {
	meta, err := e.load(migrationID, direction)
	if err != nil {
		return Metadata{}, ErrUnauthorized
	}
	if meta.Version != ProtocolVersion || meta.MigrationID != migrationID || meta.Direction != direction {
		return Metadata{}, ErrInvalidBinding
	}
	if meta.ConsumedAt != nil {
		return Metadata{}, ErrReplayed
	}
	if !e.now().UTC().Before(meta.ExpiresAt) {
		return Metadata{}, ErrExpired
	}
	actual := HashCredential(credential)
	if subtle.ConstantTimeCompare([]byte(actual), []byte(meta.CredentialHash)) != 1 {
		return Metadata{}, ErrUnauthorized
	}
	return meta, nil
}

func validateClaims(c CredentialClaims) error {
	if c.Version != ProtocolVersion || c.MigrationID == "" || c.ServerID == "" || c.SourceNodeID == "" || c.TargetNodeID == "" || c.ExpiresAt.IsZero() {
		return errors.New("incomplete transfer credential claims")
	}
	if c.Direction != DirectionSourceControl && c.Direction != DirectionDestinationUpload {
		return errors.New("invalid transfer credential direction")
	}
	if !safeID(c.MigrationID) || !safeID(c.ServerID) || !safeID(c.SourceNodeID) || !safeID(c.TargetNodeID) {
		return errors.New("invalid transfer credential binding")
	}
	return nil
}

func safeID(value string) bool {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\\\x00`) {
		return false
	}
	return filepath.Base(value) == value
}

func (e *Engine) PrepareSource(ctx context.Context, migrationID, credential string) (Metadata, error) {
	e.mu.Lock()
	meta, err := e.authorizeLocked(migrationID, DirectionSourceControl, credential)
	if err != nil {
		e.mu.Unlock()
		return Metadata{}, err
	}
	if meta.Phase == "archived" || meta.Phase == "uploaded" {
		e.mu.Unlock()
		return meta, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	e.active[migrationID] = cancel
	meta.Phase = "archiving"
	meta.UpdatedAt = e.now().UTC()
	if err := e.save(meta); err != nil {
		delete(e.active, migrationID)
		cancel()
		e.mu.Unlock()
		return Metadata{}, err
	}
	e.mu.Unlock()

	archive := e.archivePath(migrationID)
	root := filepath.Join(e.dataDir, meta.ServerID)
	checksum, size, archiveErr := createSecureArchive(ctx, root, archive)

	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.active, migrationID)
	cancel()
	if archiveErr != nil {
		meta.Phase, meta.Error = "failed", archiveErr.Error()
		meta.UpdatedAt = e.now().UTC()
		_ = e.save(meta)
		return meta, archiveErr
	}
	meta.Phase, meta.Checksum, meta.ArchiveSize, meta.Offset, meta.Error = "archived", checksum, size, 0, ""
	meta.UpdatedAt = e.now().UTC()
	if err := e.save(meta); err != nil {
		return Metadata{}, err
	}
	return meta, nil
}

func createSecureArchive(ctx context.Context, serverRoot, archivePath string) (checksum string, size int64, err error) {
	fsys, err := rootfs.New(serverRoot)
	if err != nil {
		return "", 0, err
	}
	defer fsys.Close()
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o750); err != nil {
		return "", 0, err
	}
	out, err := os.OpenFile(archivePath+".tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return "", 0, err
	}
	committed := false
	defer func() {
		_ = out.Close()
		if !committed {
			_ = os.Remove(archivePath + ".tmp")
		}
	}()
	hasher := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(out, hasher))
	tw := tar.NewWriter(gz)
	denylist := ignore.NewIgnoreList(nil)
	if ignoreFile, openErr := fsys.Open(".pteroignore"); openErr == nil {
		if loaded, loadErr := ignore.LoadIgnoreReader(ignoreFile); loadErr == nil {
			denylist = loaded
		}
		_ = ignoreFile.Close()
	}
	if err := archiveDirectory(ctx, fsys, tw, "", denylist); err != nil {
		_ = tw.Close()
		_ = gz.Close()
		return "", 0, err
	}
	if err := tw.Close(); err != nil {
		return "", 0, err
	}
	if err := gz.Close(); err != nil {
		return "", 0, err
	}
	if err := out.Sync(); err != nil {
		return "", 0, err
	}
	if err := out.Close(); err != nil {
		return "", 0, err
	}
	info, err := os.Stat(archivePath + ".tmp")
	if err != nil {
		return "", 0, err
	}
	if err := os.Rename(archivePath+".tmp", archivePath); err != nil {
		return "", 0, err
	}
	committed = true
	return hex.EncodeToString(hasher.Sum(nil)), info.Size(), nil
}

func archiveDirectory(ctx context.Context, fsys *rootfs.FS, tw *tar.Writer, directory string, denylist interface{ IsIgnored(string) bool }) error {
	entries, err := fsys.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(directory, entry.Name()))
		if directory == "" && (entry.Name() == ".backups" || entry.Name() == ".uploads" || entry.Name() == ".transfers") {
			continue
		}
		if denylist != nil && (denylist.IsIgnored(name) || denylist.IsIgnored(entry.Name())) {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := fsys.Stat(name)
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = name
		if info.IsDir() {
			header.Name += "/"
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			if err := archiveDirectory(ctx, fsys, tw, name, denylist); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		file, err := fsys.Open(name)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tw, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func (e *Engine) SourceArchive(migrationID, credential string, offset int64) (*os.File, Metadata, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	meta, err := e.authorizeLocked(migrationID, DirectionSourceControl, credential)
	if err != nil {
		return nil, Metadata{}, err
	}
	if meta.Phase != "archived" && meta.Phase != "uploading" && meta.Phase != "uploaded" {
		return nil, meta, errors.New("source archive is not ready")
	}
	if offset < 0 || offset > meta.ArchiveSize {
		return nil, meta, ErrOffsetMismatch
	}
	file, err := os.Open(e.archivePath(migrationID))
	if err != nil {
		return nil, meta, err
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, meta, err
	}
	return file, meta, nil
}

func (e *Engine) DestinationOffset(migrationID, credential string) (Metadata, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.authorizeLocked(migrationID, DirectionDestinationUpload, credential)
}

func (e *Engine) AppendDestination(ctx context.Context, migrationID, credential string, offset, total int64, checksum string, body io.Reader) (Metadata, error) {
	e.mu.Lock()
	meta, err := e.authorizeLocked(migrationID, DirectionDestinationUpload, credential)
	if err != nil {
		e.mu.Unlock()
		return Metadata{}, err
	}
	if meta.Phase == "restored" || meta.Phase == "activated" {
		e.mu.Unlock()
		return meta, ErrReplayed
	}
	if total <= 0 || total > 32*1024*1024*1024 || offset < 0 || offset != meta.Offset || offset > total {
		e.mu.Unlock()
		return meta, ErrOffsetMismatch
	}
	if meta.ArchiveSize != 0 && meta.ArchiveSize != total {
		e.mu.Unlock()
		return meta, ErrTransferBounds
	}
	if meta.Checksum != "" && !strings.EqualFold(meta.Checksum, checksum) {
		e.mu.Unlock()
		return meta, ErrChecksumMismatch
	}
	meta.ArchiveSize, meta.Checksum, meta.Phase, meta.UpdatedAt = total, strings.ToLower(checksum), "uploading", e.now().UTC()
	if err := e.save(meta); err != nil {
		e.mu.Unlock()
		return Metadata{}, err
	}
	path := e.incomingPath(migrationID)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		e.mu.Unlock()
		return meta, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		e.mu.Unlock()
		return meta, err
	}
	info, err := file.Stat()
	if err != nil || info.Size() != offset {
		_ = file.Close()
		e.mu.Unlock()
		return meta, ErrOffsetMismatch
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		_ = file.Close()
		e.mu.Unlock()
		return meta, err
	}
	ctx, cancel := context.WithCancel(ctx)
	e.active[migrationID] = cancel
	e.mu.Unlock()

	remaining := total - offset
	written, copyErr := io.Copy(file, io.LimitReader(&contextReader{ctx: ctx, reader: body}, remaining+1))
	syncErr := file.Sync()
	closeErr := file.Close()

	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.active, migrationID)
	cancel()
	meta.Offset = offset + written
	meta.UpdatedAt = e.now().UTC()
	if copyErr != nil || syncErr != nil || closeErr != nil || written > remaining {
		meta.Error = errors.Join(copyErr, syncErr, closeErr, func() error {
			if written > remaining {
				return ErrTransferBounds
			}
			return nil
		}()).Error()
		_ = e.save(meta)
		return meta, errors.New(meta.Error)
	}
	if meta.Offset < total {
		_ = e.save(meta)
		return meta, nil
	}
	actual, err := checksumFile(path)
	if err != nil {
		meta.Error = err.Error()
		_ = e.save(meta)
		return meta, err
	}
	if !strings.EqualFold(actual, checksum) {
		meta.Phase, meta.Error = "failed", ErrChecksumMismatch.Error()
		_ = e.save(meta)
		return meta, ErrChecksumMismatch
	}
	meta.Phase, meta.Error = "verified", ""
	_ = e.save(meta)
	return meta, nil
}

func (e *Engine) RestoreDestination(ctx context.Context, migrationID, credential string) (Metadata, error) {
	e.mu.Lock()
	meta, err := e.authorizeLocked(migrationID, DirectionDestinationUpload, credential)
	if err != nil {
		e.mu.Unlock()
		return Metadata{}, err
	}
	if meta.Phase == "restored" {
		e.mu.Unlock()
		return meta, nil
	}
	if meta.Phase != "verified" {
		e.mu.Unlock()
		return meta, errors.New("destination archive is not verified")
	}
	staging := e.restorePath(migrationID)
	_ = os.RemoveAll(staging)
	if err := os.MkdirAll(staging, 0o750); err != nil {
		e.mu.Unlock()
		return meta, err
	}
	ctx, cancel := context.WithCancel(ctx)
	e.active[migrationID] = cancel
	meta.Phase, meta.UpdatedAt = "restoring", e.now().UTC()
	_ = e.save(meta)
	e.mu.Unlock()

	restoreErr := extractSecureArchive(ctx, e.incomingPath(migrationID), staging)
	if restoreErr == nil {
		restoreErr = activateWithRollback(e.dataDir, meta.ServerID, staging, migrationID)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.active, migrationID)
	cancel()
	if restoreErr != nil {
		meta.Phase, meta.Error, meta.UpdatedAt = "failed", restoreErr.Error(), e.now().UTC()
		_ = e.save(meta)
		return meta, restoreErr
	}
	now := e.now().UTC()
	meta.Phase, meta.Error, meta.UpdatedAt = "restored", "", now
	if err := e.save(meta); err != nil {
		return Metadata{}, err
	}
	_ = os.Remove(e.incomingPath(migrationID))
	return meta, nil
}

func extractSecureArchive(ctx context.Context, archivePath, staging string) error {
	fsys, err := rootfs.New(staging)
	if err != nil {
		return err
	}
	defer fsys.Close()
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name, err := rootfs.Clean(strings.TrimSuffix(h.Name, "/"))
		if err != nil || name == "" {
			if h.Name == "./" {
				continue
			}
			return rootfs.ErrInvalidPath
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := fsys.MkdirAll(name, h.FileInfo().Mode().Perm()); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if h.Size < 0 || h.Size > 32*1024*1024*1024 {
				return ErrTransferBounds
			}
			if err := fsys.AtomicWriteExact(name, tr, h.Size, h.Size, h.FileInfo().Mode().Perm()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry type %d", h.Typeflag)
		}
	}
}

func activateWithRollback(dataDir, serverID, staging, migrationID string) error {
	canonical := filepath.Join(dataDir, serverID)
	backup := filepath.Join(dataDir, ".transfers", migrationID, "previous")
	_ = os.RemoveAll(backup)
	hadCanonical := false
	if _, err := os.Lstat(canonical); err == nil {
		hadCanonical = true
		if err := os.Rename(canonical, backup); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(staging, canonical); err != nil {
		if hadCanonical {
			_ = os.Rename(backup, canonical)
		}
		return err
	}
	return nil
}

func (e *Engine) FinalizeDestination(migrationID, credential string) error {
	e.mu.Lock()
	meta, err := e.authorizeLocked(migrationID, DirectionDestinationUpload, credential)
	if err != nil {
		e.mu.Unlock()
		return err
	}
	if meta.Phase != "restored" {
		e.mu.Unlock()
		return errors.New("destination is not restored")
	}
	now := e.now().UTC()
	meta.Phase, meta.ConsumedAt, meta.UpdatedAt = "activated", &now, now
	if err := e.save(meta); err != nil {
		e.mu.Unlock()
		return err
	}
	e.mu.Unlock()
	dir := e.transferDir(migrationID)
	_ = os.RemoveAll(filepath.Join(dir, "previous"))
	_ = os.RemoveAll(filepath.Join(dir, "restored"))
	_ = os.Remove(filepath.Join(dir, "incoming.tar.gz"))
	return nil
}

func (e *Engine) Cancel(migrationID string) error {
	e.mu.Lock()
	if cancel := e.active[migrationID]; cancel != nil {
		cancel()
		delete(e.active, migrationID)
	}
	for _, direction := range []string{DirectionSourceControl, DirectionDestinationUpload} {
		meta, err := e.load(migrationID, direction)
		if err == nil {
			now := e.now().UTC()
			meta.Phase, meta.ConsumedAt, meta.UpdatedAt = "cancelled", &now, now
			_ = e.save(meta)
		}
	}
	e.mu.Unlock()
	dir := filepath.Join(e.dataDir, ".transfers", migrationID)
	previous := filepath.Join(dir, "previous")
	if entries, err := os.ReadDir(previous); err == nil {
		_ = entries
		for _, direction := range []string{DirectionDestinationUpload, DirectionSourceControl} {
			if meta, loadErr := e.load(migrationID, direction); loadErr == nil {
				canonical := filepath.Join(e.dataDir, meta.ServerID)
				_ = os.RemoveAll(canonical)
				_ = os.Rename(previous, canonical)
				break
			}
		}
	}
	_ = os.RemoveAll(dir)
	return nil
}

func (e *Engine) CleanupSource(migrationID, credential string) error {
	e.mu.Lock()
	meta, err := e.authorizeLocked(migrationID, DirectionSourceControl, credential)
	if err != nil {
		e.mu.Unlock()
		return err
	}
	now := e.now().UTC()
	meta.Phase, meta.ConsumedAt, meta.UpdatedAt = "cleaned", &now, now
	if err := e.save(meta); err != nil {
		e.mu.Unlock()
		return err
	}
	e.mu.Unlock()
	return os.RemoveAll(filepath.Join(e.dataDir, meta.ServerID))
}

func (e *Engine) Status(migrationID, direction, credential string) (Metadata, error) {
	return e.Authorize(migrationID, direction, credential)
}
func (e *Engine) ActiveCount() int { e.mu.Lock(); defer e.mu.Unlock(); return len(e.active) }

func (e *Engine) transferDir(id string) string { return filepath.Join(e.dataDir, ".transfers", id) }
func (e *Engine) metadataPath(id, direction string) string {
	return filepath.Join(e.transferDir(id), direction+".json")
}
func (e *Engine) archivePath(id string) string {
	return filepath.Join(e.transferDir(id), "source.tar.gz")
}
func (e *Engine) incomingPath(id string) string {
	return filepath.Join(e.transferDir(id), "incoming.tar.gz")
}
func (e *Engine) restorePath(id string) string { return filepath.Join(e.transferDir(id), "restored") }

func (e *Engine) load(id, direction string) (Metadata, error) {
	var meta Metadata
	body, err := os.ReadFile(e.metadataPath(id, direction))
	if err != nil {
		return meta, err
	}
	return meta, json.Unmarshal(body, &meta)
}
func (e *Engine) save(meta Metadata) error {
	dir := e.transferDir(meta.MigrationID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	body, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmp := e.metadataPath(meta.MigrationID, meta.Direction) + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, e.metadataPath(meta.MigrationID, meta.Direction))
}

func checksumFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
