package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// GitWebhookEvent is a single provider webhook hit recorded for the
// source→environment event map. SourceID is nullable: pull-request events
// arrive before any source is bound to the repository.
type GitWebhookEvent struct {
	ID             string    `json:"id"`
	SourceID       *string   `json:"sourceId,omitempty"`
	Provider       string    `json:"provider"`
	EventType      string    `json:"eventType"`
	Repository     string    `json:"repository"`
	Ref            string    `json:"ref"`
	SHA            string    `json:"sha"`
	PRNumber       *int      `json:"prNumber,omitempty"`
	Action         string    `json:"action"`
	InstallationID string    `json:"installationId,omitempty"`
	ReceivedAt     time.Time `json:"receivedAt"`
}

// CreateGitWebhookEvent appends a webhook hit to the event map.
func (s *Store) CreateGitWebhookEvent(ctx context.Context, e GitWebhookEvent) error {
	if e.ReceivedAt.IsZero() {
		e.ReceivedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO git_webhook_events (id, source_id, provider, event_type, repository, ref, sha, pr_number, action, installation_id, received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, uuid.NewString(), e.SourceID, e.Provider, e.EventType, e.Repository, e.Ref, e.SHA, e.PRNumber, e.Action, e.InstallationID, e.ReceivedAt)
	return err
}

// ListGitWebhookEvents returns the event map rows for a git source, newest
// first, capped at limit (defaults to 50).
func (s *Store) ListGitWebhookEvents(ctx context.Context, sourceID string, limit int) ([]GitWebhookEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, source_id, COALESCE(provider,''), COALESCE(event_type,''),
		       COALESCE(repository,''), COALESCE(ref,''), COALESCE(sha,''),
		       pr_number, COALESCE(action,''), COALESCE(installation_id,''), received_at
		FROM git_webhook_events WHERE source_id = $1 ORDER BY received_at DESC LIMIT $2
	`, sourceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []GitWebhookEvent{}
	for rows.Next() {
		var e GitWebhookEvent
		if err := rows.Scan(&e.ID, &e.SourceID, &e.Provider, &e.EventType,
			&e.Repository, &e.Ref, &e.SHA, &e.PRNumber, &e.Action, &e.InstallationID, &e.ReceivedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// OAuthState is a single-use OAuth connect record.
type OAuthState struct {
	State       string    `json:"state"`
	UserID      string    `json:"userId"`
	Provider    string    `json:"provider"`
	RedirectURI string    `json:"redirectUri"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// SetOAuthState persists a new OAuth state for the connect flow.
func (s *Store) SetOAuthState(ctx context.Context, st OAuthState) error {
	if st.ExpiresAt.IsZero() {
		st.ExpiresAt = time.Now().UTC().Add(15 * time.Minute)
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO git_oauth_states (id, user_id, provider, state, redirect_uri, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, now(), $6)
	`, uuid.NewString(), st.UserID, st.Provider, st.State, st.RedirectURI, st.ExpiresAt)
	return err
}

// GetOAuthState returns an unused, unexpired OAuth state or an error when the
// state is unknown, already consumed, or expired.
func (s *Store) GetOAuthState(ctx context.Context, state string) (OAuthState, error) {
	var st OAuthState
	err := s.db.QueryRow(ctx, `
		SELECT state, user_id, COALESCE(provider,''), COALESCE(redirect_uri,''), expires_at
		FROM git_oauth_states WHERE state = $1
	`, state).Scan(&st.State, &st.UserID, &st.Provider, &st.RedirectURI, &st.ExpiresAt)
	if err != nil {
		return OAuthState{}, errors.New("oauth state not found or expired")
	}
	if time.Now().UTC().After(st.ExpiresAt) {
		return OAuthState{}, errors.New("oauth state expired")
	}
	return st, nil
}

// DeleteOAuthState consumes an OAuth state.
func (s *Store) DeleteOAuthState(ctx context.Context, state string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM git_oauth_states WHERE state = $1`, state)
	return err
}

// GitSourceLink carries the organization / environment binding of a git
// source (the "env map" half of the gitlinks substrate).
type GitSourceLink struct {
	SourceID      string  `json:"sourceId"`
	OrgID         *string `json:"orgId,omitempty"`
	EnvironmentID *string `json:"environmentId,omitempty"`
}

// GetGitSourceLink reads the org/env binding stored on a git source.
func (s *Store) GetGitSourceLink(ctx context.Context, sourceID string) (GitSourceLink, error) {
	var link GitSourceLink
	err := s.db.QueryRow(ctx, `
		SELECT id, org_id, environment_id FROM git_sources WHERE id = $1
	`, sourceID).Scan(&link.SourceID, &link.OrgID, &link.EnvironmentID)
	if err != nil {
		return GitSourceLink{}, errors.New("git source not found")
	}
	return link, nil
}

// SetGitSourceOrg binds a git source to an organization (post-create fill-in
// for sources created before org tenancy existed).
func (s *Store) SetGitSourceOrgID(ctx context.Context, sourceID, orgID string) error {
	cmd, err := s.db.Exec(ctx, `
		UPDATE git_sources SET org_id = $1, updated_at = now() WHERE id = $2
	`, orgID, sourceID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("git source not found")
	}
	return nil
}

// UpdateGitSourceCredentialID attaches a credential row to a git source
// (used by the deploy-key provisioning flow).
func (s *Store) UpdateGitSourceCredentialID(ctx context.Context, sourceID, credentialID string) error {
	cmd, err := s.db.Exec(ctx, `
		UPDATE git_sources SET credential_id = $1, updated_at = now() WHERE id = $2
	`, credentialID, sourceID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("git source not found")
	}
	return nil
}

// LinkGitSourceToEnvironment binds a git source to an environment for the
// webhook→event→env map. Passing an empty environmentID clears the binding.
func (s *Store) LinkGitSourceToEnvironment(ctx context.Context, sourceID, environmentID string) error {
	cmd, err := s.db.Exec(ctx, `
		UPDATE git_sources SET environment_id = NULLIF($1, ''), updated_at = now() WHERE id = $2
	`, environmentID, sourceID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("git source not found")
	}
	return nil
}

// CanAccessOrganization reports whether a user may operate inside an
// organization (owner or team member).
func (s *Store) CanAccessOrganization(ctx context.Context, userID, orgID string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM organizations WHERE id = $1 AND owner_id = $2
			UNION ALL
			SELECT 1 FROM team_members WHERE org_id = $1 AND user_id = $2
		)
	`, orgID, userID).Scan(&ok)
	if err != nil {
		return false, err
	}
	return ok, nil
}

// EnvironmentProjectOrg resolves the owning organization of an environment
// through the tenancy chain (environment → project → organization).
func (s *Store) EnvironmentProjectOrg(ctx context.Context, environmentID string) (projectID string, orgID string, err error) {
	err = s.db.QueryRow(ctx, `
		SELECT p.id, p.org_id
		FROM environments e
		JOIN projects p ON p.id = e.project_id
		WHERE e.id = $1
	`, environmentID).Scan(&projectID, &orgID)
	if err != nil {
		return "", "", errors.New("environment not found")
	}
	return projectID, orgID, nil
}
