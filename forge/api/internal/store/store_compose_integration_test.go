package store

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func composeTestStore(t *testing.T) *Store {
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
	schema := "compose_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	if err := s.RunMigrations(ctx, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return s
}

// A8 - Compose lifecycle: deploy/create/fetch

func TestA8_ComposeStackCRUD(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	userID := uuid.NewString()
	_, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, userID, userID+"@example.test")
	requireNoError(t, err)

	stack := &ComposeStack{
		ID:          "cps-" + uuid.NewString()[:12],
		UserID:      userID,
		Name:        "test-stack",
		NodeID:      uuid.NewString(),
		Status:      "deploying",
		ComposeYAML: "version: '3'\nservices:\n  web:\n    image: nginx:latest",
		ComposeHash: "abc123",
		EnvVars:     map[string]string{"ENV": "prod"},
		MemoryMB:    512,
		CPUShares:   100,
		DiskMB:      1024,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err = s.CreateComposeStack(ctx, stack)
	requireNoError(t, err)

	fetched, err := s.GetComposeStack(ctx, stack.ID)
	requireNoError(t, err)
	assertEqual(t, stack.ID, fetched.ID)
	assertEqual(t, stack.Name, fetched.Name)
	assertEqual(t, stack.UserID, fetched.UserID)
	assertEqual(t, stack.Status, fetched.Status)
	assertEqual(t, stack.ComposeYAML, fetched.ComposeYAML)
	assertEqual(t, stack.MemoryMB, fetched.MemoryMB)
	assertEqual(t, "prod", fetched.EnvVars["ENV"])

	stack.Status = "running"
	stack.MemoryMB = 1024
	err = s.UpdateComposeStack(ctx, stack)
	requireNoError(t, err)

	fetched, err = s.GetComposeStack(ctx, stack.ID)
	requireNoError(t, err)
	assertEqual(t, "running", fetched.Status)
	assertEqual(t, int64(1024), fetched.MemoryMB)
}

func TestA8_ComposeStackCreateWithDefaults(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	userID := uuid.NewString()
	_, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, userID, userID+"@example.test")
	requireNoError(t, err)

	stack := &ComposeStack{
		ID:          "cps-" + uuid.NewString()[:12],
		UserID:      userID,
		Name:        "defaults-stack",
		NodeID:      uuid.NewString(),
		ComposeYAML: "version: '3'",
	}
	stack.CreatedAt = time.Now().UTC()
	stack.UpdatedAt = stack.CreatedAt

	err = s.CreateComposeStack(ctx, stack)
	requireNoError(t, err)

	fetched, err := s.GetComposeStack(ctx, stack.ID)
	requireNoError(t, err)
	assertEqual(t, "deploying", fetched.Status)
	assertEqual(t, int64(0), fetched.MemoryMB)
	assertEqual(t, int64(0), fetched.CPUShares)
	assertEqual(t, int64(0), fetched.DiskMB)
	assertEqual(t, "", fetched.Error)
}

func TestA8_ComposeStackList(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	userID := uuid.NewString()
	otherUserID := uuid.NewString()
	_, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, userID, userID+"@example.test")
	requireNoError(t, err)
	_, err = s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, otherUserID, otherUserID+"@example.test")
	requireNoError(t, err)

	stack1 := &ComposeStack{
		ID:          "cps-" + uuid.NewString()[:12],
		UserID:      userID,
		Name:        "stack-1",
		NodeID:      uuid.NewString(),
		Status:      "running",
		ComposeYAML: "version: '3'",
		CreatedAt:   time.Now().UTC().Add(-2 * time.Hour),
		UpdatedAt:   time.Now().UTC(),
	}
	stack2 := &ComposeStack{
		ID:          "cps-" + uuid.NewString()[:12],
		UserID:      userID,
		Name:        "stack-2",
		NodeID:      uuid.NewString(),
		Status:      "stopped",
		ComposeYAML: "version: '3'",
		CreatedAt:   time.Now().UTC().Add(-1 * time.Hour),
		UpdatedAt:   time.Now().UTC(),
	}
	stack3 := &ComposeStack{
		ID:          "cps-" + uuid.NewString()[:12],
		UserID:      otherUserID,
		Name:        "stack-3",
		NodeID:      uuid.NewString(),
		Status:      "running",
		ComposeYAML: "version: '3'",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	for _, st := range []*ComposeStack{stack1, stack2, stack3} {
		err = s.CreateComposeStack(ctx, st)
		requireNoError(t, err)
	}

	userStacks, err := s.ListComposeStacks(ctx, userID)
	requireNoError(t, err)
	assertEqual(t, 2, len(userStacks))
	ids := map[string]bool{}
	for _, st := range userStacks {
		ids[st.ID] = true
	}
	assertTrue(t, ids[stack1.ID])
	assertTrue(t, ids[stack2.ID])

	allStacks, err := s.ListComposeStacks(ctx, "")
	requireNoError(t, err)
	assertEqual(t, 3, len(allStacks))
}

