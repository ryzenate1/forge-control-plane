package store

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	DatabaseStatePending = "pending"
	DatabaseStateReady   = "ready"
	DatabaseStateFailed  = "failed"

	databaseHostSelectionSQL = `
		SELECT h.id::text
		FROM servers s
		JOIN database_hosts h ON (
		    EXISTS (SELECT 1 FROM database_host_node dhn WHERE dhn.database_host_id = h.id AND dhn.node_id = s.node_id)
		    OR NOT EXISTS (SELECT 1 FROM database_host_node dhn WHERE dhn.database_host_id = h.id)
		)
		WHERE s.id = $1
		  AND (h.max_databases IS NULL OR (SELECT count(*) FROM server_databases d WHERE d.database_host_id = h.id AND d.provisioning_state <> 'failed') < h.max_databases)
		ORDER BY (NOT EXISTS (SELECT 1 FROM database_host_node dhn WHERE dhn.database_host_id = h.id)), h.created_at
		LIMIT 1`
)

var (
	databaseIdentifierPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)
	adminUsernamePattern      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.@-]{0,127}$`)
	hostnamePattern           = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$`)
)

// ListServerDatabases always redacts passwords. The includePassword argument is
// retained for API compatibility, but credentials are write-only after creation/rotation.
func (s *Store) ListServerDatabases(ctx context.Context, serverID string, _ bool) ([]ServerDatabase, error) {
	rows, err := s.db.Query(ctx, `
		SELECT d.id::text, d.database_name, d.username, d.remote, h.engine, h.host, h.port,
		       d.max_connections, d.provisioning_state, d.provisioning_error
		FROM server_databases d
		JOIN database_hosts h ON h.id = d.database_host_id
		WHERE d.server_id = $1
		ORDER BY d.created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	databases := []ServerDatabase{}
	for rows.Next() {
		var database ServerDatabase
		if err := rows.Scan(&database.ID, &database.DatabaseName, &database.Username, &database.Remote, &database.Engine, &database.Host, &database.Port, &database.MaxConnections, &database.ProvisioningState, &database.ProvisioningError); err != nil {
			return nil, err
		}
		databases = append(databases, database)
	}
	return databases, rows.Err()
}

func (s *Store) GetServerDatabaseForProvisioning(ctx context.Context, serverID, databaseID string) (ServerDatabase, error) {
	var database ServerDatabase
	var plaintextPassword, encryptedPassword string
	err := s.db.QueryRow(ctx, `
		SELECT d.id::text, d.database_name, d.username, d.remote, h.engine, h.host, h.port,
		       d.max_connections, d.provisioning_state, d.provisioning_error, COALESCE(d.password,''), COALESCE(d.password_encrypted,'')
		FROM server_databases d
		JOIN database_hosts h ON h.id = d.database_host_id
		WHERE d.id = $1 AND d.server_id = $2
	`, databaseID, serverID).Scan(&database.ID, &database.DatabaseName, &database.Username, &database.Remote, &database.Engine, &database.Host, &database.Port, &database.MaxConnections, &database.ProvisioningState, &database.ProvisioningError, &plaintextPassword, &encryptedPassword)
	if err != nil {
		return ServerDatabase{}, err
	}
	password, err := s.decryptSecret(encryptedPassword, plaintextPassword, secretAAD("server_databases", database.ID, "password"))
	if err != nil {
		return ServerDatabase{}, err
	}
	database.Password = &password
	return database, nil
}

func (s *Store) GetServerDatabase(ctx context.Context, serverID, databaseID string) (ServerDatabase, error) {
	var database ServerDatabase
	err := s.db.QueryRow(ctx, `
		SELECT d.id::text, d.database_name, d.username, d.remote, h.engine, h.host, h.port,
		       d.max_connections, d.provisioning_state, d.provisioning_error
		FROM server_databases d
		JOIN database_hosts h ON h.id = d.database_host_id
		WHERE d.id = $1 AND d.server_id = $2
	`, databaseID, serverID).Scan(&database.ID, &database.DatabaseName, &database.Username, &database.Remote, &database.Engine, &database.Host, &database.Port, &database.MaxConnections, &database.ProvisioningState, &database.ProvisioningError)
	if err != nil {
		return ServerDatabase{}, err
	}
	return database, nil
}

func (s *Store) CreateServerDatabase(ctx context.Context, serverID string, req CreateServerDatabaseRequest, actorID *string) (ServerDatabase, error) {
	name, err := validateRequestedDatabaseIdentifier(req.Database)
	if err != nil {
		return ServerDatabase{}, err
	}
	remote, err := ValidateDatabaseRemote(req.Remote)
	if err != nil {
		return ServerDatabase{}, err
	}
	var limit int
	if err := s.db.QueryRow(ctx, `SELECT database_limit FROM servers WHERE id = $1`, serverID).Scan(&limit); err != nil {
		return ServerDatabase{}, err
	}
	if limit > 0 {
		var count int
		if err := s.db.QueryRow(ctx, `SELECT count(*) FROM server_databases WHERE server_id = $1 AND provisioning_state <> 'failed'`, serverID).Scan(&count); err != nil {
			return ServerDatabase{}, err
		}
		if count >= limit {
			return ServerDatabase{}, errors.New("server database limit reached")
		}
	}
	var hostID string
	if err := s.db.QueryRow(ctx, databaseHostSelectionSQL, serverID).Scan(&hostID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServerDatabase{}, errors.New("no database host available for this server")
		}
		return ServerDatabase{}, err
	}
	serverUUID, err := uuid.Parse(serverID)
	if err != nil {
		return ServerDatabase{}, errors.New("invalid server id")
	}
	short := strings.ReplaceAll(serverUUID.String(), "-", "")[:8]
	databaseName := "s" + short + "_" + strings.ToLower(name)
	username := "u" + short + "_" + strings.ToLower(name)
	if len(username) > 32 {
		username = username[:32]
	}
	if !databaseIdentifierPattern.MatchString(databaseName) || !databaseIdentifierPattern.MatchString(username) {
		return ServerDatabase{}, errors.New("generated database identifier is invalid")
	}
	id := uuid.NewString()
	password := newDaemonToken()[:32]
	encryptedPassword, err := s.encryptSecret(password, secretAAD("server_databases", id, "password"))
	if err != nil {
		return ServerDatabase{}, err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO server_databases
		    (id, server_id, database_host_id, database_name, username, password, password_encrypted, remote, max_connections, provisioning_state)
		VALUES ($1, $2, $3, $4, $5, '', $6, $7, $8, 'pending')
	`, id, serverID, hostID, databaseName, username, encryptedPassword, remote, req.MaxConnections)
	if err != nil {
		return ServerDatabase{}, err
	}
	_ = s.AppendAudit(ctx, actorID, "server database provisioning started", "server", &serverID, fmt.Sprintf(`{"databaseId":%q}`, id))
	return s.GetServerDatabaseForProvisioning(ctx, serverID, id)
}

