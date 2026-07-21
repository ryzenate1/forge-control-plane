package backup

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gamepanel/beacon/internal/ignore"
	"gamepanel/beacon/internal/rootfs"
	"golang.org/x/time/rate"
)

const metadataSuffix = ".metadata.json"

type localMetadata struct {
	Checksum    string    `json:"checksum"`
	Size        int64     `json:"size"`
	Created     time.Time `json:"created"`
	IgnoredFiles []string `json:"ignored_files,omitempty"`
}

type restoreJournal struct {
	Staging  string `json:"staging"`
	Rollback string `json:"rollback"`
	Phase    string `json:"phase"`
}

// LocalBackup stores archives under backupRoot/<server namespace>. backupRoot
// must be daemon-owned and must not be inside a user server root.
type LocalBackup struct {
	backupRoot     string
	legacyDataRoot string
	migrationMu    sync.Mutex
	progress       ProgressFunc
	progressMu     sync.Mutex
	writeLimit     int64
}

// NewLocalBackup configures daemon-owned storage. legacyDataRoot is optional;
// when supplied, archives from <legacyDataRoot>/<namespace>/.backups are
// safely migrated on first access.
func NewLocalBackup(backupRoot string, legacyDataRoot ...string) (*LocalBackup, error) {
	root, err := canonicalDirectory(backupRoot, true)
	if err != nil {
		return nil, fmt.Errorf("initialize local backup root: %w", err)
	}
	adapter := &LocalBackup{backupRoot: root}
	if len(legacyDataRoot) > 0 && strings.TrimSpace(legacyDataRoot[0]) != "" {
		legacy, err := canonicalDirectory(legacyDataRoot[0], true)
		if err != nil {
			return nil, fmt.Errorf("initialize legacy server root: %w", err)
		}
		adapter.legacyDataRoot = legacy
	}
	return adapter, nil
}

func (l *LocalBackup) Type() AdapterType { return LocalAdapter }

func (l *LocalBackup) SetProgressCallback(fn ProgressFunc) {
	l.progressMu.Lock()
	defer l.progressMu.Unlock()
	l.progress = fn
}

func (l *LocalBackup) SetWriteLimit(bytesPerSec int64) {
	l.writeLimit = bytesPerSec
}

func (l *LocalBackup) reportProgress(bytesProcessed, totalBytes int64, phase string) {
	l.progressMu.Lock()
	fn := l.progress
	l.progressMu.Unlock()
	if fn != nil {
		fn(BackupProgress{BytesProcessed: bytesProcessed, TotalBytes: totalBytes, Phase: phase})
	}
}

func (l *LocalBackup) namespaceDir(namespace string, create bool) (string, error) {
	if !validNamespace(namespace) {
		return "", ErrInvalidNamespace
	}
	dir := filepath.Join(l.backupRoot, namespace)
	if create || l.legacyDataRoot != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return "", err
		}
	}
	if l.legacyDataRoot != "" {
		if err := l.migrateLegacyBackups(namespace, dir); err != nil {
			return "", fmt.Errorf("migrate legacy backups: %w", err)
		}
	}
	return dir, nil
}

func (l *LocalBackup) migrateLegacyBackups(namespace, destinationDir string) error {
	l.migrationMu.Lock()
	defer l.migrationMu.Unlock()
	serverRoot := filepath.Join(l.legacyDataRoot, namespace)
	fsys, err := rootfs.New(serverRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer fsys.Close()
	entries, err := fsys.ReadDir(".backups")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !validBackupName(name) {
			continue
		}
		target := filepath.Join(destinationDir, name)
		if _, err := os.Stat(target); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		source, err := fsys.Open(path.Join(".backups", name))
		if err != nil {
			return err
		}
		info, err := source.Stat()
		if err != nil || !info.Mode().IsRegular() {
			_ = source.Close()
			if err != nil {
				return err
			}
			return fmt.Errorf("legacy backup %q is not a regular file", name)
		}
		temp, err := os.CreateTemp(destinationDir, ".legacy-backup-*")
		if err != nil {
			_ = source.Close()
			return err
		}
		tempName := temp.Name()
		copyErr := temp.Chmod(0o640)
		if copyErr == nil {
			_, copyErr = io.Copy(temp, source)
		}
		sourceCloseErr := source.Close()
		if copyErr == nil {
			copyErr = sourceCloseErr
		}
		if copyErr == nil {
			copyErr = temp.Sync()
		}
		if closeErr := temp.Close(); copyErr == nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			_ = os.Remove(tempName)
			return copyErr
		}
		if err := os.Rename(tempName, target); err != nil {
			_ = os.Remove(tempName)
			return err
		}
		migratedInfo, err := os.Stat(target)
		if err != nil {
			return err
		}
		checksum, err := calculateChecksum(target)
		if err != nil {
			return err
		}
		if err := writeMetadata(target, localMetadata{Checksum: checksum, Size: migratedInfo.Size(), Created: migratedInfo.ModTime().UTC()}); err != nil {
			_ = os.Remove(target)
			return err
		}
		if err := fsys.RemoveAll(path.Join(".backups", name)); err != nil {
			return err
		}
	}
	return nil
}

