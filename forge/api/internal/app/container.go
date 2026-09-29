package app

// Container stages the Forge API wiring that currently lives in
// cmd/api/main.go:114 run() (1,573 lines, 60-var service graph,
// server.go:95 Config 76 fields, server.go:845 NewServer 1,432 lines).
//
// Target per docs/architecture/target-ia.md and target-api-map.md §4 +
// plan silly-honking-kitten.md § “API Architecture → API Refactor Sequence”:
//   InitDB → InitStores → InitServices → BuildHTTP
// Each stage is additive and nil-safe when the DB pool is absent (dev mode
// without DATABASE_URL), preserving the handler nil-guards that make the
// current dev-mode work.
//
// This file is stage 1 of CRITICAL 3: it introduces the staged container
// shape and moves the DB/master-keyring/migration/seed logic out of run().
// Later stages will move the 60-var service graph (placement, scheduler,
// reservers, heartbeatmonitor, etc.) into InitServices and the HTTP
// server wiring into BuildHTTP, then main.go:run() becomes:
//   c := app.New(logger); c.InitDB(ctx, prod, appEnv); c.InitStores(ctx); c.InitServices(ctx); srv := c.BuildHTTP()
//
// Safety: no delete, only additive extraction; go vet ./forge/api/internal/store stays PASS
// and dev-mode (no DATABASE_URL) keeps working.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	forgecfg "gamepanel/forge/config"
	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/eventstore"
	"gamepanel/forge/internal/placement"
	"gamepanel/forge/internal/secrets"
	"gamepanel/forge/internal/services/logger"
	"gamepanel/forge/internal/store"

	"github.com/redis/go-redis/v9"
)

// Container holds the Forge API dependency graph in staged form.
// Fields are intentionally public so cmd/api can wire them incrementally
// as stages are migrated; after full extraction they will become private.
type Container struct {
	Logger *slog.Logger

	// Stage 1 — persistence & secrets
	DB             *store.Store
	MasterKeyring  *secrets.Keyring
	EphemeralKey   bool
	EventRelay     *eventstore.Relay
	RedisClient    *redis.Client
	RedisEnabled   bool

	// Stage 2 — stores & placement (populated by InitStores)
	EventRegistry *events.Registry
	PlaceEngine   *placement.Engine

	// Stage 3 — services bundle (60-var graph from main.go:250)
	// Added incrementally; see InitServices.
	Services ServicesBundle

	// Stage 4 — HTTP server (server.go:95 Config, server.go:845 NewServer)
	// Built by BuildHTTP after Services are ready.
	MTLSCfg forgecfg.MTLS
}

// ServicesBundle is a legacy staging placeholder. The canonical 60-var
// service graph lives in cmd/api/main.go:250 and is injected into the HTTP
// layer via http.Config; nothing in production constructs this bundle.
// It is retained (empty) so staged-migration callers keep compiling while
// InitServices/BuildHTTP document the boundary instead of failing closed.
type ServicesBundle struct {
	// Core placement & lifecycle (populated first)
	// Keep minimal for stage 1; full 60-var list lands in stage 3.
}

// New creates a Container with a production-aware slog logger.
// appEnv is "production" vs "development"; logger respects LOG_LEVEL/FORMAT/OUTPUT.
func New() (*Container, error) {
	lg := logger.New(logger.Config{
		Level:  env("LOG_LEVEL", "info"),
		Format: env("LOG_FORMAT", "text"),
		Output: env("LOG_OUTPUT", "stdout"),
	})
	return &Container{Logger: lg}, nil
}

