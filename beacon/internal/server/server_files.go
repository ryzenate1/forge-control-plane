package server

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gamepanel/beacon/internal/ignore"
	"gamepanel/beacon/internal/rootfs"
	"gamepanel/beacon/internal/serverid"
)

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	fsys, err := s.serverFilesystem(serverID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	directory, err := rootfs.Clean(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entries, err := fsys.ReadDir(directory)
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, rootfs.ErrSymlink) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	denylist := ignore.NewIgnoreList(nil)
	if ignoreFile, err := fsys.Open(".pteroignore"); err == nil {
		if parsed, parseErr := ignore.LoadIgnoreReader(ignoreFile); parseErr == nil {
			denylist = parsed
		}
		_ = ignoreFile.Close()
	}
	files := []map[string]any{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 || denylist.IsIgnored(path.Join(directory, entry.Name())) {
			continue
		}
		mode := fmt.Sprintf("%04o", info.Mode().Perm())
		// Determine if file is editable (text file under 1MB)
		isEditable := !entry.IsDir() && info.Size() < 1_048_576 && isTextFile(path.Join(directory, entry.Name()))
		files = append(files, map[string]any{
			"name": entry.Name(), "path": path.Join(directory, entry.Name()),
			"directory": entry.IsDir(), "size": info.Size(),
			"modTime": info.ModTime().UTC().Format(time.RFC3339),
			"mode":    mode, "is_editable": isEditable,
		})
	}
	writeJSON(w, http.StatusOK, files)
}

func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	file, err := fsys.Open(filePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "path is not a regular file", http.StatusBadRequest)
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(filePath)})
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := io.Copy(w, file); err != nil {
		return
	}
}

