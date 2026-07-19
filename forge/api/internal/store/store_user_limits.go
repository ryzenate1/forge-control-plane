package store

import (
	"context"
	"errors"
)

// ErrUserLimitExceeded is returned when an operation would push a user over
// one of their resource limits (cpu/memory/disk/backup/database/allocation/
// subuser/schedule/server).
type ErrUserLimitExceeded struct {
	Resource string
	Limit    int
	Current  int
}

func (e *ErrUserLimitExceeded) Error() string {
	return "user " + e.Resource + " limit exceeded"
}

// CheckUserCanCreateServer enforces per-user server limits and aggregate
// resource caps (memory, disk, cpu). Returns an error if the user has
// already reached their server count cap or if the aggregate would exceed
// any aggregate cap.
//
// limit=0 means unlimited for all caps.
func (s *Store) CheckUserCanCreateServer(ctx context.Context, userID string, memMB, diskMB, cpu int) error {
	if s == nil || s.db == nil {
		return nil
	}
	user, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return nil // user not found shouldn't block create flow (might be system-level call)
	}
	// Server count
	if user.ServerLimit > 0 {
		var count int
		if err := s.db.QueryRow(ctx, `SELECT count(*) FROM servers WHERE owner_id = $1`, userID).Scan(&count); err == nil {
			if count >= user.ServerLimit {
				return &ErrUserLimitExceeded{Resource: "server", Limit: user.ServerLimit, Current: count}
			}
		}
	}
	// Aggregate resource limits
	if user.MemoryMBLimit > 0 {
		var used int
		_ = s.db.QueryRow(ctx, `SELECT COALESCE(SUM(memory_mb),0) FROM servers WHERE owner_id = $1`, userID).Scan(&used)
		if used+memMB > user.MemoryMBLimit {
			return &ErrUserLimitExceeded{Resource: "memory", Limit: user.MemoryMBLimit, Current: used}
		}
	}
	if user.DiskMBLimit > 0 {
		var used int
		_ = s.db.QueryRow(ctx, `SELECT COALESCE(SUM(disk_mb),0) FROM servers WHERE owner_id = $1`, userID).Scan(&used)
		if used+diskMB > user.DiskMBLimit {
			return &ErrUserLimitExceeded{Resource: "disk", Limit: user.DiskMBLimit, Current: used}
		}
	}
	if user.CPULimit > 0 {
		var used int
		_ = s.db.QueryRow(ctx, `SELECT COALESCE(SUM(cpu_limit),0) FROM servers WHERE owner_id = $1`, userID).Scan(&used)
		if used+cpu > user.CPULimit {
			return &ErrUserLimitExceeded{Resource: "cpu", Limit: user.CPULimit, Current: used}
		}
	}
	return nil
}

// CheckUserCanCreateBackup enforces the per-user backup count cap.
func (s *Store) CheckUserCanCreateBackup(ctx context.Context, userID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	user, err := s.GetUserByID(ctx, userID)
	if err != nil || user.BackupLimit == 0 {
		return nil
	}
	var used int
	_ = s.db.QueryRow(ctx, `
		SELECT count(*) FROM backups b
		JOIN servers sv ON sv.id = b.server_id
		WHERE sv.owner_id = $1
	`, userID).Scan(&used)
	if used >= user.BackupLimit {
		return &ErrUserLimitExceeded{Resource: "backup", Limit: user.BackupLimit, Current: used}
	}
	return nil
}

// CheckUserCanCreateDatabase enforces the per-user database count cap.
func (s *Store) CheckUserCanCreateDatabase(ctx context.Context, userID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	user, err := s.GetUserByID(ctx, userID)
	if err != nil || user.DatabaseLimit == 0 {
		return nil
	}
	var used int
	_ = s.db.QueryRow(ctx, `
		SELECT count(*) FROM server_databases d
		JOIN servers sv ON sv.id = d.server_id
		WHERE sv.owner_id = $1
	`, userID).Scan(&used)
	if used >= user.DatabaseLimit {
		return &ErrUserLimitExceeded{Resource: "database", Limit: user.DatabaseLimit, Current: used}
	}
	return nil
}

// CheckUserCanCreateAllocation enforces the per-user allocation count cap.
func (s *Store) CheckUserCanCreateAllocation(ctx context.Context, userID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	user, err := s.GetUserByID(ctx, userID)
	if err != nil || user.AllocationLimit == 0 {
		return nil
	}
	var used int
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM servers WHERE owner_id = $1`, userID).Scan(&used)
	if used >= user.AllocationLimit {
		return &ErrUserLimitExceeded{Resource: "allocation", Limit: user.AllocationLimit, Current: used}
	}
	return nil
}

// CheckUserCanCreateSubuser enforces the per-user subuser count cap.
func (s *Store) CheckUserCanCreateSubuser(ctx context.Context, userID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	user, err := s.GetUserByID(ctx, userID)
	if err != nil || user.SubuserLimit == 0 {
		return nil
	}
	var used int
	_ = s.db.QueryRow(ctx, `
		SELECT count(*) FROM subusers WHERE owner_id = $1
	`, userID).Scan(&used)
	if used >= user.SubuserLimit {
		return &ErrUserLimitExceeded{Resource: "subuser", Limit: user.SubuserLimit, Current: used}
	}
	return nil
}

// CheckUserCanCreateSchedule enforces the per-user schedule count cap.
func (s *Store) CheckUserCanCreateSchedule(ctx context.Context, userID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	user, err := s.GetUserByID(ctx, userID)
	if err != nil || user.ScheduleLimit == 0 {
		return nil
	}
	var used int
	_ = s.db.QueryRow(ctx, `
		SELECT count(*) FROM schedules s
		JOIN servers sv ON sv.id = s.server_id
		WHERE sv.owner_id = $1
	`, userID).Scan(&used)
	if used >= user.ScheduleLimit {
		return &ErrUserLimitExceeded{Resource: "schedule", Limit: user.ScheduleLimit, Current: used}
	}
	return nil
}

// IsUserLimitError returns true if the error is a per-user limit-exceeded
// error. Use it in handlers to map to HTTP 422.
func IsUserLimitError(err error) bool {
	var e *ErrUserLimitExceeded
	return errors.As(err, &e)
}
