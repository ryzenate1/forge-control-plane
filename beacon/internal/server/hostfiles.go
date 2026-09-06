package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// validateHostPath mirrors forge's validateHostFilePath: absolute, canonical,
// no backslashes or null bytes, no traversal. Returns cleaned absolute path.
func validateHostPath(raw string) (string, error) {
	if raw == "" || strings.ContainsRune(raw, '\x00') || !strings.HasPrefix(raw, "/") {
		return "", errors.New("path must be an absolute path")
	}
	cleaned := path.Clean(raw)
	normalized := strings.TrimSuffix(raw, "/")
	if normalized == "" {
		normalized = "/"
	}
	if cleaned != normalized || strings.Contains(raw, "\\") {
		return "", errors.New("path must be canonical and may not contain traversal segments")
	}
	return cleaned, nil
}

// hostFileDenylistPrefixes are system locations that must never be reachable
// through the host filesystem API even in denylist (unconfigured) mode.
var hostFileDenylistPrefixes = []string{
	"/etc", "/proc", "/sys", "/dev", "/boot",
	"/usr", "/bin", "/sbin", "/lib", "/lib64",
	"/root", "/var/run", "/run",
}

// resolveHostPath validates raw and applies the host-file access policy:
//
//   - If an explicit allowlist is configured
//     (SetHostFileAllowlist / DAEMON_HOST_FILES_ALLOWLIST), the path MUST be
//     inside one of the allowed roots.
//   - With no allowlist configured, a conservative denylist still blocks
//     system locations and the daemon's own data directory.
//
// Symlinks are resolved exactly once via EvalSymlinks and the resolved target
// is re-checked against the same policy, so a symlink inside an allowed root
// cannot point at /etc/shadow. Callers that then open the path must use
// O_NOFOLLOW and re-verify after open (see openHostFileNoFollow); the
// resolution here closes the pre-open window, the post-open check closes the
// race between check and use.
//
// The beacon's own data directory is always denied regardless of mode.
func (s *Server) resolveHostPath(raw string) (string, error) {
	cleaned, err := validateHostPath(raw)
	if err != nil {
		return "", err
	}
	if err := s.verifyHostPolicy(cleaned); err != nil {
		return "", err
	}
	resolved := cleaned
	if target, err := filepath.EvalSymlinks(cleaned); err == nil {
		resolved = target
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("resolve path %q: %w", cleaned, err)
	} else {
		// The path itself does not exist yet (mkdir/upload/copy target):
		// resolve the nearest existing parent so a symlinked parent cannot
		// redirect the new file outside the policy.
		parent := filepath.Dir(cleaned)
		if resolvedParent, parentErr := filepath.EvalSymlinks(parent); parentErr == nil {
			resolved = filepath.Join(resolvedParent, filepath.Base(cleaned))
		} else if !os.IsNotExist(parentErr) {
			return "", fmt.Errorf("resolve parent of %q: %w", cleaned, parentErr)
		}
	}
	if resolved != cleaned {
		if err := s.verifyHostPolicy(resolved); err != nil {
			return "", err
		}
	}
	return resolved, nil
}

// verifyHostPolicy enforces the allowlist/denylist on an already-cleaned
// absolute path. Both the pre-resolution and post-resolution forms of a path
// pass through here.
func (s *Server) verifyHostPolicy(cleaned string) error {
	if s != nil && s.dataDir != "" {
		dd := strings.TrimSuffix(s.dataDir, "/")
		if cleaned == dd || strings.HasPrefix(cleaned, dd+"/") {
			return errors.New("access to the beacon data directory is not permitted")
		}
	}

	s.hostFileRootsMu.RLock()
	roots := s.hostFileRoots
	s.hostFileRootsMu.RUnlock()

	underAny := func(p string, prefixes []string) bool {
		for _, root := range prefixes {
			if p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/") {
				return true
			}
		}
		return false
	}

	if len(roots) > 0 {
		if !underAny(cleaned, roots) {
			return fmt.Errorf("path %q is outside the configured host file allowlist", cleaned)
		}
		return nil
	}

	if cleaned == "/" || underAny(cleaned, hostFileDenylistPrefixes) {
		return fmt.Errorf("path %q is in a protected system location; configure DAEMON_HOST_FILES_ALLOWLIST to grant explicit roots", cleaned)
	}
	return nil
}

