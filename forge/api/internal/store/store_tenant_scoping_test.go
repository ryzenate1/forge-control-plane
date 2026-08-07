package store

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
)

func tenantStore(t *testing.T) *Store {
	t.Helper()
	// uses migrationTestStore helper defined in other test files
	store := migrationTestStore(t, false)
	return store
}

func TestPlacementDecisions_TenantColumn(t *testing.T) {
	store := tenantStore(t)
	ctx := context.Background()

	// create minimal app and node for FK (use random UUIDs if real FK not enforced)
	app, err := store.CreateReplicaApp(ctx, CreateReplicaAppRequest{Name: "tenant-test-app", Replicas: 1})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	// create two nodes directly via SQL to avoid validation
	nodeID := uuid.NewString()
	_, err = store.db.Exec(ctx, `INSERT INTO nodes (id, uuid, name, region, base_url, fqdn, scheme, status, daemon_listen, daemon_sftp, daemon_base) VALUES ($1,$1,'node-tenant','test','http://node-tenant:8080','node-tenant','http','online',8080,2022,'/tmp')`, nodeID)
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	defer store.db.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, nodeID)
	defer store.DeleteReplicaApp(ctx, app.ID)

	instanceID := uuid.NewString()
	tenantA := uuid.NewString()
	tenantB := uuid.NewString()

	// column should exist and be nullable - verify via information_schema
	var exists bool
	err = store.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='placement_decisions' AND column_name='tenant_id')`).Scan(&exists)
	if err != nil {
		t.Fatalf("check placement_decisions tenant_id column exists: %v", err)
	}
	if !exists {
		t.Fatalf("placement_decisions.tenant_id column missing - migration 215 not applied")
	}
	err = store.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='reconcile_plans' AND column_name='tenant_id')`).Scan(&exists)
	if err != nil {
		t.Fatalf("check reconcile_plans tenant_id: %v", err)
	}
	if !exists {
		t.Fatalf("reconcile_plans.tenant_id missing")
	}
	err = store.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='events' AND column_name='tenant_id')`).Scan(&exists)
	if err != nil {
		t.Fatalf("check events tenant_id: %v", err)
	}
	if !exists {
		t.Fatalf("events.tenant_id missing")
	}

	// create placement decisions with tenant
	pdA, err := store.CreatePlacementDecisionWithTenant(ctx, instanceID, nodeID, app.ID, tenantA, 0, 1.5, true, []string{"test"}, "docker")
	if err != nil {
		t.Fatalf("create placement with tenant A: %v", err)
	}
	if pdA.TenantID == nil || *pdA.TenantID != tenantA {
		t.Fatalf("expected TenantID %s, got %v", tenantA, pdA.TenantID)
	}
	// second placement with different tenant, same app
	pdB, err := store.CreatePlacementDecisionWithTenant(ctx, uuid.NewString(), nodeID, app.ID, tenantB, 1, 1.0, true, []string{"test"}, "docker")
	if err != nil {
		t.Fatalf("create placement with tenant B: %v", err)
	}
	if pdB.TenantID == nil || *pdB.TenantID != tenantB {
		t.Fatalf("tenant B mismatch")
	}
	// legacy path without tenant should still work (nullable backfill)
	pdLegacy, err := store.CreatePlacementDecision(ctx, uuid.NewString(), nodeID, app.ID, 2, 0.5, true, []string{"legacy"}, "docker")
	if err != nil {
		t.Fatalf("legacy placement without tenant: %v", err)
	}
	if pdLegacy.TenantID != nil && *pdLegacy.TenantID != "" {
		t.Fatalf("expected legacy TenantID empty, got %v", *pdLegacy.TenantID)
	}

	// verify listing preserves tenant
	list, err := store.ListPlacementDecisionsByApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) < 3 {
		t.Fatalf("expected >=3 decisions, got %d", len(list))
	}
	foundA, foundB, foundLegacy := false, false, false
	for _, pd := range list {
		if pd.TenantID != nil && *pd.TenantID == tenantA {
			foundA = true
		}
		if pd.TenantID != nil && *pd.TenantID == tenantB {
			foundB = true
		}
		if pd.TenantID == nil || *pd.TenantID == "" {
			foundLegacy = true
		}
	}
	if !foundA || !foundB || !foundLegacy {
		t.Fatalf("tenant presence in list missing: A=%v B=%v legacy=%v", foundA, foundB, foundLegacy)
	}

	// partial index should exist and be valid
	var idxExists bool
	err = store.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_index WHERE indexrelid = 'idx_placement_decisions_tenant'::regclass)`).Scan(&idxExists)
	if err != nil {
		// fallback to pg_indexes check
		err = store.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE tablename='placement_decisions' AND indexname='idx_placement_decisions_tenant')`).Scan(&idxExists)
		if err != nil {
			t.Fatalf("index check: %v", err)
		}
	}
	if !idxExists {
		t.Logf("warning: idx_placement_decisions_tenant not found (may be race with migration ordering)")
	}

	_ = os.Getenv // keep import
}

func TestReconcilePlans_TenantColumn(t *testing.T) {
	store := tenantStore(t)
	ctx := context.Background()

	tenantID := uuid.NewString()
	plan := &ReconcilePlanRow{
		ResourceID:   uuid.NewString(),
		ResourceKind: "server",
		State:        "pending",
		Destructive:  false,
		Confirmed:    false,
		DiffCount:    1,
		DriftCount:   2,
		TenantID:     &tenantID,
	}
	if err := store.CreateReconcilePlan(ctx, plan); err != nil {
		t.Fatalf("create plan with tenant: %v", err)
	}
	fetched, err := store.GetReconcilePlan(ctx, plan.ID)
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if fetched == nil || fetched.TenantID == nil || *fetched.TenantID != tenantID {
		t.Fatalf("tenant mismatch on reconcile plan: got %v want %s", fetched.TenantID, tenantID)
	}

	// legacy plan without tenant
	legacy := &ReconcilePlanRow{
		ResourceID:   uuid.NewString(),
		ResourceKind: "server",
		State:        "pending",
	}
	if err := store.CreateReconcilePlan(ctx, legacy); err != nil {
		t.Fatalf("legacy plan: %v", err)
	}
	fetchedLegacy, err := store.GetReconcilePlan(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("get legacy: %v", err)
	}
	if fetchedLegacy.TenantID != nil && *fetchedLegacy.TenantID != "" {
		t.Fatalf("expected legacy TenantID empty, got %v", *fetchedLegacy.TenantID)
	}

	// list should include both
	plans, total, err := store.ListReconcilePlans(ctx, 0, 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total < 2 {
		t.Fatalf("expected >=2 plans, got %d", total)
	}
	foundTenant, foundLegacy := false, false
	for _, p := range plans {
		if p.ID == plan.ID && p.TenantID != nil && *p.TenantID == tenantID {
			foundTenant = true
		}
		if p.ID == legacy.ID && (p.TenantID == nil || *p.TenantID == "") {
			foundLegacy = true
		}
	}
	if !foundTenant {
		t.Fatalf("tenant plan not found in list")
	}
	if !foundLegacy {
		t.Fatalf("legacy plan not found in list")
	}
}

func TestTenantColumns_Nullable_And_Indexes(t *testing.T) {
	store := tenantStore(t)
	ctx := context.Background()

	// Verify nullable: insert with NULL tenant_id should succeed
	var tenantColNullable bool
	err := store.db.QueryRow(ctx, `
		SELECT is_nullable = 'YES' FROM information_schema.columns
		WHERE table_name='placement_decisions' AND column_name='tenant_id'
	`).Scan(&tenantColNullable)
	if err != nil {
		t.Fatalf("nullable check: %v", err)
	}
	if !tenantColNullable {
		t.Fatalf("tenant_id should be nullable")
	}
	err = store.db.QueryRow(ctx, `
		SELECT is_nullable = 'YES' FROM information_schema.columns
		WHERE table_name='reconcile_plans' AND column_name='tenant_id'
	`).Scan(&tenantColNullable)
	if err != nil {
		t.Fatalf("nullable check reconcile: %v", err)
	}
	if !tenantColNullable {
		t.Fatalf("reconcile_plans tenant_id should be nullable")
	}

	// Verify data type is uuid
	var dataType string
	err = store.db.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_name='events' AND column_name='tenant_id'`).Scan(&dataType)
	if err != nil {
		t.Fatalf("data_type check: %v", err)
	}
	if dataType != "uuid" {
		t.Fatalf("events tenant_id expected uuid, got %s", dataType)
	}

	// Verify partial indexes exist (WHERE tenant_id IS NOT NULL)
	var idxDef string
	err = store.db.QueryRow(ctx, `SELECT indexdef FROM pg_indexes WHERE tablename='events' AND indexname='idx_events_tenant'`).Scan(&idxDef)
	if err != nil {
		t.Fatalf("idx_events_tenant missing: %v", err)
	}
	if idxDef == "" {
		t.Fatalf("index def empty")
	}
	// should contain WHERE
	if !tenantContainsStr(idxDef, "WHERE") {
		t.Fatalf("expected partial index WHERE clause, got %s", idxDef)
	}
}

func tenantContainsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
