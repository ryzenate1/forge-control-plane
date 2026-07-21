package migration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/events"
	gpruntime "gamepanel/forge/internal/runtime"
	"gamepanel/forge/internal/services/evacuationplanner"
	reservationsvc "gamepanel/forge/internal/services/reservations"
	schedulersvc "gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/store"
)

const (
	DefaultReconcileInterval = 30 * time.Second
	maxConcurrentWorkers     = 4
	migrationTaskTimeout     = 30 * time.Second
)

type Metrics struct {
	MigrationTotal          uint64 `json:"migration_total"`
	MigrationCompletedTotal uint64 `json:"migration_completed_total"`
	MigrationFailedTotal    uint64 `json:"migration_failed_total"`
	ReconciliationTotal     uint64 `json:"migration_reconciliation_total"`
	ReconciliationFailures  uint64 `json:"migration_reconciliation_failures"`
	CleanupCompletedTotal   uint64 `json:"migration_cleanup_completed_total"`
}

type CreateMigrationRequest struct {
	ServerID     string `json:"serverId"`
	SourceNodeID string `json:"sourceNodeId,omitempty"`
	TargetNodeID string `json:"targetNodeId,omitempty"`
}

// NotImplementedError reports a migration lifecycle operation that has no
// workload executor. Callers can use errors.As to map it to an HTTP 501.
type NotImplementedError struct {
	Operation   string
	MigrationID string
}

func (e *NotImplementedError) Error() string {
	return e.Operation + " is not implemented; no workload transfer and restore executor is available"
}

func (e *NotImplementedError) Unwrap() error {
	return gpruntime.ErrNotImplemented
}

type Service struct {
	store             *store.Store
	scheduler         schedulersvc.Service
	evacuationPlanner *evacuationplanner.Service
	reservations      *reservationsvc.Manager
	runtime           gpruntime.Runtime
	publisher         events.Publisher
	daemon            *daemon.Client
	workerID          string
	interval          time.Duration
	mu                sync.Mutex
	active            map[string]context.CancelFunc
	metrics           Metrics
	startOnce         sync.Once
	reconcileMu       sync.Mutex
	wg                sync.WaitGroup
	cancel            context.CancelFunc
}

func New(store *store.Store, scheduler schedulersvc.Service, evacuationPlanner *evacuationplanner.Service, reservationManager *reservationsvc.Manager, runtime gpruntime.Runtime, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	service := &Service{store: store, scheduler: scheduler, evacuationPlanner: evacuationPlanner, reservations: reservationManager, runtime: runtime, publisher: publisher, workerID: randomID(), interval: DefaultReconcileInterval, active: make(map[string]context.CancelFunc)}
	if provider, ok := runtime.(interface{ TransferClient() *daemon.Client }); ok {
		service.daemon = provider.TransferClient()
	}
	if evacuationPlanner != nil {
		evacuationPlanner.SetMigrationExecutor(service)
	}
	return service
}

// CreateEvacuationMigration implements evacuationplanner.MigrationExecutor.
func (s *Service) CreateEvacuationMigration(ctx context.Context, serverID, sourceNodeID, targetNodeID string) (string, error) {
	migration, err := s.CreateMigration(ctx, CreateMigrationRequest{ServerID: serverID, SourceNodeID: sourceNodeID, TargetNodeID: targetNodeID})
	return migration.ID, err
}

// PrepareEvacuationMigration implements evacuationplanner.MigrationExecutor.
func (s *Service) PrepareEvacuationMigration(ctx context.Context, migrationID string) error {
	_, err := s.PrepareMigration(ctx, migrationID)
	return err
}

// ExecuteEvacuationMigration implements evacuationplanner.MigrationExecutor.
func (s *Service) ExecuteEvacuationMigration(ctx context.Context, migrationID string) error {
	_, err := s.ExecuteMigration(ctx, migrationID)
	return err
}

// CancelEvacuationMigration implements evacuationplanner.MigrationExecutor.
func (s *Service) CancelEvacuationMigration(ctx context.Context, migrationID string) error {
	_, err := s.CancelMigration(ctx, migrationID)
	return err
}