func (l *LocalBackup) archivePath(namespace, name string, createDir bool) (string, error) {
	if !validBackupName(name) {
		return "", ErrInvalidName
	}
	dir, err := l.namespaceDir(namespace, createDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func (l *LocalBackup) Create(ctx context.Context, serverRoot, namespace, name string, ignored []string) (*BackupInfo, error) {
	l.reportProgress(0, 0, "creating backup")
	canonicalRoot, err := canonicalDirectory(serverRoot, false)
	if err != nil {
		return nil, fmt.Errorf("canonicalize server root: %w", err)
	}
	backupPath, err := l.archivePath(namespace, name, true)
	if err != nil {
		return nil, err
	}
	if sameOrDescendant(canonicalRoot, l.backupRoot) || sameOrDescendant(l.backupRoot, canonicalRoot) {
		return nil, errors.New("backup root and server root must be separate")
	}
	sourceFS, err := rootfs.New(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("secure server root: %w", err)
	}
	defer sourceFS.Close()
	patterns := append([]string(nil), ignored...)
	if ignoreFile, openErr := sourceFS.Open(".pteroignore"); openErr == nil {
		parsed, parseErr := ignore.LoadIgnoreReader(ignoreFile)
		closeErr := ignoreFile.Close()
		if parseErr != nil {
			return nil, fmt.Errorf("parse .pteroignore: %w", parseErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close .pteroignore: %w", closeErr)
		}
		patterns = append(patterns, parsed.Patterns()...)
	} else if !errors.Is(openErr, os.ErrNotExist) {
		return nil, fmt.Errorf("open .pteroignore: %w", openErr)
	}

	l.reportProgress(0, 0, "archiving files")

	temp, err := os.OpenFile(backupPath+".partial", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, fmt.Errorf("create backup staging file: %w", err)
	}
	partial := temp.Name()
	committed := false
	defer func() {
		_ = temp.Close()
		if !committed {
			_ = os.Remove(partial)
		}
	}()

	var zipTarget io.Writer = temp
	if l.writeLimit > 0 {
		limiter := rate.NewLimiter(rate.Limit(l.writeLimit), int(l.writeLimit))
		zipTarget = &rateLimitedWriter{writer: temp, limiter: limiter, ctx: ctx}
	}
	zipper := zip.NewWriter(zipTarget)
	denylist := ignore.NewIgnoreList(patterns)
	walkErr := filepath.WalkDir(canonicalRoot, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(canonicalRoot, filePath)
		if err != nil || rel == "." {
			return err
		}
		rel = filepath.ToSlash(rel)
		firstComponent := strings.SplitN(rel, "/", 2)[0]
		if firstComponent == ".backups" || firstComponent == ".uploads" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if denylist.IsIgnored(rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			header := &zip.FileHeader{Name: rel + "/", Method: zip.Store}
			header.SetMode(info.Mode().Perm() | os.ModeDir)
			header.SetModTime(info.ModTime())
			_, err = zipper.CreateHeader(header)
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		source, err := sourceFS.Open(rel)
		if err != nil {
			return err
		}
		info, err = source.Stat()
		if err != nil || !info.Mode().IsRegular() {
			_ = source.Close()
			if err != nil {
				return err
			}
			return fmt.Errorf("source changed type while backing up %q", rel)
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			_ = source.Close()
			return err
		}
		header.Name = rel
		header.Method = zip.Deflate
		writer, err := zipper.CreateHeader(header)
		if err == nil {
			_, err = copyWithContext(ctx, writer, source)
		}
		closeErr := source.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
	if walkErr != nil {
		_ = zipper.Close()
		return nil, fmt.Errorf("create backup archive: %w", walkErr)
	}
	if err := zipper.Close(); err != nil {
		return nil, fmt.Errorf("finalize backup archive: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return nil, fmt.Errorf("sync backup archive: %w", err)
	}
	if err := temp.Close(); err != nil {
		return nil, fmt.Errorf("close backup archive: %w", err)
	}
	if err := os.Rename(partial, backupPath); err != nil {
		return nil, fmt.Errorf("commit backup archive: %w", err)
	}
	committed = true

	info, err := os.Stat(backupPath)
	if err != nil {
		_ = os.Remove(backupPath)
		return nil, err
	}
	checksum, err := calculateChecksum(backupPath)
	if err != nil {
		_ = os.Remove(backupPath)
		return nil, err
	}
	metadata := localMetadata{Checksum: checksum, Size: info.Size(), Created: info.ModTime().UTC(), IgnoredFiles: patterns}
	if err := writeMetadata(backupPath, metadata); err != nil {
		_ = os.Remove(backupPath)
		return nil, err
	}
	l.reportProgress(info.Size(), info.Size(), "completed")
	return backupInfo(name, metadata, LocalAdapter, ""), nil
}

func (l *LocalBackup) List(namespace string) ([]BackupInfo, error) {
	dir, err := l.namespaceDir(namespace, false)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []BackupInfo{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read backup directory: %w", err)
	}
	backups := make([]BackupInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !validBackupName(entry.Name()) {
			continue
		}
		metadata, err := readOrCreateMetadata(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read metadata for %s: %w", entry.Name(), err)
		}
		backups = append(backups, *backupInfo(entry.Name(), metadata, LocalAdapter, ""))
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].Created.Before(backups[j].Created) })
	return backups, nil
}

func (l *LocalBackup) Get(namespace, name string) (*BackupInfo, error) {
	backupPath, err := l.archivePath(namespace, name, false)
	if err != nil {
		return nil, err
	}
	metadata, err := readOrCreateMetadata(backupPath)
	if err != nil {
		return nil, fmt.Errorf("get backup: %w", err)
	}
	return backupInfo(name, metadata, LocalAdapter, ""), nil
}

func (l *LocalBackup) Delete(namespace, name string) error {
	backupPath, err := l.archivePath(namespace, name, false)
	if err != nil {
		return err
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	if err := os.Remove(backupPath + metadataSuffix); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete backup metadata: %w", err)
	}
	return nil
}

func (l *LocalBackup) Restore(ctx context.Context, namespace, name, serverRoot string, truncate bool) error {
	l.reportProgress(0, 0, "restoring backup")
	defer l.reportProgress(0, 0, "completed")
	backupPath, err := l.archivePath(namespace, name, false)
	if err != nil {
		return err
	}
	metadata, err := readOrCreateMetadata(backupPath)
	if err != nil {
		return fmt.Errorf("load backup metadata: %w", err)
	}
	actualChecksum, err := calculateChecksum(backupPath)
	if err != nil {
		return fmt.Errorf("checksum backup: %w", err)
	}
	if !strings.EqualFold(actualChecksum, metadata.Checksum) {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, metadata.Checksum, actualChecksum)
	}

	absoluteRoot, err := filepath.Abs(serverRoot)
	if err != nil {
		return fmt.Errorf("resolve live root: %w", err)
	}
	canonicalParent, err := filepath.EvalSymlinks(filepath.Dir(absoluteRoot))
	if err != nil {
		return fmt.Errorf("canonicalize live parent: %w", err)
	}
	canonicalRoot := filepath.Join(canonicalParent, filepath.Base(absoluteRoot))
	if err := recoverInterruptedRestore(canonicalRoot); err != nil {
		return fmt.Errorf("recover interrupted restore: %w", err)
	}
	canonicalRoot, err = canonicalDirectory(canonicalRoot, false)
	if err != nil {
		return fmt.Errorf("canonicalize live root: %w", err)
	}
	if sameOrDescendant(canonicalRoot, l.backupRoot) || sameOrDescendant(l.backupRoot, canonicalRoot) {
		return errors.New("backup root and server root must be separate")
	}
	reader, err := zip.OpenReader(backupPath)
	if err != nil {
		return fmt.Errorf("open backup archive: %w", err)
	}
	defer reader.Close()
	if err := validateArchive(reader.File); err != nil {
		return err
	}

	if !truncate {
		// Non-truncate restore: extract directly to target directory
		targetFS, err := rootfs.New(canonicalRoot)
		if err != nil {
			return fmt.Errorf("secure server root: %w", err)
		}
		defer targetFS.Close()
		if err := extractArchive(ctx, targetFS, reader.File); err != nil {
			return err
		}
		return syncDirectory(filepath.Dir(canonicalRoot))
	}

	parent := filepath.Dir(canonicalRoot)
	base := filepath.Base(canonicalRoot)
	staging, err := os.MkdirTemp(parent, "."+base+".restore-")
	if err != nil {
		return fmt.Errorf("create restore staging directory: %w", err)
	}
	stagingBase := filepath.Base(staging)
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = os.RemoveAll(staging)
		}
	}()
	stagingFS, err := rootfs.New(staging)
	if err != nil {
		return fmt.Errorf("secure restore staging root: %w", err)
	}
	if err := extractArchive(ctx, stagingFS, reader.File); err != nil {
		_ = stagingFS.Close()
		return err
	}
	if err := stagingFS.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	rollback := filepath.Join(parent, "."+base+".rollback-"+fmt.Sprint(time.Now().UnixNano()))
	journalPath := restoreJournalPath(canonicalRoot)
	journal := restoreJournal{Staging: staging, Rollback: rollback, Phase: "prepared"}
	if err := writeRestoreJournal(journalPath, journal); err != nil {
		return fmt.Errorf("prepare restore journal: %w", err)
	}
	journalCommitted := true
	defer func() {
		if !journalCommitted {
			_ = os.Remove(journalPath)
		}
	}()
	if err := os.Rename(canonicalRoot, rollback); err != nil {
		journalCommitted = false
		return fmt.Errorf("move live data to rollback directory: %w", err)
	}
	journal.Phase = "live-moved"
	if err := writeRestoreJournal(journalPath, journal); err != nil {
		rollbackErr := os.Rename(rollback, canonicalRoot)
		if rollbackErr == nil {
			journalCommitted = false
		}
		return fmt.Errorf("record live-data move: %w (rollback error: %v)", err, rollbackErr)
	}
	if err := os.Rename(filepath.Join(parent, stagingBase), canonicalRoot); err != nil {
		rollbackErr := os.Rename(rollback, canonicalRoot)
		if rollbackErr == nil {
			journalCommitted = false
		}
		if rollbackErr != nil {
			return fmt.Errorf("activate restored data: %w (rollback failed: %v; original data remains at %s)", err, rollbackErr, rollback)
		}
		return fmt.Errorf("activate restored data: %w", err)
	}
	cleanupStaging = false
	journal.Phase = "activated"
	if err := writeRestoreJournal(journalPath, journal); err != nil {
		return fmt.Errorf("record restored-data activation: %w (original data retained at %s)", err, rollback)
	}
	if err := syncDirectory(parent); err != nil {
		// The restored tree is live and the rollback tree is retained for manual
		// recovery if the parent directory could not be made durable.
		return fmt.Errorf("sync restored directory: %w (original data retained at %s)", err, rollback)
	}
	if err := os.RemoveAll(rollback); err != nil {
		return fmt.Errorf("restored successfully but could not remove rollback data: %w", err)
	}
	if err := os.Remove(journalPath); err != nil {
		return fmt.Errorf("restored successfully but could not remove restore journal: %w", err)
	}
	journalCommitted = false
	return syncDirectory(parent)
}

func (l *LocalBackup) Download(namespace, name string) (io.ReadCloser, error) {
	backupPath, err := l.archivePath(namespace, name, false)
	if err != nil {
		return nil, err
	}
	metadata, err := readOrCreateMetadata(backupPath)
	if err != nil {
		return nil, err
	}
	actual, err := calculateChecksum(backupPath)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(actual, metadata.Checksum) {
		return nil, fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, metadata.Checksum, actual)
	}
	return os.Open(backupPath)
}

