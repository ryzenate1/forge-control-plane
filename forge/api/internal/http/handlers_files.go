package http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	fiberws "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	gorilla "github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

// nodeTarget holds the resolved Beacon node connection details.
type nodeTarget struct {
	BaseURL   string
	NodeToken string
	NodeID    string
}

// resolveNode looks up a node from the store by nodeId query param,
// or picks the first active node.
func resolveNode(cfg Config, c *fiber.Ctx) (*nodeTarget, error) {
	if cfg.Store == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
	}
	ctx, cancel := requestContext()
	defer cancel()

	nodeID := c.Query("nodeId")
	if nodeID == "" {
		nodes, err := cfg.Store.ListNodes(ctx)
		if err != nil {
			return nil, respondInternalError(c, err)
		}
		for _, n := range nodes {
			if n.Status == "active" {
				nodeID = n.ID
				break
			}
		}
		if nodeID == "" && len(nodes) > 0 {
			nodeID = nodes[0].ID
		}
		if nodeID == "" {
			return nil, fiber.NewError(fiber.StatusServiceUnavailable, "no nodes available — add a node in admin settings")
		}
	}

	node, err := cfg.Store.GetNode(ctx, nodeID)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusNotFound, "node not found")
	}
	nodeToken, err := cfg.Store.GetNodeDaemonToken(ctx, nodeID)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusInternalServerError, "failed to get node token")
	}
	return &nodeTarget{BaseURL: node.BaseURL, NodeToken: nodeToken, NodeID: nodeID}, nil
}

func resolveNodeMiddleware(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		target, err := resolveNode(cfg, c)
		if err != nil {
			return err
		}
		c.Locals("nodeTarget", target)
		return c.Next()
	}
}

func registerHostFileRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	host := protected.Group("/host/files", requireRole("admin"))

	if cfg.Store == nil || cfg.Daemon == nil {
		unavailable := func(c *fiber.Ctx) error {
			return fiber.NewError(fiber.StatusServiceUnavailable, "host file service requires postgres and daemon")
		}
		host.Get("/list", unavailable)
		host.Post("/read", unavailable)
		host.Post("/write", unavailable)
		host.Post("/mkdir", unavailable)
		host.Post("/remove", unavailable)
		host.Post("/rename", unavailable)
		host.Post("/copy", unavailable)
		host.Post("/chmod", unavailable)
		host.Post("/upload", unavailable)
		host.Get("/download", unavailable)
		return
	}

	host.Get("/list", hostFilesList(cfg))
	host.Post("/read", mutationLimiter, hostFilesRead(cfg))
	host.Post("/write", mutationLimiter, hostFilesWrite(cfg))
	host.Post("/mkdir", mutationLimiter, hostFilesMkdir(cfg))
	host.Post("/remove", mutationLimiter, hostFilesRemove(cfg))
	host.Post("/rename", mutationLimiter, hostFilesRename(cfg))
	host.Post("/copy", mutationLimiter, hostFilesCopy(cfg))
	host.Post("/chmod", mutationLimiter, hostFilesChmod(cfg))
	host.Post("/upload", mutationLimiter, hostFilesUpload(cfg))
	host.Get("/download", hostFilesDownload(cfg))
}

