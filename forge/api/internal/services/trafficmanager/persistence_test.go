package trafficmanager

import (
	"context"
	"sync"
	"testing"

	"gamepanel/forge/internal/store"
)

func TestCreateRoutingRulePersistsToDB(t *testing.T) {
	ctx := context.Background()
	ruleStore := newMockRoutingStore()
	svc := NewWithPersistence(nil, nil, ruleStore, nil, nil)

	rule := &RoutingRule{
		Name:       "persist-test",
		Domain:     "persist.example.com",
		TargetPort: 8080,
		Protocol:   "http",
	}
	if err := svc.CreateRoutingRule(ctx, rule); err != nil {
		t.Fatalf("CreateRoutingRule: %v", err)
	}
	if rule.ID == "" {
		t.Fatal("expected rule ID to be set")
	}

	persisted, err := ruleStore.GetRoutingRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("store.GetRoutingRule: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected rule to be persisted in store")
	}
	if persisted.Name != "persist-test" {
		t.Errorf("expected persisted name 'persist-test', got %s", persisted.Name)
	}
	if persisted.Domain != "persist.example.com" {
		t.Errorf("expected persisted domain 'persist.example.com', got %s", persisted.Domain)
	}
	if persisted.TargetPort != 8080 {
		t.Errorf("expected persisted targetPort 8080, got %d", persisted.TargetPort)
	}
}

func TestCreateRoutingRuleDBFailure(t *testing.T) {
	ctx := context.Background()
	failingStore := &failingRoutingStore{}
	svc := NewWithPersistence(nil, nil, failingStore, nil, nil)

	rule := &RoutingRule{
		Name:       "fail-test",
		Domain:     "fail.example.com",
		TargetPort: 9090,
		Protocol:   "http",
	}
	err := svc.CreateRoutingRule(ctx, rule)
	if err == nil {
		t.Fatal("expected error from failing store")
	}

	_, err = svc.GetRoutingRule(ctx, rule.ID)
	if err == nil {
		t.Error("expected rule NOT to be in local cache after DB failure")
	}
}

func TestInitFromDBLoadsAllRules(t *testing.T) {
	ctx := context.Background()
	ruleStore := newMockRoutingStore()

	preExisting := store.RoutingRuleRow{
		ID: "pre-1", Name: "pre-existing", Domain: "pre.example.com",
		TargetPort: 80, Protocol: "http",
	}
	if err := ruleStore.CreateRoutingRule(ctx, preExisting); err != nil {
		t.Fatalf("create pre-existing rule: %v", err)
	}

	svc := NewWithPersistence(nil, nil, ruleStore, nil, nil)
	if err := svc.InitFromDB(ctx); err != nil {
		t.Fatalf("InitFromDB: %v", err)
	}

	rules, err := svc.ListRoutingRules(ctx)
	if err != nil {
		t.Fatalf("ListRoutingRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule loaded from DB, got %d", len(rules))
	}
	if rules[0].ID != "pre-1" {
		t.Errorf("expected rule ID pre-1, got %s", rules[0].ID)
	}
}

func TestRestartLoadsRulesFromDB(t *testing.T) {
	ctx := context.Background()
	ruleStore := newMockRoutingStore()

	svc1 := NewWithPersistence(nil, nil, ruleStore, nil, nil)
	rule := &RoutingRule{
		Name: "survivor", Domain: "survivor.example.com", TargetPort: 3000, Protocol: "https",
	}
	if err := svc1.CreateRoutingRule(ctx, rule); err != nil {
		t.Fatalf("CreateRoutingRule: %v", err)
	}

	svc2 := NewWithPersistence(nil, nil, ruleStore, nil, nil)
	if err := svc2.InitFromDB(ctx); err != nil {
		t.Fatalf("InitFromDB: %v", err)
	}

	loaded, err := svc2.GetRoutingRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("GetRoutingRule after restart: %v", err)
	}
	if loaded.Name != "survivor" {
		t.Errorf("expected name 'survivor', got %s", loaded.Name)
	}

	rules, err := svc2.ListRoutingRules(ctx)
	if err != nil {
		t.Fatalf("ListRoutingRules: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("expected 1 rule after restart, got %d", len(rules))
	}
}