func TestA8_ComposeStackDelete(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	userID := uuid.NewString()
	_, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, userID, userID+"@example.test")
	requireNoError(t, err)

	stack := &ComposeStack{
		ID:          "cps-" + uuid.NewString()[:12],
		UserID:      userID,
		Name:        "delete-me",
		NodeID:      uuid.NewString(),
		Status:      "running",
		ComposeYAML: "version: '3'",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err = s.CreateComposeStack(ctx, stack)
	requireNoError(t, err)

	_, err = s.GetComposeStack(ctx, stack.ID)
	requireNoError(t, err)

	err = s.DeleteComposeStack(ctx, stack.ID)
	requireNoError(t, err)

	_, err = s.GetComposeStack(ctx, stack.ID)
	if err == nil {
		t.Fatal("expected stack to be deleted")
	}
}

func TestA8_ComposeStackUpdateReservationID(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	userID := uuid.NewString()
	_, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, userID, userID+"@example.test")
	requireNoError(t, err)

	stack := &ComposeStack{
		ID:          "cps-" + uuid.NewString()[:12],
		UserID:      userID,
		Name:        "reservation-stack",
		NodeID:      uuid.NewString(),
		Status:      "deploying",
		ComposeYAML: "version: '3'",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err = s.CreateComposeStack(ctx, stack)
	requireNoError(t, err)

	fetched, err := s.GetComposeStack(ctx, stack.ID)
	requireNoError(t, err)
	assertEqual(t, "", fetched.ReservationID)

	stack.ReservationID = "res-123456"
	err = s.UpdateComposeStack(ctx, stack)
	requireNoError(t, err)

	fetched, err = s.GetComposeStack(ctx, stack.ID)
	requireNoError(t, err)
	assertEqual(t, "res-123456", fetched.ReservationID)
}

func TestA8_ComposeStackErrorField(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	userID := uuid.NewString()
	_, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, userID, userID+"@example.test")
	requireNoError(t, err)

	stack := &ComposeStack{
		ID:          "cps-" + uuid.NewString()[:12],
		UserID:      userID,
		Name:        "error-stack",
		NodeID:      uuid.NewString(),
		Status:      "failed",
		Error:       "node connection refused",
		ComposeYAML: "version: '3'",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err = s.CreateComposeStack(ctx, stack)
	requireNoError(t, err)

	fetched, err := s.GetComposeStack(ctx, stack.ID)
	requireNoError(t, err)
	assertEqual(t, "failed", fetched.Status)
	assertEqual(t, "node connection refused", fetched.Error)
}

// A8 - ComposeStack lifecycle status transitions

func TestA8_ComposeStackStatusTransitions(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	userID := uuid.NewString()
	_, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, userID, userID+"@example.test")
	requireNoError(t, err)

	stack := &ComposeStack{
		ID:          "cps-" + uuid.NewString()[:12],
		UserID:      userID,
		Name:        "transitions-stack",
		NodeID:      uuid.NewString(),
		Status:      "deploying",
		ComposeYAML: "version: '3'",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err = s.CreateComposeStack(ctx, stack)
	requireNoError(t, err)

	statuses := []string{"running", "stopped", "updating", "running", "degraded", "stopped", "deleting", "deleted", "failed"}
	for _, status := range statuses {
		stack.Status = status
		stack.UpdatedAt = time.Now().UTC()
		err = s.UpdateComposeStack(ctx, stack)
		requireNoError(t, err)

		fetched, err := s.GetComposeStack(ctx, stack.ID)
		requireNoError(t, err)
		assertEqual(t, status, fetched.Status)
	}
}

