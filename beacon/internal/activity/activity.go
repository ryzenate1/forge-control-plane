package activity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Activity struct {
	ID        int64
	Event     string
	User      string
	ServerID  string
	IP        string
	Timestamp time.Time
	Metadata  map[string]interface{}
}

func (a Activity) MetadataJSON() string {
	if len(a.Metadata) == 0 {
		return "{}"
	}
	data, err := json.Marshal(a.Metadata)
	if err != nil {
		return "{}"
	}
	return string(data)
}

type Sender interface {
	SendActivityLogs(context.Context, []Activity) error
}

type Database struct {
	db *sql.DB
}

func NewDatabase(path string) (*Database, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS activities (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event TEXT NOT NULL,
			user TEXT NOT NULL DEFAULT '',
			server_id TEXT NOT NULL,
			ip TEXT NOT NULL DEFAULT '',
			timestamp DATETIME NOT NULL,
			metadata TEXT NOT NULL DEFAULT '{}'
		)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Database{db: db}, nil
}

func (d *Database) Record(ctx context.Context, activity Activity) error {
	if d == nil || d.db == nil {
		return errors.New("activity database is not initialized")
	}
	_, err := d.db.ExecContext(ctx, `
		INSERT INTO activities (event, user, server_id, ip, timestamp, metadata)
		VALUES (?, ?, ?, ?, ?, ?)`, activity.Event, activity.User, activity.ServerID,
		activity.IP, activity.Timestamp.UTC(), activity.MetadataJSON())
	return err
}

func (d *Database) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

func (d *Database) Flush(ctx context.Context, sender Sender) error {
	if d == nil || d.db == nil || sender == nil {
		return nil
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, event, user, server_id, ip, timestamp, metadata
		FROM activities ORDER BY id ASC LIMIT 100`)
	if err != nil {
		return err
	}
	defer rows.Close()

	activities := make([]Activity, 0, 100)
	for rows.Next() {
		var activity Activity
		var metadata string
		if err := rows.Scan(&activity.ID, &activity.Event, &activity.User, &activity.ServerID,
			&activity.IP, &activity.Timestamp, &metadata); err != nil {
			return err
		}
		_ = json.Unmarshal([]byte(metadata), &activity.Metadata)
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil || len(activities) == 0 {
		return err
	}
	if err := sender.SendActivityLogs(ctx, activities); err != nil {
		return err
	}

	ids := make([]interface{}, len(activities))
	query := "DELETE FROM activities WHERE id IN (?"
	for i, activity := range activities {
		ids[i] = activity.ID
		if i > 0 {
			query += ",?"
		}
	}
	query += ")"
	_, err = d.db.ExecContext(ctx, query, ids...)
	return err
}
