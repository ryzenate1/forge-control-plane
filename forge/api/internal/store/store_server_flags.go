package store

import (
	"context"
	"encoding/json"
	"errors"
)

type ServerFlags struct {
	AutoStart   bool `json:"autoStart"`
	AutoRestart bool `json:"autoRestart"`
}

func (s *Store) GetServerFlags(ctx context.Context, serverID string) (ServerFlags, error) {
	var rawFlags []byte
	err := s.db.QueryRow(ctx, `SELECT COALESCE(flags, '{}'::jsonb) FROM servers WHERE id = $1`, serverID).Scan(&rawFlags)
	if err != nil {
		return ServerFlags{}, errors.New("server not found")
	}
	var flags ServerFlags
	if len(rawFlags) > 0 {
		_ = json.Unmarshal(rawFlags, &flags)
	}
	return flags, nil
}

func (s *Store) SetServerFlags(ctx context.Context, serverID string, flags ServerFlags) error {
	raw, err := json.Marshal(flags)
	if err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, `UPDATE servers SET flags = $1 WHERE id = $2`, string(raw), serverID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	return nil
}