func (s *Store) SetServerDatabaseProvisioningState(ctx context.Context, serverID, databaseID, state, detail string) error {
	if state != DatabaseStatePending && state != DatabaseStateReady && state != DatabaseStateFailed {
		return errors.New("invalid database provisioning state")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE server_databases
		SET provisioning_state = $1, provisioning_error = $2, updated_at = now()
		WHERE id = $3 AND server_id = $4
	`, state, detail, databaseID, serverID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("database not found")
	}
	_ = s.AppendAudit(ctx, nil, "server database provisioning "+state, "server", &serverID, fmt.Sprintf(`{"databaseId":%q}`, databaseID))
	return nil
}

func (s *Store) NewServerDatabasePasswordCandidate() string { return newDaemonToken()[:32] }

func (s *Store) CommitServerDatabasePassword(ctx context.Context, serverID, databaseID, expectedPassword, password string, actorID *string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var plaintext, encrypted string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(password,''), COALESCE(password_encrypted,'') FROM server_databases WHERE id=$1 AND server_id=$2 FOR UPDATE`, databaseID, serverID).Scan(&plaintext, &encrypted); err != nil {
		return errors.New("database credential changed concurrently or database not found")
	}
	current, err := s.decryptSecret(encrypted, plaintext, secretAAD("server_databases", databaseID, "password"))
	if err != nil || current != expectedPassword {
		return errors.New("database credential changed concurrently or database not found")
	}
	replacement, err := s.encryptSecret(password, secretAAD("server_databases", databaseID, "password"))
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE server_databases SET password='', password_encrypted=$1, provisioning_state='ready', provisioning_error='', updated_at=now() WHERE id=$2 AND server_id=$3`, replacement, databaseID, serverID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	_ = s.AppendAudit(ctx, actorID, "server database password rotated", "server", &serverID, fmt.Sprintf(`{"databaseId":%q}`, databaseID))
	return nil
}

func (s *Store) GetDatabaseHost(ctx context.Context, id string) (DatabaseHost, error) {
	row := s.db.QueryRow(ctx, databaseHostSelect+` WHERE h.id = $1 GROUP BY h.id`, id)
	return scanDatabaseHost(row)
}

const databaseHostSelect = `
	SELECT h.id::text,
	       COALESCE(array_remove(array_agg(DISTINCT dhn.node_id::text), NULL), '{}'),
	       COALESCE(array_remove(array_agg(DISTINCT n.name), NULL), '{}'),
	       h.engine, h.name, h.host, h.port,
	       h.username, h.tls_mode, h.tls_server_name, h.max_databases,
	       count(d.id)::int
	FROM database_hosts h
	LEFT JOIN database_host_node dhn ON dhn.database_host_id = h.id
	LEFT JOIN nodes n ON n.id = dhn.node_id
	LEFT JOIN server_databases d ON d.database_host_id = h.id`

type rowScanner interface{ Scan(...any) error }

func scanDatabaseHost(row rowScanner) (DatabaseHost, error) {
	var host DatabaseHost
	if err := row.Scan(&host.ID, &host.NodeIDs, &host.NodeNames, &host.Engine, &host.Name, &host.Host, &host.Port, &host.Username, &host.TLSMode, &host.TLSServerName, &host.MaxDatabases, &host.Databases); err != nil {
		return DatabaseHost{}, err
	}
	if len(host.NodeIDs) > 0 {
		host.NodeID = host.NodeIDs[0]
	}
	return host, nil
}

func (s *Store) ListDatabaseHosts(ctx context.Context) ([]DatabaseHost, error) {
	rows, err := s.db.Query(ctx, databaseHostSelect+` GROUP BY h.id ORDER BY h.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hosts := []DatabaseHost{}
	for rows.Next() {
		host, err := scanDatabaseHost(rows)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}
	return hosts, rows.Err()
}

