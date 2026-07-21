package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh"
)

var (
	bcryptCost     int
	bcryptCostOnce sync.Once
)

// BcryptCost returns the configurable bcrypt hashing cost.
// The cost can be set via the BCRYPT_COST environment variable (default: 10, min: 4, max: 31).
func BcryptCost() int {
	bcryptCostOnce.Do(func() {
		bcryptCost = bcrypt.DefaultCost
		if v := os.Getenv("BCRYPT_COST"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= bcrypt.MinCost && n <= bcrypt.MaxCost {
				bcryptCost = n
			}
		}
	})
	return bcryptCost
}

func (s *Store) Authenticate(ctx context.Context, email, password string) (User, error) {
	var user User
	var hash string
	var useTotp bool
	err := s.db.QueryRow(ctx, `
		SELECT u.id::text, u.email,
		       CASE WHEN COALESCE(r.is_admin, FALSE) THEN 'admin' ELSE COALESCE(r.key, 'user') END AS role,
		       u.password_hash, u.use_totp, u.session_version, u.disabled
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		WHERE lower(u.email) = lower($1) AND NOT u.disabled
		ORDER BY r.is_admin DESC NULLS LAST, ur.assigned_at ASC NULLS LAST
		LIMIT 1
	`, email).
		Scan(&user.ID, &user.Email, &user.Role, &hash, &useTotp, &user.SessionVersion, &user.Disabled)
	if err != nil {
		return User{}, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return User{}, errors.New("invalid credentials")
	}
	user.UseTOTP = useTotp
	return user, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.Query(ctx, `
		SELECT u.id::text, u.email,
		       CASE WHEN COALESCE(r.is_admin, FALSE) THEN 'admin' ELSE COALESCE(r.key, 'user') END AS role,
		       COALESCE(u.cpu_limit, 0), COALESCE(u.memory_mb_limit, 0),
		       COALESCE(u.disk_mb_limit, 0), COALESCE(u.backup_limit, 0),
		       COALESCE(u.database_limit, 0), COALESCE(u.allocation_limit, 0),
		       COALESCE(u.subuser_limit, 0), COALESCE(u.schedule_limit, 0),
		       COALESCE(u.server_limit, 0)
		FROM users u
		LEFT JOIN LATERAL (
			SELECT r.key, r.is_admin
			FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = u.id
			ORDER BY r.is_admin DESC, ur.assigned_at ASC
			LIMIT 1
		) r ON TRUE
		ORDER BY u.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Email, &user.Role,
			&user.CPULimit, &user.MemoryMBLimit, &user.DiskMBLimit,
			&user.BackupLimit, &user.DatabaseLimit, &user.AllocationLimit,
			&user.SubuserLimit, &user.ScheduleLimit, &user.ServerLimit); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) CreateUser(ctx context.Context, req CreateUserRequest, actorID *string) (User, error) {
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" || req.Password == "" {
		return User{}, errors.New("email and password are required")
	}
	if err := ValidatePassword(req.Password); err != nil {
		return User{}, err
	}
	if req.Role == "" {
		req.Role = "user"
	}
	if req.Role != "admin" && req.Role != "user" {
		return User{}, errors.New("invalid role")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), BcryptCost())
	if err != nil {
		return User{}, err
	}
	user := User{ID: uuid.NewString(), Email: email, Role: req.Role}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO users (
			id, email, password_hash, role,
			cpu_limit, memory_mb_limit, disk_mb_limit,
			backup_limit, database_limit, allocation_limit,
			subuser_limit, schedule_limit, server_limit
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, user.ID, user.Email, string(hash), user.Role,
		req.CPULimit, req.MemoryMBLimit, req.DiskMBLimit,
		req.BackupLimit, req.DatabaseLimit, req.AllocationLimit,
		req.SubuserLimit, req.ScheduleLimit, req.ServerLimit)
	if err != nil {
		return User{}, err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, r.id FROM roles r WHERE r.key = $2
		ON CONFLICT (user_id, role_id) DO NOTHING
	`, user.ID, user.Role); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	_ = s.AppendAudit(ctx, actorID, "user created", "user", &user.ID, fmt.Sprintf(`{"email":"%s","role":"%s"}`, user.Email, user.Role))
	// Re-fetch so the response carries the freshly-populated resource limits.
	return s.GetUserByID(ctx, user.ID)
}