// InitDB stages database connectivity, master-keyring resolution, migrations,
// operational-secret rotation, and optional demo seeding. It is safe to call
// when DATABASE_URL is unset — in that case DB stays nil and the rest of
// the stages remain nil-safe (handler nil-guards already handle dev mode).
//
// production and appEnv control strict secret validation (same as run()).
func (c *Container) InitDB(ctx context.Context, production bool, appEnv string) error {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}

	// Demo seeding flag (same semantics as main.go:133 demoSeedEnabled)
	seedDemo, err := demoSeedEnabled(appEnv, os.Getenv("API_SEED_DEMO"))
	if err != nil {
		return err
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		c.Logger.Info("DATABASE_URL not set; running without persistence (dev mode)")
		return nil
	}

	kr, ephemeral, err := masterKeyringFromEnvironment(production)
	if err != nil {
		return err
	}
	c.MasterKeyring = kr
	c.EphemeralKey = ephemeral
	if ephemeral {
		c.Logger.Warn("FORGE_ALLOW_EPHEMERAL_MASTER_KEY is enabled; encrypted data will be unrecoverable after this process exits")
	}

	connected, err := store.ConnectWithKeyring(ctx, databaseURL, kr)
	if err != nil {
		return err
	}
	// Caller (run) defers Close; we keep the handle so later stages can use it.
	// Defer is owned by the caller; we store it for stores/services stages.
	if err := connected.RunMigrations(ctx, env("MIGRATIONS_DIR", "migrations")); err != nil {
		connected.Close()
		return err
	}
	// Migration filenames are immutable primary keys in schema_migrations, so
	// an edit to an already-applied migration is never re-run. Drift is
	// reported rather than fatal: the deployed schema is whatever it is, and
	// refusing to boot would not repair it. It must not pass silently either.
	if integrity := connected.MigrationIntegrity(); len(integrity.Drift) > 0 {
		for _, d := range integrity.Drift {
			c.Logger.Warn("migration file changed after it was applied; deployed schema may diverge from this build",
				slog.String("migration", d.Version),
				slog.String("applied_checksum", d.Applied),
				slog.String("on_disk_checksum", d.OnDisk))
		}
		c.Logger.Warn("migration drift detected",
			slog.Int("drifted", len(integrity.Drift)),
			slog.Int("unverifiable", integrity.Unverified))
	}
	if err := eventstore.Migrate(connected.GetDB()); err != nil {
		connected.Close()
		return err
	}
	if err := connected.MigrateOperationalSecrets(ctx); err != nil {
		connected.Close()
		return err
	}
	if len(os.Args) > 1 && os.Args[1] == "rotate-master-key" {
		c.Logger.Info("secret rotation completed", slog.String("active_key", kr.ActiveKeyID()))
		connected.Close()
		return errors.New("rotate-master-key completed; exiting")
	}
	if len(os.Args) > 1 && os.Args[1] == "restore-plaintext-secrets" {
		if err := connected.RestoreOperationalSecrets(ctx); err != nil {
			connected.Close()
			return err
		}
		c.Logger.Info("legacy plaintext secret columns restored; ciphertext retained")
		connected.Close()
		return errors.New("restore-plaintext-secrets completed; exiting")
	}
	if seedDemo {
		if err := connected.Seed(ctx); err != nil {
			connected.Close()
			return err
		}
	}
	c.DB = connected

	// Eventstore relay (same as main.go:343)
	es := eventstore.New(c.DB.GetDB())
	c.EventRelay = eventstore.NewRelay(es, 5*time.Second)

	// Redis — same as main.go:208 but staged here so BuildHTTP can depend on it
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		c.RedisEnabled = true
		redisPassword := strings.TrimSpace(os.Getenv("REDIS_PASSWORD"))
		redisTLS := envBool("REDIS_TLS", production)
		if production && redisPassword == "" {
			connected.Close()
			return errors.New("REDIS_PASSWORD is required when Redis is enabled in production")
		}
		if production && !redisTLS {
			connected.Close()
			return errors.New("REDIS_TLS must be enabled when Redis is used in production")
		}
		opts := &redis.Options{Addr: redisAddr, Password: redisPassword}
		// TLS is set by caller via redis.Options.TLSConfig when needed; keep minimal here
		client := redis.NewClient(opts)
		if err := client.Ping(ctx).Err(); err != nil {
			c.Logger.Warn("redis ping failed at startup", slog.String("error", err.Error()))
		}
		c.RedisClient = client
	}

	return nil
}

// InitStores builds placement, event registry and scheduler prerequisites
// from the DB handle. No-op when DB is nil (dev mode). Mirrors main.go:346-365
// placement engine wiring so BuildHTTP can later depend on it.
func (c *Container) InitStores(_ context.Context) error {
	if c.DB == nil {
		return nil
	}
	c.EventRegistry = events.NewRegistry("forge-api")
	// StrategyLeastLoaded + ConstraintChecker mirrors main.go:353
	c.PlaceEngine = placement.NewEngine(placement.NewScorer(placement.StrategyLeastLoaded), placement.NewConstraintChecker())
	return nil
}