// EvacuationMigrationStatus implements evacuationplanner.MigrationExecutor.
func (s *Service) EvacuationMigrationStatus(ctx context.Context, migrationID string) (string, error) {
	migration, err := s.GetMigration(ctx, migrationID)
	return migration.Status, err
}

func (s *Service) Metrics() Metrics {
	if s == nil {
		return Metrics{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metrics
}

func (s *Service) CreateMigration(ctx context.Context, req CreateMigrationRequest) (store.Migration, error) {
	normalized, err := s.ValidateMigration(ctx, req)
	if err != nil {
		return store.Migration{}, err
	}
	migration, err := s.store.CreateMigration(ctx, normalized)
	if err != nil {
		return store.Migration{}, err
	}
	if s.reservations != nil {
		server, err := s.store.GetServer(ctx, normalized.ServerID)
		if err != nil {
			_, _ = s.store.UpdateMigrationStatus(ctx, migration.ID, store.MigrationStatusFailed, "migration reservation server lookup failed")
			return store.Migration{}, err
		}
		if _, err := s.reservations.CreateReservation(ctx, store.CreatePlacementReservationRequest{
			NodeID:          migration.TargetNodeID,
			ServerID:        migration.ServerID,
			MigrationID:     migration.ID,
			ReservationType: store.PlacementReservationTypeMigration,
			CPU:             server.CPUShares,
			Memory:          int64(server.MemoryMB),
			Disk:            int64(server.DiskMB),
			Status:          store.PlacementReservationStatusActive,
			ExpiresAt:       time.Now().UTC().Add(30 * time.Minute),
		}); err != nil {
			_, _ = s.store.UpdateMigrationStatus(ctx, migration.ID, store.MigrationStatusFailed, "migration reservation failed")
			return store.Migration{}, err
		}
	}
	s.increment(func(metrics *Metrics) {
		metrics.MigrationTotal++
	})
	s.publish(ctx, events.EventMigrationCreated, "migration", migration.ID, map[string]any{
		"serverId":     migration.ServerID,
		"sourceNodeId": migration.SourceNodeID,
		"targetNodeId": migration.TargetNodeID,
		"status":       migration.Status,
	})
	return migration, nil
}

func (s *Service) ValidateMigration(ctx context.Context, req CreateMigrationRequest) (store.CreateMigrationRequest, error) {
	if s == nil || s.store == nil {
		return store.CreateMigrationRequest{}, errors.New("store unavailable")
	}
	serverID := strings.TrimSpace(req.ServerID)
	if serverID == "" {
		return store.CreateMigrationRequest{}, errors.New("serverId is required")
	}
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return store.CreateMigrationRequest{}, err
	}
	sourceNodeID := strings.TrimSpace(req.SourceNodeID)
	if sourceNodeID == "" {
		sourceNodeID, err = s.store.ServerNodeID(ctx, serverID)
		if err != nil {
			return store.CreateMigrationRequest{}, err
		}
	}
	source, err := s.store.GetNode(ctx, sourceNodeID)
	if err != nil {
		return store.CreateMigrationRequest{}, err
	}
	targetNodeID := strings.TrimSpace(req.TargetNodeID)
	if targetNodeID == "" {
		targetNodeID, err = s.planTarget(ctx, server, source)
		if err != nil {
			return store.CreateMigrationRequest{}, err
		}
	}
	if sourceNodeID == targetNodeID {
		return store.CreateMigrationRequest{}, errors.New("target node must be different from source node")
	}
	target, err := s.store.GetNode(ctx, targetNodeID)
	if err != nil {
		return store.CreateMigrationRequest{}, err
	}
	if err := s.validateTarget(ctx, server, source, target); err != nil {
		return store.CreateMigrationRequest{}, err
	}
	return store.CreateMigrationRequest{
		ServerID:     serverID,
		SourceNodeID: sourceNodeID,
		TargetNodeID: targetNodeID,
	}, nil
}

func (s *Service) PrepareMigration(ctx context.Context, migrationID string) (store.Migration, error) {
	if s == nil || s.store == nil || s.daemon == nil || s.runtime == nil {
		return store.Migration{}, &NotImplementedError{Operation: "migration preparation", MigrationID: migrationID}
	}
	migration, err := s.store.GetMigration(ctx, migrationID)
	if err != nil {
		return store.Migration{}, err
	}
	if migration.Status != string(store.MigrationStatusPlanned) && migration.Status != string(store.MigrationStatusPreparing) {
		return store.Migration{}, errors.New("migration is not in a preparable state")
	}
	if _, err := s.store.EnsureMigrationRun(ctx, migrationID, daemon.TransferProtocolVersion); err != nil {
		return store.Migration{}, err
	}
	if migration.Status == string(store.MigrationStatusPlanned) {
		if _, err := s.store.UpdateMigrationStatus(ctx, migrationID, store.MigrationStatusPreparing, "real transfer run prepared"); err != nil {
			return store.Migration{}, err
		}
	}
	return s.store.GetMigration(ctx, migrationID)
}

func (s *Service) ExecuteMigration(ctx context.Context, migrationID string) (store.Migration, error) {
	if _, err := s.PrepareMigration(ctx, migrationID); err != nil {
		return store.Migration{}, err
	}
	s.startRun(context.Background(), migrationID)
	return s.store.GetMigration(ctx, migrationID)
}

// Start continuously reconciles durable migration work. It is safe to call
// more than once; only the first call starts the lifecycle goroutine.
func (s *Service) Start(ctx context.Context) {
	if s == nil || s.store == nil || s.daemon == nil {
		return
	}
	s.startOnce.Do(func() {
		ctx, s.cancel = context.WithCancel(ctx)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.RunOnce(ctx)
			ticker := time.NewTicker(s.interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s.RunOnce(ctx)
				}
			}
		}()
	})
}