// DatabaseHostForConnectionTest normalizes and validates a prospective host
// without resolving its node or persisting its credentials.
func DatabaseHostForConnectionTest(req CreateDatabaseHostRequest) (DatabaseHost, string, error) {
	if err := validateDatabaseHostRequest(&req.Engine, &req.Name, &req.Host, &req.Port, &req.Username, &req.TLSMode, &req.TLSCA, &req.TLSServerName, req.MaxDatabases); err != nil {
		return DatabaseHost{}, "", err
	}
	if strings.TrimSpace(req.Password) == "" {
		return DatabaseHost{}, "", errors.New("password is required")
	}
	return DatabaseHost{
		Engine:        req.Engine,
		Name:          req.Name,
		Host:          req.Host,
		Port:          req.Port,
		Username:      req.Username,
		TLSMode:       req.TLSMode,
		TLSCA:         req.TLSCA,
		TLSServerName: req.TLSServerName,
		MaxDatabases:  req.MaxDatabases,
	}, req.Password, nil
}

func (s *Store) CreateDatabaseHost(ctx context.Context, req CreateDatabaseHostRequest, actorID *string) (DatabaseHost, error) {
	host, password, err := DatabaseHostForConnectionTest(req)
	if err != nil {
		return DatabaseHost{}, err
	}
	nodeIDs, err := s.resolveDatabaseHostNodeIDs(ctx, req)
	if err != nil {
		return DatabaseHost{}, err
	}
	id := uuid.NewString()
	encryptedPassword, err := s.encryptSecret(password, secretAAD("database_hosts", id, "password"))
	if err != nil {
		return DatabaseHost{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return DatabaseHost{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO database_hosts
		    (id, engine, name, host, port, username, password, password_encrypted, tls_mode, tls_ca, tls_server_name, max_databases)
		VALUES ($1, $2, $3, $4, $5, $6, $7, '', $8, $9, $10, $11, $12)
	`, id, host.Engine, host.Name, host.Host, host.Port, host.Username, encryptedPassword, host.TLSMode, host.TLSCA, host.TLSServerName, host.MaxDatabases)
	if err != nil {
		return DatabaseHost{}, err
	}
	for _, nodeID := range nodeIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO database_host_node (database_host_id, node_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, id, nodeID); err != nil {
			return DatabaseHost{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return DatabaseHost{}, err
	}
	_ = s.AppendAudit(ctx, actorID, "database host created", "database_host", &id, fmt.Sprintf(`{"engine":%q,"host":%q}`, host.Engine, host.Host))
	return s.GetDatabaseHost(ctx, id)
}

func (s *Store) resolveDatabaseHostNodeIDs(ctx context.Context, req CreateDatabaseHostRequest) ([]string, error) {
	ids := req.NodeIDs
	if len(ids) == 0 && strings.TrimSpace(req.NodeID) != "" {
		ids = []string{strings.TrimSpace(req.NodeID)}
	}
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return nil, fmt.Errorf("invalid node id: %q", id)
		}
		var exists bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nodes WHERE id = $1)`, id).Scan(&exists); err != nil || !exists {
			return nil, fmt.Errorf("node %q does not exist", id)
		}
	}
	return ids, nil
}

func (s *Store) UpdateDatabaseHost(ctx context.Context, hostID string, req UpdateDatabaseHostRequest, actorID *string) (DatabaseHost, error) {
	tlsCA := ""
	if req.TLSCA != nil {
		tlsCA = *req.TLSCA
	}
	if err := validateDatabaseHostRequest(&req.Engine, &req.Name, &req.Host, &req.Port, &req.Username, &req.TLSMode, &tlsCA, &req.TLSServerName, req.MaxDatabases); err != nil {
		return DatabaseHost{}, err
	}
	if req.TLSCA != nil {
		req.TLSCA = &tlsCA
	}
	nodeIDs, err := s.resolveUpdateDatabaseHostNodeIDs(ctx, req)
	if err != nil {
		return DatabaseHost{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return DatabaseHost{}, err
	}
	defer tx.Rollback(ctx)
	var tag pgconn.CommandTag
	if strings.TrimSpace(req.Password) != "" {
		encryptedPassword, encryptErr := s.encryptSecret(req.Password, secretAAD("database_hosts", hostID, "password"))
		if encryptErr != nil {
			return DatabaseHost{}, encryptErr
		}
		tag, err = tx.Exec(ctx, `UPDATE database_hosts SET engine=$1, name=$2, host=$3, port=$4, username=$5, password='', password_encrypted=$6, tls_mode=$7, tls_ca=COALESCE($8, tls_ca), tls_server_name=$9, max_databases=$10, updated_at=now() WHERE id=$11`, req.Engine, req.Name, req.Host, req.Port, req.Username, encryptedPassword, req.TLSMode, req.TLSCA, req.TLSServerName, req.MaxDatabases, hostID)
	} else {
		tag, err = tx.Exec(ctx, `UPDATE database_hosts SET engine=$1, name=$2, host=$3, port=$4, username=$5, tls_mode=$6, tls_ca=COALESCE($7, tls_ca), tls_server_name=$8, max_databases=$9, updated_at=now() WHERE id=$10`, req.Engine, req.Name, req.Host, req.Port, req.Username, req.TLSMode, req.TLSCA, req.TLSServerName, req.MaxDatabases, hostID)
	}
	if err != nil {
		return DatabaseHost{}, err
	}
	if tag.RowsAffected() == 0 {
		return DatabaseHost{}, errors.New("database host not found")
	}
	if nodeIDs != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM database_host_node WHERE database_host_id = $1`, hostID); err != nil {
			return DatabaseHost{}, err
		}
		for _, nodeID := range nodeIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO database_host_node (database_host_id, node_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, hostID, nodeID); err != nil {
				return DatabaseHost{}, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return DatabaseHost{}, err
	}
	_ = s.AppendAudit(ctx, actorID, "database host updated", "database_host", &hostID, fmt.Sprintf(`{"engine":%q,"host":%q}`, req.Engine, req.Host))
	return s.GetDatabaseHost(ctx, hostID)
}

func (s *Store) resolveUpdateDatabaseHostNodeIDs(ctx context.Context, req UpdateDatabaseHostRequest) ([]string, error) {
	ids := req.NodeIDs
	if len(ids) == 0 && strings.TrimSpace(req.NodeID) != "" {
		ids = []string{strings.TrimSpace(req.NodeID)}
	}
	if ids == nil {
		return nil, nil
	}
	for i, id := range ids {
		ids[i] = strings.TrimSpace(id)
		if ids[i] == "" {
			return nil, errors.New("node id must not be empty")
		}
		if _, err := uuid.Parse(ids[i]); err != nil {
			return nil, fmt.Errorf("invalid node id: %q", ids[i])
		}
		var exists bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nodes WHERE id = $1)`, ids[i]).Scan(&exists); err != nil || !exists {
			return nil, fmt.Errorf("node %q does not exist", ids[i])
		}
	}
	return ids, nil
}