func (s *Store) ListServerSubusers(ctx context.Context, serverID string) ([]ServerSubuser, error) {
	rows, err := s.db.Query(ctx, `
		SELECT su.id::text, su.server_id::text, su.user_id::text, u.email, su.permissions,
		       su.created_at, COALESCE(su.updated_at, su.created_at)
		FROM subusers su
		JOIN users u ON u.id = su.user_id
		WHERE su.server_id = $1
		ORDER BY u.email
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	subusers := []ServerSubuser{}
	for rows.Next() {
		var subuser ServerSubuser
		var raw []byte
		if err := rows.Scan(&subuser.ID, &subuser.ServerID, &subuser.UserID, &subuser.Email, &raw, &subuser.CreatedAt, &subuser.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &subuser.Permissions)
		subusers = append(subusers, subuser)
	}
	return subusers, rows.Err()
}

func (s *Store) UpsertServerSubuser(ctx context.Context, serverID string, req UpsertServerSubuserRequest, actorID *string) (ServerSubuser, error) {
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		return ServerSubuser{}, errors.New("email is required")
	}
	permissions := normalizeSubuserPermissions(req.Permissions)
	if len(permissions) == 0 {
		return ServerSubuser{}, errors.New("at least one permission is required")
	}
	raw, err := json.Marshal(permissions)
	if err != nil {
		return ServerSubuser{}, err
	}
	var userID string
	var ownerID string
	if err := s.db.QueryRow(ctx, `SELECT owner_id::text FROM servers WHERE id = $1`, serverID).Scan(&ownerID); err != nil {
		return ServerSubuser{}, errors.New("server not found")
	}
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM users WHERE lower(email) = $1`, email).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ServerSubuser{}, errors.New("user not found")
		}
		return ServerSubuser{}, err
	}
	if userID == ownerID {
		return ServerSubuser{}, errors.New("server owner cannot be added as a subuser")
	}
	subuserID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO subusers (id, server_id, user_id, permissions, created_at, updated_at)
		VALUES ($1, $2, $3, $4::jsonb, now(), now())
		ON CONFLICT (server_id, user_id) DO UPDATE
		SET permissions = EXCLUDED.permissions, updated_at = now()
	`, subuserID, serverID, userID, string(raw)); err != nil {
		return ServerSubuser{}, err
	}
	_ = s.AppendAudit(ctx, actorID, "server subuser upserted", "server", &serverID, fmt.Sprintf(`{"email":"%s"}`, email))
	return s.GetServerSubuser(ctx, serverID, userID)
}

func (s *Store) GetServerSubuser(ctx context.Context, serverID, userID string) (ServerSubuser, error) {
	var subuser ServerSubuser
	var raw []byte
	if err := s.db.QueryRow(ctx, `
		SELECT su.id::text, su.server_id::text, su.user_id::text, u.email, su.permissions,
		       su.created_at, COALESCE(su.updated_at, su.created_at)
		FROM subusers su
		JOIN users u ON u.id = su.user_id
		WHERE su.server_id = $1 AND su.user_id = $2
	`, serverID, userID).Scan(&subuser.ID, &subuser.ServerID, &subuser.UserID, &subuser.Email, &raw, &subuser.CreatedAt, &subuser.UpdatedAt); err != nil {
		return ServerSubuser{}, err
	}
	_ = json.Unmarshal(raw, &subuser.Permissions)
	return subuser, nil
}

func (s *Store) UserCanAccessServer(ctx context.Context, serverID, userID, role, permission string) (bool, error) {
	if role == "admin" {
		return true, nil
	}
	var ownerID string
	if err := s.db.QueryRow(ctx, `SELECT owner_id::text FROM servers WHERE id = $1`, serverID).Scan(&ownerID); err != nil {
		return false, err
	}
	if ownerID == userID {
		return true, nil
	}
	var raw []byte
	if err := s.db.QueryRow(ctx, `
		SELECT permissions
		FROM subusers
		WHERE server_id = $1 AND user_id = $2
	`, serverID, userID).Scan(&raw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	var permissions []string
	_ = json.Unmarshal(raw, &permissions)
	if permission == "" {
		return true, nil
	}
	return HasPermission(permissions, permission), nil
}

func (s *Store) DeleteServerSubuser(ctx context.Context, serverID, userID string, actorID *string) error {
	commandTag, err := s.db.Exec(ctx, `DELETE FROM subusers WHERE server_id = $1 AND user_id = $2`, serverID, userID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("subuser not found")
	}
	_ = s.AppendAudit(ctx, actorID, "server subuser deleted", "server", &serverID, fmt.Sprintf(`{"userId":"%s"}`, userID))
	return nil
}

func (s *Store) AuthenticateSFTP(ctx context.Context, nodeID, username, password string) (SFTPAuthResult, error) {
	username = strings.TrimSpace(strings.ToLower(username))
	idx := strings.LastIndex(username, ".")
	if idx <= 0 || idx == len(username)-1 {
		return SFTPAuthResult{}, errors.New("invalid username")
	}
	email := username[:idx]
	serverID := username[idx+1:]
	var result SFTPAuthResult
	var hash string
	var role string
	var ownerID string
	var serverStatus string
	if err := s.db.QueryRow(ctx, `
		SELECT u.id::text, u.password_hash,
		       CASE WHEN COALESCE(r.is_admin, FALSE) THEN 'admin' ELSE COALESCE(r.key, 'user') END,
		       s.id::text, s.owner_id::text, s.status, COALESCE(s.disk_mb, 0)
		FROM users u
		JOIN servers s ON s.id = $2 AND s.node_id = $3
		LEFT JOIN LATERAL (
			SELECT r.key, r.is_admin
			FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = u.id
			ORDER BY r.is_admin DESC, ur.assigned_at ASC
			LIMIT 1
		) r ON TRUE
		WHERE lower(u.email) = $1 AND NOT u.disabled
	`, email, serverID, nodeID).Scan(&result.UserID, &hash, &role, &result.ServerID, &ownerID, &serverStatus, &result.DiskLimitMB); err != nil {
		return SFTPAuthResult{}, errors.New("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return SFTPAuthResult{}, errors.New("invalid credentials")
	}
	if serverStatus == "deleted" || serverStatus == "installing" {
		return SFTPAuthResult{}, errors.New("server state does not allow sftp")
	}
	result.Suspended = serverStatus == "suspended"
	if result.Suspended {
		return SFTPAuthResult{}, errors.New("server is suspended")
	}
	if role == "admin" || result.UserID == ownerID {
		result.Permissions = []string{"*"}
		return result, nil
	}
	var raw []byte
	if err := s.db.QueryRow(ctx, `
		SELECT permissions
		FROM subusers
		WHERE server_id = $1 AND user_id = $2
	`, result.ServerID, result.UserID).Scan(&raw); err != nil {
		return SFTPAuthResult{}, errors.New("sftp access denied")
	}
	_ = json.Unmarshal(raw, &result.Permissions)
	for _, permission := range result.Permissions {
		if permission == "*" || permission == "file.sftp" {
			return result, nil
		}
	}
	return SFTPAuthResult{}, errors.New("sftp access denied")
}

func (s *Store) AuthenticateSFTPPublicKey(ctx context.Context, nodeID, username, supplied string) (SFTPAuthResult, error) {
	username = strings.TrimSpace(strings.ToLower(username))
	idx := strings.LastIndex(username, ".")
	if idx <= 0 || idx == len(username)-1 {
		return SFTPAuthResult{}, errors.New("invalid username")
	}
	email, serverID := username[:idx], username[idx+1:]
	var userID string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM users WHERE lower(email) = $1 AND NOT disabled`, email).Scan(&userID); err != nil {
		return SFTPAuthResult{}, errors.New("invalid credentials")
	}
	suppliedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(supplied)))
	if err != nil {
		return SFTPAuthResult{}, errors.New("invalid public key")
	}
	rows, err := s.db.Query(ctx, `SELECT public_key FROM user_ssh_keys WHERE user_id = $1`, userID)
	if err != nil {
		return SFTPAuthResult{}, errors.New("invalid credentials")
	}
	defer rows.Close()
	matched := false
	for rows.Next() {
		var stored string
		if rows.Scan(&stored) != nil {
			continue
		}
		key, _, _, _, parseErr := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(stored)))
		if parseErr == nil && bytes.Equal(key.Marshal(), suppliedKey.Marshal()) {
			matched = true
			break
		}
	}
	if !matched {
		return SFTPAuthResult{}, errors.New("invalid credentials")
	}
	return s.AuthorizeSFTPSession(ctx, nodeID, userID, serverID)
}

