package server

// Container file manager (server-scoped).
//
// These handlers back the panel's "Container File Manager" feature: browsing,
// reading, writing, uploading, and removing files INSIDE the running
// container that hosts a given server workload. Unlike the admin container
// routes in container_admin.go (which take a raw Docker container ID from the
// caller), every route here takes a *server* ID from the path and resolves the
// container itself through the runtime binding — a client can never point an
// operation at an arbitrary container it does not own.
//
// Transport uses the Docker archive API (CopyFromContainer/CopyToContainer)
// for reads/writes so no in-container shell is required, and narrowly scoped
// exec (rm -rf / mkdir -p) for the two mutations the archive API cannot
// express. This surface is Docker-family only (docker, podman): the archive
// IDs, exec protocol and tar framing do not exist on containerd/firecracker/
// kubernetes, so those providers are refused with 501 rather than having a
// Docker client pointed at a pod UID or microVM ID it cannot resolve. All paths are validated server-side: absolute, canonical, no NUL
// bytes, and recursive deletes refuse system roots and the workload's own
// data root. Authentication is the panel HMAC enforced by the shared
// middleware — exactly like the host file routes in server.go: per-user
// authorization already happened at the panel, which only forwards requests
// for servers the caller may manage.

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"gamepanel/beacon/internal/runtime"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	// containerFileReadLimit caps inline file reads. Anything bigger should
	// go through the download endpoint instead of a text preview.
	containerFileReadLimit = 2 * 1024 * 1024
	// containerFileUploadLimit caps a single multipart upload batch. It is not
	// a free choice: /servers/{id}/.../files/upload is one of the streaming
	// routes accepted by authenticate() with an empty-body signature, and that
	// layer already wraps the body in a MaxBytesReader of maxUploadChunkBytes.
	// Anything larger is rejected before the handler ever runs, so the whole
	// chain (panel API, daemon, UI) shares this ceiling.
	containerFileUploadLimit = int64(maxUploadChunkBytes)
	// containerFileListingLimit caps how many immediate children a single
	// ls response returns so huge directories do not balloon the payload.
	containerFileListingLimit = 5000
	// containerFileListingMaxBytes caps how much of the recursive Docker tar
	// stream we consume for one listing. CopyFromContainer archives the whole
	// subtree, so reading it to find immediate children is otherwise unbounded
	// in time and memory — a workload directory with a deep/huge subtree would
	// make the daemon stream the entire tree into Beacon on every ls. Beyond
	// this budget the listing is returned truncated.
	containerFileListingMaxBytes = 256 << 20
	// containerFilePathLimit caps pathological path lengths.
	containerFilePathLimit = 4096
)

// containerFileEntry is one row of a directory listing.
type containerFileEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Mode     string `json:"mode"`
	IsDir    bool   `json:"isDir"`
	Modified string `json:"modified"`
	Type     string `json:"type"`
}

// containerFileListing is the ls response envelope.
type containerFileListing struct {
	Path      string               `json:"path"`
	Entries   []containerFileEntry `json:"entries"`
	Truncated bool                 `json:"truncated"`
}