// Shutdown signals the reconcile goroutine to stop and blocks until it
// exits or the provided context is cancelled.
func (s *Service) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RunOnce reclaims expired transfer leases and retries completed migrations
// whose remote cleanup was interrupted. A later pass retries transient errors.
func (s *Service) RunOnce(ctx context.Context) {
	if s == nil || s.store == nil || s.daemon == nil {
		return
	}
	if !s.reconcileMu.TryLock() {
		return
	}
	defer s.reconcileMu.Unlock()
	s.increment(func(metrics *Metrics) { metrics.ReconciliationTotal++ })
	if s.runtime != nil {
		if err := s.reclaim(ctx); err != nil {
			s.increment(func(metrics *Metrics) { metrics.ReconciliationFailures++ })
		}
	}
	if err := s.cleanup(ctx); err != nil {
		s.increment(func(metrics *Metrics) { metrics.ReconciliationFailures++ })
	}
}

func (s *Service) reclaim(ctx context.Context) error {
	queryCtx, cancel := context.WithTimeout(ctx, migrationTaskTimeout)
	defer cancel()
	ids, err := s.store.ReclaimableMigrationIDs(queryCtx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		s.startRun(ctx, id)
	}
	return nil
}

func (s *Service) startRun(parent context.Context, migrationID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, running := s.active[migrationID]; running || len(s.active) >= maxConcurrentWorkers {
		return false
	}
	workerCtx, cancel := context.WithCancel(parent)
	s.active[migrationID] = cancel
	go s.run(workerCtx, migrationID)
	return true
}

func (s *Service) cleanup(ctx context.Context) error {
	ids, err := s.store.CleanupPendingMigrationIDs(ctx)
	if err != nil {
		return err
	}
	semaphore := make(chan struct{}, maxConcurrentWorkers)
	var workers sync.WaitGroup
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		semaphore <- struct{}{}
		workers.Add(1)
		go func(migrationID string) {
			defer workers.Done()
			defer func() { <-semaphore }()
			taskCtx, cancel := context.WithTimeout(ctx, migrationTaskTimeout)
			defer cancel()
			if s.cleanupMigration(taskCtx, migrationID) == nil {
				s.increment(func(metrics *Metrics) { metrics.CleanupCompletedTotal++ })
			}
		}(id)
	}
	workers.Wait()
	return nil
}