func (s *Store) AuthorizeSFTPSession(ctx context.Context, nodeID, userID, serverID string) (SFTPAuthResult, error) {
	result := SFTPAuthResult{UserID: userID, ServerID: serverID}
	var ownerID, status, role string
	if err := s.db.QueryRow(ctx, `
		SELECT s.owner_id::text, s.status, COALESCE(s.disk_mb, 0),
		       COALESCE((SELECT CASE WHEN r.is_admin THEN 'admin' ELSE r.key END FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = $1 ORDER BY r.is_admin DESC, ur.assigned_at ASC LIMIT 1), 'user')
		FROM servers s WHERE s.id = $2 AND s.node_id = $3
	`, userID, serverID, nodeID).Scan(&ownerID, &status, &result.DiskLimitMB, &role); err != nil {
		return SFTPAuthResult{}, errors.New("sftp access denied")
	}
	if status == "deleted" || status == "installing" {
		return SFTPAuthResult{}, errors.New("server state does not allow sftp")
	}
	result.Suspended = status == "suspended"
	if result.Suspended {
		return SFTPAuthResult{}, errors.New("server is suspended")
	}
	if role == "admin" || userID == ownerID {
		result.Permissions = []string{"*"}
		return result, nil
	}
	var raw []byte
	if err := s.db.QueryRow(ctx, `SELECT permissions FROM subusers WHERE server_id = $1 AND user_id = $2`, serverID, userID).Scan(&raw); err != nil {
		return SFTPAuthResult{}, errors.New("sftp access denied")
	}
	_ = json.Unmarshal(raw, &result.Permissions)
	for _, permission := range result.Permissions {
		if permission == "*" || permission == "file.sftp" {
			return result, nil
		}
	}
	return SFTPAuthResult{}, errors.New("sftp access denied")
}