func registerHostTerminalRoute(protected fiber.Router, cfg Config) {
	if cfg.Store == nil || cfg.Daemon == nil {
		protected.Get("/host/terminal/ws", requireRole("admin"), func(c *fiber.Ctx) error {
			return fiber.NewError(fiber.StatusServiceUnavailable, "host terminal requires postgres and daemon")
		})
		return
	}

	protected.Get("/host/terminal/ws", requireRole("admin"), resolveNodeMiddleware(cfg), fiberws.New(func(client *fiberws.Conn) {
		defer client.Close()

		targetRaw := client.Locals("nodeTarget")
		target, ok := targetRaw.(*nodeTarget)
		if !ok {
			_ = client.WriteJSON(map[string]any{"error": "node not resolved"})
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		upstreamURL, requestURI := cfg.Daemon.HostTerminalWSURL(target.BaseURL)
		headers, err := cfg.Daemon.SignedHeaders(target.NodeToken, http.MethodGet, requestURI, nil)
		if err != nil {
			_ = client.WriteJSON(map[string]any{"error": err.Error()})
			return
		}
		upstream, _, err := gorilla.DefaultDialer.DialContext(ctx, upstreamURL, headers)
		if err != nil {
			_ = client.WriteJSON(map[string]any{"error": err.Error()})
			return
		}
		defer upstream.Close()

		configureClientSocket(client)
		configureUpstreamSocket(upstream)

		// Ping keepalive — detects half-open connections.
		pingTicker := time.NewTicker(30 * time.Second)
		defer pingTicker.Stop()
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("file proxy keepalive panicked", "panic", r)
				}
			}()
			for {
				select {
				case <-pingTicker.C:
					if err := upstream.WriteControl(gorilla.PingMessage, []byte("keepalive"), time.Now().Add(5*time.Second)); err != nil {
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()

		errs := make(chan error, 2)
		clientLimiter := rate.NewLimiter(rate.Limit(10), 20)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("file proxy upstream reader panicked", "panic", r)
				}
			}()
			for {
				if ctx.Err() != nil {
					errs <- ctx.Err()
					return
				}
				messageType, payload, readErr := upstream.ReadMessage()
				if readErr != nil {
					errs <- readErr
					return
				}
				_ = client.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if writeErr := client.WriteMessage(messageType, payload); writeErr != nil {
					errs <- writeErr
					return
				}
			}
		}()
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("file proxy client reader panicked", "panic", r)
				}
			}()
			for {
				if ctx.Err() != nil {
					errs <- ctx.Err()
					return
				}
				if err := clientLimiter.Wait(ctx); err != nil {
					errs <- err
					return
				}
				messageType, payload, readErr := client.ReadMessage()
				if readErr != nil {
					errs <- readErr
					return
				}
				_ = upstream.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if writeErr := upstream.WriteMessage(messageType, payload); writeErr != nil {
					errs <- writeErr
					return
				}
			}
		}()
		<-errs
		cancel()
		_ = client.Close()
		_ = upstream.Close()
		<-errs
	}, fiberws.Config{
		RecoverHandler: func(conn *fiberws.Conn) {
			if err := recover(); err != nil {
				_ = conn.WriteJSON(fiber.Map{"error": "internal error"})
				_ = conn.Close()
			}
		},
		Origins: getWebSocketAllowedOrigins(cfg),
	}))
}

func hostFilesList(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		target, err := resolveNode(cfg, c)
		if err != nil {
			return err
		}
		filePath := c.Query("path", "/")
		if err := validateHostFilePath(filePath); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
		defer cancel()
		data, err := cfg.Daemon.HostFilesList(ctx, target.BaseURL, target.NodeToken, filePath)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.Send(data)
	}
}

func hostFilesRead(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		target, err := resolveNode(cfg, c)
		if err != nil {
			return err
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := validateHostFilePath(body.Path); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
		defer cancel()
		content, err := cfg.Daemon.HostFilesRead(ctx, target.BaseURL, target.NodeToken, body.Path)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.SendString(content)
	}
}