func (s *Service) cleanupMigration(ctx context.Context, migrationID string) error {
	migration, err := s.store.GetMigration(ctx, migrationID)
	if err != nil {
		return err
	}
	if migration.Status != string(store.MigrationStatusCompleted) || !migration.CleanupPending {
		return nil
	}
	source, target, err := s.store.MigrationProvisionTargets(ctx, migrationID)
	if err != nil {
		return err
	}
	expires := time.Now().UTC().Add(5 * time.Minute)
	sourceCredential, err := newCredential()
	if err != nil {
		return err
	}
	destinationCredential, err := newCredential()
	if err != nil {
		return err
	}
	claims := daemon.TransferCredentialClaims{Version: daemon.TransferProtocolVersion, MigrationID: migration.ID, ServerID: migration.ServerID, SourceNodeID: migration.SourceNodeID, TargetNodeID: migration.TargetNodeID, ExpiresAt: expires}
	claims.Direction = daemon.TransferDirectionSourceControl
	if err := s.daemon.RegisterTransferCredential(ctx, source.NodeURL, source.NodeToken, daemon.TransferCredentialRegistration{Claims: claims, CredentialHash: credentialHash(sourceCredential)}); err != nil {
		return err
	}
	claims.Direction = daemon.TransferDirectionDestinationUpload
	if err := s.daemon.RegisterTransferCredential(ctx, target.NodeURL, target.NodeToken, daemon.TransferCredentialRegistration{Claims: claims, CredentialHash: credentialHash(destinationCredential)}); err != nil {
		return err
	}
	if err := s.daemon.FinalizeTransferDestination(ctx, target.NodeURL, migrationID, destinationCredential); err != nil {
		return err
	}
	if err := s.daemon.CleanupTransferSource(ctx, source.NodeURL, migrationID, sourceCredential); err != nil {
		return err
	}
	return s.store.MarkMigrationCleanupComplete(ctx, migrationID)
}

