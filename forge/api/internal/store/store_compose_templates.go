package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrComposeTemplateNotFound is returned by the compose-template getters when no
// row matches the requested id. Handlers map it to HTTP 404.
var ErrComposeTemplateNotFound = errors.New("compose template not found")

// ComposeTemplate is the persistence shape for a Portainer-style stack template.
// Parameters holds a JSON array of parameter descriptors; the caller (the
// composetemplates service) owns the encoding so the store stays free of the
// service's rich types and no import cycle is introduced.
type ComposeTemplate struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	LogoURL     string    `json:"logoUrl"`
	ComposeYAML string    `json:"composeYaml"`
	Parameters  []byte    `json:"parameters"`
	Visibility  string    `json:"visibility"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// ComposeTemplateInstance records a single instantiation of a template into a
// deployed compose stack. ValuesJSON maps parameter keys to chosen values.
type ComposeTemplateInstance struct {
	ID         string    `json:"id"`
	TemplateID string    `json:"templateId"`
	StackID    string    `json:"stackId"`
	ValuesJSON []byte    `json:"values"`
	CreatedAt  time.Time `json:"createdAt"`
}

const composeTemplateColumns = `id, name, description, category, logo_url, compose_yaml, parameters, visibility, created_by, created_at, updated_at`

func (s *Store) CreateComposeTemplate(ctx context.Context, t *ComposeTemplate) error {
	if s.db == nil {
		return nil
	}
	if t.ID == "" {
		return errors.New("compose template id is required")
	}
	if t.Visibility == "" {
		t.Visibility = "private"
	}
	params := t.Parameters
	if len(params) == 0 {
		params = []byte(`[]`)
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = t.CreatedAt
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO compose_templates (id, name, description, category, logo_url, compose_yaml, parameters, visibility, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			category = EXCLUDED.category,
			logo_url = EXCLUDED.logo_url,
			compose_yaml = EXCLUDED.compose_yaml,
			parameters = EXCLUDED.parameters,
			visibility = EXCLUDED.visibility,
			updated_at = EXCLUDED.updated_at
	`, t.ID, t.Name, t.Description, t.Category, t.LogoURL, t.ComposeYAML, params, t.Visibility, t.CreatedBy, t.CreatedAt, t.UpdatedAt)
	return err
}

func scanComposeTemplate(row interface{ Scan(dest ...any) error }) (*ComposeTemplate, error) {
	var t ComposeTemplate
	var params []byte
	if err := row.Scan(&t.ID, &t.Name, &t.Description, &t.Category, &t.LogoURL, &t.ComposeYAML, &params, &t.Visibility, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	if len(params) == 0 {
		params = []byte(`[]`)
	}
	t.Parameters = params
	return &t, nil
}

func (s *Store) GetComposeTemplate(ctx context.Context, id string) (*ComposeTemplate, error) {
	if s.db == nil {
		return nil, ErrComposeTemplateNotFound
	}
	row := s.db.QueryRow(ctx, `SELECT `+composeTemplateColumns+` FROM compose_templates WHERE id = $1`, id)
	t, err := scanComposeTemplate(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrComposeTemplateNotFound
		}
		return nil, err
	}
	return t, nil
}

func (s *Store) ListComposeTemplates(ctx context.Context) ([]ComposeTemplate, error) {
	if s.db == nil {
		return []ComposeTemplate{}, nil
	}
	rows, err := s.db.Query(ctx, `SELECT `+composeTemplateColumns+` FROM compose_templates ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	templates := []ComposeTemplate{}
	for rows.Next() {
		t, err := scanComposeTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, *t)
	}
	return templates, rows.Err()
}

func (s *Store) UpdateComposeTemplate(ctx context.Context, t *ComposeTemplate) error {
	return s.CreateComposeTemplate(ctx, t)
}

func (s *Store) DeleteComposeTemplate(ctx context.Context, id string) error {
	if s.db == nil {
		return nil
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM compose_templates WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrComposeTemplateNotFound
	}
	return nil
}

func (s *Store) CreateComposeTemplateInstance(ctx context.Context, in *ComposeTemplateInstance) error {
	if s.db == nil {
		return nil
	}
	if in.ID == "" {
		return errors.New("compose template instance id is required")
	}
	values := in.ValuesJSON
	if len(values) == 0 {
		values = []byte(`{}`)
	}
	if in.CreatedAt.IsZero() {
		in.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO compose_template_instances (id, template_id, stack_id, values_json, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, in.ID, in.TemplateID, in.StackID, values, in.CreatedAt)
	return err
}

func (s *Store) ListComposeTemplateInstances(ctx context.Context, templateID string) ([]ComposeTemplateInstance, error) {
	if s.db == nil {
		return []ComposeTemplateInstance{}, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, template_id, stack_id, values_json, created_at
		FROM compose_template_instances
		WHERE template_id = $1
		ORDER BY created_at DESC
	`, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	instances := []ComposeTemplateInstance{}
	for rows.Next() {
		var in ComposeTemplateInstance
		var values []byte
		if err := rows.Scan(&in.ID, &in.TemplateID, &in.StackID, &values, &in.CreatedAt); err != nil {
			return nil, err
		}
		if len(values) == 0 {
			values = []byte(`{}`)
		}
		in.ValuesJSON = values
		instances = append(instances, in)
	}
	return instances, rows.Err()
}
