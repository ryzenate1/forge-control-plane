package http

import (
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// maxBrandingUploadBytes bounds a branding upload so a large file cannot fill
// the panel's data volume.
const maxBrandingUploadBytes = 8 << 20

func copyStream(dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, io.LimitReader(src, maxBrandingUploadBytes))
}

// brandingDataDir resolves the local branding asset folder. DATA_DIR defaults
// to ./data (mirroring the repo layout and the LANGS_DIR convention); the
// assets land in <DATA_DIR>/branding.
func brandingDataDir() (string, error) {
	root := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if root == "" {
		root = "data"
	}
	dir := filepath.Join(root, "branding")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create branding data dir: %w", err)
	}
	return dir, nil
}

var allowedBrandingTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

func extensionFor(contentType string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".bin"
	}
}

// brandingUploadHandler persists an uploaded brand image to the local data
// folder (name branding-{unix-ts}.{ext}). When persist is true it also sets
// the panel logo_url (the primary branding key surfaced by the public
// settings endpoint) via the existing panel settings store.
func brandingUploadHandler(cfg *Config, persist bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		fileHeader, err := c.FormFile("file")
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "multipart field \"file\" is required")
		}
		contentType := fileHeader.Header.Get("Content-Type")
		if !allowedBrandingTypes[contentType] {
			return fiber.NewError(fiber.StatusUnsupportedMediaType, "only PNG, JPEG, WebP or GIF images are allowed")
		}
		const maxBrandingBytes = 2 * 1024 * 1024
		if fileHeader.Size > maxBrandingBytes {
			return fiber.NewError(fiber.StatusRequestEntityTooLarge, "branding image must be 2 MB or smaller")
		}
		src, err := fileHeader.Open()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not open upload")
		}
		defer src.Close()

		dir, err := brandingDataDir()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		filename := fmt.Sprintf("branding-%d%s", time.Now().UnixMilli(), extensionFor(contentType))
		destPath := filepath.Join(dir, filename)
		dest, err := os.Create(destPath)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not write upload")
		}
		if _, err := copyStream(dest, src); err != nil {
			dest.Close()
			_ = os.Remove(destPath)
			return fiber.NewError(fiber.StatusInternalServerError, "could not store upload")
		}
		dest.Close()

		urlPath := "/api/v1/admin/branding/file/" + filename
		response := fiber.Map{
			"url":         urlPath,
			"filename":    filename,
			"contentType": contentType,
			"size":        fileHeader.Size,
		}
		if persist {
			ctx, cancel := requestContext()
			defer cancel()
			settings, err := cfg.Store.GetPanelSettings(ctx)
			if err != nil {
				settings = defaultPanelSettings()
			}
			settings.LogoURL = urlPath
			if err := cfg.Store.UpdatePanelSettings(ctx, settings); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "logo saved but settings update failed: "+err.Error())
			}
			maskPanelSettingsSecrets(&settings)
			response["settings"] = settings
		}
		return c.Status(fiber.StatusCreated).JSON(response)
	}
}

// brandingFileHandler serves stored brand assets (protected route).
func brandingFileHandler(cfg *Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		name := c.Params("name")
		if !safeBrandingName(name) {
			return fiber.NewError(fiber.StatusBadRequest, "invalid asset name")
		}
		dir, err := brandingDataDir()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		path := filepath.Join(dir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "asset not found")
		}
		contentType := mime.TypeByExtension(filepath.Ext(name))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		c.Set(fiber.HeaderContentType, contentType)
		c.Set(fiber.HeaderCacheControl, "public, max-age=3600")
		return c.Send(content)
	}
}

// safeBrandingName restricts served names to branding-<digits>.<ext> so path
// traversal through the public file route is impossible.
func safeBrandingName(name string) bool {
	if name == "" || strings.ContainsAny(name, "/\\..") && name != "branding-" {
		return false
	}
	base := filepath.Base(name)
	if base != name {
		return false
	}
	if !strings.HasPrefix(name, "branding-") {
		return false
	}
	ext := filepath.Ext(name)
	return ext == ".png" || ext == ".jpg" || ext == ".webp" || ext == ".gif"
}