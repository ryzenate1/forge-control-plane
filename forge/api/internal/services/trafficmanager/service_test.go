package trafficmanager

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"gamepanel/forge/internal/store"
)

type mockRoutingStore struct {
	mu    sync.Mutex
	rules map[string]store.RoutingRuleRow
}

func newMockRoutingStore() *mockRoutingStore {
	return &mockRoutingStore{rules: make(map[string]store.RoutingRuleRow)}
}

func (m *mockRoutingStore) CreateRoutingRule(ctx context.Context, rule store.RoutingRuleRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules[rule.ID] = rule
	return nil
}

func (m *mockRoutingStore) UpdateRoutingRule(ctx context.Context, rule store.RoutingRuleRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rules[rule.ID]; !ok {
		return errors.New("rule not found")
	}
	m.rules[rule.ID] = rule
	return nil
}

func (m *mockRoutingStore) DeleteRoutingRule(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rules[id]; !ok {
		return errors.New("rule not found")
	}
	delete(m.rules, id)
	return nil
}

func (m *mockRoutingStore) GetRoutingRule(ctx context.Context, id string) (*store.RoutingRuleRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rules[id]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (m *mockRoutingStore) ListRoutingRules(ctx context.Context) ([]store.RoutingRuleRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rules := make([]store.RoutingRuleRow, 0, len(m.rules))
	for _, r := range m.rules {
		rules = append(rules, r)
	}
	return rules, nil
}

func (m *mockRoutingStore) ListRoutingRulesForServer(ctx context.Context, serverID string) ([]store.RoutingRuleRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []store.RoutingRuleRow
	for _, r := range m.rules {
		if r.ServerID == serverID {
			result = append(result, r)
		}
	}
	return result, nil
}

func TestCreateReadUpdateDeleteWithPersistence(t *testing.T) {
	ctx := context.Background()
	ruleStore := newMockRoutingStore()
	svc := NewWithPersistence(nil, nil, ruleStore, nil, nil)
	defer svc.Start(ctx)

	// Create a rule
	rule := &RoutingRule{
		Name:       "test-rule",
		Domain:     "example.com",
		TargetPort: 8080,
		Protocol:   "http",
	}
	if err := svc.CreateRoutingRule(ctx, rule); err != nil {
		t.Fatalf("CreateRoutingRule: %v", err)
	}
	if rule.ID == "" {
		t.Fatal("expected rule ID to be set")
	}

	// Verify it's in the in-memory map
	got, err := svc.GetRoutingRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("GetRoutingRule: %v", err)
	}
	if got.Name != "test-rule" {
		t.Errorf("expected name test-rule, got %s", got.Name)
	}

	// Verify it's persisted in the mock store
	persisted, err := ruleStore.GetRoutingRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("store.GetRoutingRule: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected rule to be persisted in store")
	}
	if persisted.Name != "test-rule" {
		t.Errorf("expected persisted name test-rule, got %s", persisted.Name)
	}

	// Update the rule
	rule.Name = "updated-rule"
	if err := svc.UpdateRoutingRule(ctx, rule); err != nil {
		t.Fatalf("UpdateRoutingRule: %v", err)
	}

	// Verify update in memory
	got, err = svc.GetRoutingRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("GetRoutingRule after update: %v", err)
	}
	if got.Name != "updated-rule" {
		t.Errorf("expected name updated-rule, got %s", got.Name)
	}

	// Verify update persisted
	persisted, err = ruleStore.GetRoutingRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("store.GetRoutingRule after update: %v", err)
	}
	if persisted.Name != "updated-rule" {
		t.Errorf("expected persisted name updated-rule, got %s", persisted.Name)
	}

	// Delete the rule
	if err := svc.DeleteRoutingRule(ctx, rule.ID); err != nil {
		t.Fatalf("DeleteRoutingRule: %v", err)
	}

	// Verify deleted from memory
	_, err = svc.GetRoutingRule(ctx, rule.ID)
	if err == nil {
		t.Error("expected error after delete")
	}

	// Verify deleted from store
	persisted, err = ruleStore.GetRoutingRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("store.GetRoutingRule after delete: %v", err)
	}
	if persisted != nil {
		t.Error("expected rule to be deleted from store")
	}
}