func hostFilesWrite(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		target, err := resolveNode(cfg, c)
		if err != nil {
			return err
		}
		var body struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := validateHostFilePath(body.Path); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
		defer cancel()
		if err := cfg.Daemon.HostFilesWrite(ctx, target.BaseURL, target.NodeToken, body.Path, body.Content); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

func hostFilesMkdir(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		target, err := resolveNode(cfg, c)
		if err != nil {
			return err
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := validateHostFilePath(body.Path); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
		defer cancel()
		if err := cfg.Daemon.HostFilesMkdir(ctx, target.BaseURL, target.NodeToken, body.Path); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

func hostFilesRemove(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		target, err := resolveNode(cfg, c)
		if err != nil {
			return err
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := validateHostFilePath(body.Path); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
		defer cancel()
		if err := cfg.Daemon.HostFilesRemove(ctx, target.BaseURL, target.NodeToken, body.Path); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

func hostFilesUpload(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		target, err := resolveNode(cfg, c)
		if err != nil {
			return err
		}
		destPath := c.Query("path", "/")
		if err := validateHostFilePath(destPath); err != nil {
			return err
		}
		contentType := strings.ToLower(string(c.Request().Header.ContentType()))
		if contentType != "application/octet-stream" && !strings.HasPrefix(contentType, "multipart/form-data;") {
			return fiber.NewError(fiber.StatusUnsupportedMediaType, "upload must be application/octet-stream or multipart/form-data")
		}
		targetURL := strings.TrimRight(target.BaseURL, "/") + "/v1/files/upload?path=" + url.QueryEscape(destPath)

		rawBody := c.Request().Body()
		req, err := http.NewRequestWithContext(c.Context(), http.MethodPost, targetURL, bytes.NewReader(rawBody))
		if err != nil {
			return respondInternalError(c, err)
		}
		req.Header.Set("Content-Type", string(c.Request().Header.ContentType()))
		req.ContentLength = int64(len(rawBody))

		headers, err := cfg.Daemon.SignedHeaders(target.NodeToken, http.MethodPost, req.URL.RequestURI(), rawBody)
		if err != nil {
			return respondInternalError(c, err)
		}
		for k, vals := range headers {
			for _, v := range vals {
				req.Header.Add(k, v)
			}
		}

		resp, err := cfg.Daemon.HTTPClient().Do(req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fiber.NewError(resp.StatusCode, string(respBody))
		}
		return c.Send(respBody)
	}
}

func hostFilesDownload(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		target, err := resolveNode(cfg, c)
		if err != nil {
			return err
		}
		filePath := c.Query("path")
		if filePath == "" {
			return fiber.NewError(fiber.StatusBadRequest, "path is required")
		}
		if err := validateHostFilePath(filePath); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.Context(), 5*time.Minute)
		defer cancel()
		reader, err := cfg.Daemon.HostFilesDownload(ctx, target.BaseURL, target.NodeToken, filePath)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		defer reader.Close()

		filename := filePath
		if idx := strings.LastIndex(filePath, "/"); idx >= 0 {
			filename = filePath[idx+1:]
		}

		c.Set("Content-Type", "application/octet-stream")
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(filename)))
		return c.SendStream(reader)
	}
}

func hostFilesRename(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		target, err := resolveNode(cfg, c)
		if err != nil {
			return err
		}
		var body struct {
			OldPath string `json:"oldPath"`
			NewPath string `json:"newPath"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := validateHostFilePath(body.OldPath); err != nil {
			return err
		}
		if err := validateHostFilePath(body.NewPath); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
		defer cancel()
		if err := cfg.Daemon.HostFilesRename(ctx, target.BaseURL, target.NodeToken, body.OldPath, body.NewPath); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

func hostFilesCopy(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		target, err := resolveNode(cfg, c)
		if err != nil {
			return err
		}
		var body struct {
			SourcePath string `json:"sourcePath"`
			DestPath   string `json:"destPath"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := validateHostFilePath(body.SourcePath); err != nil {
			return err
		}
		if err := validateHostFilePath(body.DestPath); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
		defer cancel()
		if err := cfg.Daemon.HostFilesCopy(ctx, target.BaseURL, target.NodeToken, body.SourcePath, body.DestPath); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

func hostFilesChmod(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		target, err := resolveNode(cfg, c)
		if err != nil {
			return err
		}
		var body struct {
			Path string `json:"path"`
			Mode string `json:"mode"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := validateHostFilePath(body.Path); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
		defer cancel()
		if err := cfg.Daemon.HostFilesChmod(ctx, target.BaseURL, target.NodeToken, body.Path, body.Mode); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

func validateHostFilePath(value string) error {
	if value == "" || strings.ContainsRune(value, '\x00') || !strings.HasPrefix(value, "/") {
		return fiber.NewError(fiber.StatusBadRequest, "path must be an absolute path")
	}
	cleaned := path.Clean(value)
	normalized := strings.TrimSuffix(value, "/")
	if normalized == "" {
		normalized = "/"
	}
	if cleaned != normalized || strings.Contains(value, `\`) {
		return fiber.NewError(fiber.StatusBadRequest, "path must be canonical and may not contain traversal segments")
	}
	return nil
}
