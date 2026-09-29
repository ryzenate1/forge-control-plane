package http

// Container file manager (panel side).
//
// Forge already exposes host filesystem management (handlers_files.go) and
// per-server *volume* file management (the /servers/:id/files routes, which
// talk to the server's data directory on the node). Neither of those can look
// INSIDE the running workload container at the paths that are not backed by a
// bind mount — /etc, /usr/local/bin, an application's own image layout.
//
// These routes add exactly that: they proxy to Beacon's container file API
// (beacon/internal/server/container_files.go), which uses the Docker
// archive/exec APIs. The security-critical part is that the client never
// supplies a container: it supplies a *server*, the caller must hold a
// file permission on that server, and the daemon resolves the container from
// the server's own runtime binding. Paths are validated here and again on the
// daemon (defence in depth, not trust).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

const containerFilesRegistrarName = "container-files"

const (
	// containerFilesInlineLimit bounds responses that are buffered in memory
	// (listings, file reads). The daemon caps reads at 2 MiB; the extra room
	// keeps a large directory listing from being truncated silently.
	containerFilesInlineLimit = 4 * 1024 * 1024
	// containerFilesWriteLimit matches the daemon's inline write ceiling. It
	// stays below the daemon's 1 MiB HMAC body buffer so the signature can
	// still be verified.
	containerFilesWriteLimit = 896 * 1024
	// containerFilesUploadLimit mirrors the daemon's ceiling for signed
	// streaming uploads (beacon maxUploadChunkBytes). The daemon authenticates
	// /files/upload with an empty-body signature and caps the streamed body at
	// this size, so accepting more here would only produce a confusing 413.
	containerFilesUploadLimit = 8 * 1024 * 1024
)

// containerDangerousPaths mirrors the daemon's guard: recursive deletes must
// never target the container's filesystem roots.
var containerDangerousPaths = []string{
	"/", "/bin", "/boot", "/dev", "/etc", "/home", "/lib", "/lib32", "/lib64",
	"/media", "/mnt", "/opt", "/proc", "/root", "/run", "/sbin", "/srv",
	"/sys", "/tmp", "/usr", "/var",
}

func init() {
	RegisterPhaseRegistrar(containerFilesRegistrarName, 220, registerContainerFilesRoutesPhase)
}

func registerContainerFilesRoutesPhase(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("%s: nil config", containerFilesRegistrarName)
	}
	if cfg.Store == nil || cfg.Daemon == nil {
		return fmt.Errorf("%w: postgres and daemon are required, container file routes skipped", ErrPhaseSkipped)
	}
	mutationLimiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis, cfg.RedisEnabled && strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production")))
	registerContainerFileRoutes(protected, *cfg, mutationLimiter)
	return nil
}

func registerContainerFileRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	protected.Get("/servers/:id/container/files/ls", requireServerPermission(cfg, store.PermFileRead), containerFilesList(cfg))
	protected.Get("/servers/:id/container/files/read", requireServerPermission(cfg, store.PermFileReadContent), containerFilesRead(cfg))
	protected.Get("/servers/:id/container/files/download", requireServerPermission(cfg, store.PermFileReadContent), containerFilesDownload(cfg))
	protected.Put("/servers/:id/container/files/write", mutationLimiter, requireServerPermission(cfg, store.PermFileUpdate), containerFilesWrite(cfg))
	protected.Post("/servers/:id/container/files/upload", mutationLimiter, requireServerPermission(cfg, store.PermFileCreate), containerFilesUpload(cfg))
	protected.Post("/servers/:id/container/files/mkdir", mutationLimiter, requireServerPermission(cfg, store.PermFileCreate), containerFilesMkdir(cfg))
	protected.Delete("/servers/:id/container/files", mutationLimiter, requireServerPermission(cfg, store.PermFileDelete), containerFilesRemove(cfg))
}

