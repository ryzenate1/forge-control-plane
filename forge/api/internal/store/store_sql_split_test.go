package store

import (
	"testing"
)

func TestSplitSQLStatements(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple statements",
			input:    "SELECT 1; SELECT 2;",
			expected: []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:     "trailing whitespace no semicolon",
			input:    "SELECT 1; SELECT 2",
			expected: []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:     "whitespace only",
			input:    "   \n\n  ",
			expected: nil,
		},
		{
			name: "dollar-quoted block with semicolons inside",
			input: `DO $$ BEGIN
    BEGIN
        CREATE TYPE foo AS ENUM ('a', 'b');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

SELECT 1;`,
			expected: []string{
				"DO $$ BEGIN\n    BEGIN\n        CREATE TYPE foo AS ENUM ('a', 'b');\n    EXCEPTION\n        WHEN duplicate_object THEN NULL;\n    END;\nEND $$",
				"SELECT 1",
			},
		},
		{
			name: "multiple dollar-quoted blocks",
			input: `DO $$ BEGIN
    BEGIN
        CREATE TYPE t1 AS ENUM ('x');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

DO $$ BEGIN
    BEGIN
        CREATE TYPE t2 AS ENUM ('y');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

CREATE TABLE IF NOT EXISTS foo (id uuid PRIMARY KEY);`,
			expected: []string{
				"DO $$ BEGIN\n    BEGIN\n        CREATE TYPE t1 AS ENUM ('x');\n    EXCEPTION\n        WHEN duplicate_object THEN NULL;\n    END;\nEND $$",
				"DO $$ BEGIN\n    BEGIN\n        CREATE TYPE t2 AS ENUM ('y');\n    EXCEPTION\n        WHEN duplicate_object THEN NULL;\n    END;\nEND $$",
				"CREATE TABLE IF NOT EXISTS foo (id uuid PRIMARY KEY)",
			},
		},
		{
			name: "named dollar-quote tag",
			input: `CREATE FUNCTION test() RETURNS void AS $fn$
BEGIN
    RAISE NOTICE 'hello;world';
END;
$fn$;`,
			expected: []string{
				"CREATE FUNCTION test() RETURNS void AS $fn$\nBEGIN\n    RAISE NOTICE 'hello;world';\nEND;\n$fn$",
			},
		},
		{
			name: "single-line comments stripped",
			input: `-- This is a comment
SELECT 1;
-- Another comment
SELECT 2;`,
			expected: []string{"SELECT 1", "SELECT 2"},
		},
		{
			name: "real migration 025 pattern",
			input: `DO $$ BEGIN
    BEGIN
        CREATE TYPE node_heartbeat_state AS ENUM ('healthy', 'suspected', 'unreachable', 'offline', 'recovering');
    EXCEPTION
        WHEN duplicate_object THEN NULL;
    END;
END $$;

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS heartbeat_state node_heartbeat_state NOT NULL DEFAULT 'offline',
    ADD COLUMN IF NOT EXISTS heartbeat_state_changed_at timestamptz,
    ADD COLUMN IF NOT EXISTS heartbeat_recovery_count integer NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_nodes_heartbeat_state ON nodes (heartbeat_state);
CREATE INDEX IF NOT EXISTS idx_nodes_last_seen_at ON nodes (last_seen_at);`,
			expected: []string{
				"DO $$ BEGIN\n    BEGIN\n        CREATE TYPE node_heartbeat_state AS ENUM ('healthy', 'suspected', 'unreachable', 'offline', 'recovering');\n    EXCEPTION\n        WHEN duplicate_object THEN NULL;\n    END;\nEND $$",
				"ALTER TABLE nodes\n    ADD COLUMN IF NOT EXISTS heartbeat_state node_heartbeat_state NOT NULL DEFAULT 'offline',\n    ADD COLUMN IF NOT EXISTS heartbeat_state_changed_at timestamptz,\n    ADD COLUMN IF NOT EXISTS heartbeat_recovery_count integer NOT NULL DEFAULT 0",
				"CREATE INDEX IF NOT EXISTS idx_nodes_heartbeat_state ON nodes (heartbeat_state)",
				"CREATE INDEX IF NOT EXISTS idx_nodes_last_seen_at ON nodes (last_seen_at)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitSQLStatements(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("expected %d statements, got %d:\n%v", len(tt.expected), len(got), got)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("statement %d mismatch:\nexpected: %q\ngot:      %q", i, tt.expected[i], got[i])
				}
			}
		})
	}
}

func TestScanDollarTag(t *testing.T) {
	tests := []struct {
		input    string
		pos      int
		expected string
	}{
		{"$$", 0, "$$"},
		{"$fn$", 0, "$fn$"},
		{"$body$", 0, "$body$"},
		{"$ $", 0, ""}, // space is not valid in tag
		{"$123$", 0, "$123$"},
		{"abc$$", 3, "$$"},
		{"", 0, ""},
	}
	for _, tt := range tests {
		runes := []rune(tt.input)
		got := scanDollarTag(runes, tt.pos)
		if got != tt.expected {
			t.Errorf("scanDollarTag(%q, %d) = %q, want %q", tt.input, tt.pos, got, tt.expected)
		}
	}
}