func TestMultipleRulesPersistence(t *testing.T) {
	ctx := context.Background()
	ruleStore := newMockRoutingStore()
	svc := NewWithPersistence(nil, nil, ruleStore, nil, nil)

	names := []string{"alpha", "beta", "gamma"}
	for _, name := range names {
		rule := &RoutingRule{
			Name: name, Domain: name + ".example.com", TargetPort: 8080,
		}
		if err := svc.CreateRoutingRule(ctx, rule); err != nil {
			t.Fatalf("CreateRoutingRule %s: %v", name, err)
		}
	}

	persisted, err := ruleStore.ListRoutingRules(ctx)
	if err != nil {
		t.Fatalf("ListRoutingRules: %v", err)
	}
	if len(persisted) != 3 {
		t.Errorf("expected 3 persisted rules, got %d", len(persisted))
	}
}

func TestUpdateRoutingRulePersistsToDB(t *testing.T) {
	ctx := context.Background()
	ruleStore := newMockRoutingStore()
	svc := NewWithPersistence(nil, nil, ruleStore, nil, nil)

	rule := &RoutingRule{
		Name: "updatable", Domain: "updatable.example.com", TargetPort: 8080,
	}
	if err := svc.CreateRoutingRule(ctx, rule); err != nil {
		t.Fatalf("CreateRoutingRule: %v", err)
	}

	rule.Name = "updated"
	rule.TargetPort = 9090
	if err := svc.UpdateRoutingRule(ctx, rule); err != nil {
		t.Fatalf("UpdateRoutingRule: %v", err)
	}

	persisted, err := ruleStore.GetRoutingRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("store.GetRoutingRule: %v", err)
	}
	if persisted.Name != "updated" {
		t.Errorf("expected name 'updated', got %s", persisted.Name)
	}
	if persisted.TargetPort != 9090 {
		t.Errorf("expected targetPort 9090, got %d", persisted.TargetPort)
	}
}

func TestDeleteRoutingRuleRemovesFromDB(t *testing.T) {
	ctx := context.Background()
	ruleStore := newMockRoutingStore()
	svc := NewWithPersistence(nil, nil, ruleStore, nil, nil)

	rule := &RoutingRule{
		ID: "delete-me", Name: "deletable", Domain: "del.example.com", TargetPort: 80,
	}
	if err := svc.CreateRoutingRule(ctx, rule); err != nil {
		t.Fatalf("CreateRoutingRule: %v", err)
	}

	if err := svc.DeleteRoutingRule(ctx, rule.ID); err != nil {
		t.Fatalf("DeleteRoutingRule: %v", err)
	}

	persisted, err := ruleStore.GetRoutingRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("store.GetRoutingRule: %v", err)
	}
	if persisted != nil {
		t.Error("expected rule to be removed from store")
	}
}

type failingRoutingStore struct {
	mu sync.Mutex
}

func (f *failingRoutingStore) CreateRoutingRule(_ context.Context, _ store.RoutingRuleRow) error {
	return errMockStoreFail
}

func (f *failingRoutingStore) UpdateRoutingRule(_ context.Context, _ store.RoutingRuleRow) error {
	return errMockStoreFail
}

func (f *failingRoutingStore) DeleteRoutingRule(_ context.Context, _ string) error {
	return errMockStoreFail
}

func (f *failingRoutingStore) GetRoutingRule(_ context.Context, _ string) (*store.RoutingRuleRow, error) {
	return nil, nil
}

func (f *failingRoutingStore) ListRoutingRules(_ context.Context) ([]store.RoutingRuleRow, error) {
	return nil, nil
}

func (f *failingRoutingStore) ListRoutingRulesForServer(_ context.Context, _ string) ([]store.RoutingRuleRow, error) {
	return nil, nil
}

var errMockStoreFail = &mockStoreError{}

type mockStoreError struct{}

func (m *mockStoreError) Error() string { return "mock store failure" }