func TestA8_ComposeStackListOrderedByCreatedAtDesc(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	userID := uuid.NewString()
	_, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, userID, userID+"@example.test")
	requireNoError(t, err)

	for i := 0; i < 5; i++ {
		stack := &ComposeStack{
			ID:          "cps-" + uuid.NewString()[:12],
			UserID:      userID,
			Name:        "stack-" + string(rune('A'+i)),
			NodeID:      uuid.NewString(),
			Status:      "running",
			ComposeYAML: "version: '3'",
			CreatedAt:   time.Now().UTC().Add(-time.Duration(5-i) * time.Minute),
			UpdatedAt:   time.Now().UTC(),
		}
		err = s.CreateComposeStack(ctx, stack)
		requireNoError(t, err)
	}

	stacks, err := s.ListComposeStacks(ctx, userID)
	requireNoError(t, err)
	assertEqual(t, 5, len(stacks))
	for i := 1; i < len(stacks); i++ {
		assertTrue(t, !stacks[i].CreatedAt.After(stacks[i-1].CreatedAt),
			"stacks should be ordered by created_at DESC")
	}
}

// Compose Project CRUD tests

func TestA8_ComposeProjectCRUD(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	p := &ProjectDocument{
		ID:             uuid.NewString(),
		Name:           "my-project",
		ComposeContent: "version: '3'\nservices:\n  app:\n    image: alpine",
		ParsedConfig:   []byte(`{"services":[{"name":"app","image":"alpine"}]}`),
		Status:         "imported",
		Revision:       1,
	}
	err := s.CreateComposeProject(ctx, p)
	requireNoError(t, err)

	fetched, err := s.GetComposeProject(ctx, p.ID)
	requireNoError(t, err)

	assertEqual(t, p.ID, fetched.ID)
	assertEqual(t, p.Name, fetched.Name)
	assertEqual(t, p.ComposeContent, fetched.ComposeContent)
	assertEqual(t, p.Status, fetched.Status)
	assertEqual(t, p.Revision, fetched.Revision)

	p.Status = "deployed"
	p.Revision = 2
	p.Name = "updated-project"
	err = s.UpdateComposeProject(ctx, p)
	requireNoError(t, err)

	fetched, err = s.GetComposeProject(ctx, p.ID)
	requireNoError(t, err)
	assertEqual(t, "updated-project", fetched.Name)
	assertEqual(t, "deployed", fetched.Status)
	assertEqual(t, 2, fetched.Revision)
}

func TestA8_ComposeProjectList(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	p1 := &ProjectDocument{
		ID:             uuid.NewString(),
		Name:           "project-1",
		ComposeContent: "version: '3'\nservices:\n  a:\n    image: a",
		Status:         "imported",
		Revision:       1,
	}
	p2 := &ProjectDocument{
		ID:             uuid.NewString(),
		Name:           "project-2",
		ComposeContent: "version: '3'\nservices:\n  b:\n    image: b",
		Status:         "deployed",
		Revision:       1,
	}
	for _, p := range []*ProjectDocument{p1, p2} {
		err := s.CreateComposeProject(ctx, p)
		requireNoError(t, err)
	}

	projects, err := s.ListComposeProjects(ctx)
	requireNoError(t, err)
	assertEqual(t, 2, len(projects))
}

func TestA8_ComposeProjectDelete(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	p := &ProjectDocument{
		ID:             uuid.NewString(),
		Name:           "delete-project",
		ComposeContent: "version: '3'",
		ParsedConfig:   []byte(`{}`),
		Status:         "imported",
		Revision:       1,
	}
	err := s.CreateComposeProject(ctx, p)
	requireNoError(t, err)

	_, err = s.GetComposeProject(ctx, p.ID)
	requireNoError(t, err)

	err = s.DeleteComposeProject(ctx, p.ID)
	requireNoError(t, err)

	_, err = s.GetComposeProject(ctx, p.ID)
	if err == nil {
		t.Fatal("expected project to be deleted")
	}
}

