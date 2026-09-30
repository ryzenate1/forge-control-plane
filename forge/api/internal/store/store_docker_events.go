package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DockerEvent is one container lifecycle fact reported by a Beacon node:
// start, stop, die, kill, oom, recreate or destroy. It is the stored form of
// what Docker publishes on its event stream, keyed to the node that observed
// it (see migrations/229_a_docker_events.sql).
type DockerEvent struct {
	ID            int64  `json:"id"`
	NodeID        string `json:"nodeId"`
	NodeName      string `json:"nodeName,omitempty"`
	EventType     string `json:"eventType"`
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName"`
	Image         string `json:"image"`
	// ActorAttrs mirrors events.Message.Actor.Attributes. Docker types it as
	// map[string]string, so the JSON round trip stays lossless for real
	// producers; a nil map means "no attributes", never "unknown".
	ActorAttrs map[string]string `json:"actorAttributes"`
	Timestamp  time.Time         `json:"timestamp"`
	IngestedAt time.Time         `json:"ingestedAt"`
}

// DockerEventFilter narrows a timeline query. Empty string fields are
// "no filter"; the time bounds are inclusive of From and exclusive of To so
// successive pages cannot double-report an event that straddles a boundary.
type DockerEventFilter struct {
	NodeID    string
	EventType string
	// Container matches a case-insensitive substring of either the container
	// name or (short) id, which is how an operator searches a feed.
	Container string
	From      *time.Time
	To        *time.Time
	Limit     int
	Offset    int
}

// dockerEventColumns is the single projection the timeline reads, so the list
// query and any future sibling cannot disagree about what a "docker event" row
// contains. Node name is joined rather than duplicated into the table.
const dockerEventColumns = `de.id, de.node_id::text, COALESCE(n.name, ''), de.event_type,
		de.container_id, de.container_name, de.image, de.actor_attrs, de."timestamp", de.ingested_at`

// maxDockerEventInsertRows bounds one ingest batch. Beacon flushes at most a
// few hundred events per request, so this only guards against a client (or a
// bug) shipping enough rows to build an unwieldy statement.
const maxDockerEventInsertRows = 500

// dockerEventInsertChunk keeps each generated VALUES list small enough to stay
// well under Postgres' 65535-parameter ceiling.
const dockerEventInsertChunk = 200

// InsertDockerEvents persists a batch of node-reported events.
//
// Duplicate (node, container, type, timestamp) rows are discarded rather than
// erroring: Beacon delivery is at-least-once, so a reconnect replaying its
// in-memory buffer is expected and must not fail the whole ingest. An empty
// slice is a no-op, and every error returned is wrapped so the caller can say
// which row count it lost.
func (s *Store) InsertDockerEvents(ctx context.Context, events []DockerEvent) error {
	if len(events) == 0 {
		return nil
	}
	if len(events) > maxDockerEventInsertRows {
		// Failing the whole batch is better than silently storing part of it: the
		// caller can split, and the timeline never sees a partial flush.
		return fmt.Errorf("docker event batch of %d exceeds the %d row limit", len(events), maxDockerEventInsertRows)
	}
	for index, event := range events {
		if strings.TrimSpace(event.NodeID) == "" {
			return fmt.Errorf("docker event %d: node id is required", index)
		}
		if strings.TrimSpace(event.EventType) == "" {
			return fmt.Errorf("docker event %d: event type is required", index)
		}
	}

	for start := 0; start < len(events); start += dockerEventInsertChunk {
		end := start + dockerEventInsertChunk
		if end > len(events) {
			end = len(events)
		}
		chunk := events[start:end]
		query, args, buildErr := buildDockerEventInsert(chunk)
		if buildErr != nil {
			return fmt.Errorf("prepare docker event batch at offset %d: %w", start, buildErr)
		}
		if _, execErr := s.db.Exec(ctx, query, args...); execErr != nil {
			return fmt.Errorf("insert %d docker events at offset %d: %w", len(chunk), start, execErr)
		}
	}
	return nil
}

// buildDockerEventInsert renders one VALUES-based INSERT for a chunk of rows.
//
// actor_attrs is sent as JSON text and cast in SQL, because pgx cannot infer
// the jsonb type from an untyped parameter and Postgres will not accept text
// where jsonb is required without the cast.
func buildDockerEventInsert(events []DockerEvent) (string, []any, error) {
	var builder strings.Builder
	builder.WriteString(`INSERT INTO docker_events (
			node_id, event_type, container_id, container_name, image, actor_attrs, "timestamp", ingested_at
		) VALUES `)
	args := make([]any, 0, len(events)*8)
	placeholder := 1
	for index, event := range events {
		if index > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d::jsonb,$%d,$%d)",
			placeholder, placeholder+1, placeholder+2, placeholder+3, placeholder+4, placeholder+5, placeholder+6, placeholder+7))
		placeholder += 8

		attrs := "{}"
		if len(event.ActorAttrs) > 0 {
			encoded, err := json.Marshal(event.ActorAttrs)
			if err != nil {
				return "", nil, fmt.Errorf("encode actor attributes for event %d: %w", index, err)
			}
			attrs = string(encoded)
		}
		timestamp := event.Timestamp.UTC()
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
		}
		ingestedAt := event.IngestedAt.UTC()
		if ingestedAt.IsZero() {
			ingestedAt = time.Now().UTC()
		}
		args = append(args, event.NodeID, event.EventType, event.ContainerID, event.ContainerName, event.Image, attrs, timestamp, ingestedAt)
	}
	builder.WriteString(` ON CONFLICT DO NOTHING`)
	return builder.String(), args, nil
}