func (s *Service) run(ctx context.Context, migrationID string) {
	defer func() {
		s.mu.Lock()
		if cancel := s.active[migrationID]; cancel != nil {
			cancel()
		}
		delete(s.active, migrationID)
		s.mu.Unlock()
	}()
	run, err := s.store.ClaimMigrationRun(ctx, migrationID, s.workerID, 2*time.Minute)
	if err != nil {
		return
	}
	migration, err := s.store.GetMigration(ctx, migrationID)
	if err != nil {
		s.fail(ctx, migrationID, err, store.ServerProvisionTarget{}, store.ServerProvisionTarget{}, false, "", "")
		return
	}
	source, target, err := s.store.MigrationProvisionTargets(ctx, migrationID)
	if err != nil {
		s.fail(ctx, migrationID, err, source, target, false, "", "")
		return
	}
	sourceCredential, err := newCredential()
	if err != nil {
		s.fail(ctx, migrationID, fmt.Errorf("generate source credential: %w", err), source, target, false, "", "")
		return
	}
	destinationCredential, err := newCredential()
	if err != nil {
		s.fail(ctx, migrationID, fmt.Errorf("generate destination credential: %w", err), source, target, false, sourceCredential, "")
		return
	}
	expires := time.Now().UTC().Add(30 * time.Minute)
	claims := daemon.TransferCredentialClaims{Version: daemon.TransferProtocolVersion, MigrationID: migration.ID, ServerID: migration.ServerID, SourceNodeID: migration.SourceNodeID, TargetNodeID: migration.TargetNodeID, ExpiresAt: expires}
	claims.Direction = daemon.TransferDirectionSourceControl
	sourceRegistration := daemon.TransferCredentialRegistration{Claims: claims, CredentialHash: credentialHash(sourceCredential)}
	claims.Direction = daemon.TransferDirectionDestinationUpload
	destinationRegistration := daemon.TransferCredentialRegistration{Claims: claims, CredentialHash: credentialHash(destinationCredential)}
	if err := s.store.SetMigrationCredentialHashes(ctx, migrationID, sourceRegistration.CredentialHash, destinationRegistration.CredentialHash, expires); err != nil {
		s.fail(ctx, migrationID, err, source, target, false, sourceCredential, destinationCredential)
		return
	}
	if err := s.daemon.RegisterTransferCredential(ctx, source.NodeURL, source.NodeToken, sourceRegistration); err != nil {
		s.fail(ctx, migrationID, fmt.Errorf("register source credential: %w", err), source, target, false, sourceCredential, destinationCredential)
		return
	}
	if err := s.daemon.RegisterTransferCredential(ctx, target.NodeURL, target.NodeToken, destinationRegistration); err != nil {
		s.fail(ctx, migrationID, fmt.Errorf("register destination credential: %w", err), source, target, false, sourceCredential, destinationCredential)
		return
	}
	if !phaseAtLeast(run.Phase, "source_archived") {
		_, _ = s.store.UpdateMigrationRun(ctx, migrationID, "credentials_registered", "", 0, "")
	}
	if run.Phase == "destination_created" {
		s.finalize(ctx, migrationID, source, target, sourceCredential, destinationCredential)
		return
	}
	archive := daemon.TransferMetadata{ArchiveSize: run.ArchiveSize, Checksum: run.ArchiveChecksum, Phase: "archived"}
	if !phaseAtLeast(run.Phase, "source_archived") {
		archive, err = s.daemon.PrepareTransferSource(ctx, source.NodeURL, migrationID, sourceCredential)
		if err != nil {
			s.fail(ctx, migrationID, fmt.Errorf("prepare source: %w", err), source, target, false, sourceCredential, destinationCredential)
			return
		}
		if archive.Phase != "archived" || archive.ArchiveSize <= 0 || archive.Checksum == "" {
			s.fail(ctx, migrationID, errors.New("source did not report a complete archive"), source, target, false, sourceCredential, destinationCredential)
			return
		}
		_, _ = s.store.UpdateMigrationStatus(ctx, migrationID, store.MigrationStatusTransferring, "source stopped and archive created")
		_, _ = s.store.UpdateMigrationRun(ctx, migrationID, "source_archived", "", archive.ArchiveSize, archive.Checksum)
	}
	if !phaseAtLeast(run.Phase, "destination_verified") {
		verified, pushErr := s.daemon.PushTransferSource(ctx, source.NodeURL, migrationID, sourceCredential, daemon.TransferPushRequest{
			DestinationURL: target.NodeURL, DestinationCredential: destinationCredential,
			IdempotencyKey: migration.IdempotencyKey + ":upload",
		})
		if pushErr != nil {
			s.fail(ctx, migrationID, fmt.Errorf("transfer archive: %w", pushErr), source, target, false, sourceCredential, destinationCredential)
			return
		}
		if verified.Phase != "verified" || verified.Offset != archive.ArchiveSize || !strings.EqualFold(verified.Checksum, archive.Checksum) {
			s.fail(ctx, migrationID, errors.New("destination verification report is inconsistent with source archive"), source, target, false, sourceCredential, destinationCredential)
			return
		}
		_, _ = s.store.UpdateMigrationRun(ctx, migrationID, "destination_verified", "", verified.ArchiveSize, verified.Checksum)
		_, _ = s.store.UpdateMigrationStatus(ctx, migrationID, store.MigrationStatusRestoring, "destination checksum verified")
	}
	if !phaseAtLeast(run.Phase, "destination_restored") {
		restored, restoreErr := s.daemon.RestoreTransferDestination(ctx, target.NodeURL, migrationID, destinationCredential)
		if restoreErr != nil || restored.Phase != "restored" {
			if restoreErr == nil {
				restoreErr = errors.New("destination did not report restored")
			}
			s.fail(ctx, migrationID, fmt.Errorf("restore destination: %w", restoreErr), source, target, false, sourceCredential, destinationCredential)
			return
		}
		_, _ = s.store.UpdateMigrationRun(ctx, migrationID, "destination_restored", "", 0, "")
	}
	runtimeTarget := runtimeTarget(target)
	if !phaseAtLeast(run.Phase, "destination_configured") {
		if err := s.runtime.SyncServerConfiguration(ctx, runtimeTarget, runtimeConfiguration(target)); err != nil {
			s.fail(ctx, migrationID, fmt.Errorf("sync destination configuration: %w", err), source, target, false, sourceCredential, destinationCredential)
			return
		}
		_, _ = s.store.UpdateMigrationRun(ctx, migrationID, "destination_configured", "", 0, "")
	}
	created, err := s.runtime.CreateServer(ctx, runtimeTarget, runtimeCreateRequest(target))
	if err != nil || !created.Accepted {
		if err == nil {
			err = errors.New("destination rejected container creation")
		}
		s.fail(ctx, migrationID, fmt.Errorf("create destination container: %w", err), source, target, false, sourceCredential, destinationCredential)
		return
	}
	exists, err := s.runtime.Exists(ctx, runtimeTarget)
	if err != nil || !exists {
		if err == nil {
			err = errors.New("post-creation health check failed: server does not exist on destination")
		}
		s.fail(ctx, migrationID, fmt.Errorf("destination health check: %w", err), source, target, true, sourceCredential, destinationCredential)
		return
	}
	_, _ = s.store.UpdateMigrationRun(ctx, migrationID, "destination_created", "", 0, "")
	s.finalize(ctx, migrationID, source, target, sourceCredential, destinationCredential)
}

