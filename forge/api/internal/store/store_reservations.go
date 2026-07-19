package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CreatePlacementReservation(ctx context.Context, req CreatePlacementReservationRequest) (PlacementReservation, error) {
	if req.NodeID == "" {
		return PlacementReservation{}, errors.New("nodeId is required")
	}
	if req.ReservationType == "" {
		req.ReservationType = PlacementReservationTypePlacement
	}
	if req.Status == "" {
		req.Status = PlacementReservationStatusActive
	}
	if req.ExpiresAt.IsZero() {
		req.ExpiresAt = time.Now().UTC().Add(10 * time.Minute)
	}
	id := uuid.NewString()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PlacementReservation{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE placement_reservations
		SET status = 'expired',
		    expired_at = now(),
		    updated_at = now()
		WHERE status IN ('pending', 'active')
		  AND expires_at <= now()
	`); err != nil {
		return PlacementReservation{}, err
	}

	var nodeID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM nodes WHERE id = $1 FOR UPDATE`, req.NodeID).Scan(&nodeID); err != nil {
		return PlacementReservation{}, errors.New("node not found")
	}

	if req.ServerID != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM placement_reservations
				WHERE server_id = $1
				  AND status IN ('pending', 'active')
				  AND expires_at > now()
			)
		`, req.ServerID).Scan(&exists); err != nil {
			return PlacementReservation{}, err
		}
		if exists {
			return PlacementReservation{}, errors.New("server already has an active placement reservation")
		}
	}
	if req.MigrationID != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM placement_reservations
				WHERE migration_id = $1
				  AND status IN ('pending', 'active')
				  AND expires_at > now()
			)
		`, req.MigrationID).Scan(&exists); err != nil {
			return PlacementReservation{}, err
		}
		if exists {
			return PlacementReservation{}, errors.New("migration already has an active placement reservation")
		}
	}

	capacity, err := s.nodeCapacitySnapshotTx(ctx, tx, req.NodeID)
	if err != nil {
		return PlacementReservation{}, err
	}
	if req.CPU > 0 && capacity.AvailableCPU < req.CPU {
		return PlacementReservation{}, errors.New("reserved cpu exceeds available capacity")
	}
	if req.Memory > 0 && int64(capacity.AvailableMemory) < req.Memory {
		return PlacementReservation{}, errors.New("reserved memory exceeds available capacity")
	}
	if req.Disk > 0 && int64(capacity.AvailableDisk) < req.Disk {
		return PlacementReservation{}, errors.New("reserved disk exceeds available capacity")
	}

	var serverID any
	if req.ServerID != "" {
		serverID = req.ServerID
	}
	var migrationID any
	if req.MigrationID != "" {
		migrationID = req.MigrationID
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO placement_reservations (
			id, node_id, server_id, migration_id, reservation_type, cpu, memory, disk, status, expires_at
		)
		VALUES ($1, $2, $3, $4, $5::placement_reservation_type, $6, $7, $8, $9::placement_reservation_status, $10)
	`, id, req.NodeID, serverID, migrationID, string(req.ReservationType), req.CPU, req.Memory, req.Disk, string(req.Status), req.ExpiresAt); err != nil {
		return PlacementReservation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PlacementReservation{}, err
	}
	return s.GetPlacementReservation(ctx, id)
}

func (s *Store) GetPlacementReservation(ctx context.Context, reservationID string) (PlacementReservation, error) {
	var reservation PlacementReservation
	var serverID, migrationID sql.NullString
	var confirmedAt, cancelledAt, expiredAt sql.NullTime
	err := s.db.QueryRow(ctx, `
		SELECT id::text, node_id::text, server_id::text, migration_id::text,
		       reservation_type::text, cpu, memory, disk, status::text, expires_at,
		       created_at, updated_at, confirmed_at, cancelled_at, expired_at
		FROM placement_reservations
		WHERE id = $1
	`, reservationID).Scan(
		&reservation.ID,
		&reservation.NodeID,
		&serverID,
		&migrationID,
		&reservation.ReservationType,
		&reservation.CPU,
		&reservation.Memory,
		&reservation.Disk,
		&reservation.Status,
		&reservation.ExpiresAt,
		&reservation.CreatedAt,
		&reservation.UpdatedAt,
		&confirmedAt,
		&cancelledAt,
		&expiredAt,
	)
	if err != nil {
		return PlacementReservation{}, err
	}
	if serverID.Valid {
		s := serverID.String
		reservation.ServerID = &s
	}
	if migrationID.Valid {
		m := migrationID.String
		reservation.MigrationID = &m
	}
	if confirmedAt.Valid {
		reservation.ConfirmedAt = &confirmedAt.Time
	}
	if cancelledAt.Valid {
		reservation.CancelledAt = &cancelledAt.Time
	}
	if expiredAt.Valid {
		reservation.ExpiredAt = &expiredAt.Time
	}
	return reservation, nil
}

func (s *Store) ListPlacementReservations(ctx context.Context) ([]PlacementReservation, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text
		FROM placement_reservations
		ORDER BY created_at DESC, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reservations := []PlacementReservation{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		reservation, err := s.GetPlacementReservation(ctx, id)
		if err != nil {
			return nil, err
		}
		reservations = append(reservations, reservation)
	}
	return reservations, rows.Err()
}

func (s *Store) UpdatePlacementReservationStatus(ctx context.Context, reservationID string, status PlacementReservationStatus) (PlacementReservation, error) {
	column := "updated_at"
	switch status {
	case PlacementReservationStatusCompleted:
		column = "confirmed_at"
	case PlacementReservationStatusCancelled:
		column = "cancelled_at"
	case PlacementReservationStatusExpired:
		column = "expired_at"
	}
	commandTag, err := s.db.Exec(ctx, `
		UPDATE placement_reservations
		SET status = $2::placement_reservation_status,
		    updated_at = now(),
		    confirmed_at = CASE WHEN $3 = 'confirmed_at' THEN now() ELSE confirmed_at END,
		    cancelled_at = CASE WHEN $3 = 'cancelled_at' THEN now() ELSE cancelled_at END,
		    expired_at = CASE WHEN $3 = 'expired_at' THEN now() ELSE expired_at END
		WHERE id = $1
		  AND status IN ('pending', 'active')
	`, reservationID, string(status), column)
	if err != nil {
		return PlacementReservation{}, err
	}
	if commandTag.RowsAffected() == 0 {
		return PlacementReservation{}, errors.New("reservation not found or already terminal")
	}
	return s.GetPlacementReservation(ctx, reservationID)
}

func (s *Store) ExpirePlacementReservations(ctx context.Context) ([]PlacementReservation, error) {
	rows, err := s.db.Query(ctx, `
		UPDATE placement_reservations
		SET status = 'expired',
		    expired_at = now(),
		    updated_at = now()
		WHERE status IN ('pending', 'active')
		  AND expires_at <= now()
		RETURNING id::text
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reservations := []PlacementReservation{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		reservation, err := s.GetPlacementReservation(ctx, id)
		if err != nil {
			return nil, err
		}
		reservations = append(reservations, reservation)
	}
	return reservations, rows.Err()
}

