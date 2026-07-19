package commands

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type Type string

const (
	Start     Type = "start"
	Stop      Type = "stop"
	Restart   Type = "restart"
	Kill      Type = "kill"
	Install   Type = "install"
	Reinstall Type = "reinstall"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Operation struct {
	ID          string    `json:"id"`
	CommandID   string    `json:"commandId,omitempty"`
	ServerID    string    `json:"serverId"`
	Type        Type      `json:"type"`
	Status      Status    `json:"status"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
}

type Handler func(context.Context, *Operation) error

var (
	ErrQueueFull         = errors.New("operation queue is full")
	ErrQueueStopped      = errors.New("operation queue is stopped")
	ErrOperationNotFound = errors.New("operation not found")
)

type Queue struct {
	mu          sync.Mutex
	operations  map[string]*Operation
	serverOps   map[string][]string
	ch          chan *Operation
	concurrency int
	handler     Handler
	cancel      context.CancelFunc
	done        chan struct{}
	nextID      int
	db          *sql.DB
	started     bool
	closed      bool
}

func NewQueue(concurrency int, handler Handler) *Queue {
	return newQueue(concurrency, handler, nil)
}

func NewPersistentQueue(path string, concurrency int, handler Handler) (*Queue, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create command journal directory: %w", err)
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=5000&_journal_mode=WAL&_synchronous=FULL")
	if err != nil {
		return nil, fmt.Errorf("open command journal: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`CREATE TABLE IF NOT EXISTS beacon_operations (
		id TEXT PRIMARY KEY, command_id TEXT UNIQUE, server_id TEXT NOT NULL, type TEXT NOT NULL,
		status TEXT NOT NULL, error TEXT NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL,
		started_at TIMESTAMP, completed_at TIMESTAMP);
		CREATE INDEX IF NOT EXISTS idx_beacon_operations_status ON beacon_operations(status, created_at);
		CREATE INDEX IF NOT EXISTS idx_beacon_operations_server ON beacon_operations(server_id, created_at);`); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate command journal: %w", err)
	}
	q := newQueue(concurrency, handler, db)
	if err := q.loadJournal(); err != nil {
		db.Close()
		return nil, err
	}
	return q, nil
}

func newQueue(concurrency int, handler Handler, db *sql.DB) *Queue {
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 4 {
		concurrency = 4
	}
	return &Queue{operations: make(map[string]*Operation), serverOps: make(map[string][]string),
		ch: make(chan *Operation, 64), concurrency: concurrency, handler: handler, done: make(chan struct{}), db: db}
}

func (q *Queue) loadJournal() error {
	rows, err := q.db.Query(`SELECT id,COALESCE(command_id,''),server_id,type,status,error,created_at,started_at,completed_at
		FROM beacon_operations ORDER BY created_at`)
	if err != nil {
		return fmt.Errorf("load command journal: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var op Operation
		var typ, status string
		var started, completed sql.NullTime
		if err := rows.Scan(&op.ID, &op.CommandID, &op.ServerID, &typ, &status, &op.Error, &op.CreatedAt, &started, &completed); err != nil {
			return err
		}
		op.Type, op.Status = Type(typ), Status(status)
		if started.Valid {
			op.StartedAt = started.Time
		}
		if completed.Valid {
			op.CompletedAt = completed.Time
		}
		if op.Status == StatusRunning {
			op.Status = StatusPending
			op.StartedAt = time.Time{}
			op.Error = ""
			_ = q.persist(&op)
		}
		copyOp := op
		q.operations[op.ID] = &copyOp
		q.serverOps[op.ServerID] = append(q.serverOps[op.ServerID], op.ID)
	}
	return rows.Err()
}

func (q *Queue) Start(ctx context.Context) {
	q.mu.Lock()
	if q.started {
		q.mu.Unlock()
		return
	}
	q.started = true
	ctx, q.cancel = context.WithCancel(ctx)
	pending := make([]*Operation, 0)
	for _, op := range q.operations {
		if op.Status == StatusPending {
			pending = append(pending, op)
		}
	}
	q.mu.Unlock()
	var wg sync.WaitGroup
	for i := 0; i < q.concurrency; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); q.worker(ctx) }()
	}
	go func() { wg.Wait(); close(q.done) }()
	go func() {
		for _, op := range pending {
			select {
			case q.ch <- op:
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (q *Queue) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case op, ok := <-q.ch:
			if !ok {
				return
			}
			q.processOp(ctx, op)
		}
	}
}

func (q *Queue) processOp(ctx context.Context, op *Operation) {
	q.mu.Lock()
	if op.Status != StatusPending {
		q.mu.Unlock()
		return
	}
	op.Status = StatusRunning
	op.StartedAt = time.Now().UTC()
	_ = q.persist(op)
	q.mu.Unlock()
	var opErr string
	if q.handler != nil {
		if err := q.handler(ctx, op); err != nil {
			opErr = err.Error()
		}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.db != nil && ctx.Err() != nil {
		op.Status = StatusPending
		op.Error = ""
		op.StartedAt = time.Time{}
		op.CompletedAt = time.Time{}
		_ = q.persist(op)
		return
	}
	op.CompletedAt = time.Now().UTC()
	op.Error = opErr
	if opErr != "" {
		op.Status = StatusFailed
	} else {
		op.Status = StatusCompleted
	}
	_ = q.persist(op)
}

func (q *Queue) Enqueue(ctx context.Context, serverID string, typ Type) (*Operation, error) {
	return q.EnqueueCommand(ctx, "", serverID, typ)
}

func (q *Queue) EnqueueCommand(ctx context.Context, commandID, serverID string, typ Type) (*Operation, error) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil, ErrQueueStopped
	}
	if commandID != "" {
		for _, existing := range q.operations {
			if existing.CommandID == commandID {
				cp := *existing
				q.mu.Unlock()
				return &cp, nil
			}
		}
	}
	q.nextID++
	id := uuid.NewString()
	if q.db == nil {
		id = fmt.Sprintf("op-%d", q.nextID)
	}
	op := &Operation{ID: id, CommandID: commandID, ServerID: serverID, Type: typ, Status: StatusPending, CreatedAt: time.Now().UTC()}
	q.operations[id] = op
	q.serverOps[serverID] = append(q.serverOps[serverID], id)
	if err := q.persist(op); err != nil {
		delete(q.operations, id)
		q.serverOps[serverID] = q.serverOps[serverID][:len(q.serverOps[serverID])-1]
		q.mu.Unlock()
		return nil, err
	}
	q.mu.Unlock()
	select {
	case <-ctx.Done():
		return op, ctx.Err()
	case q.ch <- op:
		cp := *op
		return &cp, nil
	default:
		return op, ErrQueueFull
	}
}

func (q *Queue) persist(op *Operation) error {
	if q.db == nil {
		return nil
	}
	_, err := q.db.Exec(`INSERT INTO beacon_operations(id,command_id,server_id,type,status,error,created_at,started_at,completed_at)
		VALUES(?,NULLIF(?,''),?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET status=excluded.status,error=excluded.error,
		started_at=excluded.started_at,completed_at=excluded.completed_at`, op.ID, op.CommandID, op.ServerID, string(op.Type), string(op.Status), op.Error,
		op.CreatedAt, nullTime(op.StartedAt), nullTime(op.CompletedAt))
	return err
}

func nullTime(v time.Time) any {
	if v.IsZero() {
		return nil
	}
	return v
}

func (q *Queue) GetStatus(id string) (*Operation, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	op, ok := q.operations[id]
	if !ok {
		return nil, ErrOperationNotFound
	}
	cp := *op
	return &cp, nil
}
func (q *Queue) ListByServer(id string) []Operation {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Operation, 0, len(q.serverOps[id]))
	for _, opID := range q.serverOps[id] {
		if op := q.operations[opID]; op != nil {
			out = append(out, *op)
		}
	}
	return out
}

func (q *Queue) Shutdown() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	if q.db == nil {
		for _, op := range q.operations {
			if op.Status == StatusPending {
				op.Status = StatusCancelled
				op.CompletedAt = time.Now().UTC()
			}
		}
	}
	cancel := q.cancel
	started := q.started
	q.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if started {
		<-q.done
	}
	q.mu.Lock()
	for _, op := range q.operations {
		if op.Status == StatusPending || op.Status == StatusRunning {
			if q.db != nil {
				op.Status = StatusPending
				op.StartedAt = time.Time{}
				op.Error = ""
			} else {
				op.Status = StatusCancelled
				op.CompletedAt = time.Now().UTC()
			}
			_ = q.persist(op)
		}
	}
	q.mu.Unlock()
	if q.db != nil {
		_ = q.db.Close()
	}
}
