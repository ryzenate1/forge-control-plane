package http

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// itoa converts a non-negative int to its decimal string representation.
// It avoids pulling strconv into hot-path code that only needs single-digit
// conversions.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	negative := false
	if i < 0 {
		negative = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// ---- Plugins ----
// Manifest metadata can be imported and queried. Lifecycle operations remain
// unavailable until Forge has a plugin runtime that can apply their effects.

func ListPlugins(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plugins, err := cfg.Store.ListPlugins(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(plugins)
	}
}

func GetPlugin(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plugin, err := cfg.Store.GetPlugin(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(plugin)
	}
}

// ImportPluginFromFile accepts a multipart upload named "file" containing a
// JSON plugin manifest. The plugin is registered but not yet installed.
func ImportPluginFromFile(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		fileHeader, err := c.FormFile("file")
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "file form field required")
		}
		f, err := fileHeader.Open()
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "cannot open uploaded file")
		}
		defer f.Close()
		body, err := io.ReadAll(f)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "cannot read uploaded file")
		}
		return importPluginManifest(c, cfg, body, "file:"+fileHeader.Filename)
	}
}

// ImportPluginFromURL fetches a JSON manifest from a remote URL.
func ImportPluginFromURL(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			URL string `json:"url"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.URL) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "url is required")
		}
		client := &http.Client{Timeout: 15 * time.Second}
		httpResp, err := client.Get(req.URL)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "failed to fetch plugin: "+err.Error())
		}
		defer httpResp.Body.Close()
		if httpResp.StatusCode >= 300 {
			return fiber.NewError(fiber.StatusBadGateway, "remote returned "+httpResp.Status)
		}
		body, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "failed to read response: "+err.Error())
		}
		return importPluginManifest(c, cfg, body, "url:"+req.URL)
	}
}

func importPluginManifest(c *fiber.Ctx, cfg Config, body []byte, source string) error {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "manifest is not valid JSON: "+err.Error())
	}
	name, _ := raw["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "manifest.name is required")
	}
	description, _ := raw["description"].(string)
	kind, _ := raw["kind"].(string)
	version, _ := raw["version"].(string)
	if kind == "" {
		kind = "integration"
	}
	if version == "" {
		version = "0.0.0"
	}
	ctx, cancel := requestContext()
	defer cancel()
	plugin, err := cfg.Store.CreatePlugin(ctx, store.CreatePluginRequest{
		Name:        name,
		Description: description,
		Kind:        kind,
		Version:     version,
		Manifest:    json.RawMessage(body),
		Source:      source,
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(plugin)
}

func pluginRuntimeUnavailable(operation string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
			"error":     "plugin runtime is not available",
			"operation": operation,
		})
	}
}

// InstallPlugin is retained for API compatibility. Installing code requires a
// plugin runtime, so manifests remain registry metadata until that exists.
func InstallPlugin(_ Config, _ string) fiber.Handler {
	return pluginRuntimeUnavailable("install")
}

func EnablePlugin(_ Config) fiber.Handler {
	return pluginRuntimeUnavailable("enable")
}

func DisablePlugin(_ Config) fiber.Handler {
	return pluginRuntimeUnavailable("disable")
}

func UpdatePlugin(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Name        *string          `json:"name"`
			Description *string          `json:"description"`
			Kind        *string          `json:"kind"`
			Version     *string          `json:"version"`
			Manifest    *json.RawMessage `json:"manifest"`
			InstallPath *string          `json:"installPath"`
			Source      *string          `json:"source"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plugin, err := cfg.Store.UpdatePlugin(ctx, c.Params("id"), store.UpdatePluginRequest{
			Name:        req.Name,
			Description: req.Description,
			Kind:        req.Kind,
			Version:     req.Version,
			Manifest:    req.Manifest,
			InstallPath: req.InstallPath,
			Source:      req.Source,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(plugin)
	}
}

func UninstallPlugin(_ Config, _ string) fiber.Handler {
	return pluginRuntimeUnavailable("uninstall")
}

func DeletePlugin(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeletePlugin(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

// pluginHash is exported so tests can compute stable checksums of plugin
// manifests.
func pluginHash(manifest []byte) string {
	h := sha256.Sum256(manifest)
	return hex.EncodeToString(h[:])
}

// pluginError is a small helper for wrapping install errors with context.
func pluginError(pluginName string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("plugin %s: %w", pluginName, err)
}