func validateArchive(files []*zip.File) error {
	seen := make(map[string]bool, len(files)) // value is directory
	for _, file := range files {
		name := file.Name
		if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || path.Clean(name) == "." {
			return fmt.Errorf("invalid archive path %q", name)
		}
		clean := strings.TrimSuffix(path.Clean(name), "/")
		if clean == "" || clean == ".." || strings.HasPrefix(clean, "../") || clean != strings.TrimSuffix(name, "/") {
			return fmt.Errorf("invalid archive path %q", name)
		}
		mode := file.Mode()
		isDir := strings.HasSuffix(name, "/") && mode.IsDir()
		if !isDir && !mode.IsRegular() {
			return fmt.Errorf("unsupported archive entry type for %q", name)
		}
		if strings.HasSuffix(name, "/") != isDir {
			return fmt.Errorf("conflicting archive entry type for %q", name)
		}
		if _, exists := seen[clean]; exists {
			return fmt.Errorf("duplicate archive entry %q", name)
		}
		for ancestor := path.Dir(clean); ancestor != "."; ancestor = path.Dir(ancestor) {
			if ancestorIsDir, exists := seen[ancestor]; exists && !ancestorIsDir {
				return fmt.Errorf("archive entry %q is beneath file %q", name, ancestor)
			}
		}
		if !isDir {
			prefix := clean + "/"
			for prior := range seen {
				if strings.HasPrefix(prior, prefix) {
					return fmt.Errorf("archive file %q conflicts with child %q", name, prior)
				}
			}
		}
		seen[clean] = isDir
	}
	return nil
}