func (s *Service) finalize(ctx context.Context, migrationID string, source, target store.ServerProvisionTarget, sourceCredential, destinationCredential string) {
	if err := s.store.FinalizeMigration(ctx, migrationID); err != nil {
		s.fail(ctx, migrationID, fmt.Errorf("commit migration ownership: %w", err), source, target, true, sourceCredential, destinationCredential)
		return
	}
	// Rollback data is discarded and the source is destroyed only after the ownership/allocation commit.
	if err := s.daemon.FinalizeTransferDestination(ctx, target.NodeURL, migrationID, destinationCredential); err == nil {
		if err := s.daemon.CleanupTransferSource(ctx, source.NodeURL, migrationID, sourceCredential); err == nil {
			_ = s.store.MarkMigrationCleanupComplete(ctx, migrationID)
		}
	}
	if s.reservations != nil {
		s.reservations.CompleteMigrationReservations(ctx, migrationID)
	}
	s.increment(func(metrics *Metrics) { metrics.MigrationCompletedTotal++ })
	s.publish(ctx, events.EventMigrationCompleted, "migration", migrationID, map[string]any{"status": "completed"})
}

func phaseAtLeast(current, expected string) bool {
	order := map[string]int{"planned": 0, "credentials_registered": 1, "source_archived": 2, "destination_verified": 3, "destination_restored": 4, "destination_configured": 5, "destination_created": 6, "completed": 7}
	return order[current] >= order[expected]
}

func (s *Service) fail(ctx context.Context, migrationID string, cause error, source, target store.ServerProvisionTarget, destinationCreated bool, sourceCredential, destinationCredential string) {
	rollbackCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if destinationCredential != "" && target.NodeURL != "" {
		_ = s.daemon.CancelTransfer(rollbackCtx, target.NodeURL, migrationID, destinationCredential)
	}
	if destinationCreated && target.NodeURL != "" {
		_, _ = s.runtime.DeleteServer(rollbackCtx, runtimeTarget(target))
	}
	if sourceCredential != "" && source.NodeURL != "" {
		_ = s.daemon.CancelTransfer(rollbackCtx, source.NodeURL, migrationID, sourceCredential)
	}
	if source.NodeURL != "" {
		_, _ = s.runtime.StartServer(rollbackCtx, runtimeTarget(source))
	}
	_ = s.store.FailMigrationRun(rollbackCtx, migrationID, cause.Error())
	_, _ = s.MarkFailed(rollbackCtx, migrationID, cause.Error())
}

func (s *Service) CancelMigration(ctx context.Context, migrationID string) (store.Migration, error) {
	migration, err := s.store.GetMigration(ctx, migrationID)
	if err != nil {
		return store.Migration{}, err
	}
	switch migration.Status {
	case string(store.MigrationStatusCompleted), string(store.MigrationStatusFailed), string(store.MigrationStatusCancelled):
		return store.Migration{}, errors.New("terminal migration cannot be cancelled")
	}
	s.mu.Lock()
	if cancel := s.active[migrationID]; cancel != nil {
		cancel()
	}
	s.mu.Unlock()
	// Rotate fresh scoped credentials so cancellation also propagates after an API restart.
	if s.daemon != nil {
		if source, target, targetErr := s.store.MigrationProvisionTargets(ctx, migrationID); targetErr == nil {
			expires := time.Now().UTC().Add(5 * time.Minute)
			for _, item := range []struct {
				target    store.ServerProvisionTarget
				direction string
			}{{source, daemon.TransferDirectionSourceControl}, {target, daemon.TransferDirectionDestinationUpload}} {
				credential, err := newCredential()
				if err != nil {
					continue
				}
				claims := daemon.TransferCredentialClaims{Version: daemon.TransferProtocolVersion, MigrationID: migration.ID, ServerID: migration.ServerID, SourceNodeID: migration.SourceNodeID, TargetNodeID: migration.TargetNodeID, Direction: item.direction, ExpiresAt: expires}
				if s.daemon.RegisterTransferCredential(ctx, item.target.NodeURL, item.target.NodeToken, daemon.TransferCredentialRegistration{Claims: claims, CredentialHash: credentialHash(credential)}) == nil {
					_ = s.daemon.CancelTransfer(ctx, item.target.NodeURL, migrationID, credential)
				}
			}
		}
	}
	_ = s.store.CancelMigrationRun(ctx, migrationID)
	migration, err = s.store.UpdateMigrationStatus(ctx, migrationID, store.MigrationStatusCancelled, "migration cancelled")
	if err != nil {
		return store.Migration{}, err
	}
	if s.reservations != nil {
		s.reservations.CancelMigrationReservations(ctx, migration.ID)
	}
	s.publish(ctx, events.EventMigrationCancelled, "migration", migration.ID, map[string]any{"status": migration.Status})
	return migration, nil
}