// dockerEventWhere builds the shared WHERE clause for the list and count
// queries, returning the argument slice in placeholder order.
func dockerEventWhere(filter DockerEventFilter) (string, []any) {
	clauses := []string{"1=1"}
	args := make([]any, 0, 5)
	if value := strings.TrimSpace(filter.NodeID); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("de.node_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.EventType); value != "" {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("de.event_type = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.Container); value != "" {
		args = append(args, "%"+value+"%")
		clauses = append(clauses, fmt.Sprintf("(de.container_name ILIKE $%d OR de.container_id ILIKE $%d)", len(args), len(args)))
	}
	if filter.From != nil {
		args = append(args, filter.From.UTC())
		clauses = append(clauses, fmt.Sprintf(`de."timestamp" >= $%d`, len(args)))
	}
	if filter.To != nil {
		args = append(args, filter.To.UTC())
		clauses = append(clauses, fmt.Sprintf(`de."timestamp" < $%d`, len(args)))
	}
	return strings.Join(clauses, " AND "), args
}

// ListDockerEvents returns the newest matching events first, bounded and
// offset for pagination. An empty result is an empty slice, never nil, so the
// feed renders "no events" instead of a missing key.
func (s *Store) ListDockerEvents(ctx context.Context, filter DockerEventFilter) ([]DockerEvent, error) {
	limit, offset := dockerEventPaging(filter)
	where, args := dockerEventWhere(filter)
	query := fmt.Sprintf(`SELECT %s
		FROM docker_events de
		LEFT JOIN nodes n ON n.id = de.node_id
		WHERE %s
		ORDER BY de."timestamp" DESC, de.id DESC
		LIMIT $%d OFFSET $%d`, dockerEventColumns, where, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list docker events: %w", err)
	}
	defer rows.Close()

	result := []DockerEvent{}
	for rows.Next() {
		event, err := scanDockerEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan docker event: %w", err)
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

// CountDockerEvents reports how many events match the filter, ignoring paging,
// so the admin timeline can show a real total instead of a page length.
func (s *Store) CountDockerEvents(ctx context.Context, filter DockerEventFilter) (int64, error) {
	where, args := dockerEventWhere(filter)
	var total int64
	query := fmt.Sprintf(`SELECT COUNT(*) FROM docker_events de WHERE %s`, where)
	if err := s.db.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("count docker events: %w", err)
	}
	return total, nil
}

// dockerEventPaging applies the same defaults and ceiling as the rest of the
// metrics/list stores: a sane page when the caller omits one, and a hard cap
// so `limit=1000000` cannot turn the feed into a full-table read.
func dockerEventPaging(filter DockerEventFilter) (int, int) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

type dockerEventScanner interface {
	Scan(dest ...any) error
}

// scanDockerEvent reads one row. Every projected column is either NOT NULL in
// the schema or wrapped in COALESCE, so plain string targets are honest here —
// no null-string plumbing.
func scanDockerEvent(row dockerEventScanner) (DockerEvent, error) {
	var (
		event    DockerEvent
		rawAttrs []byte
	)
	err := row.Scan(&event.ID, &event.NodeID, &event.NodeName, &event.EventType,
		&event.ContainerID, &event.ContainerName, &event.Image, &rawAttrs, &event.Timestamp, &event.IngestedAt)
	if err != nil {
		return DockerEvent{}, err
	}
	if len(rawAttrs) > 0 {
		attrs := map[string]string{}
		// Undecodable attributes are not worth failing a whole page over: the
		// row still carries the type, container and timestamp that matter.
		if json.Unmarshal(rawAttrs, &attrs) == nil {
			event.ActorAttrs = attrs
		}
	}
	return event, nil
}

// PruneDockerEvents deletes ingested events whose Docker timestamp is older
// than before, returning how many rows went away. The retention sweep in
// handlers_docker_events.go calls this hourly with now-30d.
func (s *Store) PruneDockerEvents(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.Exec(ctx, `DELETE FROM docker_events WHERE "timestamp" < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("prune docker events: %w", err)
	}
	return res.RowsAffected(), nil
}
