// Package backupengine adds Restic and Kopia as selectable backup engines,
// alongside the existing dbprovisioner / dbbackupsvc paths.
//
// Operators register a repository (S3, local filesystem, rest:, sftp:, gcrypt:
// …), snapshot it on the panel's cron schedule, and browse / verify / restore /
// prune snapshots. All work is delegated to the restic/kopia CLIs, which run
// either on the control-plane host (optionally inside a tooling container,
// exactly how services/dbbackup drives pg_dump and mysqldump) or on a bound
// beacon node through the daemon's admin exec endpoint.
package backupengine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/robfig/cron/v3"
)

// Engine identifiers accepted by this service.
const (
	EngineRestic = "restic"
	EngineKopia  = "kopia"
)

// Restore job statuses.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

const (
	defaultSnapshotTimeout = 30 * time.Minute
	defaultRestoreTimeout  = 2 * time.Hour
	maxErrorLength         = 512
	// envPasswordPrefix marks a stored password as the name of an environment
	// variable resolved at run time rather than secret material itself.
	envPasswordPrefix = "env:"
)

var errNotInitialized = errors.New("repository has not been initialised; run init first")

// Repository is a configured restic/kopia repository.
type Repository struct {
	ID string `json:"id"`
	// Name is the operator-facing label.
	Name string `json:"name"`
	// Engine is either "restic" or "kopia".
	Engine string `json:"engine"`
	// Location is the backend URL/path (s3:…, rest:…, /mnt/backups, …).
	Location string `json:"location"`
	// PasswordRef is a non-secret descriptor: an "env:VAR" reference when the
	// password comes from the environment, otherwise a marker that a sealed
	// password is configured. The secret itself is never echoed back.
	PasswordRef string `json:"passwordRef,omitempty"`
	Encryption  string `json:"encryption"`
	// PrunePolicy carries retention rules plus the snapshot cron schedule.
	PrunePolicy json.RawMessage `json:"prunePolicy"`
	// NodeID / ServerID route CLI execution; ArtifactID links the repository
	// to a Forge backup artifact when it was produced by the classic pipeline.
	NodeID      *string `json:"nodeId,omitempty"`
	ServerID    *string `json:"serverId,omitempty"`
	ArtifactID  *string `json:"artifactId,omitempty"`
	Initialized bool    `json:"initialized"`
	// Run state surfaced to the UI.
	LastSnapshotAt *time.Time `json:"lastSnapshotAt,omitempty"`
	NextRunAt      *time.Time `json:"nextRunAt,omitempty"`
	LastError      string     `json:"lastError,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

// Snapshot is a single point-in-time snapshot inside a repository.
type Snapshot struct {
	ID             string     `json:"id"`
	RepoID         string     `json:"repoId"`
	SnapshotID     string     `json:"snapshotId"`
	Paths          []string   `json:"paths"`
	Hostname       string     `json:"hostname"`
	Timestamp      *time.Time `json:"timestamp,omitempty"`
	Parent         string     `json:"parent,omitempty"`
	Tree           string     `json:"tree,omitempty"`
	DataFiles      int64      `json:"dataFiles"`
	TotalSizeBytes int64      `json:"totalSizeBytes"`
	VerifiedAt     *time.Time `json:"verifiedAt,omitempty"`
}

// RestoreJob tracks the outcome of restoring a snapshot to a target path.
type RestoreJob struct {
	ID          string     `json:"id"`
	RepoID      string     `json:"repoId"`
	SnapshotID  string     `json:"snapshotId"`
	TargetPath  string     `json:"targetPath"`
	Status      string     `json:"status"`
	ProgressPct int        `json:"progressPct"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// AddRepositoryRequest describes a new repository registration. Password is
// accepted on input only; it is sealed by the store's secret layer.
type AddRepositoryRequest struct {
	Name        string          `json:"name"`
	Engine      string          `json:"engine"`
	Location    string          `json:"location"`
	Password    string          `json:"password"`
	Encryption  string          `json:"encryption"`
	PrunePolicy json.RawMessage `json:"prunePolicy"`
	NodeID      string          `json:"nodeId"`
	ServerID    string          `json:"serverId"`
	ArtifactID  string          `json:"artifactId"`
	Initialized bool            `json:"initialized"`
}

// PrunePolicy is the decoded form of Repository.PrunePolicy. Schedule is a
// standard 5-field cron expression handled by the panel's cron machinery.
type PrunePolicy struct {
	Schedule    string   `json:"schedule"`
	KeepLast    int      `json:"keepLast"`
	KeepHourly  int      `json:"keepHourly"`
	KeepDaily   int      `json:"keepDaily"`
	KeepWeekly  int      `json:"keepWeekly"`
	KeepMonthly int      `json:"keepMonthly"`
	Prune       bool     `json:"prune"`
	Paths       []string `json:"paths"`
}

func (p PrunePolicy) hasKeepRules() bool {
	return p.KeepLast > 0 || p.KeepHourly > 0 || p.KeepDaily > 0 || p.KeepWeekly > 0 || p.KeepMonthly > 0
}

// ExecTarget identifies where a CLI invocation runs.
type ExecTarget struct {
	// OnHost is true when the command runs on the control-plane host.
	OnHost bool
	// NodeURL / NodeToken address the beacon daemon for remote execution.
	NodeURL   string
	NodeToken string
	// ContainerID is the tooling container hosting the CLI binary. When empty
	// on the host path, the binary is invoked directly.
	ContainerID string
}

// CommandResult is the outcome of a CLI invocation.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// ok reports whether the command ran and exited successfully.
func (r CommandResult) ok() bool { return r.ExitCode == 0 }

// ExecRequest is one CLI invocation: argv, environment, the secret files that
// must exist on the target for the duration of the run (path -> content), and
// the resolved target.
type ExecRequest struct {
	Target     ExecTarget
	Argv       []string
	Env        map[string]string
	StageFiles map[string]string
}

// CommandExecutor transports restic/kopia invocations to their target.
// Implementations must not log StageFiles contents.
type CommandExecutor interface {
	Execute(ctx context.Context, req ExecRequest) (CommandResult, error)
}

// Service manages restic/kopia repositories, snapshots and restores.
type Service struct {
	store  *store.Store
	daemon *daemon.Client
	logger *slog.Logger

	runner  CommandExecutor
	cron    *cron.Cron
	mu      sync.Mutex
	entries map[string]cron.EntryID

	snapshotTimeout time.Duration
	restoreTimeout  time.Duration
}

// New builds a backup-engine service. Every argument is optional: a nil store
// makes each method return a clear error, a nil daemon forces control-plane
// host execution, and a nil logger falls back to slog.Default().
func New(db *store.Store, dmn *daemon.Client, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{
		store:           db,
		daemon:          dmn,
		logger:          log,
		cron:            cron.New(),
		entries:         make(map[string]cron.EntryID),
		snapshotTimeout: defaultSnapshotTimeout,
		restoreTimeout:  defaultRestoreTimeout,
	}
	host := &hostExecutor{logger: log}
	if dmn != nil {
		s.runner = &beaconExecutor{client: dmn, fallback: host, logger: log}
	} else {
		s.runner = host
	}
	return s
}

// SetExecutor overrides how restic/kopia commands reach their target. Intended
// for tests and alternative transports (job queues, SSH brokers).
func (s *Service) SetExecutor(exec CommandExecutor) {
	if exec != nil {
		s.runner = exec
	}
}

// ---------------------------------------------------------------------------
// Repositories
// ---------------------------------------------------------------------------

// ListRepositories returns every configured backup repository, without secrets.
func (s *Service) ListRepositories(ctx context.Context) ([]Repository, error) {
	if s.store == nil {
		return nil, errors.New("backup engine requires a database")
	}
	records, err := s.store.ListBackupRepositories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list backup repositories: %w", err)
	}
	out := make([]Repository, 0, len(records))
	for _, record := range records {
		out = append(out, repositoryFromRecord(record))
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// AddRepository registers a new restic/kopia repository and arms its snapshot
// schedule.
func (s *Service) AddRepository(ctx context.Context, req AddRepositoryRequest) (*Repository, error) {
	if s.store == nil {
		return nil, errors.New("backup engine requires a database")
	}
	engine := strings.ToLower(strings.TrimSpace(req.Engine))
	if engine != EngineRestic && engine != EngineKopia {
		return nil, fmt.Errorf("unsupported backup engine %q (expected restic or kopia)", req.Engine)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("repository name is required")
	}
	location := strings.TrimSpace(req.Location)
	if location == "" {
		return nil, errors.New("repository location is required")
	}
	if !singleLine(name, location, req.Password) {
		return nil, errors.New("repository name, location and password must be single-line values")
	}
	policy := req.PrunePolicy
	if len(policy) == 0 {
		policy = json.RawMessage(`{}`)
	}
	if !json.Valid(policy) {
		return nil, errors.New("prune policy must be valid JSON")
	}
	decoded := decodePrunePolicy(policy)
	if decoded.Schedule != "" {
		if _, err := cron.ParseStandard(decoded.Schedule); err != nil {
			return nil, fmt.Errorf("invalid snapshot schedule %q: %w", decoded.Schedule, err)
		}
	}

	record := store.BackupEngineRepository{
		ID:          uuid.NewString(),
		Name:        name,
		Engine:      engine,
		Location:    location,
		PasswordRef: strings.TrimSpace(req.Password),
		Encryption:  normalizeEncryption(engine, req.Encryption),
		PrunePolicy: policy,
		NodeID:      optionalString(req.NodeID),
		ServerID:    optionalString(req.ServerID),
		ArtifactID:  optionalString(req.ArtifactID),
		Initialized: req.Initialized,
	}
	if err := s.store.CreateBackupRepository(ctx, &record); err != nil {
		return nil, fmt.Errorf("persist backup repository: %w", err)
	}
	repo := repositoryFromRecord(record)
	s.reschedule(repo)
	s.logger.Info("registered backup repository", "id", repo.ID, "name", repo.Name, "engine", repo.Engine)
	return &repo, nil
}

// RemoveRepository deletes a repository registration along with its tracked
// snapshot and restore rows. The remote repository itself is left untouched.
func (s *Service) RemoveRepository(ctx context.Context, id string) error {
	if s.store == nil {
		return errors.New("backup engine requires a database")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("repository id is required")
	}
	s.unschedule(id)
	if err := s.store.DeleteBackupRepository(ctx, id); err != nil {
		return fmt.Errorf("delete backup repository: %w", err)
	}
	s.logger.Info("removed backup repository", "id", id)
	return nil
}

// TestRepository proves the CLI can reach and decrypt the repository.
func (s *Service) TestRepository(ctx context.Context, id string) error {
	repo, password, err := s.loadRepository(ctx, id)
	if err != nil {
		return err
	}
	var req ExecRequest
	switch repo.Engine {
	case EngineRestic:
		req = s.buildRequest(repo, password, "cat", "config")
	case EngineKopia:
		req = s.buildRequest(repo, password, "repository", "status", "--json")
	default:
		return fmt.Errorf("unsupported engine %q", repo.Engine)
	}
	res, err := s.execute(ctx, req)
	if err != nil {
		s.recordError(repo.ID, err)
		return err
	}
	if !res.ok() {
		cmdErr := fmt.Errorf("%s cannot reach the repository: %s", repo.Engine, clipCommandError(res))
		s.recordError(repo.ID, cmdErr)
		return cmdErr
	}
	s.clearError(repo.ID)
	return nil
}

// InitRepository creates the repository on its backend if it does not exist
// yet, then marks the registration initialised. Re-running against an existing
// repository is a no-op.
func (s *Service) InitRepository(ctx context.Context, id string) error {
	repo, password, err := s.loadRepository(ctx, id)
	if err != nil {
		return err
	}
	var req ExecRequest
	switch repo.Engine {
	case EngineRestic:
		req = s.buildRequest(repo, password, "init")
	case EngineKopia:
		backend, path := splitKopiaLocation(repo.Location)
		req = s.buildRequest(repo, password, "repository", "connect", backend, path, "--init", "--no-prompt")
	default:
		return fmt.Errorf("unsupported engine %q", repo.Engine)
	}
	res, err := s.execute(ctx, req)
	if err != nil {
		s.recordError(repo.ID, err)
		return err
	}
	if !res.ok() && !looksAlreadyPresent(res) {
		initErr := fmt.Errorf("%s init failed: %s", repo.Engine, clipCommandError(res))
		s.recordError(repo.ID, initErr)
		return initErr
	}
	if err := s.store.MarkBackupRepositoryInitialized(ctx, repo.ID); err != nil {
		return fmt.Errorf("mark repository initialised: %w", err)
	}
	repo.Initialized = true
	s.reschedule(repo)
	s.logger.Info("initialised backup repository", "id", repo.ID, "engine", repo.Engine)
	return nil
}

// ---------------------------------------------------------------------------
// Snapshots
// ---------------------------------------------------------------------------

// ListSnapshots returns the snapshots of a repository. The CLI listing is
// reconciled into the tracking table first so external forget/prune runs are
// reflected in the browser.
func (s *Service) ListSnapshots(ctx context.Context, repoID string) ([]Snapshot, error) {
	repo, password, err := s.loadRepository(ctx, repoID)
	if err != nil {
		return nil, err
	}
	if err := s.reconcileSnapshots(ctx, repo, password); err != nil {
		// Remote listing is unavailable — fall back to what we already know.
		s.logger.Warn("snapshot reconciliation failed", "repo", repo.ID, "error", err)
	}
	records, err := s.store.ListBackupSnapshots(ctx, repo.ID)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	out := make([]Snapshot, 0, len(records))
	for _, record := range records {
		out = append(out, snapshotFromRecord(record))
	}
	return out, nil
}

// CreateSnapshot backs up paths into the repository and records the resulting
// snapshot.
func (s *Service) CreateSnapshot(ctx context.Context, repoID string, paths []string) (*Snapshot, error) {
	repo, password, err := s.loadRepository(ctx, repoID)
	if err != nil {
		return nil, err
	}
	if !repo.Initialized {
		return nil, errNotInitialized
	}
	cleaned := cleanPaths(paths)
	if len(cleaned) == 0 {
		return nil, errors.New("at least one snapshot path is required")
	}

	var req ExecRequest
	switch repo.Engine {
	case EngineRestic:
		req = s.buildRequest(repo, password, append([]string{"backup", "--json"}, cleaned...)...)
	case EngineKopia:
		// kopia captures one root per invocation.
		req = s.buildRequest(repo, password, "snapshot", "create", cleaned[0], "--json")
	default:
		return nil, fmt.Errorf("unsupported engine %q", repo.Engine)
	}

	runCtx, cancel := context.WithTimeout(ctx, s.timeout(s.snapshotTimeout))
	res, err := s.execute(runCtx, req)
	cancel()
	if err != nil {
		s.recordError(repo.ID, err)
		return nil, err
	}
	if !res.ok() {
		cmdErr := fmt.Errorf("%s backup failed: %s", repo.Engine, clipCommandError(res))
		s.recordError(repo.ID, cmdErr)
		return nil, cmdErr
	}

	snap := Snapshot{ID: uuid.NewString(), RepoID: repo.ID, Paths: cleaned, Hostname: hostName()}
	switch repo.Engine {
	case EngineRestic:
		if summary := parseResticBackupSummary(res.Stdout); summary != nil {
			snap.SnapshotID = summary.snapshotID()
			snap.Tree = summary.tree()
			snap.TotalSizeBytes = summary.TotalByteSize
			snap.DataFiles = summary.TotalFiles
		}
	case EngineKopia:
		snap.SnapshotID = parseKopiaSnapshotID(res.Stdout)
	}
	if strings.TrimSpace(snap.SnapshotID) == "" {
		// The CLI did not report an id: reconcile and take the newest snapshot.
		if err := s.reconcileSnapshots(ctx, repo, password); err != nil {
			return nil, err
		}
		latest, err := s.newestSnapshot(ctx, repo.ID)
		if err != nil {
			s.touchSnapshot(repo.ID)
			return nil, err
		}
		s.touchSnapshot(repo.ID)
		return latest, nil
	}

	now := time.Now().UTC()
	snap.Timestamp = &now
	record := store.BackupEngineSnapshot{
		ID:             snap.ID,
		RepoID:         snap.RepoID,
		SnapshotID:     snap.SnapshotID,
		Paths:          mustJSON(snap.Paths),
		Hostname:       snap.Hostname,
		Timestamp:      snap.Timestamp,
		Tree:           snap.Tree,
		DataFiles:      snap.DataFiles,
		TotalSizeBytes: snap.TotalSizeBytes,
		CreatedAt:      now,
	}
	if err := s.store.UpsertBackupSnapshot(ctx, &record); err != nil {
		return nil, fmt.Errorf("persist snapshot: %w", err)
	}
	s.logger.Info("created snapshot", "repo", repo.ID, "engine", repo.Engine, "files", snap.DataFiles)
	s.touchSnapshot(repo.ID)
	return &snap, nil
}

// VerifySnapshot validates a snapshot's integrity and stamps verified_at on
// success.
func (s *Service) VerifySnapshot(ctx context.Context, repoID, snapshotID string) error {
	repo, password, err := s.loadRepository(ctx, repoID)
	if err != nil {
		return err
	}
	snapshotID = strings.TrimSpace(snapshotID)
	if snapshotID == "" || !singleLine(snapshotID) {
		return errors.New("a valid snapshot id is required")
	}
	var req ExecRequest
	switch repo.Engine {
	case EngineRestic:
		// restic validates repository integrity, which covers the snapshot's
		// trees; a data subset read keeps the check affordable.
		req = s.buildRequest(repo, password, "check", "--read-data-subset=5%", "--json")
	case EngineKopia:
		req = s.buildRequest(repo, password, "snapshot", "verify", "--snapshot-id", snapshotID, "--json")
	default:
		return fmt.Errorf("unsupported engine %q", repo.Engine)
	}
	res, err := s.execute(ctx, req)
	if err != nil {
		return err
	}
	if !res.ok() {
		return fmt.Errorf("%s verify failed: %s", repo.Engine, clipCommandError(res))
	}
	if err := s.store.MarkBackupSnapshotVerified(ctx, repo.ID, snapshotID, time.Now().UTC()); err != nil {
		return fmt.Errorf("record verification: %w", err)
	}
	s.clearError(repo.ID)
	return nil
}

// PruneRepository drops snapshots no longer covered by the retention policy and
// garbage-collects the data they referenced.
func (s *Service) PruneRepository(ctx context.Context, repoID string) error {
	repo, password, err := s.loadRepository(ctx, repoID)
	if err != nil {
		return err
	}
	policy := decodePrunePolicy(repo.PrunePolicy)
	var req ExecRequest
	switch repo.Engine {
	case EngineRestic:
		flags := resticKeepFlags(policy)
		if len(flags) == 0 {
			return errors.New("a restic prune needs at least one keep rule in the prune policy")
		}
		req = s.buildRequest(repo, password, append([]string{"forget", "--prune", "--json"}, flags...)...)
	case EngineKopia:
		// kopia's maintenance run performs GC and version history compaction;
		// retention itself is applied per snapshot by `snapshot expire --all`.
		if policy.hasKeepRules() {
			expire := append([]string{"snapshot", "expire", "--all"}, kopiaKeepFlags(policy)...)
			expireReq := s.buildRequest(repo, password, expire...)
			if expireRes, expireErr := s.execute(ctx, expireReq); expireErr != nil {
				return expireErr
			} else if !expireRes.ok() {
				return fmt.Errorf("kopia expire failed: %s", clipCommandError(expireRes))
			}
		}
		req = s.buildRequest(repo, password, "maintenance", "run", "--full")
	default:
		return fmt.Errorf("unsupported engine %q", repo.Engine)
	}
	res, err := s.execute(ctx, req)
	if err != nil {
		return err
	}
	if !res.ok() {
		return fmt.Errorf("%s prune failed: %s", repo.Engine, clipCommandError(res))
	}
	// Pruned snapshots vanish from the CLI listing — reconcile so the browser
	// stops offering restore targets that no longer exist.
	if err := s.reconcileSnapshots(ctx, repo, password); err != nil {
		s.logger.Warn("post-prune reconciliation failed", "repo", repo.ID, "error", err)
	}
	s.logger.Info("pruned backup repository", "id", repo.ID, "engine", repo.Engine)
	s.clearError(repo.ID)
	return nil
}

// ---------------------------------------------------------------------------
// Restores
// ---------------------------------------------------------------------------

// RestoreSnapshot restores a snapshot into targetPath and records a job.
func (s *Service) RestoreSnapshot(ctx context.Context, repoID, snapshotID, targetPath string) (*RestoreJob, error) {
	repo, password, err := s.loadRepository(ctx, repoID)
	if err != nil {
		return nil, err
	}
	snapshotID = strings.TrimSpace(snapshotID)
	targetPath = filepath.Clean(strings.TrimSpace(targetPath))
	if snapshotID == "" || !singleLine(snapshotID) {
		return nil, errors.New("a valid snapshot id is required")
	}
	if !filepath.IsAbs(targetPath) || !singleLine(targetPath) {
		return nil, errors.New("target path must be a single-line absolute path")
	}

	var req ExecRequest
	switch repo.Engine {
	case EngineRestic:
		req = s.buildRequest(repo, password, "restore", snapshotID, "--target", targetPath, "--json")
	case EngineKopia:
		req = s.buildRequest(repo, password, "snapshot", "restore", snapshotID, targetPath)
	default:
		return nil, fmt.Errorf("unsupported engine %q", repo.Engine)
	}

	now := time.Now().UTC()
	job := store.BackupEngineRestoreJob{
		ID:          uuid.NewString(),
		RepoID:      repo.ID,
		SnapshotID:  snapshotID,
		TargetPath:  targetPath,
		Status:      StatusRunning,
		ProgressPct: 0,
		StartedAt:   &now,
	}
	if err := s.store.CreateBackupRestoreJob(ctx, &job); err != nil {
		return nil, fmt.Errorf("persist restore job: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, s.timeout(s.restoreTimeout))
	res, runErr := s.execute(runCtx, req)
	cancel()

	completed := time.Now().UTC()
	status, progress, errMsg := StatusCompleted, 100, ""
	switch {
	case runErr != nil:
		status, progress, errMsg = StatusFailed, 0, runErr.Error()
	case !res.ok():
		status, progress = StatusFailed, 0
		errMsg = clipCommandError(res)
	}
	var failure *string
	if errMsg != "" {
		failure = &errMsg
	}
	if err := s.store.UpdateBackupRestoreJob(ctx, job.ID, status, progress, nil, &completed, failure); err != nil {
		s.logger.Error("update restore job", "id", job.ID, "error", err)
	}
	if failure != nil {
		s.recordError(repo.ID, errors.New(errMsg))
	} else {
		s.clearError(repo.ID)
	}
	s.logger.Info("restore finished", "repo", repo.ID, "status", status)

	return &RestoreJob{
		ID:          job.ID,
		RepoID:      job.RepoID,
		SnapshotID:  job.SnapshotID,
		TargetPath:  job.TargetPath,
		Status:      status,
		ProgressPct: progress,
		StartedAt:   job.StartedAt,
		CompletedAt: &completed,
		Error:       errMsg,
		CreatedAt:   job.CreatedAt,
	}, runErr
}

// ListRestoreJobs returns recent restore jobs, newest first. An empty repoID
// lists across every repository.
func (s *Service) ListRestoreJobs(ctx context.Context, repoID string) ([]RestoreJob, error) {
	if s.store == nil {
		return nil, errors.New("backup engine requires a database")
	}
	records, err := s.store.ListBackupRestoreJobs(ctx, strings.TrimSpace(repoID), 100)
	if err != nil {
		return nil, fmt.Errorf("list restore jobs: %w", err)
	}
	out := make([]RestoreJob, 0, len(records))
	for _, record := range records {
		out = append(out, restoreJobFromRecord(record))
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Scheduling — reuses the panel's cron machinery
// ---------------------------------------------------------------------------

// Start loads every repository schedule and starts the cron loop.
func (s *Service) Start(ctx context.Context) error {
	if s.store == nil {
		return errors.New("backup engine requires a database")
	}
	repos, err := s.ListRepositories(ctx)
	if err != nil {
		return err
	}
	for _, repo := range repos {
		s.reschedule(repo)
	}
	s.cron.Start()
	s.logger.Info("backup engine scheduler started", "repositories", len(repos))
	return nil
}

// Stop halts the cron loop and waits for in-flight entries to return.
func (s *Service) Stop() {
	if s.cron != nil {
		<-s.cron.Stop().Done()
	}
}

// NextRun computes the next scheduled snapshot time for a repository, if any.
func (s *Service) NextRun(repo Repository) *time.Time {
	schedule := decodePrunePolicy(repo.PrunePolicy).Schedule
	if schedule == "" {
		return nil
	}
	parsed, err := cron.ParseStandard(schedule)
	if err != nil {
		return nil
	}
	next := parsed.Next(time.Now().UTC())
	if next.IsZero() {
		return nil
	}
	return &next
}

func (s *Service) reschedule(repo Repository) {
	if s.store == nil || s.cron == nil {
		return
	}
	s.mu.Lock()
	if existing, ok := s.entries[repo.ID]; ok {
		s.cron.Remove(existing)
		delete(s.entries, repo.ID)
	}
	s.mu.Unlock()

	schedule := decodePrunePolicy(repo.PrunePolicy).Schedule
	if schedule == "" {
		return
	}
	entryID, err := s.cron.AddFunc(schedule, func() { s.runScheduled(repo.ID) })
	if err != nil {
		s.logger.Error("schedule backup engine snapshot", "repo", repo.ID, "schedule", schedule, "error", err)
		return
	}
	s.mu.Lock()
	s.entries[repo.ID] = entryID
	s.mu.Unlock()

	if next := s.NextRun(repo); next != nil {
		if err := s.store.UpdateBackupRepositoryRunState(context.Background(), repo.ID, nil, next, nil); err != nil {
			s.logger.Warn("persist next snapshot time", "repo", repo.ID, "error", err)
		}
	}
}

func (s *Service) unschedule(repoID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entries[repoID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, repoID)
	}
}

// runScheduled performs the cron-driven snapshot (and optional prune) for a
// repository. Failures are recorded on the repository, never swallowed.
func (s *Service) runScheduled(repoID string) {
	ctx := context.Background()
	repo, _, err := s.loadRepository(ctx, repoID)
	if err != nil {
		s.logger.Error("scheduled snapshot skipped", "repo", repoID, "error", err)
		return
	}
	policy := decodePrunePolicy(repo.PrunePolicy)
	paths := cleanPaths(policy.Paths)
	if len(paths) == 0 {
		s.logger.Warn("scheduled snapshot has no paths configured", "repo", repoID)
		return
	}
	if _, err := s.CreateSnapshot(ctx, repoID, paths); err != nil {
		s.logger.Error("scheduled snapshot failed", "repo", repoID, "error", err)
		return
	}
	if policy.Prune || policy.hasKeepRules() {
		if err := s.PruneRepository(ctx, repoID); err != nil {
			s.logger.Error("scheduled prune failed", "repo", repoID, "error", err)
		}
	}
	if next := s.NextRun(repo); next != nil {
		if err := s.store.UpdateBackupRepositoryRunState(ctx, repoID, nil, next, nil); err != nil {
			s.logger.Warn("persist next snapshot time", "repo", repoID, "error", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Internals
// ---------------------------------------------------------------------------

// loadRepository resolves a repository plus its unlocked password.
func (s *Service) loadRepository(ctx context.Context, id string) (Repository, string, error) {
	if s.store == nil {
		return Repository{}, "", errors.New("backup engine requires a database")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Repository{}, "", errors.New("repository id is required")
	}
	record, err := s.store.GetBackupRepository(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Repository{}, "", errors.New("backup repository not found")
		}
		return Repository{}, "", fmt.Errorf("load backup repository: %w", err)
	}
	repo := repositoryFromRecord(record)
	password, err := s.store.DecryptBackupRepositoryPassword(record)
	if err != nil {
		return repo, "", fmt.Errorf("unlock repository password: %w", err)
	}
	if strings.HasPrefix(password, envPasswordPrefix) {
		// Stored as an environment variable name, resolved at run time so the
		// secret never lands in the panel database.
		name := strings.TrimSpace(strings.TrimPrefix(password, envPasswordPrefix))
		resolved, ok := os.LookupEnv(name)
		if !ok || resolved == "" {
			return repo, "", fmt.Errorf("repository password environment variable %s is not set", name)
		}
		password = resolved
	}
	return repo, password, nil
}

// buildRequest assembles a CLI invocation for the engine. Secrets travel as
// staged files / environment entries — never as argv on the host path — so
// they stay out of process listings and logs.
func (s *Service) buildRequest(repo Repository, password string, sub ...string) ExecRequest {
	target := s.resolveTarget(repo)
	switch repo.Engine {
	case EngineKopia:
		configPath := kopiaConfigPath(repo.ID)
		env := map[string]string{
			"KOPIA_PASSWORD":    password,
			"KOPIA_CONFIG_PATH": configPath,
		}
		argv := append([]string{"kopia", "--config-file", configPath, "--no-prompt"}, sub...)
		return ExecRequest{Target: target, Argv: argv, Env: env}
	default:
		passwordFile := resticPasswordFile(repo.ID)
		env := map[string]string{"RESTIC_PASSWORD": password, "RESTIC_REPOSITORY": repo.Location}
		argv := append([]string{"restic", "-r", repo.Location, "--password-file", passwordFile}, sub...)
		return ExecRequest{
			Target:     target,
			Argv:       argv,
			Env:        env,
			StageFiles: map[string]string{passwordFile: password},
		}
	}
}

// resolveTarget decides whether the CLI runs on the control-plane host or on a
// bound beacon node.
func (s *Service) resolveTarget(repo Repository) ExecTarget {
	target := ExecTarget{OnHost: true}
	if s.store == nil {
		return target
	}
	ctx := context.Background()
	if repo.ServerID != nil && strings.TrimSpace(*repo.ServerID) != "" {
		control, err := s.store.ServerControlTarget(ctx, *repo.ServerID)
		if err != nil {
			s.logger.Debug("resolve server control target", "server", *repo.ServerID, "error", err)
			return target
		}
		target.NodeURL, target.NodeToken = control.NodeURL, control.NodeToken
		if container, cErr := s.store.BackupEngineExecContainer(ctx, *repo.ServerID); cErr == nil {
			target.ContainerID = container
		}
		return target
	}
	if repo.NodeID != nil && strings.TrimSpace(*repo.NodeID) != "" {
		control, err := s.store.BackupEngineNodeTarget(ctx, *repo.NodeID)
		if err != nil {
			s.logger.Debug("resolve node control target", "node", *repo.NodeID, "error", err)
			return target
		}
		if control.NodeURL != "" {
			target.NodeURL, target.NodeToken, target.OnHost = control.NodeURL, control.NodeToken, false
		}
	}
	return target
}

func (s *Service) execute(ctx context.Context, req ExecRequest) (CommandResult, error) {
	if s.runner == nil {
		return CommandResult{}, errors.New("backup engine executor is not configured")
	}
	if len(req.Argv) == 0 {
		return CommandResult{}, errors.New("empty command")
	}
	return s.runner.Execute(ctx, req)
}

// reconcileSnapshots upserts the CLI's snapshot listing into the tracking table.
func (s *Service) reconcileSnapshots(ctx context.Context, repo Repository, password string) error {
	var req ExecRequest
	switch repo.Engine {
	case EngineRestic:
		req = s.buildRequest(repo, password, "snapshots", "--json")
	case EngineKopia:
		req = s.buildRequest(repo, password, "snapshot", "list", "--all", "--json")
	default:
		return fmt.Errorf("unsupported engine %q", repo.Engine)
	}
	res, err := s.execute(ctx, req)
	if err != nil {
		return err
	}
	if !res.ok() {
		return fmt.Errorf("%s snapshot list failed: %s", repo.Engine, clipCommandError(res))
	}
	var snaps []Snapshot
	switch repo.Engine {
	case EngineRestic:
		snaps = resticSnapshots(res.Stdout, repo.ID)
	case EngineKopia:
		snaps = kopiaSnapshots(res.Stdout, repo.ID)
	}
	for _, snap := range snaps {
		if strings.TrimSpace(snap.SnapshotID) == "" {
			continue
		}
		record := store.BackupEngineSnapshot{
			RepoID:         snap.RepoID,
			SnapshotID:     snap.SnapshotID,
			Paths:          mustJSON(snap.Paths),
			Hostname:       snap.Hostname,
			Timestamp:      snap.Timestamp,
			Parent:         snap.Parent,
			Tree:           snap.Tree,
			DataFiles:      snap.DataFiles,
			TotalSizeBytes: snap.TotalSizeBytes,
			CreatedAt:      time.Now().UTC(),
		}
		if err := s.store.UpsertBackupSnapshot(ctx, &record); err != nil {
			return fmt.Errorf("record snapshot %s: %w", snap.SnapshotID, err)
		}
	}
	return nil
}

func (s *Service) newestSnapshot(ctx context.Context, repoID string) (*Snapshot, error) {
	records, err := s.store.ListBackupSnapshots(ctx, repoID)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("snapshot completed but no snapshot id could be determined")
	}
	snap := snapshotFromRecord(records[0])
	return &snap, nil
}

func (s *Service) touchSnapshot(repoID string) {
	now := time.Now().UTC()
	if err := s.store.UpdateBackupRepositoryRunState(context.Background(), repoID, &now, nil, nil); err != nil {
		s.logger.Warn("persist last snapshot time", "repo", repoID, "error", err)
	}
}

func (s *Service) recordError(repoID string, err error) {
	if err == nil || s.store == nil {
		return
	}
	msg := truncate(err.Error(), maxErrorLength)
	if cErr := s.store.UpdateBackupRepositoryRunState(context.Background(), repoID, nil, nil, &msg); cErr != nil {
		s.logger.Warn("persist repository error", "repo", repoID, "error", cErr)
	}
}

func (s *Service) clearError(repoID string) {
	if s.store == nil {
		return
	}
	empty := ""
	if err := s.store.UpdateBackupRepositoryRunState(context.Background(), repoID, nil, nil, &empty); err != nil {
		s.logger.Warn("clear repository error", "repo", repoID, "error", err)
	}
}

func (s *Service) timeout(d time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return defaultSnapshotTimeout
}

// ---------------------------------------------------------------------------
// Executors
// ---------------------------------------------------------------------------

// hostExecutor runs the CLI on the control-plane host, optionally inside a
// tooling container via `docker exec` — the technique dbbackup uses for pg_dump
// and mysqldump.
type hostExecutor struct {
	logger *slog.Logger
}

func (e *hostExecutor) Execute(ctx context.Context, req ExecRequest) (CommandResult, error) {
	if len(req.Argv) == 0 {
		return CommandResult{}, errors.New("empty command")
	}
	cleanup, err := stageFiles(req.StageFiles)
	defer cleanup()
	if err != nil {
		return CommandResult{}, err
	}

	if req.Target.ContainerID != "" {
		if err := copyIntoContainer(ctx, req.Target.ContainerID, req.StageFiles); err != nil {
			return CommandResult{}, err
		}
		defer removeFromContainer(ctx, req.Target.ContainerID, req.StageFiles)
		args := append([]string{"exec", "-i"}, dockerEnvFlags(req.Env)...)
		args = append(args, req.Target.ContainerID)
		args = append(args, req.Argv...)
		return runCommand(ctx, "docker", args, nil)
	}

	env := os.Environ()
	for key, value := range req.Env {
		env = append(env, key+"="+value)
	}
	return runCommand(ctx, req.Argv[0], req.Argv[1:], env)
}

// beaconExecutor runs the CLI inside a container on a beacon node through the
// daemon's admin exec endpoint, staging secret files with the daemon's admin
// file API. Repositories without a reachable node/container fall back to the
// host executor.
type beaconExecutor struct {
	client   *daemon.Client
	fallback CommandExecutor
	logger   *slog.Logger
}

func (e *beaconExecutor) Execute(ctx context.Context, req ExecRequest) (CommandResult, error) {
	if len(req.Argv) == 0 {
		return CommandResult{}, errors.New("empty command")
	}
	target := req.Target
	if target.OnHost || target.NodeURL == "" || target.ContainerID == "" || e.client == nil {
		if e.fallback != nil {
			return e.fallback.Execute(ctx, req)
		}
		return CommandResult{}, errors.New("backup engine requires a bound beacon node or host execution")
	}

	for path, content := range req.StageFiles {
		if err := e.upload(ctx, target, path, content); err != nil {
			e.cleanup(ctx, target, req.StageFiles)
			return CommandResult{}, err
		}
	}
	defer e.cleanup(ctx, target, req.StageFiles)

	// The admin exec API carries no environment, so a password that kopia
	// would normally read from KOPIA_PASSWORD is passed as its global flag.
	argv := append([]string{}, req.Argv...)
	if password := req.Env["KOPIA_PASSWORD"]; password != "" {
		argv = insertFlag(argv, 1, "--password="+password)
	}

	raw, err := e.client.AdminContainerExec(ctx, target.NodeURL, target.NodeToken, target.ContainerID, argv, false)
	if err != nil {
		return CommandResult{}, fmt.Errorf("beacon exec %s: %w", argv[0], err)
	}
	var payload struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode *int   `json:"exitCode"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return CommandResult{}, fmt.Errorf("decode beacon exec response: %w", err)
	}
	res := CommandResult{Stdout: payload.Stdout, Stderr: payload.Stderr}
	switch {
	case payload.ExitCode != nil:
		res.ExitCode = *payload.ExitCode
	case payload.Error != "":
		res.ExitCode = 1
		res.Stderr = strings.TrimSpace(res.Stderr + "\n" + payload.Error)
	}
	return res, nil
}

func (e *beaconExecutor) upload(ctx context.Context, target ExecTarget, path, content string) error {
	reader := strings.NewReader(content)
	if err := e.client.AdminContainerFilesUpload(ctx, target.NodeURL, target.NodeToken, target.ContainerID, path, reader, "application/octet-stream"); err != nil {
		return fmt.Errorf("stage %s on node: %w", filepath.Base(path), err)
	}
	return nil
}

func (e *beaconExecutor) cleanup(ctx context.Context, target ExecTarget, files map[string]string) {
	for path := range files {
		if err := e.client.AdminContainerFilesDelete(ctx, target.NodeURL, target.NodeToken, target.ContainerID, path); err != nil {
			e.logger.Debug("remove staged file from node", "path", path, "error", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Host helpers
// ---------------------------------------------------------------------------

func runCommand(ctx context.Context, name string, args []string, env []string) (CommandResult, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if env != nil {
		cmd.Env = env
	}
	err := cmd.Run()
	res := CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	case err != nil:
		return res, fmt.Errorf("run %s: %w", name, err)
	}
	return res, nil
}

// stageFiles writes the secret files a CLI needs onto the host filesystem at
// the exact paths argv references, returning a cleanup func.
func stageFiles(files map[string]string) (func(), error) {
	if len(files) == 0 {
		return func() {}, nil
	}
	written := make([]string, 0, len(files))
	cleanup := func() {
		for _, path := range written {
			_ = os.Remove(path)
		}
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			cleanup()
			return func() {}, fmt.Errorf("create staging directory: %w", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			cleanup()
			return func() {}, fmt.Errorf("stage credential file: %w", err)
		}
		written = append(written, path)
	}
	return cleanup, nil
}

func dockerEnvFlags(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var flags []string
	for _, key := range keys {
		flags = append(flags, "-e", key+"="+env[key])
	}
	return flags
}

func copyIntoContainer(ctx context.Context, containerID string, files map[string]string) error {
	for path := range files {
		if err := exec.CommandContext(ctx, "docker", "cp", path, "--", containerID+":"+path).Run(); err != nil {
			return fmt.Errorf("copy credential into container: %w", err)
		}
	}
	return nil
}

func removeFromContainer(ctx context.Context, containerID string, files map[string]string) {
	for path := range files {
		if err := exec.CommandContext(ctx, "docker", "exec", containerID, "rm", "-f", path).Run(); err != nil {
			slog.Default().Debug("remove staged file from container", "path", path, "error", err)
		}
	}
}

// insertFlag places a CLI flag at index pos of argv (after the binary name).
func insertFlag(argv []string, pos int, flag string) []string {
	if pos < 1 {
		pos = 1
	}
	if pos > len(argv) {
		pos = len(argv)
	}
	out := make([]string, 0, len(argv)+1)
	out = append(out, argv[:pos]...)
	out = append(out, flag)
	return append(out, argv[pos:]...)
}

// ---------------------------------------------------------------------------
// CLI output parsing
// ---------------------------------------------------------------------------

type resticSnapshotEntry struct {
	Time     string   `json:"time"`
	Hostname string   `json:"hostname"`
	ShortID  string   `json:"short_id"`
	ID       string   `json:"id"`
	Tree     string   `json:"tree"`
	Parent   string   `json:"parent"`
	Paths    []string `json:"paths"`
}

// resticSnapshots parses `restic snapshots --json`, accepting both a JSON array
// and a JSON-lines stream.
func resticSnapshots(stdout, repoID string) []Snapshot {
	entries := parseResticSnapshotEntries(stdout)
	out := make([]Snapshot, 0, len(entries))
	for _, entry := range entries {
		id := entry.ShortID
		if id == "" {
			id = entry.ID
		}
		snap := Snapshot{
			ID:         uuid.NewString(),
			RepoID:     repoID,
			SnapshotID: id,
			Paths:      cleanPaths(entry.Paths),
			Hostname:   entry.Hostname,
			Parent:     entry.Parent,
			Tree:       entry.Tree,
		}
		snap.Timestamp = parseTimestamp(entry.Time)
		out = append(out, snap)
	}
	return out
}

func parseResticSnapshotEntries(stdout string) []resticSnapshotEntry {
	text := strings.TrimSpace(stdout)
	if text == "" {
		return nil
	}
	var array []resticSnapshotEntry
	if err := json.Unmarshal([]byte(text), &array); err == nil {
		return array
	}
	var lines []resticSnapshotEntry
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var entry resticSnapshotEntry
		if err := json.Unmarshal([]byte(line), &entry); err == nil && (entry.ID != "" || entry.ShortID != "") {
			lines = append(lines, entry)
		}
	}
	return lines
}

type resticSummary struct {
	MessageType   string `json:"message_type"`
	SnapshotID    string `json:"snapshot_id"`
	SnapshotAlt   string `json:"snapshot"`
	Tree          string `json:"tree"`
	TotalFiles    int64  `json:"total_files"`
	TotalByteSize int64  `json:"total_byte_size"`
	ExtendedStats struct {
		Tree string `json:"tree"`
	} `json:"extended_stats"`
}

func (s resticSummary) snapshotID() string {
	if s.SnapshotID != "" {
		return s.SnapshotID
	}
	return s.SnapshotAlt
}

func (s resticSummary) tree() string {
	if s.ExtendedStats.Tree != "" {
		return s.ExtendedStats.Tree
	}
	return s.Tree
}

// parseResticBackupSummary returns the trailing `summary` message of a
// `restic backup --json` stream.
func parseResticBackupSummary(stdout string) *resticSummary {
	var found *resticSummary
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var msg resticSummary
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.MessageType == "summary" || msg.snapshotID() != "" {
			local := msg
			found = &local
		}
	}
	return found
}

type kopiaSnapshotEntry struct {
	SnapshotID string `json:"snapshotID"`
	ID         string `json:"id"`
	StartTime  string `json:"startTime"`
	Source     struct {
		Path string `json:"path"`
		Host string `json:"host"`
	} `json:"source"`
	Stats struct {
		TotalSize int64 `json:"totalSize"`
		FileCount int64 `json:"fileCount"`
	} `json:"stats"`
	Parents []string `json:"parents"`
}

// kopiaSnapshots parses `kopia snapshot list --json`.
func kopiaSnapshots(stdout, repoID string) []Snapshot {
	var entries []kopiaSnapshotEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &entries); err != nil {
		return nil
	}
	out := make([]Snapshot, 0, len(entries))
	for _, entry := range entries {
		id := entry.SnapshotID
		if id == "" {
			id = entry.ID
		}
		snap := Snapshot{
			ID:             uuid.NewString(),
			RepoID:         repoID,
			SnapshotID:     id,
			Hostname:       entry.Source.Host,
			DataFiles:      entry.Stats.FileCount,
			TotalSizeBytes: entry.Stats.TotalSize,
		}
		if path := strings.TrimSpace(entry.Source.Path); path != "" {
			snap.Paths = []string{path}
		}
		snap.Timestamp = parseTimestamp(entry.StartTime)
		if len(entry.Parents) > 0 {
			snap.Parent = entry.Parents[len(entry.Parents)-1]
		}
		out = append(out, snap)
	}
	return out
}

// parseKopiaSnapshotID extracts the snapshot id from `kopia snapshot create
// --json` output, tolerating the plain-text form.
func parseKopiaSnapshotID(stdout string) string {
	text := strings.TrimSpace(stdout)
	if text == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err == nil {
		for _, key := range []string{"snapshotID", "snapshot_id", "id", "snapshot"} {
			if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
		if inner, ok := payload["snapshot"].(map[string]any); ok {
			if value, ok := inner["id"].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	for _, line := range strings.Split(text, "\n") {
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "ks") && len(field) > 4 {
				return field
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Conversion + misc helpers
// ---------------------------------------------------------------------------

func repositoryFromRecord(r store.BackupEngineRepository) Repository {
	return Repository{
		ID:             r.ID,
		Name:           r.Name,
		Engine:         r.Engine,
		Location:       r.Location,
		PasswordRef:    passwordDescriptor(r.PasswordRef),
		Encryption:     r.Encryption,
		PrunePolicy:    defaultRaw(r.PrunePolicy),
		NodeID:         r.NodeID,
		ServerID:       r.ServerID,
		ArtifactID:     r.ArtifactID,
		Initialized:    r.Initialized,
		LastSnapshotAt: r.LastSnapshotAt,
		NextRunAt:      r.NextRunAt,
		LastError:      r.LastError,
		CreatedAt:      r.CreatedAt,
	}
}

// passwordDescriptor keeps the stored secret out of API responses: env refs
// are names (safe to show), literal passwords are reported as "sealed".
func passwordDescriptor(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, envPasswordPrefix) {
		return ref
	}
	return "sealed"
}

func snapshotFromRecord(s store.BackupEngineSnapshot) Snapshot {
	return Snapshot{
		ID:             s.ID,
		RepoID:         s.RepoID,
		SnapshotID:     s.SnapshotID,
		Paths:          decodePaths(s.Paths),
		Hostname:       s.Hostname,
		Timestamp:      s.Timestamp,
		Parent:         s.Parent,
		Tree:           s.Tree,
		DataFiles:      s.DataFiles,
		TotalSizeBytes: s.TotalSizeBytes,
		VerifiedAt:     s.VerifiedAt,
	}
}

func restoreJobFromRecord(j store.BackupEngineRestoreJob) RestoreJob {
	return RestoreJob{
		ID:          j.ID,
		RepoID:      j.RepoID,
		SnapshotID:  j.SnapshotID,
		TargetPath:  j.TargetPath,
		Status:      j.Status,
		ProgressPct: j.ProgressPct,
		StartedAt:   j.StartedAt,
		CompletedAt: j.CompletedAt,
		Error:       j.Error,
		CreatedAt:   j.CreatedAt,
	}
}

func decodePrunePolicy(raw json.RawMessage) PrunePolicy {
	var policy PrunePolicy
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &policy)
	}
	policy.Schedule = strings.TrimSpace(policy.Schedule)
	return policy
}

func resticKeepFlags(policy PrunePolicy) []string {
	var flags []string
	add := func(name string, value int) {
		if value > 0 {
			flags = append(flags, "--"+name, strconv.Itoa(value))
		}
	}
	add("keep-last", policy.KeepLast)
	add("keep-hourly", policy.KeepHourly)
	add("keep-daily", policy.KeepDaily)
	add("keep-weekly", policy.KeepWeekly)
	add("keep-monthly", policy.KeepMonthly)
	return flags
}

// kopiaKeepFlags maps the restic-style keep rules onto kopia's `snapshot
// expire` policy flags: --min-retain keeps the newest N snapshots and
// --min-age protects anything younger than the daily retention window.
func kopiaKeepFlags(policy PrunePolicy) []string {
	var flags []string
	if policy.KeepLast > 0 {
		flags = append(flags, "--min-retain", strconv.Itoa(policy.KeepLast))
	}
	if policy.KeepDaily > 0 {
		flags = append(flags, "--min-age", strconv.Itoa(policy.KeepDaily*24)+"h")
	}
	return flags
}

func normalizeEncryption(engine, requested string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested != "" {
		return requested
	}
	if engine == EngineKopia {
		return "salsa2012-sha256"
	}
	return "aes-256"
}

// splitKopiaLocation maps a repository location onto kopia's
// `repository connect <backend> <path>` argument shape. Locations are written
// as "<backend>:<path>" (filesystem:/backups, s3:bucket/prefix, gs:…); a bare
// path is treated as the filesystem backend.
func splitKopiaLocation(location string) (string, string) {
	idx := strings.Index(location, ":")
	if idx <= 0 {
		return "filesystem", location
	}
	backend := strings.ToLower(location[:idx])
	rest := strings.TrimLeft(location[idx+1:], "/")
	switch backend {
	case "", "fs", "file", "local", "filesystem":
		return "filesystem", rest
	default:
		return backend, rest
	}
}

func resticPasswordFile(repoID string) string {
	return filepath.Join(os.TempDir(), "forge-restic-"+sanitizeID(repoID))
}

func kopiaConfigPath(repoID string) string {
	return filepath.Join(os.TempDir(), "forge-kopia-"+sanitizeID(repoID)+".cfg")
}

func sanitizeID(value string) string {
	return strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-").Replace(value)
}

func cleanPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || !singleLine(p) {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func singleLine(values ...string) bool {
	for _, value := range values {
		if strings.ContainsAny(value, "\r\n\x00") {
			return false
		}
	}
	return true
}

func clipCommandError(res CommandResult) string {
	msg := strings.TrimSpace(res.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(res.Stdout)
	}
	if msg == "" {
		msg = "command exited with code " + strconv.Itoa(res.ExitCode)
	}
	// Never surface staged credential paths or password material.
	for _, marker := range []string{"password-file", "--password=", "KOPIA_PASSWORD", "RESTIC_PASSWORD"} {
		if idx := strings.Index(msg, marker); idx >= 0 {
			msg = truncate(msg[:idx], maxErrorLength) + " [output redacted]"
			break
		}
	}
	return truncate(msg, maxErrorLength)
}

func truncate(msg string, limit int) string {
	if len(msg) <= limit {
		return msg
	}
	return msg[:limit] + "…"
}

// looksAlreadyPresent recognises the idempotent "repository already exists"
// responses of both engines so re-init is not reported as a failure.
func looksAlreadyPresent(res CommandResult) bool {
	joined := strings.ToLower(res.Stdout + " " + res.Stderr)
	for _, marker := range []string{"already initialized", "already in use", "repository is already", "already connected"} {
		if strings.Contains(joined, marker) {
			return true
		}
	}
	return false
}

func defaultRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func mustJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`[]`)
	}
	return raw
}

func decodePaths(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err == nil {
		return out
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil && strings.TrimSpace(single) != "" {
		return []string{strings.TrimSpace(single)}
	}
	return nil
}

func parseTimestamp(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			utc := parsed.UTC()
			return &utc
		}
	}
	return nil
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func hostName() string {
	name, err := os.Hostname()
	if err != nil {
		return "forge"
	}
	return name
}