func normalizeSubuserPermissions(input []string) []string {
	allowed := map[string]bool{}
	for _, permission := range defaultSubuserPermissions() {
		allowed[permission] = true
	}
	seen := map[string]bool{}
	output := []string{}
	for _, permission := range input {
		permission = strings.TrimSpace(permission)
		if permission == "" || seen[permission] || (!allowed[permission] && permission != "*") {
			continue
		}
		seen[permission] = true
		output = append(output, permission)
	}
	sort.Strings(output)
	return output
}

func defaultSubuserPermissions() []string {
	return []string{
		"websocket.connect",
		"control.console", "control.start", "control.stop", "control.restart",
		"user.read", "user.create", "user.update", "user.delete",
		"file.read", "file.create", "file.update", "file.delete", "file.archive", "file.sftp",
		"backup.read", "backup.create", "backup.delete",
		"allocation.read", "allocation.update",
		"startup.read", "startup.update",
		"database.read", "database.create", "database.update", "database.delete",
		"schedule.read", "schedule.create", "schedule.update", "schedule.delete",
		"settings.read", "settings.reinstall", "settings.description", "settings.change-icon",
		"mount.read", "mount.update",
	}
}

func (s *Store) GetUserByID(ctx context.Context, id string) (User, error) {
	var user User
	err := s.db.QueryRow(ctx, `
		SELECT u.id::text, u.email,
		       CASE WHEN COALESCE(r.is_admin, FALSE) THEN 'admin' ELSE COALESCE(r.key, 'user') END AS role,
		       COALESCE(u.cpu_limit, 0), COALESCE(u.memory_mb_limit, 0),
		       COALESCE(u.disk_mb_limit, 0), COALESCE(u.backup_limit, 0),
		       COALESCE(u.database_limit, 0), COALESCE(u.allocation_limit, 0),
		       COALESCE(u.subuser_limit, 0), COALESCE(u.schedule_limit, 0),
		       COALESCE(u.server_limit, 0), u.use_totp, u.session_version, u.disabled
		FROM users u
		LEFT JOIN LATERAL (
			SELECT r.key, r.is_admin
			FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = u.id
			ORDER BY r.is_admin DESC, ur.assigned_at ASC
			LIMIT 1
		) r ON TRUE
		WHERE u.id = $1 AND NOT u.disabled
	`, id).Scan(&user.ID, &user.Email, &user.Role,
		&user.CPULimit, &user.MemoryMBLimit, &user.DiskMBLimit,
		&user.BackupLimit, &user.DatabaseLimit, &user.AllocationLimit,
		&user.SubuserLimit, &user.ScheduleLimit, &user.ServerLimit,
		&user.UseTOTP, &user.SessionVersion, &user.Disabled)
	return user, err
}