func (s *Service) ListMigrations(ctx context.Context) ([]store.Migration, error) {
	return s.store.ListMigrations(ctx)
}

func (s *Service) GetMigration(ctx context.Context, migrationID string) (store.Migration, error) {
	return s.store.GetMigration(ctx, migrationID)
}

func (s *Service) MarkFailed(ctx context.Context, migrationID, reason string) (store.Migration, error) {
	migration, err := s.store.UpdateMigrationStatus(ctx, migrationID, store.MigrationStatusFailed, reason)
	if err != nil {
		return store.Migration{}, err
	}
	if s.reservations != nil {
		s.reservations.CancelMigrationReservations(ctx, migration.ID)
	}
	s.increment(func(metrics *Metrics) {
		metrics.MigrationFailedTotal++
	})
	s.publish(ctx, events.EventMigrationFailed, "migration", migration.ID, map[string]any{"status": migration.Status, "reason": reason})
	return migration, nil
}

func (s *Service) planTarget(ctx context.Context, server store.Server, source store.Node) (string, error) {
	if s.evacuationPlanner == nil {
		return "", errors.New("targetNodeId is required")
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return "", err
	}
	selected, _, reason := s.evacuationPlanner.FindCandidates(ctx, server, source, nodes)
	if selected.ID == "" {
		return "", errors.New(reason)
	}
	return selected.ID, nil
}

func (s *Service) validateTarget(ctx context.Context, server store.Server, source store.Node, target store.Node) error {
	req := domain.PlacementRequest{
		RequiredNode: target.ID,
		MemoryMB:     server.MemoryMB,
		CPU:          server.CPUShares,
		DiskMB:       server.DiskMB,
	}
	if source.RegionID != nil {
		req.RegionID = *source.RegionID
	}
	filtered, err := s.scheduler.FilterNodes(ctx, req, []store.Node{target})
	if err != nil {
		return err
	}
	if len(filtered) == 0 {
		return errors.New("target node does not satisfy migration placement constraints")
	}
	if s.evacuationPlanner != nil {
		if _, err := s.evacuationPlanner.ValidateCapacity(ctx, target.ID, server); err != nil {
			return err
		}
	}
	return nil
}

func newCredential() (string, error) {
	body := make([]byte, 32)
	if _, err := rand.Read(body); err != nil {
		return "", err
	}
	return hex.EncodeToString(body), nil
}

func credentialHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func randomID() string {
	value, err := newCredential()
	if err != nil {
		return fmt.Sprintf("worker-%d", time.Now().UnixNano())
	}
	return value
}

func runtimeTarget(target store.ServerProvisionTarget) gpruntime.Target {
	return gpruntime.Target{NodeID: "", NodeURL: target.NodeURL, NodeToken: target.NodeToken, ServerID: target.ServerID}
}

