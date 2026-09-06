package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListFiles(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)
	serverDir := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(serverDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverDir, "test.txt"), []byte("hello"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(serverDir, "subdir"), 0o750); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/servers/"+testServerID+"/files", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var files []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&files); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	foundFile := false
	foundDir := false
	for _, f := range files {
		name, _ := f["name"].(string)
		isDir, _ := f["directory"].(bool)
		if name == "test.txt" && !isDir {
			foundFile = true
		}
		if name == "subdir" && isDir {
			foundDir = true
		}
	}
	if !foundFile {
		t.Error("expected test.txt in file listing")
	}
	if !foundDir {
		t.Error("expected subdir in file listing")
	}
}

func TestListFilesWithPath(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)
	serverDir := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(filepath.Join(serverDir, "nested", "deep"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverDir, "nested", "deep", "secret.txt"), []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/servers/"+testServerID+"/files?path=nested/deep", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var files []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&files); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	found := false
	for _, f := range files {
		if name, _ := f["name"].(string); name == "secret.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected secret.txt in nested listing")
	}
}

func TestReadFile(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)
	serverDir := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(serverDir, 0o750); err != nil {
		t.Fatal(err)
	}
	content := "hello world file content"
	if err := os.WriteFile(filepath.Join(serverDir, "readme.txt"), []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/servers/"+testServerID+"/files/content?path=readme.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != content {
		t.Errorf("expected body %q, got %q", content, rec.Body.String())
	}
}

func TestReadFileNotFound(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)
	serverDir := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(serverDir, 0o750); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/servers/"+testServerID+"/files/content?path=nonexistent.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestWriteFile(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)
	serverDir := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(serverDir, 0o750); err != nil {
		t.Fatal(err)
	}

	content := "new file content written via API"
	req := httptest.NewRequest(http.MethodPut, "/servers/"+testServerID+"/files/content?path=newfile.txt", strings.NewReader(content))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body, err := os.ReadFile(filepath.Join(serverDir, "newfile.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != content {
		t.Errorf("expected file content %q, got %q", content, string(body))
	}
}

func TestWriteFileOverwritesExisting(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)
	serverDir := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(serverDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverDir, "overwrite.txt"), []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPut, "/servers/"+testServerID+"/files/content?path=overwrite.txt", strings.NewReader("updated"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body, err := os.ReadFile(filepath.Join(serverDir, "overwrite.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "updated" {
		t.Errorf("expected updated content %q, got %q", "updated", string(body))
	}
}

func TestMakeDir(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)
	serverDir := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(serverDir, 0o750); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/servers/"+testServerID+"/files/mkdir?path=newdirectory", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	if _, err := os.Stat(filepath.Join(serverDir, "newdirectory")); os.IsNotExist(err) {
		t.Error("expected newdirectory to exist")
	}
}

func TestMakeDirNested(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)
	serverDir := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(serverDir, 0o750); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/servers/"+testServerID+"/files/mkdir?path=parent/child/grandchild", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	if _, err := os.Stat(filepath.Join(serverDir, "parent", "child", "grandchild")); os.IsNotExist(err) {
		t.Error("expected nested directory to exist")
	}
}

func TestRemoveFile(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)
	serverDir := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(serverDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverDir, "deletable.txt"), []byte("delete me"), 0o640); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/servers/"+testServerID+"/files?path=deletable.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if _, err := os.Stat(filepath.Join(serverDir, "deletable.txt")); !os.IsNotExist(err) {
		t.Error("expected file to be deleted")
	}
}

func TestRemoveDirectory(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)
	serverDir := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(filepath.Join(serverDir, "emptydir"), 0o750); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/servers/"+testServerID+"/files?path=emptydir", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if _, err := os.Stat(filepath.Join(serverDir, "emptydir")); !os.IsNotExist(err) {
		t.Error("expected directory to be deleted")
	}
}

func TestRemoveFileNotFound(t *testing.T) {
	handler := newTestHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/servers/"+testServerID+"/files?path=nonexistent.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("expected non-200 for deleting nonexistent file")
	}
}

func TestDownloadFile(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)
	serverDir := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(serverDir, 0o750); err != nil {
		t.Fatal(err)
	}
	content := "downloadable binary content"
	if err := os.WriteFile(filepath.Join(serverDir, "download.bin"), []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/servers/"+testServerID+"/files/download?path=download.bin", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("expected octet-stream content type, got %s", rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != content {
		t.Errorf("expected body %q, got %q", content, rec.Body.String())
	}
}

func TestUploadChunkedFile(t *testing.T) {
	dataDir := t.TempDir()
	handler := newTestHandlerWithDir(t, dataDir)

	first := httptest.NewRequest(http.MethodPut, "/servers/"+testServerID+"/files/upload?path=chunked.txt&uploadId=test-upload&offset=0&final=false", strings.NewReader("hello "))
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first chunk 200, got %d", firstRec.Code)
	}

	second := httptest.NewRequest(http.MethodPut, "/servers/"+testServerID+"/files/upload?path=chunked.txt&uploadId=test-upload&offset=6&final=true", strings.NewReader("world"))
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected final chunk 200, got %d", secondRec.Code)
	}

	serverDir := filepath.Join(dataDir, testServerID)
	body, err := os.ReadFile(filepath.Join(serverDir, "chunked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(body))
	}
}

func TestFileAPIRootDeletionRejected(t *testing.T) {
	handler := newTestHandler(t)

	req := httptest.NewRequest(http.MethodDelete, "/servers/"+testServerID+"/files?path=", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for root deletion, got %d", rec.Code)
	}
}

func TestFileAPIPathTraversalRejected(t *testing.T) {
	handler := newTestHandler(t)

	tests := []string{
		"/servers/" + testServerID + "/files?path=../",
		"/servers/" + testServerID + "/files?path=../../../etc/passwd",
		"/servers/" + testServerID + "/files/content?path=../outside.txt",
		"/servers/" + testServerID + "/files/download?path=../secrets",
	}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for path traversal, got %d", rec.Code)
			}
		})
	}
}