func (s *Store) UpdateUser(ctx context.Context, userID string, req UpdateUserRequest, actorID *string) (User, error) {
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" {
		return User{}, errors.New("email is required")
	}
	if req.Role == "" {
		req.Role = "user"
	}
	if req.Role != "admin" && req.Role != "user" {
		return User{}, errors.New("invalid role")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)

	// Update password if provided. Also write any resource-limit changes.
	limitSets := []string{}
	limitArgs := []any{}
	if req.CPULimit != nil {
		limitSets = append(limitSets, fmt.Sprintf("cpu_limit = $%d", len(limitArgs)+1))
		limitArgs = append(limitArgs, *req.CPULimit)
	}
	if req.MemoryMBLimit != nil {
		limitSets = append(limitSets, fmt.Sprintf("memory_mb_limit = $%d", len(limitArgs)+1))
		limitArgs = append(limitArgs, *req.MemoryMBLimit)
	}
	if req.DiskMBLimit != nil {
		limitSets = append(limitSets, fmt.Sprintf("disk_mb_limit = $%d", len(limitArgs)+1))
		limitArgs = append(limitArgs, *req.DiskMBLimit)
	}
	if req.BackupLimit != nil {
		limitSets = append(limitSets, fmt.Sprintf("backup_limit = $%d", len(limitArgs)+1))
		limitArgs = append(limitArgs, *req.BackupLimit)
	}
	if req.DatabaseLimit != nil {
		limitSets = append(limitSets, fmt.Sprintf("database_limit = $%d", len(limitArgs)+1))
		limitArgs = append(limitArgs, *req.DatabaseLimit)
	}
	if req.AllocationLimit != nil {
		limitSets = append(limitSets, fmt.Sprintf("allocation_limit = $%d", len(limitArgs)+1))
		limitArgs = append(limitArgs, *req.AllocationLimit)
	}
	if req.SubuserLimit != nil {
		limitSets = append(limitSets, fmt.Sprintf("subuser_limit = $%d", len(limitArgs)+1))
		limitArgs = append(limitArgs, *req.SubuserLimit)
	}
	if req.ScheduleLimit != nil {
		limitSets = append(limitSets, fmt.Sprintf("schedule_limit = $%d", len(limitArgs)+1))
		limitArgs = append(limitArgs, *req.ScheduleLimit)
	}
	if req.ServerLimit != nil {
		limitSets = append(limitSets, fmt.Sprintf("server_limit = $%d", len(limitArgs)+1))
		limitArgs = append(limitArgs, *req.ServerLimit)
	}

	if strings.TrimSpace(req.Password) != "" {
		if err := ValidatePassword(req.Password); err != nil {
			return User{}, err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), BcryptCost())
		if err != nil {
			return User{}, err
		}
		if len(limitSets) > 0 {
			query := fmt.Sprintf(`
				UPDATE users SET email = $1, password_hash = $2, role = $3, session_version = session_version + 1, updated_at = now(), %s
				WHERE id = $%d
			`, strings.Join(limitSets, ", "), len(limitArgs)+4)
			args := append([]any{email, string(hash), req.Role}, limitArgs...)
			args = append(args, userID)
			_, err = tx.Exec(ctx, query, args...)
		} else {
			_, err = tx.Exec(ctx, `
				UPDATE users SET email = $1, password_hash = $2, role = $3, session_version = session_version + 1, updated_at = now()
									WHERE id = $4
			`, email, string(hash), req.Role, userID)
		}
		if err != nil {
			return User{}, err
		}
	} else if len(limitSets) > 0 {
		query := fmt.Sprintf(`
			UPDATE users SET email = $1, role = $2, updated_at = now(), %s
			WHERE id = $%d
		`, strings.Join(limitSets, ", "), len(limitArgs)+3)
		args := append([]any{email, req.Role}, limitArgs...)
		args = append(args, userID)
		if _, err = tx.Exec(ctx, query, args...); err != nil {
			return User{}, err
		}
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE users SET email = $1, role = $2, updated_at = now()
							WHERE id = $3
		`, email, req.Role, userID)
		if err != nil {
			return User{}, err
		}
	}

	// Update role mapping
	if _, err = tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return User{}, err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, r.id FROM roles r WHERE r.key = $2
	`, userID, req.Role); err != nil {
		return User{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}

	_ = s.AppendAudit(ctx, actorID, "user updated", "user", &userID, fmt.Sprintf(`{"email":"%s","role":"%s"}`, email, req.Role))
	return s.GetUserByID(ctx, userID)
}

