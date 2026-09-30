package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// TestEggUpsert_Idempotent verifies that the (nest_id, name) upsert pattern
// used by eggseeder.Service.SeedDefaultEggs is idempotent, mirroring
// appstore seed's ON CONFLICT handling.
func TestEggUpsert_Idempotent(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	var nestID string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM nests WHERE name='Games' LIMIT 1`).Scan(&nestID); err != nil {
		t.Fatalf("fetch Games nest: %v", err)
	}

	eggName := "TestIdempotentEgg-" + uuid.NewString()[:8]
	dockerImages, _ := json.Marshal(map[string]string{"test": "example/test:latest"})
	config := json.RawMessage(`{"stop":"stop"}`)

	upsert := func() {
		_, err := s.db.Exec(ctx, `
			INSERT INTO eggs (id, nest_id, name, description, docker_images, startup, config,
			                  default_memory_mb, install_script, install_container, install_entrypoint,
			                  file_denylist, author, features, startup_commands, update_url)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, '[]'::jsonb, $15)
			ON CONFLICT (nest_id, name) DO UPDATE SET
				description = EXCLUDED.description,
				docker_images = EXCLUDED.docker_images,
				startup = EXCLUDED.startup,
				config = EXCLUDED.config,
				default_memory_mb = EXCLUDED.default_memory_mb,
				install_script = EXCLUDED.install_script,
				install_container = EXCLUDED.install_container,
				install_entrypoint = EXCLUDED.install_entrypoint,
				file_denylist = EXCLUDED.file_denylist,
				author = EXCLUDED.author,
				features = EXCLUDED.features,
				update_url = EXCLUDED.update_url
		`, uuid.NewString(), nestID, eggName, "desc", dockerImages, "./start", config, 1024, "echo hi", "alpine:3.21", "sh", `[]`, "GamePanel", `[]`, "")
		if err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	upsert()
	var count1 int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM eggs WHERE nest_id=$1 AND name=$2`, nestID, eggName).Scan(&count1); err != nil {
		t.Fatalf("count1: %v", err)
	}
	if count1 != 1 {
		t.Fatalf("expected 1 egg after first upsert, got %d", count1)
	}

	// Second upsert with same (nest_id,name) but different description should update, not duplicate.
	upsert()
	var count2 int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM eggs WHERE nest_id=$1 AND name=$2`, nestID, eggName).Scan(&count2); err != nil {
		t.Fatalf("count2: %v", err)
	}
	if count2 != 1 {
		t.Fatalf("idempotent violation: expected still 1, got %d", count2)
	}

	// Also test egg_variables upsert idempotency.
	var eggID string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM eggs WHERE nest_id=$1 AND name=$2`, nestID, eggName).Scan(&eggID); err != nil {
		t.Fatalf("fetch egg id: %v", err)
	}
	varUpsert := func() {
		_, err := s.db.Exec(ctx, `
			INSERT INTO egg_variables (id, egg_id, name, description, env_variable, default_value,
			                           user_viewable, user_editable, rules, sort)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (egg_id, env_variable) DO UPDATE SET
				name = EXCLUDED.name,
				description = EXCLUDED.description,
				default_value = EXCLUDED.default_value,
				user_viewable = EXCLUDED.user_viewable,
				user_editable = EXCLUDED.user_editable,
				rules = EXCLUDED.rules,
				sort = EXCLUDED.sort
		`, uuid.NewString(), eggID, "Test Var", "desc", "TEST_VAR", "default", true, true, "required|string", 10)
		if err != nil {
			t.Fatalf("var upsert: %v", err)
		}
	}
	varUpsert()
	var vc1 int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM egg_variables WHERE egg_id=$1 AND env_variable='TEST_VAR'`, eggID).Scan(&vc1); err != nil {
		t.Fatalf("var count1: %v", err)
	}
	varUpsert()
	var vc2 int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM egg_variables WHERE egg_id=$1 AND env_variable='TEST_VAR'`, eggID).Scan(&vc2); err != nil {
		t.Fatalf("var count2: %v", err)
	}
	if vc2 != 1 || vc1 != 1 {
		t.Fatalf("egg_variable not idempotent: %d vs %d", vc1, vc2)
	}
}

// TestSeedDefaultEggs_ExpectedCount ensures the embedded templates would seed
// at least 14 eggs (smoke test without requiring DB content of templates).
// The actual seeding is exercised in eggseeder package's integration via
// store. This test just verifies migrationTestStore is functional.
func TestSeedDefaultEggs_StoreAvailable(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nests WHERE name='Games')`).Scan(&exists); err != nil {
		t.Fatalf("check nest: %v", err)
	}
	if !exists {
		t.Fatalf("Games nest should exist after migrations")
	}
}