func (s *Store) databaseHostNodeID(ctx context.Context, value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if _, err := uuid.Parse(value); err != nil {
		return nil, errors.New("invalid node id")
	}
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nodes WHERE id = $1)`, value).Scan(&exists); err != nil || !exists {
		return nil, errors.New("node does not exist")
	}
	return value, nil
}

func (s *Store) DeleteDatabaseHost(ctx context.Context, hostID string, actorID *string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM database_hosts WHERE id = $1 AND NOT EXISTS (SELECT 1 FROM server_databases WHERE database_host_id = $1)`, hostID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("database host not found or in use")
	}
	return s.AppendAudit(ctx, actorID, "database host deleted", "database_host", &hostID, `{"reason":"admin delete"}`)
}

// RotateServerDatabasePassword is retained to prevent unsafe callers from
// updating the panel credential before the remote host accepts it.
func (s *Store) RotateServerDatabasePassword(context.Context, string, string, *string) (ServerDatabase, error) {
	return ServerDatabase{}, errors.New("password rotation must be performed by the database provisioner")
}

func (s *Store) DeleteServerDatabase(ctx context.Context, serverID, databaseID string, actorID *string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM server_databases WHERE id = $1 AND server_id = $2`, databaseID, serverID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("database not found")
	}
	_ = s.AppendAudit(ctx, actorID, "server database deleted", "server", &serverID, fmt.Sprintf(`{"databaseId":%q}`, databaseID))
	return nil
}

func (s *Store) ForceDeleteServerDatabase(ctx context.Context, serverID, databaseID, reason string, actorID *string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		WITH target AS (
			SELECT d.*, h.engine, h.host, h.port FROM server_databases d
			JOIN database_hosts h ON h.id = d.database_host_id
			WHERE d.id = $1 AND d.server_id = $2
		), remediation AS (
			INSERT INTO database_orphan_remediations
			    (id, server_database_id, server_id, database_host_id, engine, host, port, database_name, username, remote, reason)
			SELECT $3, id, server_id, database_host_id, engine, host, port, database_name, username, remote, $4 FROM target
			RETURNING server_database_id
		)
		DELETE FROM server_databases d USING remediation r WHERE d.id = r.server_database_id
	`, databaseID, serverID, uuid.NewString(), reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("database not found")
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	_ = s.AppendAudit(ctx, actorID, "server database force deleted with orphan remediation", "server", &serverID, fmt.Sprintf(`{"databaseId":%q,"reason":%q}`, databaseID, reason))
	return nil
}

