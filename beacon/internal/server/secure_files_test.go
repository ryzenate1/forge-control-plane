package server

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gamepanel/beacon/internal/rootfs"
)

func TestArchiveTraversalIsRejectedBeforeLiveExtraction(t *testing.T) {
	root := t.TempDir()
	fsys, err := rootfs.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()
	if err := fsys.WriteFile("existing.txt", []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}

	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	good, err := writer.Create("new.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = good.Write([]byte("new"))
	bad, err := writer.Create("../escape.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = bad.Write([]byte("escape"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fsys.WriteFile("malicious.zip", archive.Bytes(), 0o640); err != nil {
		t.Fatal(err)
	}

	if _, err := extractArchive(fsys, "malicious.zip", "", NewServerManager(nil), testServerID); err == nil {
		t.Fatal("expected traversal archive to be rejected")
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("archive partially modified live tree: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "existing.txt"))
	if err != nil || string(body) != "original" {
		t.Fatalf("existing file changed: body=%q err=%v", body, err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("archive escaped root: %v", err)
	}
}

func TestArchiveRejectsLinksAndFileParentConflicts(t *testing.T) {
	makeReader := func(entries []struct {
		name string
		mode os.FileMode
	}) *zip.Reader {
		var body bytes.Buffer
		writer := zip.NewWriter(&body)
		for _, entry := range entries {
			header := &zip.FileHeader{Name: entry.name}
			header.SetMode(entry.mode)
			file, err := writer.CreateHeader(header)
			if err != nil {
				t.Fatal(err)
			}
			if entry.mode.IsRegular() {
				_, _ = file.Write([]byte("x"))
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		reader, err := zip.NewReader(bytes.NewReader(body.Bytes()), int64(body.Len()))
		if err != nil {
			t.Fatal(err)
		}
		return reader
	}

	limits := archiveLimits{bytes: 1024, entries: 10}
	if _, err := validateZip(makeReader([]struct {
		name string
		mode os.FileMode
	}{{"link", os.ModeSymlink | 0o777}}), limits); err == nil {
		t.Fatal("expected symlink entry rejection")
	}
	if _, err := validateZip(makeReader([]struct {
		name string
		mode os.FileMode
	}{{"parent/child", 0o640}, {"parent", 0o640}}), limits); err == nil {
		t.Fatal("expected file-parent conflict rejection")
	}
}

func TestSecurePullRejectsPrivateResolutionAndRedirect(t *testing.T) {
	privateLookup := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	initial, _ := url.Parse("http://private.test/file")
	if _, err := securePullClientWithLookup(context.Background(), initial, privateLookup); err == nil {
		t.Fatal("expected private initial resolution to be rejected")
	}

	lookup := func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "redirect.test" {
			return []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	public, _ := url.Parse("https://public.test/file")
	client, err := securePullClientWithLookup(context.Background(), public, lookup)
	if err != nil {
		t.Fatal(err)
	}
	redirect, _ := http.NewRequest(http.MethodGet, "http://redirect.test/secret", nil)
	if err := client.CheckRedirect(redirect, []*http.Request{{URL: public}}); err == nil {
		t.Fatal("expected redirect to private resolution to be rejected")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestRemotePullOversizeLeavesNoDestination(t *testing.T) {
	t.Setenv("DAEMON_PULL_MAX_BYTES", "4")
	dataDir := t.TempDir()
	server, handler := NewServer(nil, dataDir)
	server.pullClientFactory = func(context.Context, *url.URL) (*http.Client, error) {
		return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Status:        "200 OK",
				ContentLength: -1,
				Body:          io.NopCloser(strings.NewReader("12345")),
				Header:        make(http.Header),
				Request:       request,
			}, nil
		})}, nil
	}

	request := httptest.NewRequest(http.MethodPost, "/servers/"+testServerID+"/files/pull", strings.NewReader(`{"url":"https://public.test/file","target":"downloads","fileName":"file.bin"}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", recorder.Code, recorder.Body.String())
	}
	target := filepath.Join(dataDir, testServerID, "downloads", "file.bin")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("oversize pull left destination behind: %v", err)
	}
}

func TestPermissionModeValidation(t *testing.T) {
	for _, mode := range []string{"644", "0644", "750", "0750"} {
		if !validPermissionMode(mode) {
			t.Fatalf("expected %q to be valid", mode)
		}
	}
	for _, mode := range []string{"", "64", "00000", "08", "u+rwx", "-644"} {
		if validPermissionMode(mode) {
			t.Fatalf("expected %q to be invalid", mode)
		}
	}
}

func TestDirectDownloadStreamsRegularFile(t *testing.T) {
	dataDir := t.TempDir()
	_, handler := NewServer(nil, dataDir)
	serverRoot := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(serverRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverRoot, "game data.bin"), []byte("binary-data"), 0o640); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/servers/"+testServerID+"/files/download?path=game%20data.bin", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "binary-data" {
		t.Fatalf("unexpected download response: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Type") != "application/octet-stream" || recorder.Header().Get("Content-Length") != "11" || !strings.Contains(recorder.Header().Get("Content-Disposition"), "game data.bin") {
		t.Fatalf("unexpected download headers: %v", recorder.Header())
	}
}

func TestCopyRejectsExistingDestination(t *testing.T) {
	dataDir := t.TempDir()
	_, handler := NewServer(nil, dataDir)
	serverRoot := filepath.Join(dataDir, testServerID)
	if err := os.MkdirAll(serverRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverRoot, "source.txt"), []byte("source"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serverRoot, "destination.txt"), []byte("keep"), 0o640); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/servers/"+testServerID+"/files/copy", strings.NewReader(`{"from":"source.txt","to":"destination.txt"}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", recorder.Code, recorder.Body.String())
	}
	body, err := os.ReadFile(filepath.Join(serverRoot, "destination.txt"))
	if err != nil || string(body) != "keep" {
		t.Fatalf("destination was replaced: body=%q err=%v", body, err)
	}
}
