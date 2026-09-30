package store

import (
	"context"
	"errors"
	"strings"
)

const searchUsersCols = `id::text, email, username, COALESCE(name_first, ''), COALESCE(name_last, ''), role, use_totp`

// SearchUsers returns a page of users matching the filter and the total
// number of rows that match. filter is matched case-insensitively against
// email and username. The (page, perPage) pair is 1-indexed.
//
// Branched queries (not a WHERE clause matching empty filter with OR):
// the OR form defeats index use on the list-all path. ORDER BY id ASC
// keeps OFFSET pagination stable.
func (s *Store) SearchUsers(ctx context.Context, filter string, page, perPage int) ([]User, int, error) {
	if s.db == nil {
		return nil, 0, errors.New("no database connection")
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 50
	}
	filter = strings.TrimSpace(filter)
	offset := (page - 1) * perPage

	var total int
	var rows pgxRows
	var err error
	if filter != "" {
		if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE email ILIKE '%' || $1 || '%' OR username ILIKE '%' || $1 || '%'`, filter).Scan(&total); err != nil {
			return nil, 0, err
		}
		rows, err = s.db.Query(ctx, `SELECT `+searchUsersCols+` FROM users WHERE email ILIKE '%' || $1 || '%' OR username ILIKE '%' || $1 || '%' ORDER BY id ASC LIMIT $3 OFFSET $2`, filter, offset, perPage)
	} else {
		if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
			return nil, 0, err
		}
		rows, err = s.db.Query(ctx, `SELECT `+searchUsersCols+` FROM users ORDER BY id ASC LIMIT $2 OFFSET $1`, offset, perPage)
	}
	if err != nil {
		return nil, 0, err
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.NameFirst, &u.NameLast, &u.Role, &u.UseTOTP); err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// UpdateAllocationAlias sets the alias (notes/ip_alias) on a single allocation.
func (s *Store) UpdateAllocationAlias(ctx context.Context, allocationID, alias string) error {
	if s.db == nil {
		return errors.New("no database connection")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE allocations
		SET ip_alias = NULLIF($2, '')
		WHERE id::text = $1
	`, allocationID, alias)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("allocation not found")
	}
	return nil
}

// DeleteNodeAllocation deletes a single allocation belonging to a node. The
// allocation must not be assigned to a server.
func (s *Store) DeleteNodeAllocation(ctx context.Context, nodeID, allocationID string) error {
	if s.db == nil {
		return errors.New("no database connection")
	}
	var serverID *string
	err := s.db.QueryRow(ctx, `SELECT server_id::text FROM allocations WHERE id::text = $1`, allocationID).Scan(&serverID)
	if err != nil {
		return err
	}
	if serverID != nil && *serverID != "" {
		return errors.New("cannot delete an allocation that is assigned to a server; unassign it first")
	}
	tag, err := s.db.Exec(ctx, `
		DELETE FROM allocations
		WHERE id::text = $1 AND node_id::text = $2
	`, allocationID, nodeID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("allocation not found on this node")
	}
	return nil
}