// GetDatabaseHostForTest returns a host and its decrypted administrator credential
// for an authenticated connection test. Callers must never serialize the password.
func (s *Store) GetDatabaseHostForTest(ctx context.Context, hostID string) (DatabaseHost, string, error) {
	var host DatabaseHost
	var plaintextPassword, encryptedPassword string
	err := s.db.QueryRow(ctx, `
		SELECT h.id::text,
		       COALESCE(array_remove(array_agg(DISTINCT dhn.node_id::text), NULL), '{}'),
		       h.engine, h.name, h.host, h.port, h.username, COALESCE(h.password,''), COALESCE(h.password_encrypted,''),
		       h.tls_mode, h.tls_ca, h.tls_server_name, h.max_databases
		FROM database_hosts h
		LEFT JOIN database_host_node dhn ON dhn.database_host_id = h.id
		WHERE h.id = $1
		GROUP BY h.id
	`, hostID).Scan(&host.ID, &host.NodeIDs, &host.Engine, &host.Name, &host.Host, &host.Port, &host.Username, &plaintextPassword, &encryptedPassword, &host.TLSMode, &host.TLSCA, &host.TLSServerName, &host.MaxDatabases)
	if err != nil {
		return DatabaseHost{}, "", err
	}
	if len(host.NodeIDs) > 0 {
		host.NodeID = host.NodeIDs[0]
	}
	password, err := s.decryptSecret(encryptedPassword, plaintextPassword, secretAAD("database_hosts", host.ID, "password"))
	if err != nil {
		return DatabaseHost{}, "", err
	}
	return host, password, nil
}

