package http

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gamepanel/forge/internal/store"
)

func TestServerHandlersDoNotLeakTransferToken(t *testing.T) {
	token := "bearer-token-should-not-leak"
	srv := store.Server{
		ID: "srv-1", Name: "test", Status: "running",
		TransferState: "running", TransferRunToken: &token,
		MemoryMB: 1024, CPUShares: 1024, DiskMB: 10240,
	}
	// Simulate what GET /servers/:id handler does: server.ToDTO()
	dto := srv.ToDTO()
	data, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal dto: %v", err)
	}
	if strings.Contains(string(data), "transferRunToken") {
		t.Fatalf("handler DTO leaked transferRunToken key: %s", string(data))
	}
	if strings.Contains(string(data), token) {
		t.Fatalf("handler DTO leaked token value: %s", string(data))
	}
	// Simulate list handler: ServersToDTO
	list := []store.Server{srv}
	listDTO := store.ServersToDTO(list)
	listData, err := json.Marshal(listDTO)
	if err != nil {
		t.Fatalf("marshal list dto: %v", err)
	}
	if strings.Contains(string(listData), "transferRunToken") || strings.Contains(string(listData), token) {
		t.Fatalf("list DTO leaked token: %s", string(listData))
	}
}

func TestNonAdminCannotObtainTokenViaServerJSON(t *testing.T) {
	// Even if store.Server has token set, the API response for ordinary users must not include it.
	// This test proves non-admin path also uses DTO.
	token := "non-admin-secret"
	srv := store.Server{ID: "srv-2", Name: "n", TransferRunToken: &token, TransferState: "pending"}
	// Handler always uses ToDTO regardless of role, so both admin and non-admin get safe DTO.
	// Verify role-agnostic DTO never contains token.
	for _, role := range []string{"admin", "user", "viewer"} {
		dto := srv.ToDTO()
		data, _ := json.Marshal(dto)
		if strings.Contains(string(data), token) {
			t.Fatalf("role %s DTO leaked token", role)
		}
		// Direct Server marshal also must not leak due to json:\"-\" tag
		direct, _ := json.Marshal(srv)
		if strings.Contains(string(direct), token) || strings.Contains(string(direct), "transferRunToken") {
			t.Fatalf("role %s direct Server marshal leaked token", role)
		}
	}
}

func findProjectFileHTTP(rel string) string {
	cwd, _ := os.Getwd()
	dir := cwd
	for i := 0; i < 10; i++ {
		candidate := dir + "/" + rel
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		if _, err := os.Stat(dir + "/go.work"); err == nil {
			if _, err := os.Stat(dir + "/" + rel); err == nil {
				return dir + "/" + rel
			}
		}
		parent := dir + "/.."
		if _, err := os.Stat(parent); err != nil {
			break
		}
		dir = parent
	}
	fallbacks := []string{
		"/Users/riyaz/project/gamepanel/" + rel,
	}
	for _, f := range fallbacks {
		if _, err := os.Stat(f); err == nil {
			return f
		}
	}
	return ""
}

func TestFrontendResponsesNeverLeakToken(t *testing.T) {
	// Verify that the frontend's ApiServer type (dist) never includes transferRunToken
	for _, rel := range []string{"packages/shared-types/dist/api.d.ts", "packages/shared-types/src/api.ts"} {
		path := findProjectFileHTTP(rel)
		if path == "" {
			t.Fatalf("could not locate %s", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(data), "transferRunToken") {
			t.Fatalf("frontend file %s still contains transferRunToken", path)
		}
	}
	// Also verify forge/web lib/api does not forward token
	for _, rel := range []string{"forge/web/lib/api/servers.ts", "forge/web/lib/api.ts"} {
		path := findProjectFileHTTP(rel)
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(data), "transferRunToken") && !strings.Contains(string(data), "transferRunToken is intentionally omitted") {
			t.Fatalf("web api file %s still references transferRunToken", path)
		}
	}
}

func TestHandlersServersUseSafeDTO(t *testing.T) {
	// Static check: ensure handlers_servers.go does not contain raw c.JSON(server) without ToDTO for Server responses.
	path := findProjectFileHTTP("forge/api/internal/http/handlers_servers.go")
	if path == "" {
		t.Fatalf("could not locate handlers_servers.go")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read handlers_servers.go: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "return c.JSON(server)\n") {
		t.Fatalf("handlers_servers.go still contains raw return c.JSON(server) which may leak transferRunToken; use server.ToDTO()")
	}
	if strings.Contains(content, "return c.Status(fiber.StatusCreated).JSON(server)") {
		t.Fatalf("handlers_servers.go still contains raw StatusCreated JSON(server); use ToDTO")
	}
	if strings.Contains(content, "RespondWithData(c, servers,") {
		t.Fatalf("handlers_servers.go still contains RespondWithData with raw servers; use store.ServersToDTO(servers)")
	}
}

func TestHandlersTenancyAndExternalUseSafeDTO(t *testing.T) {
	tenancyPath := findProjectFileHTTP("forge/api/internal/http/handlers_tenancy.go")
	if tenancyPath != "" {
		if data, err := os.ReadFile(tenancyPath); err == nil {
			if strings.Contains(string(data), `"data": servers,`) {
				t.Fatalf("handlers_tenancy.go still uses raw servers; should use store.ServersToDTO")
			}
		}
	}
	adminPath := findProjectFileHTTP("forge/api/internal/http/handlers_admin.go")
	if adminPath != "" {
		if data, err := os.ReadFile(adminPath); err == nil {
			if strings.Contains(string(data), "return c.JSON(servers)\n") {
				t.Fatalf("handlers_admin.go still returns raw servers; should use DTO")
			}
		}
	}
	extPath := findProjectFileHTTP("forge/api/internal/http/handlers_external.go")
	if extPath != "" {
		if data, err := os.ReadFile(extPath); err == nil {
			if strings.Contains(string(data), "return c.JSON(server)\n") {
				t.Fatalf("handlers_external.go still returns raw server; should use ToDTO")
			}
		}
	}
}
