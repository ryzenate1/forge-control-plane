package http

import (
	"context"
	"regexp"
	"strings"
	"time"

	"gamepanel/forge/internal/services"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// Guards and caps for the read-only diagnostics surface. The /query endpoint
// is deliberately stricter than the provisioner's internal allow-list: only a
// plain SELECT is accepted there, with a hard row cap and a request-scoped
// timeout, so a mis-typed console query cannot stall or mutate a service.
const (
	dbDiagnosticTimeout = 10 * time.Second
	dbDiagnosticMaxRows = 1000
)

var selectOnlyQueryRe = regexp.MustCompile(`(?is)^\s*select\s+\S`)

// registerDBDiagnosticRoutes adds the read-only diagnostics endpoints for
// provisioned database services:
//
//	GET  /admin/database-services/:id/diagnostics — engine-specific health snapshot
//	POST /admin/database-services/:id/query       — read-only query execution (SELECT only)
//	GET  /admin/database-services/:id/stats       — connections, size, cache hit, slow queries
//
// Queries run through the provisioner's existing admin connection path (the
// same route CreateDatabase/CreateUser use), against the service's engine.
func registerDBDiagnosticRoutes(protected fiber.Router, cfg Config) {
	ds := cfg.DatabaseServiceProvisioner

	protected.Get("/admin/database-services/:id/diagnostics", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if ds == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database service provisioner is not available")
		}
		svc, err := loadDBDiagnosticService(c, cfg)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.Context(), dbDiagnosticTimeout)
		defer cancel()

		base := dbDiagnosticEnvelope(svc)
		switch strings.ToLower(svc.Type) {
		case "postgresql":
			detail, err := postgresDBDiagnostics(ctx, ds, svc.ID)
			if err != nil {
				return respondInternalError(c, err)
			}
			for k, v := range detail {
				base[k] = v
			}
		case "mysql", "mariadb":
			detail, err := mysqlDBDiagnostics(ctx, ds, svc.ID)
			if err != nil {
				return respondInternalError(c, err)
			}
			for k, v := range detail {
				base[k] = v
			}
		default:
			base["supported"] = false
			base["note"] = "engine diagnostics not implemented; showing stored service state only"
			return c.JSON(base)
		}
		base["supported"] = true
		return c.JSON(base)
	})

	protected.Post("/admin/database-services/:id/query", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if ds == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database service provisioner is not available")
		}
		svc, err := loadDBDiagnosticService(c, cfg)
		if err != nil {
			return err
		}
		var body struct {
			Query string `json:"query"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		query := strings.TrimSpace(body.Query)
		if query == "" {
			return fiber.NewError(fiber.StatusBadRequest, "query is required")
		}
		// SELECT only. SHOW/EXPLAIN are intentionally not exposed here even
		// though internal diagnostics may use them.
		if !selectOnlyQueryRe.MatchString(query) {
			return fiber.NewError(fiber.StatusBadRequest, "only SELECT statements are allowed")
		}
		if strings.Contains(strings.TrimRight(query, "; \t\n"), ";") {
			return fiber.NewError(fiber.StatusBadRequest, "only a single statement is allowed")
		}
		ctx, cancel := context.WithTimeout(c.Context(), dbDiagnosticTimeout)
		defer cancel()
		// Ask for one more row than the cap so the response can say whether the
		// result set was truncated rather than silently cutting rows.
		rows, err := ds.RunReadOnlyQuery(ctx, svc.ID, query, dbDiagnosticMaxRows+1)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		truncated := len(rows) > dbDiagnosticMaxRows
		if truncated {
			rows = rows[:dbDiagnosticMaxRows]
		}
		return c.JSON(fiber.Map{
			"engine":    strings.ToLower(svc.Type),
			"rowLimit":  dbDiagnosticMaxRows,
			"rowCount":  len(rows),
			"truncated": truncated,
			"rows":      rows,
		})
	})

	protected.Get("/admin/database-services/:id/stats", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if ds == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database service provisioner is not available")
		}
		svc, err := loadDBDiagnosticService(c, cfg)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(c.Context(), dbDiagnosticTimeout)
		defer cancel()

		base := dbDiagnosticEnvelope(svc)
		switch strings.ToLower(svc.Type) {
		case "postgresql":
			detail, err := postgresDBStats(ctx, ds, svc.ID)
			if err != nil {
				return respondInternalError(c, err)
			}
			for k, v := range detail {
				base[k] = v
			}
		case "mysql", "mariadb":
			detail, err := mysqlDBStats(ctx, ds, svc.ID)
			if err != nil {
				return respondInternalError(c, err)
			}
			for k, v := range detail {
				base[k] = v
			}
		default:
			base["supported"] = false
			return c.JSON(base)
		}
		base["supported"] = true
		return c.JSON(base)
	})
}

func loadDBDiagnosticService(c *fiber.Ctx, cfg Config) (store.DatabaseService, error) {
	if cfg.Store == nil {
		return store.DatabaseService{}, fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
	}
	ctx, cancel := requestContext()
	defer cancel()
	svc, err := cfg.Store.GetDatabaseService(ctx, c.Params("id"))
	if err != nil {
		return store.DatabaseService{}, fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return svc, nil
}

func dbDiagnosticEnvelope(svc store.DatabaseService) fiber.Map {
	return fiber.Map{
		"serviceId":   svc.ID,
		"name":        svc.Name,
		"engine":      strings.ToLower(svc.Type),
		"version":     svc.Version,
		"status":      svc.Status,
		"collectedAt": time.Now().UTC().Format(time.RFC3339),
	}
}

func postgresDBDiagnostics(ctx context.Context, ds *services.DatabaseServiceProvisioner, serviceID string) (fiber.Map, error) {
	out := fiber.Map{}

	connections, err := ds.RunReadOnlyQuery(ctx, serviceID,
		"SELECT count(*) AS total, count(*) FILTER (WHERE state = 'active') AS active FROM pg_stat_activity WHERE backend_type = 'client backend'", 10)
	if err != nil {
		return nil, err
	}
	out["connections"] = firstRow(connections)

	size, err := ds.RunReadOnlyQuery(ctx, serviceID,
		"SELECT pg_database_size(current_database()) AS size_bytes, current_database() AS database", 1)
	if err != nil {
		return nil, err
	}
	out["database"] = firstRow(size)

	if hit, cache := cacheHitRatio(ctx, ds, serviceID); hit || cache > 0 {
		out["cacheHitRatio"] = cache
	}

	top, err := ds.RunReadOnlyQuery(ctx, serviceID,
		"SELECT relname, seq_scan, idx_scan, pg_stat_get_numscans(relid) AS total_scans FROM pg_stat_user_tables ORDER BY pg_stat_get_numscans(relid) DESC LIMIT 10", 10)
	if err != nil {
		return nil, err
	}
	out["topTablesByScans"] = top

	return out, nil
}

func postgresDBStats(ctx context.Context, ds *services.DatabaseServiceProvisioner, serviceID string) (fiber.Map, error) {
	out := fiber.Map{}

	byState, err := ds.RunReadOnlyQuery(ctx, serviceID,
		"SELECT COALESCE(state, 'unknown') AS state, count(*) AS connections FROM pg_stat_activity WHERE backend_type = 'client backend' GROUP BY state ORDER BY count(*) DESC", 50)
	if err != nil {
		return nil, err
	}
	out["connectionsByState"] = byState

	size, err := ds.RunReadOnlyQuery(ctx, serviceID,
		"SELECT pg_database_size(current_database()) AS size_bytes", 1)
	if err != nil {
		return nil, err
	}
	out["databaseSize"] = firstRow(size)

	_, ratio := cacheHitRatio(ctx, ds, serviceID)
	out["cacheHitRatio"] = ratio

	// pg_stat_statements gives real slow-query history, but the extension is
	// not installed everywhere — fall back to whatever is running long right now.
	slow, err := ds.RunReadOnlyQuery(ctx, serviceID,
		"SELECT queryid::text AS queryid, calls, mean_exec_time AS mean_ms, query FROM pg_stat_statements ORDER BY mean_exec_time DESC LIMIT 10", 10)
	if err == nil {
		out["slowQueriesSource"] = "pg_stat_statements"
		out["slowQueries"] = slow
		return out, nil
	}
	longRunning, err := ds.RunReadOnlyQuery(ctx, serviceID,
		"SELECT pid, state, EXTRACT(EPOCH FROM (now() - query_start))::bigint AS runtime_seconds, query FROM pg_stat_activity WHERE state <> 'idle' AND query_start IS NOT NULL ORDER BY query_start ASC LIMIT 10", 10)
	if err != nil {
		return nil, err
	}
	out["slowQueriesSource"] = "pg_stat_activity"
	out["slowQueries"] = longRunning
	return out, nil
}

func cacheHitRatio(ctx context.Context, ds *services.DatabaseServiceProvisioner, serviceID string) (bool, float64) {
	rows, err := ds.RunReadOnlyQuery(ctx, serviceID,
		"SELECT blks_hit, blks_read FROM pg_stat_database WHERE datname = current_database()", 1)
	if err != nil || len(rows) == 0 {
		return false, 0
	}
	hit := toInt64(rows[0]["blks_hit"])
	read := toInt64(rows[0]["blks_read"])
	if hit+read == 0 {
		return true, 0
	}
	return true, float64(hit) / float64(hit+read)
}

// mysqlStatusVars is the GLOBAL STATUS subset surfaced by the diagnostics and
// stats endpoints — connectivity, throughput and liveness counters only.
var mysqlStatusVars = map[string]bool{
	"Uptime": true, "Threads_connected": true, "Threads_running": true,
	"Questions": true, "Com_select": true, "Bytes_sent": true, "Bytes_received": true,
	"Innodb_buffer_pool_reads": true, "Innodb_buffer_pool_read_requests": true,
}

func mysqlDBDiagnostics(ctx context.Context, ds *services.DatabaseServiceProvisioner, serviceID string) (fiber.Map, error) {
	out := fiber.Map{}

	status, err := ds.RunReadOnlyQuery(ctx, serviceID, "SHOW GLOBAL STATUS", dbDiagnosticMaxRows)
	if err != nil {
		return nil, err
	}
	picked := fiber.Map{}
	for _, row := range status {
		name := toString(row["Variable_name"])
		if mysqlStatusVars[name] {
			picked[name] = row["Value"]
		}
	}
	out["globalStatus"] = picked

	procs, err := ds.RunReadOnlyQuery(ctx, serviceID,
		"SELECT count(*) AS total, SUM(CASE WHEN command <> 'Sleep' THEN 1 ELSE 0 END) AS active FROM information_schema.processlist", 1)
	if err != nil {
		return nil, err
	}
	out["processlist"] = firstRow(procs)

	size, err := mysqlDatabaseSize(ctx, ds, serviceID)
	if err != nil {
		return nil, err
	}
	out["databaseSize"] = firstRow(size)

	return out, nil
}

func mysqlDBStats(ctx context.Context, ds *services.DatabaseServiceProvisioner, serviceID string) (fiber.Map, error) {
	out := fiber.Map{}

	status, err := ds.RunReadOnlyQuery(ctx, serviceID, "SHOW GLOBAL STATUS", dbDiagnosticMaxRows)
	if err != nil {
		return nil, err
	}
	connections := fiber.Map{}
	for _, row := range status {
		name := toString(row["Variable_name"])
		switch name {
		case "Threads_connected", "Threads_running", "Uptime", "Questions":
			connections[name] = row["Value"]
		}
	}
	out["connections"] = connections

	size, err := mysqlDatabaseSize(ctx, ds, serviceID)
	if err != nil {
		return nil, err
	}
	out["databaseSize"] = firstRow(size)

	// Buffer pool hit ratio stands in for the removed query cache on modern
	// MySQL/MariaDB: read requests served from the pool over total reads.
	hitRatio := 0.0
	reads := toInt64(statusValue(status, "Innodb_buffer_pool_reads"))
	readReqs := toInt64(statusValue(status, "Innodb_buffer_pool_read_requests"))
	if readReqs > 0 {
		hitRatio = float64(readReqs-reads) / float64(readReqs)
	}
	out["cacheHitRatio"] = hitRatio

	slow, err := ds.RunReadOnlyQuery(ctx, serviceID,
		"SELECT id, user, host, db, command, time, state, info FROM information_schema.processlist WHERE command <> 'Sleep' ORDER BY time DESC LIMIT 10", 10)
	if err != nil {
		return nil, err
	}
	out["slowQueries"] = slow

	return out, nil
}

func mysqlDatabaseSize(ctx context.Context, ds *services.DatabaseServiceProvisioner, serviceID string) ([]map[string]any, error) {
	return ds.RunReadOnlyQuery(ctx, serviceID,
		"SELECT IFNULL(SUM(data_length + index_length), 0) AS size_bytes, COUNT(*) AS table_count FROM information_schema.tables WHERE table_schema = DATABASE()", 1)
}

func statusValue(rows []map[string]any, name string) any {
	for _, row := range rows {
		if toString(row["Variable_name"]) == name {
			return row["Value"]
		}
	}
	return nil
}

func firstRow(rows []map[string]any) any {
	if len(rows) == 0 {
		return fiber.Map{}
	}
	return rows[0]
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}
