package operations

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type testOp struct {
	msg string
}

func (t *testOp) Execute(ctx context.Context, serverDir string) error {
	return nil
}

func TestRegisterAndGetFactory(t *testing.T) {
	factory := func(args json.RawMessage) (Operation, error) {
		return &testOp{msg: "hello"}, nil
	}
	Register("test_op", factory)
	defer delete(registry, "test_op")

	got, ok := GetFactory("test_op")
	if !ok {
		t.Fatal("expected factory to be found")
	}
	op, err := got(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := op.(*testOp); !ok {
		t.Fatal("expected testOp type")
	}
}

func TestGetFactoryUnknown(t *testing.T) {
	_, ok := GetFactory("nonexistent")
	if ok {
		t.Fatal("expected false for unknown operation")
	}
}

func TestListRegistered(t *testing.T) {
	before := len(ListRegistered())
	factory := func(args json.RawMessage) (Operation, error) {
		return &testOp{}, nil
	}
	Register("list_test", factory)
	defer delete(registry, "list_test")

	after := len(ListRegistered())
	if after != before+1 {
		t.Fatalf("expected %d, got %d", before+1, after)
	}
}

func TestConditionAlwaysTrue(t *testing.T) {
	c := (*Condition)(nil)
	ok, err := c.ShouldExecute("/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("nil condition should always execute")
	}
}

func TestConditionFileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &Condition{FileExists: strPtr("test.txt")}
	ok, err := c.ShouldExecute(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true when file exists")
	}

	c2 := &Condition{FileExists: strPtr("nonexistent.txt")}
	ok2, err := c2.ShouldExecute(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok2 {
		t.Fatal("expected false when file does not exist")
	}
}

func TestConditionFileMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &Condition{FileMissing: strPtr("test.txt")}
	ok, err := c.ShouldExecute(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected false when file exists")
	}

	c2 := &Condition{FileMissing: strPtr("nonexistent.txt")}
	ok2, err := c2.ShouldExecute(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok2 {
		t.Fatal("expected true when file does not exist")
	}
}

func TestStepsFromJSON(t *testing.T) {
	data := []byte(`[
		{"type": "writeFile", "args": {"dest": "hello.txt", "content": "world"}},
		{"type": "downloadFile", "args": {"url": "https://example.com/file", "dest": "file.txt"}}
	]`)
	steps, err := StepsFromJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].Type != "writeFile" {
		t.Fatalf("expected writeFile, got %s", steps[0].Type)
	}
	if steps[1].Type != "downloadFile" {
		t.Fatalf("expected downloadFile, got %s", steps[1].Type)
	}
}

func TestStepsWithCondition(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "exists.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	data := []byte(`[
		{
			"type": "writeFile",
			"args": {"dest": "skipped.txt", "content": "should not run"},
			"condition": {"fileMissing": "exists.txt"}
		},
		{
			"type": "writeFile",
			"args": {"dest": "ran.txt", "content": "ran"},
			"condition": {"fileExists": "exists.txt"}
		}
	]`)
	steps, err := StepsFromJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Register writeFile for this test
	factory := func(args json.RawMessage) (Operation, error) {
		return &testOp{}, nil
	}
	Register("writeFile", factory)
	defer delete(registry, "writeFile")

	if err := ExecuteSteps(context.Background(), dir, steps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStepsUnknownType(t *testing.T) {
	steps := []Step{{Type: "nonexistent_op"}}
	err := ExecuteSteps(context.Background(), "/tmp", steps)
	if err == nil {
		t.Fatal("expected error for unknown operation type")
	}
}

func strPtr(s string) *string { return &s }

func TestResolvePath(t *testing.T) {
	tests := []struct {
		serverDir string
		target    string
		expected  string
	}{
		{"/srv/server", "file.txt", "/srv/server/file.txt"},
		{"/srv/server", "/abs/path", "/abs/path"},
		{"/srv/server", "sub/file.txt", "/srv/server/sub/file.txt"},
	}
	for _, tc := range tests {
		got := ResolvePath(tc.serverDir, tc.target)
		if got != tc.expected {
			t.Errorf("ResolvePath(%q, %q) = %q; want %q", tc.serverDir, tc.target, got, tc.expected)
		}
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ok, err := FileExists(dir, "a.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true")
	}

	ok, err = FileExists(dir, "missing.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected false")
	}
}
