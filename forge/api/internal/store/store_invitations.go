package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
)

// CreateSubuserInvitation creates a new subuser invitation
func (s *Store) CreateSubuserInvitation(ctx context.Context, serverID string, req CreateSubuserInvitationRequest, createdBy *string, expiresIn time.Duration) (SubuserInvitation, error) {
	if req.Email == "" {
		return SubuserInvitation{}, errors.New("email is required")
	}
	if len(req.Permissions) == 0 {
		return SubuserInvitation{}, errors.New("at least one permission is required")
	}

	// Generate a secure random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return SubuserInvitation{}, errors.New("failed to generate token")
	}
	token := hex.EncodeToString(tokenBytes)

	// Check if server exists
	var serverExists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM servers WHERE id = $1)`, serverID).Scan(&serverExists)
	if err != nil || !serverExists {
		return SubuserInvitation{}, errors.New("server not found")
	}

	// Check if user exists (if provided)
	if createdBy != nil {
		var userExists bool
		err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, *createdBy).Scan(&userExists)
		if err != nil || !userExists {
			return SubuserInvitation{}, errors.New("creating user not found")
		}
	}

	// Create invitation
	invitationID := uuid.NewString()
	expiresAt := time.Now().Add(expiresIn)

	_, err = s.db.Exec(ctx, `
		INSERT INTO subuser_invitations (id, server_id, email, permissions, token, created_by, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7)
	`, invitationID, serverID, req.Email, req.Permissions, token, createdBy, expiresAt)
	if err != nil {
		return SubuserInvitation{}, err
	}

	return s.GetSubuserInvitation(ctx, invitationID)
}

// GetSubuserInvitation retrieves an invitation by ID
func (s *Store) GetSubuserInvitation(ctx context.Context, id string) (SubuserInvitation, error) {
	var invitation SubuserInvitation
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, email, permissions, token, created_by::text, created_at, expires_at, accepted_at, revoked_at
		FROM subuser_invitations
		WHERE id = $1
	`, id).Scan(&invitation.ID, &invitation.ServerID, &invitation.Email, &invitation.Permissions, &invitation.Token, &invitation.CreatedBy, &invitation.CreatedAt, &invitation.ExpiresAt, &invitation.AcceptedAt, &invitation.RevokedAt)
	return invitation, err
}

// GetSubuserInvitationByToken retrieves an invitation by token
func (s *Store) GetSubuserInvitationByToken(ctx context.Context, token string) (SubuserInvitation, error) {
	var invitation SubuserInvitation
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, email, permissions, token, created_by::text, created_at, expires_at, accepted_at, revoked_at
		FROM subuser_invitations
		WHERE token = $1
	`, token).Scan(&invitation.ID, &invitation.ServerID, &invitation.Email, &invitation.Permissions, &invitation.Token, &invitation.CreatedBy, &invitation.CreatedAt, &invitation.ExpiresAt, &invitation.AcceptedAt, &invitation.RevokedAt)
	return invitation, err
}

// ListSubuserInvitations retrieves all invitations for a server
func (s *Store) ListSubuserInvitations(ctx context.Context, serverID string) ([]SubuserInvitation, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, email, permissions, token, created_by::text, created_at, expires_at, accepted_at, revoked_at
		FROM subuser_invitations
		WHERE server_id = $1
		ORDER BY created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	invitations := []SubuserInvitation{}
	for rows.Next() {
		var invitation SubuserInvitation
		if err := rows.Scan(&invitation.ID, &invitation.ServerID, &invitation.Email, &invitation.Permissions, &invitation.Token, &invitation.CreatedBy, &invitation.CreatedAt, &invitation.ExpiresAt, &invitation.AcceptedAt, &invitation.RevokedAt); err != nil {
			return nil, err
		}
		invitations = append(invitations, invitation)
	}
	return invitations, rows.Err()
}

// AcceptSubuserInvitation accepts an invitation and creates the subuser relationship
func (s *Store) AcceptSubuserInvitation(ctx context.Context, token string, userID string) (ServerSubuser, error) {
	// Get the invitation
	invitation, err := s.GetSubuserInvitationByToken(ctx, token)
	if err != nil {
		return ServerSubuser{}, errors.New("invitation not found")
	}

	// Check if invitation is expired
	if time.Now().After(invitation.ExpiresAt) {
		return ServerSubuser{}, errors.New("invitation has expired")
	}

	// Check if invitation is already accepted or revoked
	if invitation.AcceptedAt != nil {
		return ServerSubuser{}, errors.New("invitation already accepted")
	}
	if invitation.RevokedAt != nil {
		return ServerSubuser{}, errors.New("invitation has been revoked")
	}

	// Check if the user's email matches the invitation email
	var userEmail string
	err = s.db.QueryRow(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&userEmail)
	if err != nil {
		return ServerSubuser{}, errors.New("user not found")
	}
	if userEmail != invitation.Email {
		return ServerSubuser{}, errors.New("user email does not match invitation")
	}

	// Create the subuser relationship
	subuser, err := s.UpsertServerSubuser(ctx, invitation.ServerID, UpsertServerSubuserRequest{
		Email:       invitation.Email,
		Permissions: invitation.Permissions,
	}, &userID)
	if err != nil {
		return ServerSubuser{}, err
	}

	// Mark invitation as accepted
	_, err = s.db.Exec(ctx, `
		UPDATE subuser_invitations
		SET accepted_at = NOW()
		WHERE id = $1
	`, invitation.ID)
	if err != nil {
		return ServerSubuser{}, err
	}

	return subuser, nil
}

// RevokeSubuserInvitation revokes an invitation
func (s *Store) RevokeSubuserInvitation(ctx context.Context, id string) error {
	commandTag, err := s.db.Exec(ctx, `
		UPDATE subuser_invitations
		SET revoked_at = NOW()
		WHERE id = $1 AND accepted_at IS NULL
	`, id)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("invitation not found or already accepted")
	}
	return nil
}

// DeleteSubuserInvitation deletes an invitation
func (s *Store) DeleteSubuserInvitation(ctx context.Context, id string) error {
	commandTag, err := s.db.Exec(ctx, `
		DELETE FROM subuser_invitations
		WHERE id = $1
	`, id)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("invitation not found")
	}
	return nil
}

// CleanupExpiredInvitations removes expired invitations
func (s *Store) CleanupExpiredInvitations(ctx context.Context) (int, error) {
	commandTag, err := s.db.Exec(ctx, `
		DELETE FROM subuser_invitations
		WHERE expires_at < NOW() AND accepted_at IS NULL
	`)
	if err != nil {
		return 0, err
	}
	return int(commandTag.RowsAffected()), nil
}
