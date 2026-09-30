package store

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestServerTransferRunTokenNotSerialized(t *testing.T) {
	token := "bearer-secret-token-123"
	srv := Server{
		ID:               "srv-1",
		Name:             "test-server",
		Status:           "running",
		DesiredState:     ServerDesiredState("running"),
		ActualState:      ServerActualState("running"),
		TransferRunToken: &token,
		TransferState:    "running",
		MemoryMB:         1024,
		CPUShares:        1024,
		DiskMB:           10240,
	}
	// Direct Server JSON must never contain transferRunToken due to json:"-"
	data, err := json.Marshal(srv)
	if err != nil {
		t.Fatalf("marshal server: %v", err)
	}
	if strings.Contains(string(data), "transferRunToken") {
		t.Fatalf("Server JSON leaked transferRunToken: %s", string(data))
	}
	if strings.Contains(string(data), token) {
		t.Fatalf("Server JSON leaked token value: %s", string(data))
	}
	// DTO must also not contain token
	dto := srv.ToDTO()
	dtoData, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal dto: %v", err)
	}
	if strings.Contains(string(dtoData), "transferRunToken") {
		t.Fatalf("ServerDTO JSON leaked transferRunToken: %s", string(dtoData))
	}
	if strings.Contains(string(dtoData), token) {
		t.Fatalf("ServerDTO JSON leaked token value: %s", string(dtoData))
	}
	// Sanitize helpers must clear token
	sanitized := SanitizeServer(srv)
	if sanitized.TransferRunToken != nil {
		t.Fatalf("SanitizeServer did not clear token")
	}
	list := []Server{srv, srv}
	sanitizedList := SanitizeServers(list)
	for _, s := range sanitizedList {
		if s.TransferRunToken != nil {
			t.Fatalf("SanitizeServers did not clear token")
		}
	}
	dtoList := ServersToDTO(list)
	dtoListData, err := json.Marshal(dtoList)
	if err != nil {
		t.Fatalf("marshal dto list: %v", err)
	}
	if strings.Contains(string(dtoListData), "transferRunToken") || strings.Contains(string(dtoListData), token) {
		t.Fatalf("ServersToDTO list leaked token: %s", string(dtoListData))
	}
}

func TestServerDTONeverIncludesBearerEvenWhenRunning(t *testing.T) {
	// Admin sees same DTO (no token) even when transfer_state is running/pending.
	// This enforces that bearer credentials are never exposed to any user.
	cases := []string{"running", "pending", "queued", "idle", "completed"}
	for _, state := range cases {
		token := "secret-" + state
		srv := Server{ID: "id", Name: "n", TransferState: state, TransferRunToken: &token}
		dto := srv.ToDTO()
		data, _ := json.Marshal(dto)
		if strings.Contains(string(data), "transferRunToken") || strings.Contains(string(data), token) {
			t.Fatalf("state %q DTO leaked token: %s", state, string(data))
		}
	}
}

func findProjectFile(relative string) string {
	// Walk up from cwd to find project root containing the file.
	cwd, _ := os.Getwd()
	dir := cwd
	for i := 0; i < 10; i++ {
		candidate := dir + "/" + relative
		// Also try going up
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		// Try absolute from project guess: look for go.work
		if _, err := os.Stat(dir + "/go.work"); err == nil {
			if _, err := os.Stat(dir + "/" + relative); err == nil {
				return dir + "/" + relative
			}
		}
		parent := dir + "/.."
		abs, _ := os.Stat(parent)
		if abs == nil {
			break
		}
		dir = parent
		// Clean path
		if dir == "/" {
			break
		}
	}
	// Fallback to known absolute for this environment
	fallbacks := []string{
		"/Users/riyaz/project/gamepanel/" + relative,
	}
	for _, f := range fallbacks {
		if _, err := os.Stat(f); err == nil {
			return f
		}
	}
	return ""
}

func TestSharedTypesDoesNotExposeTransferRunToken(t *testing.T) {
	// Verify frontend shared-types do not expose transferRunToken
	src := findProjectFile("packages/shared-types/src/api.ts")
	if src == "" {
		t.Fatalf("could not locate packages/shared-types/src/api.ts from cwd")
	}
	content, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if strings.Contains(string(content), "transferRunToken") {
		t.Fatalf("shared-types %s still exposes transferRunToken", src)
	}
	dist := findProjectFile("packages/shared-types/dist/api.d.ts")
	if dist != "" {
		if data, err := os.ReadFile(dist); err == nil {
			if strings.Contains(string(data), "transferRunToken") {
				t.Fatalf("shared-types dist %s still exposes transferRunToken", dist)
			}
		}
	}
}

func TestFrontendWebAPIDoesNotExposeToken(t *testing.T) {
	// Ensure forge/web lib api does not reference transferRunToken as bearer leak
	candidates := []string{
		"forge/web/lib/api/servers.ts",
		"forge/web/lib/api.ts",
	}
	for _, rel := range candidates {
		path := findProjectFile(rel)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(data), "transferRunToken") {
			t.Fatalf("frontend file %s still references transferRunToken", path)
		}
	}
}