// resolveServerContainer maps the {id} path value (a panel server ID) to the
// Docker container reference actually running that workload. The container
// reference is never taken from the request body or query.
func (s *Server) resolveServerContainer(r *http.Request) (*client.Client, string, error) {
	serverID := strings.TrimSpace(r.PathValue("id"))
	if serverID == "" {
		return nil, "", &containerFilesHTTPError{status: http.StatusBadRequest, message: "server id is required"}
	}
	if s.runtime == nil {
		return nil, "", &containerFilesHTTPError{status: http.StatusServiceUnavailable, message: errRuntimeUnavailable.Error()}
	}
	// The archive API is Docker-family only. Pointing a Docker client at a
	// Kubernetes pod UID, a containerd snapshot name or a microVM ID would
	// either fail opaquely or, worse, resolve to an unrelated container.
	// Refuse honestly so callers fall back to SFTP/host files instead.
	switch provider := s.runtimeProvider(); strings.ToLower(strings.TrimSpace(provider)) {
	case runtime.ProviderDocker, runtime.ProviderPodman:
	default:
		if provider == "" || provider == "unknown" {
			return nil, "", &containerFilesHTTPError{status: http.StatusServiceUnavailable, message: "container file manager requires a docker-compatible runtime: current provider is unknown"}
		}
		return nil, "", &containerFilesHTTPError{status: http.StatusNotImplemented, message: "container file manager is not supported by the " + provider + " runtime; use SFTP or host files instead"}
	}
	state, err := s.runtime.Inspect(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, errRuntimeUnavailable) {
			return nil, "", &containerFilesHTTPError{status: http.StatusServiceUnavailable, message: err.Error()}
		}
		return nil, "", &containerFilesHTTPError{status: http.StatusBadGateway, message: "resolve workload: " + err.Error()}
	}
	if !state.Exists || state.ID == "" {
		return nil, "", &containerFilesHTTPError{status: http.StatusNotFound, message: "server workload is not deployed on this node"}
	}
	// The archive API and exec both require a running container.
	if !state.Running {
		return nil, "", &containerFilesHTTPError{status: http.StatusConflict, message: "server workload is not running"}
	}
	docker, err := s.adminDockerClient()
	if err != nil {
		return nil, "", &containerFilesHTTPError{status: http.StatusInternalServerError, message: err.Error()}
	}
	return docker, state.ID, nil
}

type containerFilesHTTPError struct {
	status  int
	message string
}

func (e *containerFilesHTTPError) Error() string { return e.message }

func (e *containerFilesHTTPError) write(w http.ResponseWriter) {
	writeError(w, e.status, e.message)
}

// containerFilesGuard resolves the {id} workload for the request. It does not
// re-check the caller's identity: the shared authenticate middleware already
// rejected any request without a valid panel signature or scoped token, and
// reading the body here to re-verify it would break streamed uploads.
func (s *Server) containerFilesGuard(w http.ResponseWriter, r *http.Request) (*client.Client, string, bool) {
	docker, ref, err := s.resolveServerContainer(r)
	if err != nil {
		var httpErr *containerFilesHTTPError
		if errors.As(err, &httpErr) {
			httpErr.write(w)
		} else {
			writeError(w, http.StatusBadGateway, err.Error())
		}
		return nil, "", false
	}
	return docker, ref, true
}