func (s *Server) readFile(w http.ResponseWriter, r *http.Request) {
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	file, err := fsys.Open(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "cannot read non-regular file", http.StatusBadRequest)
		return
	}
	if info.Size() > 1024*1024 {
		http.Error(w, "file exceeds read size limit", http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.Copy(w, file)
}

func (s *Server) writeFile(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	fsys, err := s.serverFilesystem(serverID, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	name, err := rootfs.Clean(r.URL.Query().Get("path"))
	if err != nil || name == "" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if r.ContentLength > maxFileWriteBytes {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}
	reservation := r.ContentLength
	if reservation < 0 {
		reservation = maxFileWriteBytes
	}
	if err := s.manager.HasSpaceForWriteFS(serverID, reservation, fsys); err != nil {
		http.Error(w, err.Error(), http.StatusInsufficientStorage)
		return
	}
	if err := fsys.AtomicWrite(name, r.Body, maxFileWriteBytes, 0o640); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, rootfs.ErrTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const maxUploadChunkBytes = 8 * 1024 * 1024

func (s *Server) uploadFileChunk(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	uploadID := r.URL.Query().Get("uploadId")
	if !safeUploadID(uploadID) {
		http.Error(w, "invalid upload id", http.StatusBadRequest)
		return
	}
	unlock := lockUpload(serverID, uploadID)
	defer unlock()
	fsys, err := s.serverFilesystem(serverID, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	target, err := rootfs.Clean(r.URL.Query().Get("path"))
	if err != nil || target == "" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	offset, err := parseInt64Query(r, "offset")
	if err != nil || offset < 0 {
		http.Error(w, "invalid offset", http.StatusBadRequest)
		return
	}
	if r.ContentLength > maxUploadChunkBytes {
		http.Error(w, "upload chunk too large", http.StatusRequestEntityTooLarge)
		return
	}
	maxTotal := envBytes("DAEMON_UPLOAD_MAX_BYTES", defaultMaxUploadBytes)
	if offset > maxTotal || (r.ContentLength > 0 && r.ContentLength > maxTotal-offset) {
		http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)
		return
	}
	cleanupExpiredUploads(fsys, time.Now())
	if err := fsys.MkdirAll(".uploads", 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	temp := path.Join(".uploads", uploadID+".part")
	current := int64(0)
	if info, statErr := fsys.Stat(temp); statErr == nil {
		current = info.Size()
	} else if offset != 0 {
		http.Error(w, "upload session not found", http.StatusConflict)
		return
	}
	if current != offset {
		http.Error(w, "upload offset mismatch", http.StatusConflict)
		return
	}
	reservation := r.ContentLength
	if reservation < 0 {
		reservation = maxUploadChunkBytes
	}
	if err := s.manager.HasSpaceForWriteFS(serverID, reservation, fsys); err != nil {
		http.Error(w, err.Error(), http.StatusInsufficientStorage)
		return
	}
	file, err := fsys.OpenFile(temp, os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		file.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	written, copyErr := io.Copy(file, io.LimitReader(r.Body, maxUploadChunkBytes+1))
	if copyErr != nil || written > maxUploadChunkBytes || written > maxTotal-offset {
		_ = file.Truncate(offset)
		_ = file.Close()
		if copyErr != nil {
			http.Error(w, copyErr.Error(), http.StatusInternalServerError)
		} else {
			http.Error(w, "upload chunk or total too large", http.StatusRequestEntityTooLarge)
		}
		return
	}
	if err := file.Sync(); err != nil {
		file.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := file.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	nextOffset := offset + written
	final := r.URL.Query().Get("final") == "true"
	if final {
		if err := fsys.MkdirAll(path.Dir(target), 0o750); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := fsys.Rename(temp, target); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "offset": nextOffset, "final": final})
}

func (s *Server) makeDir(w http.ResponseWriter, r *http.Request) {
	fsys, err := s.serverFilesystem(r.PathValue("id"), true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	if err := fsys.MkdirAll(r.URL.Query().Get("path"), 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func (s *Server) renameFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	to, err := rootfs.Clean(body.To)
	if err != nil || to == "" {
		http.Error(w, "invalid destination", http.StatusBadRequest)
		return
	}
	if err := fsys.MkdirAll(path.Dir(to), 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := fsys.Rename(body.From, to); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("path") == "" {
		http.Error(w, "cannot delete server root", http.StatusBadRequest)
		return
	}
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	if err := fsys.RemoveAll(r.URL.Query().Get("path")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) archiveFiles(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	fsys, err := s.serverFilesystem(serverID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	source, err := rootfs.Clean(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := fsys.Stat(source)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	name := info.Name()
	if source == "" {
		name = serverID
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.tar.gz"`)
	gzipWriter := gzip.NewWriter(w)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := archiveTree(fsys, tarWriter, source, name); err != nil {
		return
	}
	if err := tarWriter.Close(); err != nil {
		return
	}
	_ = gzipWriter.Close()
}

func (s *Server) decompressFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	fsys, err := s.serverFilesystem(serverID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	archiveName, err := rootfs.Clean(body.Path)
	if err != nil || archiveName == "" {
		http.Error(w, "invalid archive path", http.StatusBadRequest)
		return
	}
	destination := path.Dir(archiveName)
	if destination == "." {
		destination = ""
	}
	if _, err := extractArchive(fsys, archiveName, destination, s.manager, serverID); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "disk") {
			status = http.StatusInsufficientStorage
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func (s *Server) batchDeleteFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if len(body.Paths) == 0 || len(body.Paths) > 100 {
		http.Error(w, "between 1 and 100 paths are required", http.StatusBadRequest)
		return
	}
	cleaned := make([]string, len(body.Paths))
	seen := make(map[string]struct{}, len(body.Paths))
	for index, name := range body.Paths {
		value, err := rootfs.Clean(name)
		if err != nil || value == "" {
			http.Error(w, "invalid path in batch", http.StatusBadRequest)
			return
		}
		if _, duplicate := seen[value]; duplicate {
			http.Error(w, "duplicate path in batch", http.StatusBadRequest)
			return
		}
		seen[value] = struct{}{}
		cleaned[index] = value
	}
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	for _, name := range cleaned {
		if err := fsys.RemoveAll(name); err != nil {
			http.Error(w, "batch delete failed after partial execution: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": len(cleaned)})
}

func (s *Server) batchRenameFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Files []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if len(body.Files) == 0 || len(body.Files) > 100 {
		http.Error(w, "between 1 and 100 files are required", http.StatusBadRequest)
		return
	}
	type rename struct{ from, to string }
	cleaned := make([]rename, len(body.Files))
	targets := make(map[string]struct{}, len(body.Files))
	for index, file := range body.Files {
		from, fromErr := rootfs.Clean(file.From)
		to, toErr := rootfs.Clean(file.To)
		if fromErr != nil || toErr != nil || from == "" || to == "" || from == to {
			http.Error(w, "invalid rename in batch", http.StatusBadRequest)
			return
		}
		if _, duplicate := targets[to]; duplicate {
			http.Error(w, "duplicate destination in batch", http.StatusBadRequest)
			return
		}
		targets[to] = struct{}{}
		cleaned[index] = rename{from: from, to: to}
	}
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	for _, file := range cleaned {
		if _, err := fsys.Stat(file.from); err != nil {
			http.Error(w, "batch source is unavailable: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	for _, file := range cleaned {
		if err := fsys.MkdirAll(path.Dir(file.to), 0o750); err != nil {
			http.Error(w, "batch rename failed after partial execution: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := fsys.Rename(file.from, file.to); err != nil {
			http.Error(w, "batch rename failed after partial execution: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "renamed": len(cleaned)})
}

func (s *Server) chmodFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if body.Path == "" || body.Mode == "" {
		http.Error(w, "path and mode are required", http.StatusBadRequest)
		return
	}
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	if !validPermissionMode(body.Mode) {
		http.Error(w, "mode must contain three or four octal digits", http.StatusBadRequest)
		return
	}
	mode, err := strconv.ParseUint(body.Mode, 8, 32)
	if err != nil {
		http.Error(w, "invalid mode format", http.StatusBadRequest)
		return
	}
	if err := fsys.Chmod(body.Path, os.FileMode(mode)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) copyFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if body.From == "" || body.To == "" {
		http.Error(w, "from and to are required", http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	fsys, err := s.serverFilesystem(serverID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	info, err := fsys.Stat(body.From)
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "source is not a regular file", http.StatusNotFound)
		return
	}
	if _, err := fsys.Stat(body.To); err == nil {
		http.Error(w, "destination already exists", http.StatusConflict)
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.manager.HasSpaceForWriteFS(serverID, info.Size(), fsys); err != nil {
		http.Error(w, err.Error(), http.StatusInsufficientStorage)
		return
	}
	if _, err := fsys.Copy(body.From, body.To, info.Mode().Perm(), info.Size()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func (s *Server) pullRemoteFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL      string `json:"url"`
		Target   string `json:"target"`
		FileName string `json:"fileName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	parsed, err := url.Parse(body.URL)
	if err != nil {
		http.Error(w, "invalid URL", http.StatusBadRequest)
		return
	}
	client, err := s.pullClientFactory(r.Context(), parsed)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "private") {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	fileName := body.FileName
	if fileName == "" {
		fileName = path.Base(parsed.EscapedPath())
		if fileName == "." || fileName == "/" {
			fileName = "download"
		}
	}
	fileName, err = safePullFilename(fileName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	target, err := rootfs.Clean(body.Target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	fsys, err := s.serverFilesystem(serverID, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	if err := fsys.MkdirAll(target, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, parsed.String(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := client.Do(request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		http.Error(w, "remote server returned "+resp.Status, http.StatusBadGateway)
		return
	}
	maxBytes := envBytes("DAEMON_PULL_MAX_BYTES", defaultMaxPullBytes)
	if resp.ContentLength < 0 {
	} else if resp.ContentLength > maxBytes {
		http.Error(w, "remote file exceeds size limit", http.StatusRequestEntityTooLarge)
		return
	}
	reservation := resp.ContentLength
	if reservation < 0 {
		reservation = maxBytes
	}
	if err := s.manager.HasSpaceForWriteFS(serverID, reservation, fsys); err != nil {
		http.Error(w, err.Error(), http.StatusInsufficientStorage)
		return
	}
	finalName := path.Join(target, fileName)
	if resp.ContentLength >= 0 {
		err = fsys.AtomicWriteExact(finalName, resp.Body, maxBytes, resp.ContentLength, 0o640)
	} else {
		err = fsys.AtomicWrite(finalName, resp.Body, maxBytes, 0o640)
	}
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, rootfs.ErrTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "path": finalName})
}

func (s *Server) safePath(serverID, requested string) (string, error) {
	if err := serverid.Validate(serverID); err != nil {
		return "", err
	}
	if strings.ContainsRune(requested, 0) {
		return "", errors.New("invalid path")
	}
	cleaned := filepath.Clean(strings.TrimPrefix(requested, "/"))
	if cleaned == "." {
		cleaned = ""
	}
	if filepath.IsAbs(requested) || strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, string(filepath.Separator)+".."+string(filepath.Separator)) {
		return "", errors.New("path escapes server directory")
	}
	root, err := filepath.Abs(filepath.Join(s.dataDir, serverID))
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(root, cleaned))
	if err != nil {
		return "", err
	}
	// Resolve root through symlinks so Rel comparison works on systems
	// where temp directories are symlinked (e.g., macOS /var → /private/var)
	if resolvedRoot, err := filepath.EvalSymlinks(root); err == nil {
		root = resolvedRoot
		target, err = filepath.Abs(filepath.Join(root, cleaned))
		if err != nil {
			return "", err
		}
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes server directory")
	}
	// If root doesn't exist yet, no symlinks to worry about — path is safe by Rel check alone
	if _, statErr := os.Stat(root); statErr != nil {
		return target, nil
	}
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		resolvedRel, err := filepath.Rel(root, resolved)
		if err != nil || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) {
			return "", errors.New("path escapes server directory")
		}
	} else {
		parent := filepath.Dir(target)
		// Don't check parent if it is, or resolves to, the root
		if parent == root {
			return target, nil
		}
		if resolvedParent, parentErr := filepath.EvalSymlinks(parent); parentErr == nil {
			parentRel, err := filepath.Rel(root, resolvedParent)
			if err != nil || parentRel == ".." || strings.HasPrefix(parentRel, ".."+string(filepath.Separator)) {
				return "", errors.New("path escapes server directory")
			}
		}
	}
	return target, nil
}

func parseInt64Query(r *http.Request, key string) (int64, error) {
	var value int64
	text := r.URL.Query().Get(key)
	if text == "" {
		return 0, nil
	}
	for _, char := range text {
		if char < '0' || char > '9' {
			return 0, errors.New("invalid integer")
		}
		value = value*10 + int64(char-'0')
	}
	return value, nil
}

func safeUploadID(uploadID string) bool {
	if uploadID == "" || len(uploadID) > 96 {
		return false
	}
	for _, char := range uploadID {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return !strings.Contains(uploadID, "..")
}

func isStreamingUpload(r *http.Request) bool {
	return r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/files/upload")
}
