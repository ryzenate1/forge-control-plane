package sftpserver

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"gamepanel/beacon/internal/system"
)

func TestAuthenticateCallsPanelRemoteSFTPAuth(t *testing.T) {
	var sawAuth string
	var sawUsername string
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/remote/sftp/auth" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		sawAuth = r.Header.Get("Authorization")
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
			IP       string `json:"ip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		sawUsername = body.Username
		if body.Password != "secret" || body.IP == "" {
			t.Fatalf("unexpected auth payload: %+v", body)
		}
		_ = json.NewEncoder(w).Encode(AuthResult{
			UserID:      "user-1",
			ServerID:    "123e4567-e89b-12d3-a456-426614174000",
			Permissions: []string{"file.read", "file.read-content"},
		})
	}))
	defer panel.Close()

	server := &Server{PanelAPIURL: panel.URL + "/api/v1", NodeToken: "node-token", HTTPClient: panel.Client()}
	result, err := server.authenticate("admin@example.com.server-1", "secret", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if sawAuth != "Bearer node-token" {
		t.Fatalf("expected bearer token, got %q", sawAuth)
	}
	if sawUsername != "admin@example.com.server-1" {
		t.Fatalf("unexpected username %q", sawUsername)
	}
	if result.ServerID != "123e4567-e89b-12d3-a456-426614174000" || len(result.Permissions) != 2 {
		t.Fatalf("unexpected auth result: %+v", result)
	}
}

func TestHandlerRejectsPathEscape(t *testing.T) {
	handler := &handler{root: t.TempDir(), permissions: []string{"*"}}
	if _, err := handler.safePath("../outside"); err == nil {
		t.Fatal("expected path escape to be rejected")
	}
	if _, err := handler.safePath("/absolute"); err == nil {
		t.Fatal("expected absolute path to be rejected")
	}
}

func TestHandlerEnforcesReadPermission(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "server.properties"), []byte("online-mode=true"), 0o640); err != nil {
		t.Fatal(err)
	}
	handler := &handler{root: root, permissions: []string{"file.read"}}
	_, err := handler.Fileread(&sftp.Request{Filepath: "server.properties"})
	if err == nil {
		t.Fatal("expected read-content permission to be required")
	}

	handler.permissions = []string{"file.read-content"}
	reader, err := handler.Fileread(&sftp.Request{Filepath: "server.properties"})
	if err != nil {
		t.Fatal(err)
	}
	if closer, ok := reader.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	buf := make([]byte, 16)
	n, err := reader.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if string(buf[:n]) != "online-mode=true" {
		t.Fatalf("unexpected file content %q", string(buf[:n]))
	}
}

func TestHandlerEnforcesWritePermissions(t *testing.T) {
	root := t.TempDir()
	handler := &handler{root: root, permissions: []string{"file.update"}}
	if _, err := handler.Filewrite(&sftp.Request{Filepath: "new.txt"}); err == nil {
		t.Fatal("expected create permission for new files")
	}

	handler.permissions = []string{"file.create"}
	writer, err := handler.Filewrite(&sftp.Request{Filepath: "new.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteAt([]byte("hello"), 0); err != nil {
		t.Fatal(err)
	}
	if closer, ok := writer.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
	body, err := os.ReadFile(filepath.Join(root, "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello" {
		t.Fatalf("unexpected file body %q", string(body))
	}
}

func TestHandlerReadOnlyAndConcurrentQuota(t *testing.T) {
	root := t.TempDir()
	lock := &sync.Mutex{}
	readOnly := &handler{root: root, permissions: []string{"*"}, readOnly: true, writeLock: lock}
	if _, err := readOnly.Filewrite(&sftp.Request{Filepath: "denied.txt"}); err != sftp.ErrSSHFxPermissionDenied {
		t.Fatalf("read-only write returned %v", err)
	}

	limited := &handler{root: root, permissions: []string{"*"}, quotaBytes: 8, writeLock: lock}
	first, err := limited.Filewrite(&sftp.Request{Filepath: "first.txt"})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		writer, err := limited.Filewrite(&sftp.Request{Filepath: "second.txt"})
		if err == nil {
			_, err = writer.WriteAt([]byte("x"), 0)
			_ = writer.(io.Closer).Close()
		}
		result <- err
	}()
	select {
	case <-result:
		t.Fatal("second quota reservation did not wait for active atomic writer")
	case <-time.After(50 * time.Millisecond):
	}
	if _, err := first.WriteAt([]byte("12345678"), 0); err != nil {
		t.Fatal(err)
	}
	if err := first.(io.Closer).Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("concurrent writer bypassed quota")
		}
	case <-time.After(time.Second):
		t.Fatal("concurrent writer remained blocked")
	}
}

func TestHandlerWritesAtomicallyAndRejectsSymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "existing.txt"), []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	handler := &handler{root: root, permissions: []string{"*"}}
	writer, err := handler.Filewrite(&sftp.Request{Filepath: "existing.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteAt([]byte("new"), 0); err != nil {
		t.Fatal(err)
	}
	beforeClose, err := os.ReadFile(filepath.Join(root, "existing.txt"))
	if err != nil || string(beforeClose) != "old" {
		t.Fatalf("live file changed before close: body=%q err=%v", beforeClose, err)
	}
	if closer, ok := writer.(interface{ Close() error }); !ok {
		t.Fatal("atomic SFTP writer is not closable")
	} else if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	afterClose, err := os.ReadFile(filepath.Join(root, "existing.txt"))
	if err != nil || string(afterClose) != "new" {
		t.Fatalf("live file was not committed on close: body=%q err=%v", afterClose, err)
	}

	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skip(err)
	}
	if _, err := handler.Filewrite(&sftp.Request{Filepath: "escape/file.txt"}); err == nil {
		t.Fatal("expected SFTP write through symlink parent to fail")
	}
	if _, err := os.Stat(filepath.Join(outside, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("SFTP write escaped root: %v", err)
	}
}

func TestHandlerSetstatIsExplicitlyUnsupported(t *testing.T) {
	handler := &handler{root: t.TempDir(), permissions: []string{"*"}}
	if err := handler.Filecmd(&sftp.Request{Method: "Setstat", Filepath: "file.txt"}); err != sftp.ErrSSHFxOpUnsupported {
		t.Fatalf("expected unsupported Setstat, got %v", err)
	}
}

func TestLoadOrCreateHostKeyPersistsKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".sftp", "id_ed25519")
	first, err := loadOrCreateHostKey(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateHostKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.PublicKey().Type() != second.PublicKey().Type() {
		t.Fatal("expected persisted host key to reload")
	}
}

type testRegistry struct {
	mu     sync.Mutex
	closer io.Closer
}

func (r *testRegistry) TrackSession(_, _ string, closer io.Closer) func() {
	r.mu.Lock()
	r.closer = closer
	r.mu.Unlock()
	return func() {}
}
func (r *testRegistry) deauthorize() {
	r.mu.Lock()
	closer := r.closer
	r.mu.Unlock()
	if closer != nil {
		_ = closer.Close()
	}
}

func startTestSFTP(t *testing.T, suspended *atomic.Bool, publicKey string, registry SessionRegistry, activity *system.ActivityDedup, idle time.Duration) (*Server, context.CancelFunc) {
	t.Helper()
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["type"] == "public_key" && body["publicKey"] != publicKey {
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		if body["type"] == "password" && body["password"] != "secret" {
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(AuthResult{UserID: "user-1", ServerID: "123e4567-e89b-12d3-a456-426614174000", Permissions: []string{"*"}, DiskLimitMB: 1, Suspended: suspended.Load()})
	}))
	t.Cleanup(panel.Close)
	ctx, cancel := context.WithCancel(context.Background())
	server := &Server{Addr: "127.0.0.1:0", DataDir: t.TempDir(), PanelAPIURL: panel.URL, NodeToken: "token", HTTPClient: panel.Client(), IdleTimeout: idle, Sessions: registry, Activity: activity}
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	deadline := time.Now().Add(3 * time.Second)
	for server.Address() == "127.0.0.1:0" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if server.Address() == "127.0.0.1:0" {
		cancel()
		t.Fatal("SFTP server did not start")
	}
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("SFTP shutdown: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("SFTP server did not shut down")
		}
	})
	return server, cancel
}

func dialTestSFTP(t *testing.T, addr string, auth ssh.AuthMethod) (*ssh.Client, *sftp.Client) {
	t.Helper()
	client, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{User: "owner@example.com.123e4567-e89b-12d3-a456-426614174000", Auth: []ssh.AuthMethod{auth}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		t.Fatal(err)
	}
	return client, sftpClient
}

func TestInProcessSFTPPasswordQuotaActivitySetstatAndDeauthorization(t *testing.T) {
	var suspended atomic.Bool
	registry := &testRegistry{}
	activities := make(chan system.ActivityEntry, 10)
	dedup := system.NewActivityDedup(time.Hour, 100, func(_ string, entries []system.ActivityEntry) {
		for _, entry := range entries {
			activities <- entry
		}
	})
	server, _ := startTestSFTP(t, &suspended, "", registry, dedup, time.Minute)
	sshClient, client := dialTestSFTP(t, server.Address(), ssh.Password("secret"))
	defer sshClient.Close()
	defer client.Close()
	file, err := client.Create("hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Chmod("hello.txt", 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	mtime := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := client.Chtimes("hello.txt", mtime, mtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	large, err := client.Create("too-large.bin")
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := large.Write(make([]byte, 1024*1024+1))
	closeErr := large.Close()
	if writeErr == nil && closeErr == nil {
		t.Fatal("quota-exceeding write succeeded")
	}
	dedup.Flush()
	select {
	case entry := <-activities:
		if entry.User != "user-1" || entry.Client == "" || entry.SessionID == "" {
			t.Fatalf("unsafe/incomplete activity metadata: %+v", entry)
		}
	case <-time.After(time.Second):
		t.Fatal("missing activity")
	}
	registry.deauthorize()
	time.Sleep(50 * time.Millisecond)
	_, err = client.Stat("hello.txt")
	if err == nil {
		t.Fatal("deauthorization did not close SFTP session")
	}
}

func TestInProcessSFTPPublicKeySuspensionAndIdleTimeout(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	authorized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	_ = public
	var suspended atomic.Bool
	server, _ := startTestSFTP(t, &suspended, authorized, nil, nil, 100*time.Millisecond)
	sshClient, client := dialTestSFTP(t, server.Address(), ssh.PublicKeys(signer))
	defer sshClient.Close()
	defer client.Close()
	time.Sleep(250 * time.Millisecond)
	if _, err := client.Stat("."); err == nil {
		t.Fatal("idle session remained active")
	}

	suspended.Store(false)
	second, _ := startTestSFTP(t, &suspended, authorized, nil, nil, time.Minute)
	raw, err := ssh.Dial("tcp", second.Address(), &ssh.ClientConfig{User: "owner@example.com.123e4567-e89b-12d3-a456-426614174000", Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	suspended.Store(true)
	_, err = sftp.NewClient(raw)
	if err == nil {
		t.Fatal("suspension recheck allowed SFTP subsystem")
	}
	if !errors.Is(err, io.EOF) && !strings.Contains(strings.ToLower(err.Error()), "eof") {
		t.Logf("suspension rejection: %v", err)
	}
}