func extractArchive(ctx context.Context, destination *rootfs.FS, files []*zip.File) error {
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := strings.TrimSuffix(file.Name, "/")
		if file.FileInfo().IsDir() {
			if err := destination.MkdirAll(name, normalizedDirMode(file.Mode())); err != nil {
				return fmt.Errorf("create restored directory %q: %w", name, err)
			}
			continue
		}
		if parent := path.Dir(name); parent != "." {
			if err := destination.MkdirAll(parent, 0o750); err != nil {
				return fmt.Errorf("create restored parent %q: %w", parent, err)
			}
		}
		source, err := file.Open()
		if err != nil {
			return fmt.Errorf("open archive entry %q: %w", name, err)
		}
		output, err := destination.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, normalizedFileMode(file.Mode()))
		if err != nil {
			_ = source.Close()
			return fmt.Errorf("create restored file %q: %w", name, err)
		}
		_, copyErr := copyWithContext(ctx, output, source)
		closeOutputErr := output.Close()
		closeSourceErr := source.Close()
		if copyErr != nil {
			return fmt.Errorf("extract archive entry %q: %w", name, copyErr)
		}
		if closeOutputErr != nil {
			return closeOutputErr
		}
		if closeSourceErr != nil {
			return closeSourceErr
		}
	}
	return nil
}

