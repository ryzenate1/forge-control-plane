package queue

import (
	"context"
	"io/fs"
	"time"
)

type Driver[TTx any] interface {
	GetExecutor() Executor
	GetListener() Listener
	GetMigrationFS(line string) fs.FS
	GetMigrationLines() []string
	SupportsListener() bool
	SupportsListenNotify() bool
	TimePrecision() time.Duration
	UnwrapExecutor(tx TTx) ExecutorTx
}

type Executor interface {
	Begin(ctx context.Context) (ExecutorTx, error)
	Exec(ctx context.Context, sql string, args ...any) error

	JobCancel(ctx context.Context, params *JobCancelParams) (*JobRow, error)
	JobGetAvailable(ctx context.Context, params *JobGetAvailableParams) ([]*JobRow, error)
	JobGetByID(ctx context.Context, params *JobGetByIDParams) (*JobRow, error)
	JobGetStuck(ctx context.Context, params *JobGetStuckParams) ([]*JobRow, error)
	JobInsertFull(ctx context.Context, params *JobInsertFullParams) (*JobRow, error)
	JobInsertFullMany(ctx context.Context, jobs *JobInsertFullManyParams) ([]*JobRow, error)
	JobList(ctx context.Context, params *JobListParams) ([]*JobRow, error)
	JobRescueMany(ctx context.Context, params *JobRescueManyParams) error
	JobRetry(ctx context.Context, params *JobRetryParams) (*JobRow, error)
	JobSchedule(ctx context.Context, params *JobScheduleParams) ([]*JobScheduleResult, error)
	JobSetStateIfRunningMany(ctx context.Context, params *JobSetStateIfRunningManyParams) ([]*JobRow, error)
	JobUpdate(ctx context.Context, params *JobUpdateParams) (*JobRow, error)
	JobDelete(ctx context.Context, params *JobDeleteParams) (*JobRow, error)
	JobDeleteBefore(ctx context.Context, params *JobDeleteBeforeParams) (int, error)
	JobDeleteMany(ctx context.Context, params *JobDeleteManyParams) ([]*JobRow, error)
	JobGetByIDMany(ctx context.Context, params *JobGetByIDManyParams) ([]*JobRow, error)

	LeaderAttemptElect(ctx context.Context, params *LeaderElectParams) (*Leader, error)
	LeaderAttemptReelect(ctx context.Context, params *LeaderReelectParams) (*Leader, error)
	LeaderDeleteExpired(ctx context.Context, params *LeaderDeleteExpiredParams) (int, error)
	LeaderGetElectedLeader(ctx context.Context, params *LeaderGetElectedLeaderParams) (*Leader, error)
	LeaderResign(ctx context.Context, params *LeaderResignParams) (bool, error)

	NotifyMany(ctx context.Context, params *NotifyManyParams) error

	QueueCreateOrSetUpdatedAt(ctx context.Context, params *QueueCreateOrSetUpdatedAtParams) error
	QueueDeleteExpired(ctx context.Context, params *QueueDeleteExpiredParams) ([]string, error)
	QueueGet(ctx context.Context, params *QueueGetParams) (string, error)
	QueueList(ctx context.Context, params *QueueListParams) ([]string, error)
	QueuePause(ctx context.Context, params *QueuePauseParams) error
	QueueResume(ctx context.Context, params *QueueResumeParams) error

	QueryRow(ctx context.Context, sql string, args ...any) Row
}

type ExecutorTx interface {
	Executor
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type Listener interface {
	Close(ctx context.Context) error
	Connect(ctx context.Context) error
	Listen(ctx context.Context, topic string) error
	Ping(ctx context.Context) error
	Unlisten(ctx context.Context, topic string) error
	WaitForNotification(ctx context.Context) (*Notification, error)
}

type Notification struct {
	Payload string
	Topic   string
}

type Row interface {
	Scan(dest ...any) error
}

type Leader struct {
	ElectedAt time.Time
	ExpiresAt time.Time
	LeaderID  string
}

type JobCancelParams struct {
	ID     int64
	Schema string
}

type JobGetAvailableParams struct {
	ClientID   string
	MaxToLock  int
	Now        *time.Time
	ProducerID int64
	Queue      string
	Schema     string
}

type JobGetByIDParams struct {
	ID     int64
	Schema string
}

type JobGetStuckParams struct {
	Max               int
	Schema            string
	AttemptedAtBefore time.Time
}

type JobInsertFullParams struct {
	Attempt      int
	EncodedArgs  []byte
	Kind         string
	MaxAttempts  int
	Metadata     []byte
	Priority     int
	Queue        string
	ScheduledAt  *time.Time
	State        JobState
	Tags         []string
	UniqueKey    []byte
	UniqueStates byte
	Schema       string
}

type JobInsertFullManyParams struct {
	Jobs   []*JobInsertFullParams
	Schema string
}

type JobListParams struct {
	Max           int32
	Schema        string
	WhereClause   string
	OrderByClause string
	NamedArgs     map[string]any
}

type JobRescueManyParams struct {
	ID          []int64
	Error       [][]byte
	FinalizedAt []*time.Time
	ScheduledAt []time.Time
	Schema      string
	State       []string
}

type JobRetryParams struct {
	ID     int64
	Now    *time.Time
	Schema string
}

type JobScheduleParams struct {
	Max    int
	Now    *time.Time
	Schema string
}

type JobScheduleResult struct {
	Job               JobRow
	ConflictDiscarded bool
}

type JobSetStateIfRunningParams struct {
	ID              int64
	Attempt         *int
	ErrData         []byte
	FinalizedAt     *time.Time
	MetadataDoMerge bool
	MetadataUpdates []byte
	ScheduledAt     *time.Time
	Schema          string
	State           JobState
}

type JobUpdateParams struct {
	ID       int64
	Metadata []byte
	Schema   string
}

type JobDeleteParams struct {
	ID     int64
	Schema string
}

type JobDeleteBeforeParams struct {
	Max                int
	Schema             string
	FinalizedAtHorizon time.Time
	State              JobState
}

type JobDeleteManyParams struct {
	Max           int32
	NamedArgs     map[string]any
	OrderByClause string
	Schema        string
	WhereClause   string
}

type JobGetByIDManyParams struct {
	IDs    []int64
	Schema string
}

type LeaderElectParams struct {
	LeaderID string
	Now      *time.Time
	Schema   string
	TTL      time.Duration
}

type LeaderReelectParams struct {
	ElectedAt time.Time
	LeaderID  string
	Now       *time.Time
	Schema    string
	TTL       time.Duration
}

type LeaderDeleteExpiredParams struct {
	Now    *time.Time
	Schema string
}

type LeaderGetElectedLeaderParams struct {
	Schema string
}

type LeaderResignParams struct {
	ElectedAt       time.Time
	LeaderID        string
	LeadershipTopic string
	Schema          string
}

type NotifyManyParams struct {
	Payload []string
	Topic   string
	Schema  string
}

type QueueCreateOrSetUpdatedAtParams struct {
	Name      string
	Now       *time.Time
	PausedAt  *time.Time
	Schema    string
	UpdatedAt *time.Time
}

type QueueDeleteExpiredParams struct {
	Max              int
	Schema           string
	UpdatedAtHorizon time.Time
}

type QueueGetParams struct {
	Name   string
	Schema string
}

type QueueListParams struct {
	Max    int
	Schema string
}

type QueuePauseParams struct {
	Name   string
	Now    *time.Time
	Schema string
}

type QueueResumeParams struct {
	Name   string
	Now    *time.Time
	Schema string
}