// validateContainerFilePath enforces absolute, canonical container paths. It
// rejects empty strings, NUL bytes, backslashes, relative paths, and any path
// whose cleaned form differs from the input (i.e. traversal segments).
func validateContainerFilePath(value string) (string, error) {
	if value == "" || len(value) > containerFilePathLimit {
		return "", fmt.Errorf("path is required and must be shorter than %d characters", containerFilePathLimit)
	}
	if strings.ContainsRune(value, '\x00') || strings.Contains(value, `\`) {
		return "", fmt.Errorf("path contains invalid characters")
	}
	if !strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("path must be absolute")
	}
	cleaned := path.Clean(value)
	normalized := strings.TrimSuffix(value, "/")
	if normalized == "" {
		normalized = "/"
	}
	if cleaned != normalized {
		return "", fmt.Errorf("path must be canonical and may not contain traversal segments")
	}
	return cleaned, nil
}

// validateContainerFileDeletePath additionally refuses recursive deletes that
// target the filesystem root, well-known system directories, or the workload's
// entire data root.
func validateContainerFileDeletePath(value string) (string, error) {
	cleaned, err := validateContainerFilePath(value)
	if err != nil {
		return "", err
	}
	if cleaned == "/" {
		return "", fmt.Errorf("refusing to delete the container filesystem root")
	}
	if cleaned == containerDataRoot {
		return "", fmt.Errorf("refusing to delete the server's entire data directory")
	}
	for _, dangerous := range dangerousContainerPaths {
		if cleaned == path.Clean(dangerous) {
			return "", fmt.Errorf("refusing to delete restricted path %q", cleaned)
		}
	}
	return cleaned, nil
}

// containerFilePathFromQuery reads and validates a path query parameter,
// substituting fallback when the caller omitted it.
func containerFilePathFromQuery(r *http.Request, fallback string) (string, error) {
	value := r.URL.Query().Get("path")
	if strings.TrimSpace(value) == "" {
		value = fallback
	}
	return validateContainerFilePath(value)
}

// containerFileEntryFromHeader converts one tar header into the listing shape.
// full is the container-absolute path of the entry.
func containerFileEntryFromHeader(hdr *tar.Header, full string) containerFileEntry {
	info := hdr.FileInfo()
	entryType := "file"
	switch hdr.Typeflag {
	case tar.TypeDir:
		entryType = "dir"
	case tar.TypeSymlink:
		entryType = "symlink"
	case tar.TypeLink:
		entryType = "hardlink"
	}
	modified := ""
	if !info.ModTime().IsZero() {
		modified = info.ModTime().UTC().Format(time.RFC3339)
	}
	return containerFileEntry{
		Name:     path.Base(full),
		Path:     full,
		Size:     hdr.Size,
		Mode:     info.Mode().String(),
		IsDir:    info.IsDir(),
		Modified: modified,
		Type:     entryType,
	}
}

// handleServerContainerFilesLs lists the IMMEDIATE children of a directory
// inside the workload container. Docker's CopyFromContainer returns a
// recursive tar archive of the path, so we skip the root entry and any entry
// more than one level below it instead of shipping the whole subtree.
func (s *Server) handleServerContainerFilesLs(w http.ResponseWriter, r *http.Request) {
	docker, ref, ok := s.containerFilesGuard(w, r)
	if !ok {
		return
	}
	requested, err := containerFilePathFromQuery(r, "/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	stream, _, err := docker.CopyFromContainer(r.Context(), ref, requested)
	if err != nil {
		if client.IsErrNotFound(err) {
			writeError(w, http.StatusNotFound, "path not found in container")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer stream.Close()

	tr := tar.NewReader(io.LimitReader(stream, containerFileListingMaxBytes))
	rootHdr, err := tr.Next()
	if err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusNotFound, "path not found in container")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if !rootHdr.FileInfo().IsDir() {
		writeError(w, http.StatusBadRequest, "path is a file, not a directory")
		return
	}
	// The archive root entry is named after the base of the requested path
	// (e.g. "container"), or is "." / "/" when copying the filesystem root.
	rootName := strings.TrimSuffix(path.Clean("./"+rootHdr.Name), "/")
	prefix := ""
	if rootName != "." && rootName != "/" && rootName != "" {
		prefix = rootName + "/"
	}

	listing := containerFileListing{Path: requested, Entries: []containerFileEntry{}}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// The recursive archive exceeded the listing byte budget (or is
			// malformed). Return the partial listing as truncated rather than
			// streaming an unbounded number of bytes from the daemon.
			listing.Truncated = true
			break
		}
		name := strings.TrimSuffix(strings.TrimPrefix(hdr.Name, prefix), "/")
		if name == "" || strings.Contains(name, "/") {
			// Either the root itself or a deeper descendant — not an
			// immediate child of the requested directory.
			continue
		}
		if len(listing.Entries) >= containerFileListingLimit {
			listing.Truncated = true
			break
		}
		listing.Entries = append(listing.Entries, containerFileEntryFromHeader(hdr, path.Join(requested, name)))
	}

	writeJSON(w, http.StatusOK, listing)
}

// handleServerContainerFilesRead streams a single file out of the container
// as a plain body. Text files come back as text/plain; content that looks
// binary comes back as application/octet-stream so the UI can offer a
// download instead of an editor.
func (s *Server) handleServerContainerFilesRead(w http.ResponseWriter, r *http.Request) {
	docker, ref, ok := s.containerFilesGuard(w, r)
	if !ok {
		return
	}
	requested, err := validateContainerFilePath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	stream, stat, err := docker.CopyFromContainer(r.Context(), ref, requested)
	if err != nil {
		if client.IsErrNotFound(err) {
			writeError(w, http.StatusNotFound, "file not found in container")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer stream.Close()
	if stat.Mode.IsDir() {
		writeError(w, http.StatusBadRequest, "path is a directory, not a file")
		return
	}

	tr := tar.NewReader(stream)
	hdr, err := tr.Next()
	if err != nil {
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusNotFound, "file not found in container")
			return
		}
		writeError(w, http.StatusBadGateway, "read archive: "+err.Error())
		return
	}
	if hdr.FileInfo().IsDir() {
		writeError(w, http.StatusBadRequest, "path is a directory, not a file")
		return
	}
	if hdr.Size > containerFileReadLimit {
		writeError(w, http.StatusRequestEntityTooLarge, "file too large to read inline; use download")
		return
	}
	data, err := io.ReadAll(io.LimitReader(tr, containerFileReadLimit+1))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if len(data) > containerFileReadLimit {
		writeError(w, http.StatusRequestEntityTooLarge, "file too large to read inline; use download")
		return
	}

	w.Header().Set("Content-Type", sniffContainerContentType(data))
	w.Header().Set("X-File-Size", fmt.Sprintf("%d", hdr.Size))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// sniffContainerContentType reports application/octet-stream when the first
// chunk contains a NUL byte (a reliable binary marker for the formats that
// matter here: databases, archives, images, executables), text/plain otherwise.
func sniffContainerContentType(data []byte) string {
	window := data
	if len(window) > 8000 {
		window = window[:8000]
	}
	if bytes.IndexByte(window, 0) >= 0 {
		return "application/octet-stream"
	}
	return "text/plain; charset=utf-8"
}

// handleServerContainerFilesWrite writes a text file into the container with
// a single-entry tar archive delivered to the file's parent directory.
func (s *Server) handleServerContainerFilesWrite(w http.ResponseWriter, r *http.Request) {
	docker, ref, ok := s.containerFilesGuard(w, r)
	if !ok {
		return
	}
	var body struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decodeJSONBody(w, r, containerFileWriteLimit+64*1024, &body); err != nil {
		return
	}
	requested, err := validateContainerFilePath(body.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if requested == "/" {
		writeError(w, http.StatusBadRequest, "path must name a file, not the filesystem root")
		return
	}
	data := []byte(body.Content)
	if int64(len(data)) > containerFileWriteLimit {
		writeError(w, http.StatusRequestEntityTooLarge, "content exceeds the inline write limit")
		return
	}
	if err := copyFileIntoContainer(r.Context(), docker, ref, requested, data, 0o644); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": requested})
}

// containerFileWriteLimit bounds inline PUT /write payloads. The panel's HMAC
// authentication layer buffers non-streamed bodies up to 1 MiB, so writes
// larger than this would fail signature verification anyway; 896 KiB leaves
// comfortable room for JSON envelope overhead.
const containerFileWriteLimit = 896 * 1024

// copyFileIntoContainer tars a single file under its parent directory and
// copies it into the container.
func copyFileIntoContainer(ctx context.Context, docker *client.Client, ref, target string, data []byte, mode int64) error {
	dir := path.Dir(target)
	if dir == "/" && path.Base(target) == "/" {
		return fmt.Errorf("invalid target path")
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name:     path.Base(target),
		Typeflag: tar.TypeReg,
		Size:     int64(len(data)),
		Mode:     mode,
		ModTime:  time.Now().UTC(),
	}); err != nil {
		return err
	}
	if _, err := tw.Write(data); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return docker.CopyToContainer(ctx, ref, dir, &buf, container.CopyToContainerOptions{})
}

// handleServerContainerFilesUpload accepts a multipart form (field "file",
// repeatable) and extracts the files into the container directory given by
// the ?path= query parameter.
func (s *Server) handleServerContainerFilesUpload(w http.ResponseWriter, r *http.Request) {
	docker, ref, ok := s.containerFilesGuard(w, r)
	if !ok {
		return
	}
	destDir, err := containerFilePathFromQuery(r, "/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, containerFileUploadLimit)
	if err := r.ParseMultipartForm(containerFileUploadLimit); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse multipart form: "+err.Error())
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "no file provided in 'file' field")
		return
	}

	type uploadFailure struct {
		File  string `json:"file"`
		Error string `json:"error"`
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	var failures []uploadFailure
	var uploaded int
	for _, fh := range files {
		safeName := sanitizeUploadName(fh.Filename)
		if safeName == "" {
			failures = append(failures, uploadFailure{File: fh.Filename, Error: "invalid file name"})
			continue
		}
		f, err := fh.Open()
		if err != nil {
			failures = append(failures, uploadFailure{File: fh.Filename, Error: err.Error()})
			continue
		}
		hdr := &tar.Header{
			Name:     safeName,
			Typeflag: tar.TypeReg,
			Size:     fh.Size,
			Mode:     0o644,
			ModTime:  time.Now().UTC(),
		}
		if fh.Size < 0 {
			// Unknown length: read into the tar entry via -1 size is not
			// supported by the writer, so buffer honestly with a cap.
			data, readErr := io.ReadAll(io.LimitReader(f, containerFileUploadLimit))
			f.Close()
			if readErr != nil {
				failures = append(failures, uploadFailure{File: fh.Filename, Error: readErr.Error()})
				continue
			}
			hdr.Size = int64(len(data))
			if err := tw.WriteHeader(hdr); err != nil {
				failures = append(failures, uploadFailure{File: fh.Filename, Error: err.Error()})
				continue
			}
			if _, err := tw.Write(data); err != nil {
				failures = append(failures, uploadFailure{File: fh.Filename, Error: err.Error()})
				continue
			}
			uploaded++
			continue
		}
		if err := tw.WriteHeader(hdr); err != nil {
			f.Close()
			failures = append(failures, uploadFailure{File: fh.Filename, Error: err.Error()})
			continue
		}
		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			failures = append(failures, uploadFailure{File: fh.Filename, Error: err.Error()})
			continue
		}
		f.Close()
		uploaded++
	}
	if err := tw.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if uploaded == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "failed": failures})
		return
	}
	if err := docker.CopyToContainer(r.Context(), ref, destDir, &buf, container.CopyToContainerOptions{}); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if len(failures) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "uploaded": uploaded, "failed": failures})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "uploaded": uploaded})
}

// sanitizeUploadName reduces a client-supplied filename to a plain basename;
// anything that could escape the destination directory is rejected (empty).
func sanitizeUploadName(raw string) string {
	name := path.Base(strings.ReplaceAll(raw, "\\", "/"))
	if name == "." || name == ".." || name == "/" || name == "" {
		return ""
	}
	if strings.ContainsRune(name, '\x00') || strings.ContainsAny(name, "\r\n") {
		return ""
	}
	return name
}

// handleServerContainerFilesMkdir creates a directory (and parents) inside
// the container.
func (s *Server) handleServerContainerFilesMkdir(w http.ResponseWriter, r *http.Request) {
	docker, ref, ok := s.containerFilesGuard(w, r)
	if !ok {
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := decodeJSONBody(w, r, 8*1024, &body); err != nil {
		return
	}
	requested, err := validateContainerFilePath(body.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if requested == "/" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": "/"})
		return
	}
	stdout, stderr, code, err := runContainerFilesExec(r.Context(), docker, ref, []string{"mkdir", "-p", "--", requested})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if code != 0 {
		writeError(w, http.StatusBadGateway, execFailureMessage(code, stderr, stdout))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": requested})
}

// handleServerContainerFilesRemove deletes a file or subtree inside the
// container. The path goes through the same validation as the rest of the
// surface plus the dangerous-path guard before reaching `rm -rf`.
func (s *Server) handleServerContainerFilesRemove(w http.ResponseWriter, r *http.Request) {
	docker, ref, ok := s.containerFilesGuard(w, r)
	if !ok {
		return
	}
	target := r.URL.Query().Get("path")
	if strings.TrimSpace(target) == "" {
		raw, readErr := readAllBounded(limitBody(w, r, 8*1024), 8*1024)
		if readErr != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				writeError(w, http.StatusBadRequest, "invalid request body")
				return
			}
		}
		target = body.Path
	}
	safePath, err := validateContainerFileDeletePath(target)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	stdout, stderr, code, err := runContainerFilesExec(r.Context(), docker, ref, []string{"rm", "-rf", "--", safePath})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if code != 0 {
		writeError(w, http.StatusBadGateway, execFailureMessage(code, stderr, stdout))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": safePath})
}

// handleServerContainerFilesDownload streams a file out of the container as
// an octet attachment. Directories are rejected; the archive endpoint on the
// regular file manager covers those.
func (s *Server) handleServerContainerFilesDownload(w http.ResponseWriter, r *http.Request) {
	docker, ref, ok := s.containerFilesGuard(w, r)
	if !ok {
		return
	}
	requested, err := validateContainerFilePath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	stream, stat, err := docker.CopyFromContainer(r.Context(), ref, requested)
	if err != nil {
		if client.IsErrNotFound(err) {
			writeError(w, http.StatusNotFound, "file not found in container")
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer stream.Close()
	if stat.Mode.IsDir() {
		writeError(w, http.StatusBadRequest, "path is a directory; download works on files only")
		return
	}

	tr := tar.NewReader(stream)
	hdr, err := tr.Next()
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found in container")
		return
	}
	if hdr.FileInfo().IsDir() {
		writeError(w, http.StatusBadRequest, "path is a directory; download works on files only")
		return
	}
	filename := sanitizeDownloadName(path.Base(requested))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if hdr.Size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", hdr.Size))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, tr)
}

// runContainerFilesExec runs a fixed argv inside the container, waits for the
// attached streams to close, and reports the exit code with captured output.
func runContainerFilesExec(ctx context.Context, docker *client.Client, ref string, cmd []string) (string, string, int, error) {
	execResp, err := docker.ContainerExecCreate(ctx, ref, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		if client.IsErrNotFound(err) {
			return "", "", -1, fmt.Errorf("container not found")
		}
		return "", "", -1, err
	}
	attached, err := docker.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", "", -1, err
	}
	defer attached.Close()
	var stdout, stderr bytes.Buffer
	_, _ = stdcopy.StdCopy(&stdout, &stderr, attached.Reader)
	inspected, err := docker.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return stdout.String(), stderr.String(), -1, err
	}
	return stdout.String(), stderr.String(), inspected.ExitCode, nil
}

func execFailureMessage(code int, stderr, stdout string) string {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		detail = strings.TrimSpace(stdout)
	}
	if len(detail) > 512 {
		detail = detail[:512] + "…"
	}
	if detail != "" {
		return fmt.Sprintf("container command failed (exit %d): %s", code, detail)
	}
	return fmt.Sprintf("container command failed (exit %d)", code)
}

func sanitizeDownloadName(raw string) string {
	name := sanitizeUploadName(raw)
	if name == "" {
		return "download"
	}
	return strings.NewReplacer(`"`, "'", "\r", "", "\n", "").Replace(name)
}