func (s *Store) DeleteUser(ctx context.Context, userID string, actorID *string) error {
	// Check if user owns any servers
	var serverCount int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM servers WHERE owner_id = $1`, userID).Scan(&serverCount); err != nil {
		return err
	}
	if serverCount > 0 {
		return errors.New("cannot delete user who owns servers")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Delete subuser relationships
	if _, err := tx.Exec(ctx, `DELETE FROM subusers WHERE user_id = $1`, userID); err != nil {
		return err
	}

	// Delete role mappings
	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return err
	}

	// Delete user
	commandTag, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("user not found")
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	_ = s.AppendAudit(ctx, actorID, "user deleted", "user", &userID, `{}`)
	return nil
}

// GetUserByEmail looks up a user by their (case-insensitive) email address.
// The SELECT mirrors Authenticate: the LEFT JOIN user_roles/roles collapses
// to the highest-priority role a user holds via ORDER BY r.is_admin DESC NULLS
// LAST LIMIT 1. Returns pgx.ErrNoRows if no user matches.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := s.db.QueryRow(ctx, `
		SELECT u.id::text, u.email,
		       CASE WHEN COALESCE(r.is_admin, FALSE) THEN 'admin' ELSE COALESCE(r.key, 'user') END AS role,
		       u.session_version, u.disabled
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		WHERE lower(u.email) = lower($1) AND NOT u.disabled
		ORDER BY r.is_admin DESC NULLS LAST, ur.assigned_at ASC NULLS LAST
		LIMIT 1
	`, email).Scan(&user.ID, &user.Email, &user.Role, &user.SessionVersion, &user.Disabled)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if userID == "" {
		return errors.New("user id is required")
	}
	if currentPassword == "" || newPassword == "" {
		return errors.New("current and new password are required")
	}
	var email string
	if err := s.db.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email); err != nil {
		return errors.New("user not found")
	}
	if _, err := s.Authenticate(ctx, email, currentPassword); err != nil {
		return errors.New("current password is incorrect")
	}
	return s.UpdateUserPassword(ctx, userID, newPassword)
}

// UpdateUserPassword bcrypt-hashes newPassword and persists it for the given
// user. Both inputs must be non-empty. The caller is responsible for any
// ownership/current-password verification.
func (s *Store) UpdateUserPassword(ctx context.Context, userID, newPassword string) error {
	if userID == "" {
		return errors.New("user id is required")
	}
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		UPDATE users
				SET password_hash = $2, session_version = session_version + 1, updated_at = now()
				WHERE id = $1 AND NOT disabled
	`, userID, string(hash))
	return err
}

// UserSession represents a user authentication session
type UserSession struct {
	ID           string     `json:"id"`
	UserID       string     `json:"userId"`
	IPAddress    string     `json:"ipAddress"`
	UserAgent    string     `json:"userAgent"`
	LastActivity time.Time  `json:"lastActivity"`
	CreatedAt    time.Time  `json:"createdAt"`
	ExpiresAt    time.Time  `json:"expiresAt"`
	IsRevoked    bool       `json:"isRevoked"`
	RevokedAt    *time.Time `json:"revokedAt,omitempty"`
	RevokeReason string     `json:"revokeReason,omitempty"`
}

// CreateUserSession creates a new user session record
func (s *Store) CreateUserSession(ctx context.Context, userID, sessionTokenHash, ipAddress, userAgent string, ttl time.Duration) (string, error) {
	if userID == "" {
		return "", errors.New("user id is required")
	}
	if sessionTokenHash == "" {
		return "", errors.New("session token hash is required")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour // default 24 hours
	}

	id := uuid.NewString()
	expiresAt := time.Now().Add(ttl)

	_, err := s.db.Exec(ctx, `
		INSERT INTO user_sessions (id, user_id, session_token_hash, ip_address, user_agent, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), $6)
	`, id, userID, sessionTokenHash, ipAddress, userAgent, expiresAt)

	if err != nil {
		return "", err
	}

	return id, nil
}