func TestRestartSurvival(t *testing.T) {
	ctx := context.Background()
	ruleStore := newMockRoutingStore()

	// Simulate first service instance
	svc1 := NewWithPersistence(nil, nil, ruleStore, nil, nil)

	rule := &RoutingRule{
		Name:       "survivor-rule",
		Domain:     "survivor.example.com",
		TargetPort: 9090,
		Protocol:   "https",
		Enabled:    true,
	}
	if err := svc1.CreateRoutingRule(ctx, rule); err != nil {
		t.Fatalf("svc1 CreateRoutingRule: %v", err)
	}

	// Simulate service restart by creating a fresh service and loading from DB
	svc2 := NewWithPersistence(nil, nil, ruleStore, nil, nil)
	if err := svc2.InitFromDB(ctx); err != nil {
		t.Fatalf("svc2 InitFromDB: %v", err)
	}

	// Verify the rule survived the restart
	loaded, err := svc2.GetRoutingRule(ctx, rule.ID)
	if err != nil {
		t.Fatalf("svc2 GetRoutingRule: %v", err)
	}
	if loaded.Name != "survivor-rule" {
		t.Errorf("expected name survivor-rule after restart, got %s", loaded.Name)
	}
	if loaded.Domain != "survivor.example.com" {
		t.Errorf("expected domain survivor.example.com, got %s", loaded.Domain)
	}
	if loaded.TargetPort != 9090 {
		t.Errorf("expected targetPort 9090, got %d", loaded.TargetPort)
	}

	// List rules and verify count
	rules, err := svc2.ListRoutingRules(ctx)
	if err != nil {
		t.Fatalf("svc2 ListRoutingRules: %v", err)
	}
	if len(rules) != 1 {
		t.Errorf("expected 1 rule after restart, got %d", len(rules))
	}
}

func TestConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	ruleStore := newMockRoutingStore()
	svc := NewWithPersistence(nil, nil, ruleStore, nil, nil)

	const numRules = 50
	errs := make(chan error, numRules*2)

	// Concurrent creates
	for i := 0; i < numRules; i++ {
		go func(idx int) {
			rule := &RoutingRule{
				Name:       fmt.Sprintf("concurrent-rule-%d", idx),
				Domain:     fmt.Sprintf("example-%d.com", idx),
				TargetPort: 8000 + idx,
			}
			if err := svc.CreateRoutingRule(ctx, rule); err != nil {
				errs <- err
			}
			errs <- nil
		}(i)
	}

	for i := 0; i < numRules; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent create: %v", err)
		}
	}

	// Verify all were created
	rules, err := svc.ListRoutingRules(ctx)
	if err != nil {
		t.Fatalf("ListRoutingRules: %v", err)
	}
	if len(rules) != numRules {
		t.Errorf("expected %d rules, got %d", numRules, len(rules))
	}

	// Verify all persisted
	persisted, err := ruleStore.ListRoutingRules(ctx)
	if err != nil {
		t.Fatalf("store.ListRoutingRules: %v", err)
	}
	if len(persisted) != numRules {
		t.Errorf("expected %d rules in store, got %d", numRules, len(persisted))
	}
}