// containerServerTarget resolves the node that hosts the server workload. The
// container itself is resolved by the daemon from this server binding, so
// nothing downstream accepts a caller-supplied container reference.
func containerServerTarget(cfg Config, c *fiber.Ctx) (store.ServerControlTarget, error) {
	ctx, cancel := requestContext()
	defer cancel()
	target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.ServerControlTarget{}, fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		return store.ServerControlTarget{}, respondInternalError(c, err)
	}
	if strings.TrimSpace(target.NodeURL) == "" || strings.TrimSpace(target.NodeToken) == "" {
		return store.ServerControlTarget{}, fiber.NewError(fiber.StatusServiceUnavailable, "server node is missing a base url or daemon token")
	}
	return target, nil
}

func containerFilesList(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		dir := c.Query("path", "/")
		if err := validateContainerClientPath(dir); err != nil {
			return err
		}
		resp, release, err := containerFilesRoundTrip(cfg, c, http.MethodGet, "/container/files/ls", url.Values{"path": []string{dir}}, nil, 60*time.Second)
		if err != nil {
			return err
		}
		defer release()
		return containerFilesPassthrough(c, resp, containerFilesInlineLimit)
	}
}

func containerFilesRead(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		filePath := c.Query("path")
		if err := validateContainerClientPath(filePath); err != nil {
			return err
		}
		resp, release, err := containerFilesRoundTrip(cfg, c, http.MethodGet, "/container/files/read", url.Values{"path": []string{filePath}}, nil, 60*time.Second)
		if err != nil {
			return err
		}
		defer release()
		recordContainerFilesAudit(cfg, c, "server:container-files:read", filePath)
		return containerFilesPassthrough(c, resp, containerFilesInlineLimit)
	}
}

func containerFilesWrite(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := validateContainerClientPath(body.Path); err != nil {
			return err
		}
		if body.Path == "/" {
			return fiber.NewError(fiber.StatusBadRequest, "path must name a file, not the filesystem root")
		}
		if len(body.Content) > containerFilesWriteLimit {
			return fiber.NewError(fiber.StatusRequestEntityTooLarge, "content exceeds the inline write limit")
		}
		payload, err := json.Marshal(map[string]string{"path": body.Path, "content": body.Content})
		if err != nil {
			return respondInternalError(c, err)
		}
		resp, release, err := containerFilesRoundTrip(cfg, c, http.MethodPut, "/container/files/write", nil, payload, 60*time.Second)
		if err != nil {
			return err
		}
		defer release()
		recordContainerFilesAudit(cfg, c, "server:container-files:write", body.Path)
		return containerFilesPassthrough(c, resp, containerFilesInlineLimit)
	}
}

