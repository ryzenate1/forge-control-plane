package models

import "sort"

// Migration describes a single schema migration step.
type Migration struct {
	Version uint64
	Name    string
	Up      func() string
	Down    func() string
}

// MigrationTable returns the SQL used to bootstrap the schema version tracking
// table. Mirrors the codebase's existing schema_migrations convention.
func MigrationTable() string {
	return `CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)`
}

// RegisteredMigrations returns the ordered list of registered migrations
// sorted ascending by Version. The slice is a defensive copy.
func RegisteredMigrations() []Migration {
	migrations := []Migration{
		{
			Version: 1,
			Name:    "0001_init",
			Up: func() string {
				return `
CREATE TABLE IF NOT EXISTS servers (
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    owner_id    UUID NOT NULL,
    node_id     UUID NOT NULL,
    status      TEXT NOT NULL DEFAULT 'stopped',
    suspended    BOOLEAN NOT NULL DEFAULT FALSE,
    installing   BOOLEAN NOT NULL DEFAULT FALSE,
    installed    BOOLEAN NOT NULL DEFAULT FALSE,
    disk_mb      INTEGER NOT NULL DEFAULT 0,
    memory_mb    INTEGER NOT NULL DEFAULT 0,
    cpu_shares   INTEGER NOT NULL DEFAULT 0,
    swap_mb      INTEGER NOT NULL DEFAULT 0,
    egg_id       TEXT NOT NULL DEFAULT '',
    nest_id      TEXT NOT NULL DEFAULT '',
    container    JSONB NOT NULL DEFAULT '{}'::jsonb,
    env          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT NOT NULL DEFAULT '',
    role_id       BIGINT NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'active',
    last_login_at TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS roles (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    is_admin    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS permissions (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT ''
);
`
			},
			Down: func() string {
				return `
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS servers;
`
			},
		},
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})
	out := make([]Migration, len(migrations))
	copy(out, migrations)
	return out
}