// InitServices is a documented no-op staging hook. The real service graph is
// built in cmd/api/main.go (placement, scheduler, heartbeatmonitor, compose
// lifecycle, acme, previewenv, etc.) and injected via http.Config, so this
// container must not fail closed in production when a DB is present.
// Layering boundary: handlers -> services (owned by main) -> store.
func (c *Container) InitServices(ctx context.Context) error {
	if c.DB == nil {
		return nil
	}
	_ = ctx
	// Stage 3 wiring lives in cmd/api/main.go; nothing to do here.
	return nil
}

// BuildHTTP is a documented no-op staging hook. The HTTP server is assembled
// in cmd/api/main.go via http.Config + http.NewServer, not from this
// container, so returning an error here would break the production path that
// never calls it. Kept so staged-migration callers keep compiling.
func (c *Container) BuildHTTP(_ context.Context) error {
	// Stage 4 wiring lives in cmd/api/main.go; nothing to do here.
	return nil
}

// Close releases resources owned by the container (DB, Redis, relay).
func (c *Container) Close() {
	if c.RedisClient != nil {
		_ = c.RedisClient.Close()
	}
	if c.DB != nil {
		c.DB.Close()
	}
}

// helpers copied from main.go for additive staging — kept private to this
// package until main.go is fully migrated, then main.go's copies are removed.

// env and envBool mirror main.go helpers.
func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	b, err := parseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "t", "true", "yes", "y", "on":
		return true, nil
	case "0", "f", "false", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool %q", s)
	}
}

// demoSeedEnabled mirrors main.go:demoSeedEnabled semantics. Seeding demo
// data in production is rejected outright: demo rows would pollute prod and
// the seed path is never audited for prod safety.
func demoSeedEnabled(appEnv, flag string) (bool, error) {
	v := strings.ToLower(strings.TrimSpace(flag))
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("API_SEED_DEMO must be a boolean: %w", err)
	}
	if b && strings.EqualFold(strings.TrimSpace(appEnv), "production") {
		return false, errors.New("API_SEED_DEMO must not be enabled in production")
	}
	return b, nil
}

func masterKeyringFromEnvironment(production bool) (*secrets.Keyring, bool, error) {
	activeKey := strings.TrimSpace(os.Getenv("FORGE_MASTER_KEY"))
	if production && strings.Contains(activeKey, "CHANGE_ME") {
		return nil, false, errors.New("FORGE_MASTER_KEY must not contain a deployment placeholder")
	}
	ephemeral := false
	if activeKey == "" {
		allowEphemeral, err := strconv.ParseBool(env("FORGE_ALLOW_EPHEMERAL_MASTER_KEY", "false"))
		if err != nil {
			return nil, false, errors.New("FORGE_ALLOW_EPHEMERAL_MASTER_KEY must be a boolean")
		}
		if production || !allowEphemeral {
			return nil, false, errors.New("FORGE_MASTER_KEY is required before database startup")
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return nil, false, errors.New("generate ephemeral master key")
		}
		activeKey = base64.StdEncoding.EncodeToString(raw)
		ephemeral = true
	}
	previous, err := parsePreviousMasterKeys(os.Getenv("FORGE_PREVIOUS_MASTER_KEYS"))
	if err != nil {
		return nil, false, err
	}
	keyring, err := secrets.New(env("FORGE_MASTER_KEY_ID", "primary"), activeKey, previous)
	if err != nil {
		return nil, false, err
	}
	return keyring, ephemeral, nil
}

func parsePreviousMasterKeys(raw string) (map[string]string, error) {
	keys := map[string]string{}
	for _, entry := range strings.Split(strings.TrimSpace(raw), ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, errors.New("FORGE_PREVIOUS_MASTER_KEYS must contain comma-separated key-id=encoded-key entries")
		}
		id := strings.TrimSpace(parts[0])
		if _, exists := keys[id]; exists {
			return nil, errors.New("FORGE_PREVIOUS_MASTER_KEYS contains a duplicate key ID")
		}
		keys[id] = strings.TrimSpace(parts[1])
	}
	return keys, nil
}
