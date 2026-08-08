package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func migrationTestStore(t *testing.T, before043 bool) *Store {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "eggs_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = admin.Exec(cleanupCtx, `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
	})
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	s := &Store{db: pool, secrets: newTestKeyring()}
	if !before043 {
		if err := s.RunMigrations(ctx, "../../migrations"); err != nil {
			t.Fatal(err)
		}
		return s
	}

	entries, err := os.ReadDir("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") && entry.Name() < "043_unify_eggs_templates_mounts.sql" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join("../../migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range splitSQLStatements(string(body)) {
			if _, err := pool.Exec(ctx, statement); err != nil {
				t.Fatalf("apply %s: %v", name, err)
			}
		}
	}
	return s
}

func applyMigration043(t *testing.T, s *Store) {
	t.Helper()
	body, err := os.ReadFile("../../migrations/043_unify_eggs_templates_mounts.sql")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, statement := range splitSQLStatements(string(body)) {
		if _, err := s.db.Exec(ctx, statement); err != nil {
			t.Fatalf("apply migration 043: %v\nstatement: %s", err, statement)
		}
	}
}

func TestMigration043BackfillsLegacyTemplatesAndPreservesServers(t *testing.T) {
	s := migrationTestStore(t, true)
	ctx := context.Background()
	ownerID, nodeID, templateID, serverID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	variableID, mountID := uuid.NewString(), uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, ownerID, ownerID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO nodes (id, name, region, base_url, token_hash) VALUES ($1, 'legacy node', 'test', 'http://daemon.test', 'hash')`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO server_templates (id, name, image, startup_command, default_memory_mb, install_script, config_json)
		VALUES ($1, 'Legacy Game', 'legacy/game:1', './game {{PORT}}', 1536, 'install legacy', '{"stop":"quit"}')
	`, templateID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO servers (id, node_id, owner_id, template_id, name, memory_mb, cpu_shares, disk_mb) VALUES ($1, $2, $3, $4, 'legacy server', 1536, 512, 2048)`, serverID, nodeID, ownerID, templateID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO egg_variables (id, egg_id, name, env_variable, default_value, rules) VALUES ($1, $2, 'Port', 'PORT', '25565', 'required|string|max:5')`, variableID, templateID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO mounts (id, uuid, name, source, target) VALUES ($1, $2, 'legacy mount', '/srv/legacy', '/data')`, mountID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO egg_mount (mount_id, egg_id) VALUES ($1, $2)`, mountID, templateID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `CREATE TABLE server_mounts (mount_id UUID NOT NULL, server_id UUID NOT NULL, PRIMARY KEY (mount_id, server_id))`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO server_mounts VALUES ($1, $2)`, mountID, serverID); err != nil {
		t.Fatal(err)
	}

	applyMigration043(t, s)

	var eggID, serverEggID, compatibilityID, image, variableEggID string
	var defaultMemory int
	if err := s.db.QueryRow(ctx, `SELECT id::text, docker_images->>'legacy/game:1', default_memory_mb FROM eggs WHERE id = $1`, templateID).Scan(&eggID, &image, &defaultMemory); err != nil {
		t.Fatal(err)
	}
	if eggID != templateID || image != "legacy/game:1" || defaultMemory != 1536 {
		t.Fatalf("legacy egg backfill mismatch: id=%s image=%s memory=%d", eggID, image, defaultMemory)
	}
	if err := s.db.QueryRow(ctx, `SELECT egg_id::text, template_id::text FROM servers WHERE id = $1`, serverID).Scan(&serverEggID, &compatibilityID); err != nil {
		t.Fatal(err)
	}
	if serverEggID != templateID || compatibilityID != templateID {
		t.Fatalf("server identifiers not preserved: egg=%s template=%s", serverEggID, compatibilityID)
	}
	if err := s.db.QueryRow(ctx, `SELECT egg_id::text FROM egg_variables WHERE id = $1`, variableID).Scan(&variableEggID); err != nil {
		t.Fatal(err)
	}
	if variableEggID != templateID {
		t.Fatalf("variable egg = %s, want %s", variableEggID, templateID)
	}
	var mountAssignments int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM mount_server WHERE mount_id = $1 AND server_id = $2`, mountID, serverID).Scan(&mountAssignments); err != nil {
		t.Fatal(err)
	}
	if mountAssignments != 1 {
		t.Fatalf("mount assignments = %d, want 1", mountAssignments)
	}
}