func TestA8_ComposeProjectWithServerID(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	serverID := uuid.NewString()
	p := &ProjectDocument{
		ID:             uuid.NewString(),
		Name:           "server-project",
		ServerID:       serverID,
		ComposeContent: "version: '3'",
		ParsedConfig:   []byte(`{}`),
		Status:         "imported",
		Revision:       1,
	}
	err := s.CreateComposeProject(ctx, p)
	requireNoError(t, err)

	fetched, err := s.GetComposeProject(ctx, p.ID)
	requireNoError(t, err)
	assertEqual(t, serverID, fetched.ServerID)

	p.ServerID = ""
	err = s.UpdateComposeProject(ctx, p)
	requireNoError(t, err)

	fetched, err = s.GetComposeProject(ctx, p.ID)
	requireNoError(t, err)
	assertEqual(t, "", fetched.ServerID)
}

func TestA8_ComposeProjectRevisionIncrement(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	p := &ProjectDocument{
		ID:             uuid.NewString(),
		Name:           "revision-project",
		ComposeContent: "version: '3'\nservices:\n  app:\n    image: alpine:1",
		ParsedConfig:   []byte(`{}`),
		Status:         "imported",
		Revision:       1,
	}
	err := s.CreateComposeProject(ctx, p)
	requireNoError(t, err)

	for rev := 2; rev <= 5; rev++ {
		p.Revision = rev
		p.ComposeContent = "version: '3'\nservices:\n  app:\n    image: alpine:" + string(rune('0'+rev))
		err = s.UpdateComposeProject(ctx, p)
		requireNoError(t, err)

		fetched, err := s.GetComposeProject(ctx, p.ID)
		requireNoError(t, err)
		assertEqual(t, rev, fetched.Revision)
	}
}

func TestA8_ComposeProjectRequiresID(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	p := &ProjectDocument{
		Name:           "no-id-project",
		ComposeContent: "version: '3'",
	}
	err := s.CreateComposeProject(ctx, p)
	if err == nil {
		t.Fatal("expected error when creating project without ID")
	}
}

func TestA8_ComposeStackUpsertBehavior(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	userID := uuid.NewString()
	_, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, userID, userID+"@example.test")
	requireNoError(t, err)

	stack := &ComposeStack{
		ID:          "cps-" + uuid.NewString()[:12],
		UserID:      userID,
		Name:        "upsert-stack",
		NodeID:      uuid.NewString(),
		Status:      "deploying",
		ComposeYAML: "version: '3'\nservices:\n  a:\n    image: a",
		ComposeHash: "hash-v1",
		MemoryMB:    128,
		CPUShares:   50,
		DiskMB:      256,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err = s.CreateComposeStack(ctx, stack)
	requireNoError(t, err)

	stack.ID = "cps-" + uuid.NewString()[:12]
	err = s.CreateComposeStack(ctx, stack)
	requireNoError(t, err)

	stacks, err := s.ListComposeStacks(ctx, userID)
	requireNoError(t, err)
	assertEqual(t, 2, len(stacks))
}

func TestA8_ComposeStackEmptyEnvVars(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	userID := uuid.NewString()
	_, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, userID, userID+"@example.test")
	requireNoError(t, err)

	stack := &ComposeStack{
		ID:          "cps-" + uuid.NewString()[:12],
		UserID:      userID,
		Name:        "no-env-stack",
		NodeID:      uuid.NewString(),
		Status:      "running",
		ComposeYAML: "version: '3'",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err = s.CreateComposeStack(ctx, stack)
	requireNoError(t, err)

	fetched, err := s.GetComposeStack(ctx, stack.ID)
	requireNoError(t, err)
	assertNotNil(t, fetched.EnvVars)
	assertEqual(t, 0, len(fetched.EnvVars))
}

func TestA8_ComposeStackLargeYAML(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	userID := uuid.NewString()
	_, err := s.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, userID, userID+"@example.test")
	requireNoError(t, err)

	var sb strings.Builder
	sb.WriteString("version: '3.9'\nservices:\n")
	for i := 0; i < 20; i++ {
		sb.WriteString("  svc")
		sb.WriteString(string(rune('A' + i%26)))
		sb.WriteString(":\n    image: test/svc:latest\n    environment:\n      KEY: value\n    ports:\n      - \"")
		sb.WriteString(string(rune('0' + i)))
		sb.WriteString(":80\"\n")
	}

	stack := &ComposeStack{
		ID:          "cps-" + uuid.NewString()[:12],
		UserID:      userID,
		Name:        "large-stack",
		NodeID:      uuid.NewString(),
		Status:      "imported",
		ComposeYAML: sb.String(),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err = s.CreateComposeStack(ctx, stack)
	requireNoError(t, err)

	fetched, err := s.GetComposeStack(ctx, stack.ID)
	requireNoError(t, err)
	assertEqual(t, stack.ComposeYAML, fetched.ComposeYAML)
}