func runtimeCreateRequest(target store.ServerProvisionTarget) gpruntime.CreateServerRequest {
	envMap := map[string]string{"SERVER_MEMORY": fmt.Sprintf("%d", target.MemoryMB), "SERVER_IP": target.AllocationIP, "SERVER_PORT": fmt.Sprintf("%d", target.AllocationPort)}
	for key, value := range target.Environment {
		envMap[key] = value
	}
	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+envMap[key])
	}
	command := []string{"/bin/sh", "-lc", target.StartupCommand}
	if strings.Contains(target.Image, "itzg/minecraft-server") {
		command = nil
		env = append(env, "EULA=TRUE")
	}
	ports := make([]gpruntime.Port, 0, len(target.Allocations))
	for _, allocation := range target.Allocations {
		containerPort := allocation.ContainerPort
		if containerPort == 0 {
			containerPort = allocation.Port
		}
		protocol := allocation.Protocol
		if protocol == "" {
			protocol = "tcp"
		}
		ports = append(ports, gpruntime.Port{HostIP: allocation.IP, HostPort: allocation.Port, ContainerPort: containerPort, Protocol: protocol})
	}
	mounts := make([]gpruntime.Mount, 0, len(target.Mounts))
	for _, mount := range target.Mounts {
		mounts = append(mounts, gpruntime.Mount{Source: mount.Source, Target: mount.Target, ReadOnly: mount.ReadOnly})
	}
	return gpruntime.CreateServerRequest{ServerID: target.ServerID, Name: target.Name, Image: target.Image, Command: command, Env: env, Ports: ports, Mounts: mounts, MemoryMB: target.MemoryMB, SwapMB: target.SwapMB, CPUShares: target.CPUShares, CPULimit: target.CPULimit, DiskMB: target.DiskMB, IOWeight: target.IOWeight, Threads: target.Threads, OOMDisabled: target.OOMDisabled}
}

func runtimeConfiguration(target store.ServerProvisionTarget) gpruntime.ServerConfiguration {
	config := map[string]any{}
	if strings.TrimSpace(target.ConfigJSON) != "" {
		_ = json.Unmarshal([]byte(target.ConfigJSON), &config)
	}
	denylist := []string{}
	if strings.TrimSpace(target.FileDenylist) != "" {
		_ = json.Unmarshal([]byte(target.FileDenylist), &denylist)
	}
	mappings := map[string][]int{}
	for _, allocation := range target.Allocations {
		mappings[allocation.IP] = append(mappings[allocation.IP], allocation.Port)
	}
	mounts := make([]gpruntime.Mount, 0, len(target.Mounts))
	for _, mount := range target.Mounts {
		mounts = append(mounts, gpruntime.Mount{Source: mount.Source, Target: mount.Target, ReadOnly: mount.ReadOnly})
	}
	environment := map[string]string{"SERVER_MEMORY": fmt.Sprintf("%d", target.MemoryMB), "SERVER_IP": target.AllocationIP, "SERVER_PORT": fmt.Sprintf("%d", target.AllocationPort)}
	for key, value := range target.Environment {
		environment[key] = value
	}
	return gpruntime.ServerConfiguration{UUID: target.ServerID, Name: target.Name, Suspended: false, Environment: environment, Invocation: target.StartupCommand, DockerImage: target.Image, Egg: map[string]any{"id": target.EggID, "fileDenylist": denylist}, Build: map[string]any{"memoryLimit": target.MemoryMB, "diskSpace": target.DiskMB, "cpuShares": target.CPUShares, "cpuLimit": target.CPULimit, "ioWeight": target.IOWeight, "swapMb": target.SwapMB, "threads": target.Threads, "oomDisabled": target.OOMDisabled}, Allocations: map[string]any{"default": map[string]any{"ip": target.AllocationIP, "port": target.AllocationPort}, "mappings": mappings, "ports": target.Allocations}, Config: config, Mounts: mounts}
}

func (s *Service) increment(update func(*Metrics)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.metrics)
}

func (s *Service) publish(ctx context.Context, eventType events.EventType, resourceType, resourceID string, payload map[string]any) {
	if s == nil || s.publisher == nil {
		return
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if correlationID := events.CorrelationIDFromContext(ctx); correlationID != "" {
		if _, exists := payload["correlationId"]; !exists {
			payload["correlationId"] = correlationID
		}
	}
	if _, ok := payload["correlationId"]; !ok && resourceType == "migration" && resourceID != "" {
		payload["correlationId"] = resourceID
	}
	_ = s.publisher.Publish(ctx, events.NewEnvelope(eventType, "migration-service", resourceType, resourceID, payload))
}