func containerFilesMkdir(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body struct {
			Path string `json:"path"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := validateContainerClientPath(body.Path); err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]string{"path": body.Path})
		if err != nil {
			return respondInternalError(c, err)
		}
		resp, release, err := containerFilesRoundTrip(cfg, c, http.MethodPost, "/container/files/mkdir", nil, payload, 60*time.Second)
		if err != nil {
			return err
		}
		defer release()
		recordContainerFilesAudit(cfg, c, "server:container-files:mkdir", body.Path)
		return containerFilesPassthrough(c, resp, containerFilesInlineLimit)
	}
}

func containerFilesRemove(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		target := c.Query("path")
		if strings.TrimSpace(target) == "" {
			var body struct {
				Path string `json:"path"`
			}
			if err := c.BodyParser(&body); err == nil {
				target = body.Path
			}
		}
		if err := validateContainerDeletePath(target); err != nil {
			return err
		}
		resp, release, err := containerFilesRoundTrip(cfg, c, http.MethodDelete, "/container/files", url.Values{"path": []string{target}}, nil, 60*time.Second)
		if err != nil {
			return err
		}
		defer release()
		recordContainerFilesAudit(cfg, c, "server:container-files:remove", target)
		return containerFilesPassthrough(c, resp, containerFilesInlineLimit)
	}
}

// containerFilesUpload forwards the caller's multipart body verbatim. The body
// is never re-encoded: it is signed with the empty body signature the daemon
// expects for streamed uploads, exactly like the host uploader.
func containerFilesUpload(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cl := c.Request().Header.ContentLength(); cl > containerFilesUploadLimit {
			return fiber.NewError(fiber.StatusRequestEntityTooLarge, "upload exceeds maximum size of 8 MiB")
		}
		if raw := c.Request().Header.Peek("Content-Length"); len(raw) > 0 {
			if n, perr := strconv.ParseInt(string(raw), 10, 64); perr == nil && n > containerFilesUploadLimit {
				return fiber.NewError(fiber.StatusRequestEntityTooLarge, "upload exceeds maximum size of 8 MiB")
			}
		}
		destDir := c.Query("path", "/")
		if err := validateContainerClientPath(destDir); err != nil {
			return err
		}
		contentType := strings.ToLower(string(c.Request().Header.ContentType()))
		if !strings.HasPrefix(contentType, "multipart/form-data;") {
			return fiber.NewError(fiber.StatusUnsupportedMediaType, "upload must be multipart/form-data")
		}
		target, err := containerServerTarget(cfg, c)
		if err != nil {
			return err
		}
		endpoint := strings.TrimRight(target.NodeURL, "/") + "/servers/" + url.PathEscape(target.ServerID) + "/container/files/upload?path=" + url.QueryEscape(destDir)
		rawBody := c.Request().Body()
		req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, endpoint, bytes.NewReader(rawBody))
		if err != nil {
			return respondInternalError(c, err)
		}
		req.Header.Set("Content-Type", string(c.Request().Header.ContentType()))
		req.ContentLength = int64(len(rawBody))
		// Streamed-upload routes are authenticated with an empty-body signature
		// so the daemon never has to spool the body to verify it.
		headers, err := cfg.Daemon.SignedHeaders(target.NodeToken, http.MethodPost, req.URL.RequestURI(), nil)
		if err != nil {
			return respondInternalError(c, err)
		}
		for key, values := range headers {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
		resp, err := cfg.Daemon.HTTPClient().Do(req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		defer resp.Body.Close()
		recordContainerFilesAudit(cfg, c, "server:container-files:upload", destDir)
		return containerFilesPassthrough(c, resp, containerFilesInlineLimit)
	}
}

// containerFilesDownload streams the file straight through without buffering.
func containerFilesDownload(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		filePath := c.Query("path")
		if err := validateContainerClientPath(filePath); err != nil {
			return err
		}
		target, err := containerServerTarget(cfg, c)
		if err != nil {
			return err
		}
		endpoint := strings.TrimRight(target.NodeURL, "/") + "/servers/" + url.PathEscape(target.ServerID) + "/container/files/download?path=" + url.QueryEscape(filePath)
		req, err := http.NewRequestWithContext(c.Context(), http.MethodGet, endpoint, nil)
		if err != nil {
			return respondInternalError(c, err)
		}
		headers, err := cfg.Daemon.SignedHeaders(target.NodeToken, http.MethodGet, req.URL.RequestURI(), nil)
		if err != nil {
			return respondInternalError(c, err)
		}
		for key, values := range headers {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
		resp, err := cfg.Daemon.HTTPClient().Do(req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer resp.Body.Close()
			return fiber.NewError(resp.StatusCode, containerFilesErrorMessage(resp.Body, resp.StatusCode))
		}
		if disposition := resp.Header.Get("Content-Disposition"); disposition != "" {
			c.Set("Content-Disposition", disposition)
		} else {
			c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(path.Base(filePath))))
		}
		c.Set("Content-Type", "application/octet-stream")
		return c.SendStream(resp.Body)
	}
}

// containerFilesRoundTrip signs and issues a buffered request to the daemon's
// container file API. The returned release func closes the response body.
func containerFilesRoundTrip(cfg Config, c *fiber.Ctx, method, daemonPath string, query url.Values, body []byte, timeout time.Duration) (*http.Response, func(), error) {
	target, err := containerServerTarget(cfg, c)
	if err != nil {
		return nil, nil, err
	}
	endpoint := strings.TrimRight(target.NodeURL, "/") + "/servers/" + url.PathEscape(target.ServerID) + daemonPath
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	ctx, cancel := context.WithTimeout(c.Context(), timeout)
	var payload io.Reader
	if len(body) > 0 {
		payload = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		cancel()
		return nil, func() {}, respondInternalError(c, err)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = int64(len(body))
	}
	// The signature covers method + RequestURI + body, so it must be computed
	// from the request that is actually going on the wire.
	headers, err := cfg.Daemon.SignedHeaders(target.NodeToken, method, req.URL.RequestURI(), body)
	if err != nil {
		cancel()
		return nil, func() {}, respondInternalError(c, err)
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	resp, err := cfg.Daemon.HTTPClient().Do(req)
	if err != nil {
		cancel()
		return nil, func() {}, fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return resp, func() {
		_ = resp.Body.Close()
		cancel()
	}, nil
}

// containerFilesPassthrough copies a daemon response into the panel response,
// translating non-2xx statuses into fiber errors with the daemon's own
// (already sanitized) message rather than a generic one.
func containerFilesPassthrough(c *fiber.Ctx, resp *http.Response, maxBytes int64) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fiber.NewError(resp.StatusCode, containerFilesErrorMessageBytes(body, resp.StatusCode))
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		c.Set("Content-Type", contentType)
	}
	if size := resp.Header.Get("X-File-Size"); size != "" {
		c.Set("X-File-Size", size)
	}
	return c.Status(resp.StatusCode).Send(body)
}

func containerFilesErrorMessage(reader io.Reader, status int) string {
	body, _ := io.ReadAll(io.LimitReader(reader, 8*1024))
	return containerFilesErrorMessageBytes(body, status)
}

func containerFilesErrorMessageBytes(body []byte, status int) string {
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Error) != "" {
		return payload.Error
	}
	message := strings.TrimSpace(string(body))
	if len(message) > 512 {
		message = message[:512]
	}
	if message == "" {
		return fmt.Sprintf("daemon request failed with status %d", status)
	}
	return message
}

// validateContainerClientPath enforces the same absolute + canonical rules the
// daemon enforces, so a bad request is rejected without a round trip.
func validateContainerClientPath(value string) error {
	if value == "" || len(value) > 4096 {
		return fiber.NewError(fiber.StatusBadRequest, "path is required")
	}
	if strings.ContainsRune(value, '\x00') || strings.Contains(value, `\`) {
		return fiber.NewError(fiber.StatusBadRequest, "path contains invalid characters")
	}
	if !strings.HasPrefix(value, "/") {
		return fiber.NewError(fiber.StatusBadRequest, "path must be absolute")
	}
	cleaned := path.Clean(value)
	normalized := strings.TrimSuffix(value, "/")
	if normalized == "" {
		normalized = "/"
	}
	if cleaned != normalized {
		return fiber.NewError(fiber.StatusBadRequest, "path must be canonical and may not contain traversal segments")
	}
	return nil
}

// validateContainerDeletePath additionally refuses the guarded roots.
func validateContainerDeletePath(value string) error {
	if err := validateContainerClientPath(value); err != nil {
		return err
	}
	cleaned := path.Clean(value)
	for _, dangerous := range containerDangerousPaths {
		if cleaned == dangerous {
			return fiber.NewError(fiber.StatusBadRequest, "refusing to delete restricted path "+cleaned)
		}
	}
	return nil
}

func recordContainerFilesAudit(cfg Config, c *fiber.Ctx, action, filePath string) {
	serverID := c.Params("id")
	recordAudit(cfg, c, action, "server", &serverID, map[string]string{"path": filePath})
}