func (s *Store) UpdatePlacementReservationsForMigration(ctx context.Context, migrationID string, status PlacementReservationStatus) ([]PlacementReservation, error) {
	rows, err := s.db.Query(ctx, `
		UPDATE placement_reservations
		SET status = $2::placement_reservation_status,
		    updated_at = now(),
		    confirmed_at = CASE WHEN $2::text = 'completed' THEN now() ELSE confirmed_at END,
		    cancelled_at = CASE WHEN $2::text = 'cancelled' THEN now() ELSE cancelled_at END,
		    expired_at = CASE WHEN $2::text = 'expired' THEN now() ELSE expired_at END
		WHERE migration_id = $1
		  AND status IN ('pending', 'active')
		RETURNING id::text
	`, migrationID, string(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reservations := []PlacementReservation{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		reservation, err := s.GetPlacementReservation(ctx, id)
		if err != nil {
			return nil, err
		}
		reservations = append(reservations, reservation)
	}
	return reservations, rows.Err()
}

func (s *Store) ActiveReservedCapacity(ctx context.Context, nodeID string) (ReservedCapacity, error) {
	return s.activeReservedCapacity(ctx, s.db, nodeID)
}

func (s *Store) PlacementReservationsTotal(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM placement_reservations`).Scan(&total)
	return total, err
}

func (s *Store) activeReservedCapacity(ctx context.Context, querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, nodeID string) (ReservedCapacity, error) {
	var reserved ReservedCapacity
	err := querier.QueryRow(ctx, `
		SELECT COALESCE(SUM(cpu), 0)::int,
		       COALESCE(SUM(memory), 0)::int,
		       COALESCE(SUM(disk), 0)::int
		FROM placement_reservations
		WHERE node_id = $1
		  AND status IN ('pending', 'active')
		  AND expires_at > now()
	`, nodeID).Scan(&reserved.CPU, &reserved.Memory, &reserved.Disk)
	return reserved, err
}
