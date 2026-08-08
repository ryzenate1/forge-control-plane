package cron

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// transferFileRegistry tracks temp-directory transfer archives created by
// this process, keyed by their absolute path. removeOldTransferArchives only
// deletes files that either appear in this registry or are old enough that
// they're very unlikely to belong to a concurrently-running, legitimate
// transfer (the secondary sweep). This bounds the damage an attacker with
// write access to the shared temp directory could do by dropping a
// same-named file: without a matching registry entry, their file survives
// until it ages past the safety threshold, and even then it's only removed
// if it also passes the ownership/permission and symlink-real-path checks
// below.
var (
	transferFileRegistryMu sync.Mutex
	transferFileRegistry   = map[string]struct{}{}
)

// RegisterTransferArchive records path as an archive owned by this process so
// the cleanup cron can safely reap it later. Callers that create
// transfer-*.tar.gz files in os.TempDir() should call this immediately after
// creating the file.
func RegisterTransferArchive(path string) {
	transferFileRegistryMu.Lock()
	transferFileRegistry[path] = struct{}{}
	transferFileRegistryMu.Unlock()
}

// UnregisterTransferArchive removes path from the registry, e.g. once the
// owning transfer has already cleaned it up itself.
func UnregisterTransferArchive(path string) {
	transferFileRegistryMu.Lock()
	delete(transferFileRegistry, path)
	transferFileRegistryMu.Unlock()
}

func isRegisteredTransferArchive(path string) bool {
	transferFileRegistryMu.Lock()
	_, ok := transferFileRegistry[path]
	transferFileRegistryMu.Unlock()
	return ok
}

type CleanupCron struct {
	dataDir string
}

func NewCleanupCron(dataDir string) *CleanupCron {
	return &CleanupCron{dataDir: dataDir}
}

func (cc *CleanupCron) Run(ctx context.Context) error {
	if cc.dataDir == "" {
		return nil
	}
	if err := cc.removeOldTransferArchives(); err != nil {
		return err
	}
	if err := cc.removeStaleUploadSessions(); err != nil {
		return err
	}
	return cc.removeTempFiles()
}

// staleTransferSafetyMargin is added on top of the normal 24h cutoff before a
// transfer-*.tar.gz file that ISN'T in the in-memory registry (e.g. because
// the daemon restarted and lost the registry, or a legacy caller didn't
// register it) is considered for removal by name/suffix matching alone. This
// keeps the fallback sweep from being a fast path for an attacker who can
// merely predict the filename pattern.
const staleTransferSafetyMargin = 24 * time.Hour

func (cc *CleanupCron) removeOldTransferArchives() error {
	tmpDir := os.TempDir()
	canonicalTmpDir, canonicalErr := filepath.EvalSymlinks(tmpDir)
	if canonicalErr != nil {
		return nil
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	fallbackCutoff := cutoff.Add(-staleTransferSafetyMargin)
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "gamepanel-transfers-") {
			info, infoErr := entry.Info()
			path := filepath.Join(tmpDir, entry.Name())
			if infoErr == nil && info.ModTime().Before(fallbackCutoff) && ownedByCurrentUser(info) && info.Mode().Perm()&0o077 == 0 {
				if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil && filepath.Dir(resolved) == canonicalTmpDir {
					_ = os.RemoveAll(resolved)
				}
			}
			continue
		}
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), "transfer-") || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(tmpDir, entry.Name())
		registered := isRegisteredTransferArchive(path)
		if registered {
			if info.ModTime().Before(cutoff) && safeToRemove(path, tmpDir, info) {
				os.Remove(path)
				UnregisterTransferArchive(path)
			}
			continue
		}
		// Not registered: only ever removed via the slower, stricter fallback
		// sweep, and only after the extra safety checks below.
		if info.ModTime().Before(fallbackCutoff) && safeToRemove(path, tmpDir, info) {
			os.Remove(path)
		}
	}
	return nil
}

// safeToRemove guards the actual deletion with checks beyond filename
// matching: the file must still be owned by this process's UID with
// restrictive permissions (0600-equivalent, no group/other access), and its
// resolved real path (after following any symlinks) must still live inside
// dir. This defends against an attacker who has write access to the shared
// temp directory planting a same-named file (or a symlink pointing
// elsewhere) to trigger deletion of something they don't own.
func safeToRemove(path, dir string, info os.FileInfo) bool {
	if !ownedByCurrentUser(info) {
		return false
	}
	if info.Mode().Perm()&0o022 != 0 {
		// Reject anything writable by group or other: our own archives are
		// never created that way, so this indicates the file was placed (or
		// tampered with) by something other than this process.
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(resolvedDir, resolved)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return false
	}
	return true
}

func (cc *CleanupCron) removeStaleUploadSessions() error {
	uploadsDir := filepath.Join(cc.dataDir, ".uploads")
	cutoff := time.Now().Add(-24 * time.Hour)
	return cc.cleanDir(uploadsDir, cutoff)
}

func (cc *CleanupCron) removeTempFiles() error {
	tmpDir := filepath.Join(cc.dataDir, ".tmp")
	cutoff := time.Now().Add(-24 * time.Hour)
	return cc.cleanDir(tmpDir, cutoff)
}

func (cc *CleanupCron) cleanDir(dir string, cutoff time.Time) error {
	canonicalDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		path := filepath.Join(canonicalDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.ModTime().Before(cutoff) {
			continue
		}
		quarantine, err := uniqueCleanupPath(canonicalDir)
		if err != nil {
			return err
		}
		if err := os.Rename(path, quarantine); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		movedInfo, err := os.Lstat(quarantine)
		if err != nil {
			return err
		}
		if !os.SameFile(info, movedInfo) {
			_ = os.Rename(quarantine, path)
			continue
		}
		if err := os.RemoveAll(quarantine); err != nil {
			_ = os.Rename(quarantine, path)
			return err
		}
	}
	return nil
}

func uniqueCleanupPath(dir string) (string, error) {
	temp, err := os.CreateTemp(dir, ".cleanup-quarantine-*")
	if err != nil {
		return "", err
	}
	path := temp.Name()
	if err := temp.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}