// ListUserSessions returns all active sessions for a user
func (s *Store) ListUserSessions(ctx context.Context, userID string) ([]UserSession, error) {
	if userID == "" {
		return nil, errors.New("user id is required")
	}

	rows, err := s.db.Query(ctx, `
		SELECT id::text, user_id::text, ip_address, user_agent, last_activity, created_at, expires_at, is_revoked, revoked_at, revoke_reason
		FROM user_sessions
		WHERE user_id = $1
		ORDER BY last_activity DESC
	`, userID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []UserSession
	for rows.Next() {
		var session UserSession
		var revokedAt *time.Time

		err := rows.Scan(
			&session.ID,
			&session.UserID,
			&session.IPAddress,
			&session.UserAgent,
			&session.LastActivity,
			&session.CreatedAt,
			&session.ExpiresAt,
			&session.IsRevoked,
			&revokedAt,
			&session.RevokeReason,
		)

		if err != nil {
			return nil, err
		}

		session.RevokedAt = revokedAt
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// RevokeUserSession revokes a specific user session
func (s *Store) RevokeUserSession(ctx context.Context, userID, sessionID, reason string) error {
	if userID == "" {
		return errors.New("user id is required")
	}
	if sessionID == "" {
		return errors.New("session id is required")
	}

	result, err := s.db.Exec(ctx, `
		UPDATE user_sessions
		SET is_revoked = TRUE, revoked_at = NOW(), revoke_reason = $3
		WHERE id = $2 AND user_id = $1 AND NOT is_revoked
	`, userID, sessionID, reason)

	if err != nil {
		return err
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.New("session not found or already revoked")
	}

	return nil
}

// RevokeAllUserSessionsExceptCurrent revokes all sessions for a user except the specified one
func (s *Store) RevokeAllUserSessionsExceptCurrent(ctx context.Context, userID, exceptSessionID, reason string) error {
	if userID == "" {
		return errors.New("user id is required")
	}

	_, err := s.db.Exec(ctx, `
		UPDATE user_sessions
		SET is_revoked = TRUE, revoked_at = NOW(), revoke_reason = $3
		WHERE user_id = $1 AND id != $2 AND NOT is_revoked
	`, userID, exceptSessionID, reason)

	return err
}

// Password complexity errors
var (
	ErrPasswordTooShort  = errors.New("password must be at least 12 characters")
	ErrPasswordNoUpper   = errors.New("password must contain at least one uppercase letter")
	ErrPasswordNoLower   = errors.New("password must contain at least one lowercase letter")
	ErrPasswordNoDigit   = errors.New("password must contain at least one digit")
	ErrPasswordNoSpecial = errors.New("password must contain at least one special character")
)

// ValidatePassword checks password complexity requirements.
// Requires: minimum 12 chars, at least one uppercase, one lowercase, one digit, and one special character.
func ValidatePassword(password string) error {
	if len(password) < 12 {
		return ErrPasswordTooShort
	}
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case ch >= 'A' && ch <= 'Z':
			hasUpper = true
		case ch >= 'a' && ch <= 'z':
			hasLower = true
		case ch >= '0' && ch <= '9':
			hasDigit = true
		default:
			hasSpecial = true
		}
	}
	if !hasUpper {
		return ErrPasswordNoUpper
	}
	if !hasLower {
		return ErrPasswordNoLower
	}
	if !hasDigit {
		return ErrPasswordNoDigit
	}
	if !hasSpecial {
		return ErrPasswordNoSpecial
	}
	return nil
}

// CleanupExpiredSessions removes expired and old revoked sessions
func (s *Store) CleanupExpiredSessions(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		DELETE FROM user_sessions
		WHERE expires_at < NOW() OR (is_revoked = TRUE AND revoked_at < NOW() - INTERVAL '30 days')
	`)
	return err
}

// UpdateUserEmail updates a user's email address with verification
func (s *Store) UpdateUserEmail(ctx context.Context, userID, newEmail, currentPassword string) error {
	if userID == "" {
		return errors.New("user id is required")
	}
	if newEmail == "" {
		return errors.New("new email is required")
	}
	if currentPassword == "" {
		return errors.New("current password is required")
	}

	newEmail = strings.TrimSpace(strings.ToLower(newEmail))

	// Validate email format
	if !strings.Contains(newEmail, "@") || !strings.Contains(newEmail, ".") {
		return errors.New("invalid email format")
	}

	// Get current user and verify password
	var currentEmail, passwordHash string
	err := s.db.QueryRow(ctx, `
		SELECT email, password_hash FROM users WHERE id = $1 AND NOT disabled
	`, userID).Scan(&currentEmail, &passwordHash)

	if err != nil {
		return errors.New("user not found")
	}

	// Check if email is the same
	if currentEmail == newEmail {
		return errors.New("new email must be different from current email")
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(currentPassword)); err != nil {
		return errors.New("current password is incorrect")
	}

	// Check if new email already exists
	var exists bool
	err = s.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = lower($1) AND id != $2)
	`, newEmail, userID).Scan(&exists)

	if err != nil {
		return err
	}
	if exists {
		return errors.New("email already in use")
	}

	// Update email (could also trigger email verification here)
	_, err = s.db.Exec(ctx, `
		UPDATE users
		SET email = $2, updated_at = NOW()
		WHERE id = $1 AND NOT disabled
	`, userID, newEmail)

	return err
}