func TestCanonicalEggProvisioningVariablesAndMounts(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()
	ownerID, nodeID, allocationID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, ownerID, ownerID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO nodes (id, name, region, base_url, token_hash, daemon_token_id, daemon_token) VALUES ($1, 'node', 'test', 'http://daemon.test', 'hash', 'token-id', 'token-secret')`, nodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO allocations (id, node_id, ip, port) VALUES ($1, $2, '127.0.0.1', 25565)`, allocationID, nodeID); err != nil {
		t.Fatal(err)
	}
	var nestID string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM nests WHERE name = 'Games'`).Scan(&nestID); err != nil {
		t.Fatal(err)
	}
	images, _ := json.Marshal(map[string]string{"Game": "example/game:2"})
	egg, err := s.CreateEgg(ctx, CreateEggRequest{
		NestID: nestID, Name: "Canonical Game", DockerImages: images,
		Startup: "./game --port {{PORT}} --secret {{SECRET}}", Config: json.RawMessage(`{"stop":"quit","startup":{"done":["Ready"]}}`),
		DefaultMemoryMB: 1536, InstallScript: "install canonical", InstallContainer: "alpine:3.21",
		InstallEntrypoint: "sh", FileDenylist: json.RawMessage(`["/proc"]`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	compatibilityTemplate, err := s.CreateTemplate(ctx, CreateTemplateRequest{Name: "Compatibility Alias", Image: "example/alias:1", StartupCommand: "./alias", DefaultMemoryMB: 768}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetEgg(ctx, compatibilityTemplate.ID); err != nil {
		t.Fatalf("template alias did not create a canonical egg: %v", err)
	}
	var legacyTemplateRows int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM server_templates WHERE id = $1`, compatibilityTemplate.ID).Scan(&legacyTemplateRows); err != nil {
		t.Fatal(err)
	}
	if legacyTemplateRows != 0 {
		t.Fatalf("template alias created %d legacy rows", legacyTemplateRows)
	}
	if _, err := s.CreateEggVariable(ctx, egg.ID, EggVariableRequest{Name: "Port", EnvVariable: "PORT", DefaultValue: "25565", UserViewable: true, UserEditable: true, Rules: "required|string|max:5"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateEggVariable(ctx, egg.ID, EggVariableRequest{Name: "Secret", EnvVariable: "SECRET", DefaultValue: "hidden", UserViewable: false, UserEditable: false, Rules: "required|string"}, nil); err != nil {
		t.Fatal(err)
	}
	server, err := s.CreateServer(ctx, CreateServerRequest{
		Name: "canonical server", NodeID: nodeID, OwnerID: ownerID, TemplateID: egg.ID, AllocationID: allocationID,
		MemoryMB: 0, CPUShares: 512, DiskMB: 2048, IOWeight: 500, StartupVariables: map[string]string{"PORT": "25566"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if server.MemoryMB != 1536 {
		t.Fatalf("server memory = %d, want egg default 1536", server.MemoryMB)
	}
	mount, err := s.CreateMount(ctx, CreateMountRequest{Name: "canonical mount", Source: "/srv/game", Target: "/data", NodeIDs: []string{nodeID}, TemplateIDs: []string{egg.ID}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AssignMountToServer(ctx, server.ID, mount.ID, nil); err != nil {
		t.Fatal(err)
	}
	attachedServers, err := s.ServerMountsForMount(ctx, mount.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attachedServers) != 1 || attachedServers[0].ID != server.ID {
		t.Fatalf("attached servers = %#v, want server %s", attachedServers, server.ID)
	}
	ineligibleMount, err := s.CreateMount(ctx, CreateMountRequest{Name: "ineligible mount", Source: "/srv/ineligible", Target: "/ineligible", TemplateIDs: []string{egg.ID}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AttachServerToMount(ctx, ineligibleMount.ID, server.ID); err == nil {
		t.Fatal("AttachServerToMount allowed a mount unavailable on the server node")
	}
	target, err := s.ServerProvisionTarget(ctx, server.ID)
	if err != nil {
		t.Fatal(err)
	}
	if target.EggID != egg.ID || target.Image != "example/game:2" || target.InstallScript != "install canonical" {
		t.Fatalf("canonical provisioning mismatch: %+v", target)
	}
	if target.Environment["PORT"] != "25566" || target.Environment["SECRET"] != "hidden" {
		t.Fatalf("resolved environment = %#v", target.Environment)
	}
	if target.StartupCommand != "./game --port 25566 --secret hidden" {
		t.Fatalf("startup = %q", target.StartupCommand)
	}
	if len(target.Mounts) != 1 || target.Mounts[0].Target != "/data" {
		t.Fatalf("mounts = %#v", target.Mounts)
	}
}
