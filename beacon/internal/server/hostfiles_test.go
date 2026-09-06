package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newHostTestHandler(t *testing.T) http.Handler {
	t.Helper()
	// Use TempDir for dataDir but host files operate on "/" regardless
	_, handler := NewServer(nil, t.TempDir())
	return handler
}

func TestHostFilesListRequiresAuthButValidatesPath(t *testing.T) {
	// Host files should require panel signature when token configured
	_, handler := NewServer(nil, t.TempDir(), "secret")
	req := httptest.NewRequest(http.MethodGet, "/v1/files/list?path=/tmp", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated host list should be 401, got %d", rec.Code)
	}
}

func TestHostFilesListRejectsTraversal(t *testing.T) {
	handler := newHostTestHandler(t)
	// Use helper with token "secret" but handler has no token (allowInsecure)
	// So we can test validation without auth
	for _, raw := range []string{"/v1/files/list?path=../etc", "/v1/files/list?path=/etc/../etc/passwd", "/v1/files/list?path=/bad\\path"} {
		req := httptest.NewRequest(http.MethodGet, raw, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("path %q should be 400, got %d", raw, rec.Code)
		}
	}
}

func TestHostFilesOperationsEndToEnd(t *testing.T) {
	handler := newHostTestHandler(t)
	// We use a temp host root via symlink? Our hostFS is "/" so we can't directly control.
	// Instead test that endpoints exist and respond with appropriate status for valid/invalid paths
	// For list of "/" should succeed (200) even if empty
	req := httptest.NewRequest(http.MethodGet, "/v1/files/list?path=/tmp", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("host list /tmp should be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var files []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &files); err != nil {
		t.Fatalf("list response not JSON: %v", err)
	}
	// Test read of non-existent file should be 404
	req = httptest.NewRequest(http.MethodPost, "/v1/files/read", strings.NewReader(`{"path":"/nonexistent-`+t.Name()+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("read nonexistent should be 404, got %d", rec.Code)
	}
	// Test write and read back via host filesystem (using /tmp which is writable)
	tmpFile := filepath.Join("/tmp", "beacon-host-test-"+t.Name()+".txt")
	defer os.Remove(tmpFile)
	content := "hello host"
	req = httptest.NewRequest(http.MethodPost, "/v1/files/write", strings.NewReader(`{"path":"`+tmpFile+`","content":"`+content+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("host write should be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Verify via direct OS read
	body, err := os.ReadFile(tmpFile)
	if err != nil || string(body) != content {
		t.Fatalf("host write not persisted: body=%q err=%v", body, err)
	}
	// Read back via API
	req = httptest.NewRequest(http.MethodPost, "/v1/files/read", strings.NewReader(`{"path":"`+tmpFile+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("host read back should be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != content {
		t.Fatalf("host read back mismatch: got %q want %q", rec.Body.String(), content)
	}
	// Test mkdir
	tmpDir := filepath.Join("/tmp", "beacon-host-test-dir-"+t.Name())
	defer os.RemoveAll(tmpDir)
	req = httptest.NewRequest(http.MethodPost, "/v1/files/mkdir", strings.NewReader(`{"path":"`+tmpDir+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("host mkdir should be 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(tmpDir); err != nil {
		t.Fatalf("mkdir did not create dir: %v", err)
	}
	// Test chmod
	req = httptest.NewRequest(http.MethodPost, "/v1/files/chmod", strings.NewReader(`{"path":"`+tmpFile+`","mode":"0644"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("host chmod should be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// Test copy
	copyPath := tmpFile + ".copy"
	defer os.Remove(copyPath)
	req = httptest.NewRequest(http.MethodPost, "/v1/files/copy", strings.NewReader(`{"source":"`+tmpFile+`","target":"`+copyPath+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("host copy should be 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(copyPath); err != nil {
		t.Fatalf("copy did not create dest: %v", err)
	}
	// Test rename
	renamePath := tmpFile + ".renamed"
	defer os.Remove(renamePath)
	req = httptest.NewRequest(http.MethodPost, "/v1/files/rename", strings.NewReader(`{"source":"`+copyPath+`","target":"`+renamePath+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("host rename should be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(renamePath); err != nil {
		t.Fatalf("rename dest missing: %v", err)
	}
	if _, err := os.Stat(copyPath); !os.IsNotExist(err) {
		t.Fatalf("rename source should be gone")
	}
	// Test download
	req = httptest.NewRequest(http.MethodGet, "/v1/files/download?path="+tmpFile, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("host download should be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != content {
		t.Fatalf("download mismatch: got %q want %q", rec.Body.String(), content)
	}
	// Test remove
	req = httptest.NewRequest(http.MethodPost, "/v1/files/remove", strings.NewReader(`{"path":"`+tmpFile+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("host remove should be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Fatalf("remove did not delete file")
	}
	// Test upload (raw body)
	uploadPath := filepath.Join("/tmp", "beacon-host-upload-"+t.Name()+".bin")
	defer os.Remove(uploadPath)
	uploadContent := "uploaded data"
	req = httptest.NewRequest(http.MethodPost, "/v1/files/upload?path="+uploadPath, strings.NewReader(uploadContent))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("host upload should be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	data, _ := os.ReadFile(uploadPath)
	if string(data) != uploadContent {
		t.Fatalf("upload mismatch: got %q want %q", string(data), uploadContent)
	}
	// Test upload rejects root
	req = httptest.NewRequest(http.MethodPost, "/v1/files/upload?path=/", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("upload to root should be 400, got %d", rec.Code)
	}
}

func TestHostFilesChmodValidatesMode(t *testing.T) {
	handler := newHostTestHandler(t)
	for _, mode := range []string{"", "64", "999", "08", "abc"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/files/chmod", strings.NewReader(`{"path":"/tmp/foo","mode":"`+mode+`"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("chmod mode %q should be 400, got %d", mode, rec.Code)
		}
	}
}

func TestHostFilesListOutputStructure(t *testing.T) {
	handler := newHostTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/files/list?path=/tmp", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list should be 200")
	}
	body, _ := io.ReadAll(rec.Body)
	var files []map[string]any
	if err := json.Unmarshal(body, &files); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Ensure each entry has required keys
	for _, f := range files {
		if _, ok := f["name"]; !ok {
			t.Error("entry missing name")
		}
		if _, ok := f["directory"]; !ok {
			t.Error("entry missing directory")
		}
		if _, ok := f["size"]; !ok {
			t.Error("entry missing size")
		}
	}
}

func TestHostFilesUploadWithSignedStreamingAuth(t *testing.T) {
	server, handler := NewServer(nil, t.TempDir(), "secret")
	_ = server
	uploadPath := filepath.Join("/tmp", "beacon-host-signed-upload-"+t.Name()+".bin")
	defer os.Remove(uploadPath)
	content := "signed upload content"
	// Host upload is POST with streaming auth (empty body signature)
	body := content
	req := httptest.NewRequest(http.MethodPost, "/v1/files/upload?path="+uploadPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	// Use current timestamp to pass 5min skew check
	timestamp := time.Now().UTC().Format(time.RFC3339)
	nonce := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	// Streaming upload signs with empty body (nil)
	sig := sign("secret", req.Method, req.URL.RequestURI(), timestamp, nil, nonce)
	req.Header.Set("X-Panel-Timestamp", timestamp)
	req.Header.Set("X-Panel-Nonce", nonce)
	req.Header.Set("X-Panel-Signature", sig)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("signed host upload should be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	data, _ := os.ReadFile(uploadPath)
	if string(data) != content {
		t.Fatalf("signed upload mismatch: got %q want %q", string(data), content)
	}
}