func readOrCreateMetadata(backupPath string) (localMetadata, error) {
	body, err := os.ReadFile(backupPath + metadataSuffix)
	if err == nil {
		var metadata localMetadata
		if err := json.Unmarshal(body, &metadata); err != nil {
			return localMetadata{}, err
		}
		if metadata.Checksum == "" || metadata.Size < 0 {
			return localMetadata{}, errors.New("invalid backup metadata")
		}
		return metadata, nil
	}
	if !os.IsNotExist(err) {
		return localMetadata{}, err
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		return localMetadata{}, err
	}
	checksum, err := calculateChecksum(backupPath)
	if err != nil {
		return localMetadata{}, err
	}
	metadata := localMetadata{Checksum: checksum, Size: info.Size(), Created: info.ModTime().UTC()}
	if err := writeMetadata(backupPath, metadata); err != nil {
		return localMetadata{}, err
	}
	return metadata, nil
}

func writeMetadata(backupPath string, metadata localMetadata) error {
	body, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(backupPath), ".metadata-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o640); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(body); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, backupPath+metadataSuffix)
}

func backupInfo(name string, metadata localMetadata, adapter AdapterType, remotePath string) *BackupInfo {
	return &BackupInfo{
		UUID: strings.TrimSuffix(name, ".zip"), Name: name, Checksum: metadata.Checksum,
		Size: metadata.Size, Status: "completed", Created: metadata.Created,
		CompletedAt: metadata.Created, Adapter: adapter, RemotePath: remotePath,
		IgnoredFiles: metadata.IgnoredFiles,
	}
}

func copyWithContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	return io.Copy(destination, &contextReader{ctx: ctx, reader: source})
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(body []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(body)
}

func normalizedFileMode(mode os.FileMode) os.FileMode {
	mode &= os.ModePerm
	if mode == 0 {
		return 0o640
	}
	return mode
}

func normalizedDirMode(mode os.FileMode) os.FileMode {
	mode &= os.ModePerm
	if mode == 0 {
		return 0o750
	}
	return mode
}

func sameOrDescendant(candidate, root string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// RecoverRestoreJournals reconciles restore swaps interrupted by a daemon or
// host crash before servers are reconstructed and started.
func RecoverRestoreJournals(serverDataRoot string) error {
	root, err := canonicalDirectory(serverDataRoot, true)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	const suffix = ".restore-journal.json"
	var recoveryErrors []error
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, ".") || !strings.HasSuffix(name, suffix) {
			continue
		}
		namespace := strings.TrimSuffix(strings.TrimPrefix(name, "."), suffix)
		if !validNamespace(namespace) {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("invalid restore journal name %q", name))
			continue
		}
		if err := recoverInterruptedRestore(filepath.Join(root, namespace)); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("recover %s: %w", namespace, err))
		}
	}
	return errors.Join(recoveryErrors...)
}

func restoreJournalPath(serverRoot string) string {
	return filepath.Join(filepath.Dir(serverRoot), "."+filepath.Base(serverRoot)+".restore-journal.json")
}

func writeRestoreJournal(journalPath string, journal restoreJournal) error {
	body, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(journalPath), ".restore-journal-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(body); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, journalPath); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(journalPath))
}

func recoverInterruptedRestore(serverRoot string) error {
	journalPath := restoreJournalPath(serverRoot)
	body, err := os.ReadFile(journalPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal restoreJournal
	if err := json.Unmarshal(body, &journal); err != nil {
		return fmt.Errorf("invalid restore journal: %w", err)
	}
	parent := filepath.Dir(serverRoot)
	base := filepath.Base(serverRoot)
	stagingClean, _ := filepath.EvalSymlinks(journal.Staging)
	if stagingClean == "" {
		stagingClean = filepath.Clean(journal.Staging)
	}
	rollbackClean, _ := filepath.EvalSymlinks(journal.Rollback)
	if rollbackClean == "" {
		rollbackClean = filepath.Clean(journal.Rollback)
	}
	if filepath.Dir(stagingClean) != parent || filepath.Dir(rollbackClean) != parent ||
		!strings.HasPrefix(filepath.Base(stagingClean), "."+base+".restore-") ||
		!strings.HasPrefix(filepath.Base(rollbackClean), "."+base+".rollback-") {
		return errors.New("restore journal contains paths outside the server parent")
	}
	rootExists := pathExists(serverRoot)
	rollbackExists := pathExists(journal.Rollback)
	switch journal.Phase {
	case "activated":
		if !rootExists {
			if !rollbackExists {
				return errors.New("activated restore has neither live nor rollback data")
			}
			if err := os.Rename(journal.Rollback, serverRoot); err != nil {
				return err
			}
		} else if rollbackExists {
			if err := os.RemoveAll(journal.Rollback); err != nil {
				return err
			}
		}
	case "prepared", "live-moved":
		// Until activation is durably journaled, prefer the original tree.
		if rollbackExists {
			if rootExists {
				if err := os.RemoveAll(serverRoot); err != nil {
					return err
				}
			}
			if err := os.Rename(journal.Rollback, serverRoot); err != nil {
				return err
			}
		} else if !rootExists {
			return errors.New("interrupted restore has neither live nor rollback data")
		}
	default:
		return fmt.Errorf("unknown restore journal phase %q", journal.Phase)
	}
	if err := os.RemoveAll(journal.Staging); err != nil {
		return err
	}
	if err := os.Remove(journalPath); err != nil {
		return err
	}
	return syncDirectory(parent)
}

func pathExists(name string) bool {
	_, err := os.Lstat(name)
	return err == nil
}

func syncDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

type rateLimitedWriter struct {
	writer  io.Writer
	limiter *rate.Limiter
	ctx     context.Context
}

func (w *rateLimitedWriter) Write(p []byte) (int, error) {
	if err := w.limiter.WaitN(w.ctx, len(p)); err != nil {
		return 0, err
	}
	return w.writer.Write(p)
}
