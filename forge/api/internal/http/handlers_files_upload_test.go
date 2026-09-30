package http

import (
	"os"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
)

func TestHostFilesUpload_RejectsOversizedContentLength(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	handler := hostFilesUpload(Config{})
	var fctx fasthttp.RequestCtx
	fctx.Request.Header.SetMethod(fasthttp.MethodPost)
	fctx.Request.SetRequestURI("/host/files/upload?path=/tmp/test")
	fctx.Request.Header.SetContentType("application/octet-stream")
	fctx.Request.Header.SetContentLength(104857601) // 100MB +1
	c := app.AcquireCtx(&fctx)
	defer app.ReleaseCtx(c)
	err := handler(c)
	if err == nil {
		t.Fatalf("expected error for oversized Content-Length, got nil")
	}
	if e, ok := err.(*fiber.Error); ok {
		if e.Code != 413 {
			t.Fatalf("expected 413, got %d", e.Code)
		}
	} else {
		t.Fatalf("expected fiber.Error 413, got %T %v", err, err)
	}
}

func TestHostFilesUpload_RejectsViaHeaderBeforeBody(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	handler := hostFilesUpload(Config{})
	var fctx fasthttp.RequestCtx
	fctx.Request.Header.SetMethod(fasthttp.MethodPost)
	fctx.Request.SetRequestURI("/host/files/upload?path=/tmp/test")
	fctx.Request.Header.SetContentType("application/octet-stream")
	fctx.Request.Header.Set("Content-Length", "200000000")
	fctx.Request.Header.SetContentLength(200000000)
	c := app.AcquireCtx(&fctx)
	defer app.ReleaseCtx(c)
	err := handler(c)
	if err == nil {
		t.Fatalf("expected 413 for large header")
	}
	if e, ok := err.(*fiber.Error); ok {
		if e.Code != 413 {
			t.Fatalf("expected 413, got %d", e.Code)
		}
	} else {
		t.Fatalf("expected fiber.Error, got %T %v", err, err)
	}
	var fctx2 fasthttp.RequestCtx
	fctx2.Request.Header.SetMethod(fasthttp.MethodPost)
	fctx2.Request.SetRequestURI("/host/files/upload?path=/tmp/test2")
	fctx2.Request.Header.SetContentType("application/octet-stream")
	fctx2.Request.Header.Set("Content-Length", "200000000")
	c2 := app.AcquireCtx(&fctx2)
	defer app.ReleaseCtx(c2)
	err2 := handler(c2)
	if err2 == nil {
		t.Fatalf("expected 413 via header string")
	}
	if e, ok := err2.(*fiber.Error); ok && e.Code != 413 {
		t.Fatalf("expected 413 via header string, got %d", e.Code)
	}
}

func TestHostFilesUpload_MetadataOnlySigning(t *testing.T) {
	// Verify source contains metadata-only signing (nil body) not rawBody
	// This is a static check: ensure handlers_files.go uses SignedHeaders with nil
	// Read file directly via os.ReadFile
	contentBytes, err := readSourceFileForTest("handlers_files.go")
	if err != nil {
		t.Skipf("cannot read source: %v", err)
	}
	content := string(contentBytes)
	if !strings.Contains(content, `SignedHeaders(target.NodeToken, http.MethodPost, req.URL.RequestURI(), nil)`) {
		t.Error("hostFilesUpload should use metadata-only signing (nil body) for HMAC")
	}
	if strings.Contains(content, `SignedHeaders(target.NodeToken, http.MethodPost, req.URL.RequestURI(), rawBody)`) {
		t.Error("hostFilesUpload still uses rawBody for signing; should be nil for streaming")
	}
	// Check limit constant exists
	if !strings.Contains(content, "hostFilesUploadLimit = 100 * 1024 * 1024") {
		t.Error("hostFilesUploadLimit constant not found or incorrect")
	}
	// Check early cap before alloc phrase
	if !strings.Contains(content, "Early size cap before alloc") {
		t.Error("expected early size cap comment in handler")
	}
}

func readSourceFileForTest(name string) ([]byte, error) {
	return os.ReadFile(name)
}