// SetHostFileAllowlist configures the exclusive set of absolute roots the
// host filesystem API may touch. Passing nil/empty switches to conservative
// denylist mode. Each entry is validated as a canonical absolute path.
func (s *Server) SetHostFileAllowlist(roots []string) error {
	cleaned := make([]string, 0, len(roots))
	for _, r := range roots {
		c, err := validateHostPath(r)
		if err != nil {
			return fmt.Errorf("invalid allowlist root %q: %w", r, err)
		}
		cleaned = append(cleaned, c)
	}
	s.hostFileRootsMu.Lock()
	s.hostFileRoots = cleaned
	s.hostFileRootsMu.Unlock()
	return nil
}

// hostAtomicWrite writes reader to hostPath atomically via temp file + rename.
// It uses filepath operations directly so symlinked directories like /tmp are handled.
func hostAtomicWrite(hostPath string, reader io.Reader, limit int64, perm os.FileMode) error {
	dir := filepath.Dir(hostPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".host-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	written, err := io.Copy(tmp, io.LimitReader(reader, limit+1))
	if err != nil {
		tmp.Close()
		return err
	}
	if written > limit {
		tmp.Close()
		return errors.New("file exceeds size limit")
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, hostPath); err != nil {
		return err
	}
	// Ensure directory entry is durable
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// openHostFileNoFollow opens a resolved host path while refusing to follow a
// final symlink component, then re-verifies the opened file: the descriptor
// is fstat'ed (must be a regular file) and the path is re-resolved and
// re-checked against the policy to close the check-to-use race. Callers must
// close the returned file.
//
// Symlink refusal is implemented with Lstat (portable across GOOS) rather
// than O_NOFOLLOW so this file keeps building on Windows: the pre-open Lstat
// rejects a symlinked final component, and the post-open EvalSymlinks +
// policy re-check closes the race between check and use.
func (s *Server) openHostFileNoFollow(resolved string) (*os.File, error) {
	if info, err := os.Lstat(resolved); err != nil {
		return nil, err
	} else if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("refusing to open symlinked path")
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("cannot read non-regular file")
	}
	if target, err := filepath.EvalSymlinks(resolved); err == nil {
		if err := s.verifyHostPolicy(target); err != nil {
			_ = file.Close()
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		_ = file.Close()
		return nil, fmt.Errorf("re-verify path %q: %w", resolved, err)
	}
	return file, nil
}

func (s *Server) handleHostFilesList(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("path")
	if raw == "" {
		raw = "/"
	}
	cleaned, err := s.resolveHostPath(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Use os.ReadDir directly to allow symlinked dirs like /tmp
	entries, err := os.ReadDir(cleaned)
	if err != nil {
		status := http.StatusNotFound
		if os.IsPermission(err) {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	files := []map[string]any{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		// Skip symlinked entries for safety, but allow listing
		if info.Mode()&os.ModeSymlink != 0 {
			// Resolve the symlink target and re-check it against the host-file
			// policy before disclosing anything about it. A symlink inside an
			// allowed root must not leak the size/mtime/mode of (or otherwise
			// stand in for) a file the policy would deny, e.g. link ->
			// /etc/shadow. Fail closed: unresolvable or policy-denied targets are
			// skipped entirely.
			target, err := filepath.EvalSymlinks(filepath.Join(cleaned, entry.Name()))
			if err != nil {
				continue
			}
			if err := s.verifyHostPolicy(target); err != nil {
				continue
			}
			targetInfo, err := os.Stat(target)
			if err != nil || targetInfo.Mode()&os.ModeSymlink != 0 {
				continue
			}
			info = targetInfo
		}
		mode := fmt.Sprintf("%04o", info.Mode().Perm())
		files = append(files, map[string]any{
			"name":      entry.Name(),
			"path":      path.Join(cleaned, entry.Name()),
			"directory": entry.IsDir(),
			"size":      info.Size(),
			"modTime":   info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
			"mode":      mode,
		})
	}
	writeJSON(w, http.StatusOK, files)
}

func (s *Server) handleHostFilesRead(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	cleaned, err := s.resolveHostPath(body.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, err := s.openHostFileNoFollow(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "cannot read non-regular file", http.StatusBadRequest)
		return
	}
	if info.Size() > 10*1024*1024 {
		http.Error(w, "file exceeds read size limit", http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.Copy(w, io.LimitReader(file, 10*1024*1024))
}

func (s *Server) handleHostFilesWrite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16*1024*1024+1024)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	cleaned, err := s.resolveHostPath(body.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if cleaned == "/" {
		http.Error(w, "cannot write to root", http.StatusBadRequest)
		return
	}
	if int64(len(body.Content)) > maxFileWriteBytes {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err := hostAtomicWrite(cleaned, strings.NewReader(body.Content), maxFileWriteBytes, 0o640); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleHostFilesMkdir(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	cleaned, err := s.resolveHostPath(body.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if cleaned == "/" {
		http.Error(w, "cannot mkdir root", http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(cleaned, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func (s *Server) handleHostFilesRemove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	cleaned, err := s.resolveHostPath(body.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if cleaned == "/" {
		http.Error(w, "cannot delete root", http.StatusBadRequest)
		return
	}
	if err := os.RemoveAll(cleaned); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleHostFilesRename(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	srcClean, err := s.resolveHostPath(body.Source)
	if err != nil {
		http.Error(w, "invalid source: "+err.Error(), http.StatusBadRequest)
		return
	}
	dstClean, err := s.resolveHostPath(body.Target)
	if err != nil {
		http.Error(w, "invalid target: "+err.Error(), http.StatusBadRequest)
		return
	}
	if srcClean == "/" || dstClean == "/" {
		http.Error(w, "cannot rename root", http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(filepath.Dir(dstClean), 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.Rename(srcClean, dstClean); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleHostFilesCopy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Source string `json:"source"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	srcClean, err := s.resolveHostPath(body.Source)
	if err != nil {
		http.Error(w, "invalid source: "+err.Error(), http.StatusBadRequest)
		return
	}
	dstClean, err := s.resolveHostPath(body.Target)
	if err != nil {
		http.Error(w, "invalid target: "+err.Error(), http.StatusBadRequest)
		return
	}
	if srcClean == "/" || dstClean == "/" {
		http.Error(w, "cannot copy root", http.StatusBadRequest)
		return
	}
	srcInfo, err := os.Lstat(srcClean)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		http.Error(w, "refusing to copy symlinked source", http.StatusBadRequest)
		return
	}
	srcFile, err := os.Open(srcClean)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer srcFile.Close()
	info, err := srcFile.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "source is not a regular file", http.StatusNotFound)
		return
	}
	if target, err := filepath.EvalSymlinks(srcClean); err == nil {
		if err := s.verifyHostPolicy(target); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if _, err := os.Stat(dstClean); err == nil {
		http.Error(w, "destination already exists", http.StatusConflict)
		return
	} else if !os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(filepath.Dir(dstClean), 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dstFile, err := os.OpenFile(dstClean, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, copyErr := io.Copy(dstFile, srcFile)
	syncErr := dstFile.Sync()
	closeErr := dstFile.Close()
	if copyErr != nil {
		_ = os.Remove(dstClean)
		http.Error(w, copyErr.Error(), http.StatusInternalServerError)
		return
	}
	if syncErr != nil || closeErr != nil {
		_ = os.Remove(dstClean)
		http.Error(w, "failed to commit copy", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func (s *Server) handleHostFilesChmod(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	cleaned, err := s.resolveHostPath(body.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if cleaned == "/" {
		http.Error(w, "cannot chmod root", http.StatusBadRequest)
		return
	}
	if !validPermissionMode(body.Mode) {
		http.Error(w, "mode must contain three or four octal digits", http.StatusBadRequest)
		return
	}
	mode, err := strconv.ParseUint(body.Mode, 8, 32)
	if err != nil {
		http.Error(w, "invalid mode format", http.StatusBadRequest)
		return
	}
	if err := os.Chmod(cleaned, os.FileMode(mode)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleHostFilesUpload(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("path")
	if raw == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	cleaned, err := s.resolveHostPath(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if cleaned == "/" {
		http.Error(w, "cannot upload to root", http.StatusBadRequest)
		return
	}
	const hostUploadLimit = 100 * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, hostUploadLimit)
	contentType := r.Header.Get("Content-Type")
	var reader io.Reader = r.Body
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(hostUploadLimit); err != nil {
			http.Error(w, "multipart parse failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			if len(r.MultipartForm.File) > 0 {
				for _, headers := range r.MultipartForm.File {
					if len(headers) > 0 {
						f, err2 := headers[0].Open()
						if err2 == nil {
							file = f
							err = nil
							break
						}
					}
				}
			}
		}
		if err != nil {
			http.Error(w, "missing file in multipart upload", http.StatusBadRequest)
			return
		}
		defer file.Close()
		reader = file
	}
	if err := hostAtomicWrite(cleaned, reader, hostUploadLimit, 0o640); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "exceeds") {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleHostFilesDownload(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("path")
	if raw == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	cleaned, err := s.resolveHostPath(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, err := s.openHostFileNoFollow(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "path is not a regular file", http.StatusBadRequest)
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(cleaned)})
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, file)
}