// CreatePasswordResetToken stores a single-use password reset token for the
// given user. tokenHash must be the SHA-256 hex digest of the plaintext token
// the caller intends to deliver out-of-band. The returned id is the row's UUID.
func (s *Store) CreatePasswordResetToken(ctx context.Context, userID, tokenHash string, ttl time.Duration, ip string) (string, error) {
	if userID == "" {
		return "", errors.New("user id is required")
	}
	if tokenHash == "" {
		return "", errors.New("token hash is required")
	}
	if ttl <= 0 {
		return "", errors.New("ttl must be positive")
	}
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, requested_ip)
		VALUES ($1, $2, $3, now() + $4::interval, $5)
	`, id, userID, tokenHash, ttl.String(), ip)
	if err != nil {
		return "", err
	}
	return id, nil
}

// ConsumePasswordResetToken marks a token as used in a transaction and returns
// the associated user_id. Validation rules:
//   - no row matching token_hash -> "token not found"
//   - used_at already set         -> "token has already been used"
//   - expires_at < now()         -> "token has expired"
//
// The row is locked with FOR UPDATE so concurrent callers cannot race on the
// single-use UPDATE.
func (s *Store) ConsumePasswordResetToken(ctx context.Context, tokenHash string) (string, error) {
	if tokenHash == "" {
		return "", errors.New("token hash is required")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var (
		id     string
		userID string
		usedAt *time.Time
		expAt  time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT id::text, user_id::text, used_at, expires_at
		FROM password_reset_tokens
		WHERE token_hash = $1
		FOR UPDATE
	`, tokenHash).Scan(&id, &userID, &usedAt, &expAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("token not found")
		}
		return "", err
	}
	if usedAt != nil {
		return "", errors.New("token has already been used")
	}
	if now := time.Now(); expAt.Before(now) {
		return "", errors.New("token has expired")
	}
	if _, err = tx.Exec(ctx, `
		UPDATE password_reset_tokens SET used_at = now() WHERE id = $1
	`, id); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return userID, nil
}

// ResetPasswordWithToken atomically validates and consumes a reset token,
// verifies its account email, updates the password, and revokes all sessions.
func (s *Store) ResetPasswordWithToken(ctx context.Context, tokenHash, email, newPassword string) (string, error) {
	if tokenHash == "" || strings.TrimSpace(email) == "" || newPassword == "" {
		return "", errors.New("token, email, and password are required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var id, userID, storedEmail string
	var usedAt *time.Time
	var expiresAt time.Time
	if err = tx.QueryRow(ctx, `
		SELECT prt.id::text, prt.user_id::text, prt.used_at, prt.expires_at, u.email
		FROM password_reset_tokens prt
		JOIN users u ON u.id = prt.user_id
		WHERE prt.token_hash = $1 AND NOT u.disabled
		FOR UPDATE OF prt, u
	`, tokenHash).Scan(&id, &userID, &usedAt, &expiresAt, &storedEmail); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("token not found")
		}
		return "", err
	}
	if usedAt != nil {
		return "", errors.New("token has already been used")
	}
	if time.Now().After(expiresAt) {
		return "", errors.New("token has expired")
	}
	if !strings.EqualFold(strings.TrimSpace(storedEmail), strings.TrimSpace(email)) {
		return "", errors.New("email does not match token")
	}
	if _, err = tx.Exec(ctx, `UPDATE password_reset_tokens SET used_at = now() WHERE id = $1`, id); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `
		UPDATE users
		SET password_hash = $2, session_version = session_version + 1, updated_at = now()
		WHERE id = $1
	`, userID, string(hash)); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return userID, nil
}
