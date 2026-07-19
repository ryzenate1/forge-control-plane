package activity

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) InsertActivity(ctx context.Context, event *ActivityEvent) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO activity_events (id, event, description, actor_id, actor_email, actor_type, ip, user_agent, subject_type, subject_id, subject_name, properties, level, source, timestamp, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`,
		event.ID, event.Event, event.Description,
		event.ActorID, event.ActorEmail, event.ActorType,
		event.IP, event.UserAgent,
		event.SubjectType, event.SubjectID, event.SubjectName,
		event.Properties, event.Level, event.Source,
		event.Timestamp, event.ExpiresAt,
	)
	return err
}

func (s *Store) QueryActivities(ctx context.Context, filter ActivityFilter) ([]ActivityEvent, error) {
	query := `SELECT id, event, COALESCE(description, ''), actor_id, actor_email, actor_type, ip, user_agent, subject_type, subject_id, subject_name, properties, level, source, timestamp, expires_at FROM activity_events`
	conds := []string{}
	args := []any{}
	argN := 1

	if filter.ActorID != nil {
		conds = append(conds, fmt.Sprintf("actor_id = $%d", argN))
		args = append(args, *filter.ActorID)
		argN++
	}
	if filter.SubjectType != nil {
		conds = append(conds, fmt.Sprintf("subject_type = $%d", argN))
		args = append(args, *filter.SubjectType)
		argN++
	}
	if filter.SubjectID != nil {
		conds = append(conds, fmt.Sprintf("subject_id = $%d", argN))
		args = append(args, *filter.SubjectID)
		argN++
	}
	if filter.Event != nil {
		conds = append(conds, fmt.Sprintf("event = $%d", argN))
		args = append(args, *filter.Event)
		argN++
	}
	if filter.Level != nil {
		conds = append(conds, fmt.Sprintf("level = $%d", argN))
		args = append(args, *filter.Level)
		argN++
	}
	if filter.Source != nil {
		conds = append(conds, fmt.Sprintf("source = $%d", argN))
		args = append(args, *filter.Source)
		argN++
	}
	if filter.From != nil {
		conds = append(conds, fmt.Sprintf("timestamp >= $%d", argN))
		args = append(args, *filter.From)
		argN++
	}
	if filter.To != nil {
		conds = append(conds, fmt.Sprintf("timestamp <= $%d", argN))
		args = append(args, *filter.To)
		argN++
	}

	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY timestamp DESC"
	query += fmt.Sprintf(" LIMIT $%d", argN)
	args = append(args, filter.Limit)
	argN++
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argN)
		args = append(args, filter.Offset)
		argN++
	}

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []ActivityEvent{}
	for rows.Next() {
		var e ActivityEvent
		if err := rows.Scan(
			&e.ID, &e.Event, &e.Description,
			&e.ActorID, &e.ActorEmail, &e.ActorType,
			&e.IP, &e.UserAgent,
			&e.SubjectType, &e.SubjectID, &e.SubjectName,
			&e.Properties, &e.Level, &e.Source,
			&e.Timestamp, &e.ExpiresAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) CountActivities(ctx context.Context, filter ActivityFilter) (int, error) {
	query := `SELECT COUNT(*) FROM activity_events`
	conds := []string{}
	args := []any{}
	argN := 1

	if filter.ActorID != nil {
		conds = append(conds, fmt.Sprintf("actor_id = $%d", argN))
		args = append(args, *filter.ActorID)
		argN++
	}
	if filter.SubjectType != nil {
		conds = append(conds, fmt.Sprintf("subject_type = $%d", argN))
		args = append(args, *filter.SubjectType)
		argN++
	}
	if filter.SubjectID != nil {
		conds = append(conds, fmt.Sprintf("subject_id = $%d", argN))
		args = append(args, *filter.SubjectID)
		argN++
	}
	if filter.Event != nil {
		conds = append(conds, fmt.Sprintf("event = $%d", argN))
		args = append(args, *filter.Event)
		argN++
	}
	if filter.Level != nil {
		conds = append(conds, fmt.Sprintf("level = $%d", argN))
		args = append(args, *filter.Level)
		argN++
	}
	if filter.Source != nil {
		conds = append(conds, fmt.Sprintf("source = $%d", argN))
		args = append(args, *filter.Source)
		argN++
	}
	if filter.From != nil {
		conds = append(conds, fmt.Sprintf("timestamp >= $%d", argN))
		args = append(args, *filter.From)
		argN++
	}
	if filter.To != nil {
		conds = append(conds, fmt.Sprintf("timestamp <= $%d", argN))
		args = append(args, *filter.To)
		argN++
	}

	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}

	var count int
	err := s.db.QueryRow(ctx, query, args...).Scan(&count)
	return count, err
}

func (s *Store) CleanupActivities(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.Exec(ctx, `DELETE FROM activity_events WHERE timestamp < $1`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}

func (s *Store) GetActivityStats(ctx context.Context) (*ActivityStats, error) {
	stats := &ActivityStats{
		ByLevel: map[Level]int64{},
	}

	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM activity_events`).Scan(&stats.TotalEvents); err != nil {
		return nil, err
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM activity_events WHERE timestamp >= $1`, today).Scan(&stats.EventsToday); err != nil {
		return nil, err
	}

	hourAgo := time.Now().UTC().Add(-1 * time.Hour)
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM activity_events WHERE timestamp >= $1`, hourAgo).Scan(&stats.EventsThisHour); err != nil {
		return nil, err
	}

	if err := s.db.QueryRow(ctx, `SELECT COUNT(DISTINCT actor_id) FROM activity_events WHERE actor_id IS NOT NULL`).Scan(&stats.UniqueActors); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, `SELECT level, COUNT(*) FROM activity_events GROUP BY level`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var level Level
		var count int64
		if err := rows.Scan(&level, &count); err != nil {
			return nil, err
		}
		stats.ByLevel[level] = count
	}
	return stats, rows.Err()
}