func (s *Store) GetDatabaseHostForServerDatabase(ctx context.Context, databaseID string) (DatabaseHost, string, error) {
	var host DatabaseHost
	var plaintextPassword, encryptedPassword string
	err := s.db.QueryRow(ctx, `
		SELECT h.id::text,
		       COALESCE(array_remove(array_agg(DISTINCT dhn.node_id::text), NULL), '{}'),
		       h.engine, h.name, h.host, h.port, h.username, COALESCE(h.password,''), COALESCE(h.password_encrypted,''),
		       h.tls_mode, h.tls_ca, h.tls_server_name, h.max_databases
		FROM database_hosts h
		JOIN server_databases d ON d.database_host_id = h.id
		LEFT JOIN database_host_node dhn ON dhn.database_host_id = h.id
		WHERE d.id = $1
		GROUP BY h.id
	`, databaseID).Scan(&host.ID, &host.NodeIDs, &host.Engine, &host.Name, &host.Host, &host.Port, &host.Username, &plaintextPassword, &encryptedPassword, &host.TLSMode, &host.TLSCA, &host.TLSServerName, &host.MaxDatabases)
	if err != nil {
		return DatabaseHost{}, "", err
	}
	if len(host.NodeIDs) > 0 {
		host.NodeID = host.NodeIDs[0]
	}
	password, err := s.decryptSecret(encryptedPassword, plaintextPassword, secretAAD("database_hosts", host.ID, "password"))
	if err != nil {
		return DatabaseHost{}, "", err
	}
	return host, password, nil
}