func TestA8_ComposeStackDBIsolation(t *testing.T) {
	s1 := composeTestStore(t)
	ctx := context.Background()

	userID := uuid.NewString()
	_, err := s1.db.Exec(ctx, `INSERT INTO users (id, email, password_hash, role) VALUES ($1, $2, 'hash', 'admin')`, userID, userID+"@example.test")
	requireNoError(t, err)

	stack := &ComposeStack{
		ID:          "cps-" + uuid.NewString()[:12],
		UserID:      userID,
		Name:        "isolated-stack",
		NodeID:      uuid.NewString(),
		Status:      "running",
		ComposeYAML: "version: '3'",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	err = s1.CreateComposeStack(ctx, stack)
	requireNoError(t, err)

	s2 := composeTestStore(t)
	_, err = s2.GetComposeStack(ctx, stack.ID)
	if err == nil {
		t.Fatal("expected stack not found in second isolated database schema")
	}
}

// Smoke test utilities

const smokeComposeTemplate = `
version: "3.8"
name: smoke-test
services:
  web:
    image: nginx:alpine
    ports:
      - "80:80"
    environment:
      APP_ENV: production
    restart: unless-stopped
`

func TestSmoke_ComposeProjectLifecycle(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	projectID := uuid.NewString()
	p := &ProjectDocument{
		ID:             projectID,
		Name:           "smoke-project",
		ComposeContent: smokeComposeTemplate,
		ParsedConfig:   json.RawMessage(`{"version":"3.8","name":"smoke-test","services":[{"name":"web","image":"nginx:alpine","ports":["80:80"],"environment":{"APP_ENV":"production"},"restart":"unless-stopped"}]}`),
		Status:         "imported",
		Revision:       1,
	}

	err := s.CreateComposeProject(ctx, p)
	requireNoError(t, err)

	fetched, err := s.GetComposeProject(ctx, projectID)
	requireNoError(t, err)
	assertEqual(t, "imported", fetched.Status)

	p.Status = "deployed"
	p.Revision = 2
	err = s.UpdateComposeProject(ctx, p)
	requireNoError(t, err)

	fetched, err = s.GetComposeProject(ctx, projectID)
	requireNoError(t, err)
	assertEqual(t, "deployed", fetched.Status)
	assertEqual(t, 2, fetched.Revision)

	assertTrue(t, strings.Contains(string(fetched.ParsedConfig), "nginx:alpine"))

	err = s.DeleteComposeProject(ctx, projectID)
	requireNoError(t, err)

	_, err = s.GetComposeProject(ctx, projectID)
	if err == nil {
		t.Fatal("expected project to be deleted after lifecycle")
	}
}

func TestSmoke_MultiComposeProjectImport(t *testing.T) {
	s := composeTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		p := &ProjectDocument{
			ID:   uuid.NewString(),
			Name: "multi-project-" + string(rune('A'+i)),
			ComposeContent: "version: '3'\nservices:\n  svc" + string(rune('A'+i)) + ":\n    image: test:" + string(rune('A'+i)),
			Status:   "imported",
			Revision: 1,
		}
		err := s.CreateComposeProject(ctx, p)
		requireNoError(t, err)
	}

	projects, err := s.ListComposeProjects(ctx)
	requireNoError(t, err)
	assertEqual(t, 5, len(projects))
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertEqual[T comparable](t *testing.T, expected, actual T) {
	t.Helper()
	if expected != actual {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
}

func assertTrue(t *testing.T, cond bool, msg ...string) {
	t.Helper()
	if !cond {
		if len(msg) > 0 {
			t.Fatal(msg[0])
		}
		t.Fatal("expected condition to be true")
	}
}

func assertNotNil(t *testing.T, v interface{}) {
	t.Helper()
	if v == nil {
		t.Fatal("expected non-nil value")
	}
}