func TestInitFromDB_EmptyTable(t *testing.T) {
	ctx := context.Background()
	ruleStore := newMockRoutingStore()
	svc := NewWithPersistence(nil, nil, ruleStore, nil, nil)

	if err := svc.InitFromDB(ctx); err != nil {
		t.Fatalf("InitFromDB on empty store: %v", err)
	}

	rules, err := svc.ListRoutingRules(ctx)
	if err != nil {
		t.Fatalf("ListRoutingRules: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

func TestInitFromDB_NoStore(t *testing.T) {
	ctx := context.Background()
	svc := New(nil, nil, nil)

	if err := svc.InitFromDB(ctx); err != nil {
		t.Fatalf("InitFromDB with nil store: %v", err)
	}
}

func TestCreateRoutingRule_Validation(t *testing.T) {
	ctx := context.Background()
	svc := New(nil, nil, nil)

	tests := []struct {
		name string
		rule *RoutingRule
	}{
		{"nil rule", nil},
		{"empty domain", &RoutingRule{Domain: "", TargetPort: 80}},
		{"zero target port", &RoutingRule{Domain: "example.com", TargetPort: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := svc.CreateRoutingRule(ctx, tt.rule); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestDeleteRoutingRule_NotFound(t *testing.T) {
	ctx := context.Background()
	svc := New(nil, nil, nil)

	if err := svc.DeleteRoutingRule(ctx, "nonexistent"); err == nil {
		t.Error("expected error for nonexistent rule")
	}
}

func TestCreateRoutingRule_Defaults(t *testing.T) {
	ctx := context.Background()
	svc := New(nil, nil, nil)

	rule := &RoutingRule{
		Domain:     "defaults-test.com",
		TargetPort: 3000,
	}
	if err := svc.CreateRoutingRule(ctx, rule); err != nil {
		t.Fatalf("CreateRoutingRule: %v", err)
	}
	if rule.Strategy != "round_robin" {
		t.Errorf("expected default strategy round_robin, got %s", rule.Strategy)
	}
	if rule.Path != "/" {
		t.Errorf("expected default path /, got %s", rule.Path)
	}
	if rule.ID == "" {
		t.Error("expected ID to be generated")
	}
	if rule.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestListRoutingRulesByServer(t *testing.T) {
	ctx := context.Background()
	svc := New(nil, nil, nil)

	serverID := "server-1"
	rule1 := &RoutingRule{
		ID: "r1", Name: "rule1", Domain: "a.com", TargetPort: 80,
		ServerID: serverID,
	}
	rule2 := &RoutingRule{
		ID: "r2", Name: "rule2", Domain: "b.com", TargetPort: 80,
		ServerID: serverID,
	}
	rule3 := &RoutingRule{
		ID: "r3", Name: "rule3", Domain: "c.com", TargetPort: 80,
		ServerID: "server-2",
	}

	for _, r := range []*RoutingRule{rule1, rule2, rule3} {
		if err := svc.CreateRoutingRule(ctx, r); err != nil {
			t.Fatalf("CreateRoutingRule: %v", err)
		}
	}

	rules, err := svc.ListRoutingRulesByServer(ctx, serverID)
	if err != nil {
		t.Fatalf("ListRoutingRulesByServer: %v", err)
	}
	if len(rules) != 2 {
		t.Errorf("expected 2 rules for server-1, got %d", len(rules))
	}
}

func TestWithdrawnRulesBasic(t *testing.T) {
	ctx := context.Background()
	svc := NewWithPersistence(nil, nil, newMockRoutingStore(), nil, nil)

	rule := &RoutingRule{
		ID: "wdr1", Name: "withdrawn", Domain: "wdr.com",
		TargetPort: 80, ServerID: "srv-1",
	}
	if err := svc.CreateRoutingRule(ctx, rule); err != nil {
		t.Fatalf("CreateRoutingRule: %v", err)
	}

	// Move to withdrawn (simulating WithdrawNodeTargets without nodeStore)
	svc.mu.Lock()
	svc.withdrawnRules[rule.ID] = svc.rules[rule.ID]
	delete(svc.rules, rule.ID)
	svc.mu.Unlock()

	// Delete should work (deletes from withdrawn)
	if err := svc.DeleteRoutingRule(ctx, rule.ID); err != nil {
		t.Fatalf("DeleteRoutingRule after withdraw: %v", err)
	}
}