func validateRequestedDatabaseIdentifier(input string) (string, error) {
	value := strings.TrimSpace(input)
	if !databaseIdentifierPattern.MatchString(value) || len(value) > 24 {
		return "", errors.New("database name must start with a letter and contain only letters, digits, or underscores (maximum 24 characters)")
	}
	return value, nil
}

func ValidateDatabaseIdentifier(value string) error {
	if !databaseIdentifierPattern.MatchString(value) {
		return errors.New("invalid database identifier")
	}
	return nil
}

func ValidateDatabaseRemote(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "%"
	}
	if value == "%" || net.ParseIP(value) != nil || (len(value) <= 253 && hostnamePattern.MatchString(value)) {
		return value, nil
	}
	return "", errors.New("remote must be %, an IP address, or a hostname without wildcard patterns")
}

func normalizeDatabaseEngine(engine string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "postgres", "postgresql":
		return "postgresql", nil
	case "mysql", "mariadb":
		return "mysql", nil
	default:
		return "", errors.New("engine must be postgres/postgresql or mysql/mariadb")
	}
}

func validateDatabaseHostRequest(engine, name, host *string, port *int, username, tlsMode, tlsCA, tlsServerName *string, maxDatabases *int) error {
	var err error
	*engine, err = normalizeDatabaseEngine(*engine)
	if err != nil {
		return err
	}
	*name, *host, *username = strings.TrimSpace(*name), strings.TrimSpace(*host), strings.TrimSpace(*username)
	*tlsMode, *tlsCA, *tlsServerName = strings.ToLower(strings.TrimSpace(*tlsMode)), strings.TrimSpace(*tlsCA), strings.TrimSpace(*tlsServerName)
	if *name == "" || len(*name) > 128 {
		return errors.New("name is required and must not exceed 128 characters")
	}
	if net.ParseIP(*host) == nil && (len(*host) > 253 || !hostnamePattern.MatchString(*host)) {
		return errors.New("host must be a valid hostname or IP address")
	}
	if !adminUsernamePattern.MatchString(*username) {
		return errors.New("invalid database administrator username")
	}
	if *port == 0 {
		if *engine == "mysql" {
			*port = 3306
		} else {
			*port = 5432
		}
	}
	if *port < 1 || *port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if *tlsMode == "" {
		*tlsMode = "verify-full"
	}
	switch *tlsMode {
	case "disable", "required", "verify-ca", "verify-full":
	default:
		return errors.New("tlsMode must be disable, required, verify-ca, or verify-full")
	}
	if *tlsServerName != "" && net.ParseIP(*tlsServerName) == nil && !hostnamePattern.MatchString(*tlsServerName) {
		return errors.New("tlsServerName must be a valid hostname or IP address")
	}
	if *tlsCA != "" {
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM([]byte(*tlsCA)) {
			return errors.New("tlsCa must contain a valid PEM certificate")
		}
	}
	if maxDatabases != nil && *maxDatabases < 1 {
		return errors.New("maxDatabases must be at least 1 when specified")
	}
	return nil
}

// AttachDatabaseHostNode links a node to a database host via the pivot table.
func (s *Store) AttachDatabaseHostNode(ctx context.Context, hostID, nodeID string) error {
	if _, err := uuid.Parse(nodeID); err != nil {
		return errors.New("invalid node id")
	}
	_, err := s.db.Exec(ctx, `INSERT INTO database_host_node (database_host_id, node_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, hostID, nodeID)
	return err
}

// DetachDatabaseHostNode removes a node link from a database host.
func (s *Store) DetachDatabaseHostNode(ctx context.Context, hostID, nodeID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM database_host_node WHERE database_host_id = $1 AND node_id = $2`, hostID, nodeID)
	return err
}

// ListDatabaseHostNodes returns node IDs linked to a database host.
func (s *Store) ListDatabaseHostNodes(ctx context.Context, hostID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT node_id::text FROM database_host_node WHERE database_host_id = $1 ORDER BY node_id`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
