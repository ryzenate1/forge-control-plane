// Package composetemplates implements Portainer-style custom stack templates:
// reusable, parameterized docker-compose documents that administrators can save
// and instantiate into real compose stacks. A template stores the raw compose
// YAML (with ${PARAM_KEY} placeholders) plus an ordered list of parameter
// descriptors describing how each placeholder is rendered and validated.
//
// Instantiation renders the compose YAML by substituting the provided values
// (falling back to per-parameter defaults, leaving unknown variables intact)
// and hands the result to the existing compose.Service.DeployComposeStack, so a
// template never reimplements deployment.
package composetemplates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gamepanel/forge/internal/services/compose"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

// Visibility controls who may see/use a template.
const (
	VisibilityPrivate = "private"
	VisibilityPublic  = "public"
)

// Parameter types mirror the Portainer template field kinds.
const (
	ParamTypeText     = "text"
	ParamTypeNumber   = "number"
	ParamTypeSelect   = "select"
	ParamTypePassword = "password"
	ParamTypeEnvFile  = "env-file"
)

// TemplateParameter describes a single placeholder in a template's compose YAML.
type TemplateParameter struct {
	Key      string   `json:"key"`
	Label    string   `json:"label,omitempty"`
	Type     string   `json:"type"`
	Default  string   `json:"default,omitempty"`
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
	Secret   bool     `json:"secret,omitempty"`
}

// Template is the domain model for a saved stack template.
type Template struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Category    string              `json:"category,omitempty"`
	LogoURL     string              `json:"logoUrl,omitempty"`
	ComposeYAML string              `json:"composeYaml"`
	Parameters  []TemplateParameter `json:"parameters"`
	Visibility  string              `json:"visibility"`
	CreatedBy   string              `json:"createdBy,omitempty"`
	CreatedAt   time.Time           `json:"createdAt"`
	UpdatedAt   time.Time           `json:"updatedAt"`
}

// TemplateInstance records the outcome of deploying a template.
type TemplateInstance struct {
	ID         string            `json:"id"`
	TemplateID string            `json:"templateId"`
	StackID    string            `json:"stackId"`
	Values     map[string]string `json:"values"`
	CreatedAt  time.Time         `json:"createdAt"`
}

// InstantiateResult bundles the created stack and the instance audit row.
type InstantiateResult struct {
	Instance TemplateInstance      `json:"instance"`
	Stack    *compose.ComposeStack `json:"stack"`
}

// ErrTemplateNotFound is returned when a template id does not resolve.
var ErrTemplateNotFound = errors.New("compose template not found")

// paramVarPattern matches ${KEY} placeholders in the compose YAML.
var paramVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Service stores and renders templates, delegating deployment to compose.Service.
type Service struct {
	store      *store.Store
	composeSvc *compose.Service
}

// New builds a template service. composeSvc may be nil, in which case
// InstantiateTemplate returns an error but CRUD/render still work.
func New(st *store.Store, composeSvc *compose.Service) *Service {
	return &Service{store: st, composeSvc: composeSvc}
}

// ListTemplates returns every saved template (most recent first).
func (s *Service) ListTemplates(ctx context.Context) ([]Template, error) {
	if s.store == nil {
		return []Template{}, nil
	}
	rows, err := s.store.ListComposeTemplates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Template, 0, len(rows))
	for i := range rows {
		t, err := fromStoreTemplate(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// GetTemplate fetches a single template by id.
func (s *Service) GetTemplate(ctx context.Context, id string) (Template, error) {
	if s.store == nil {
		return Template{}, ErrTemplateNotFound
	}
	row, err := s.store.GetComposeTemplate(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrComposeTemplateNotFound) {
			return Template{}, ErrTemplateNotFound
		}
		return Template{}, err
	}
	return fromStoreTemplate(row)
}

// CreateTemplate validates and persists a new template, assigning an id and
// timestamps.
func (s *Service) CreateTemplate(ctx context.Context, t Template) (Template, error) {
	if s.store == nil {
		return Template{}, errors.New("store is required")
	}
	if err := normalizeAndValidate(&t); err != nil {
		return Template{}, err
	}
	t.ID = uuid.NewString()
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	row, err := toStoreTemplate(t)
	if err != nil {
		return Template{}, err
	}
	if err := s.store.CreateComposeTemplate(ctx, row); err != nil {
		return Template{}, err
	}
	return t, nil
}

// UpdateTemplate patches an existing template. Empty id-anchored fields on the
// incoming value fall back to the stored copy so callers can send partial
// payloads.
func (s *Service) UpdateTemplate(ctx context.Context, id string, patch Template) (Template, error) {
	if s.store == nil {
		return Template{}, errors.New("store is required")
	}
	existing, err := s.GetTemplate(ctx, id)
	if err != nil {
		return Template{}, err
	}
	merged := mergeTemplate(existing, patch)
	merged.ID = id
	if err := normalizeAndValidate(&merged); err != nil {
		return Template{}, err
	}
	merged.CreatedAt = existing.CreatedAt
	merged.CreatedBy = existing.CreatedBy
	merged.UpdatedAt = time.Now().UTC()
	row, err := toStoreTemplate(merged)
	if err != nil {
		return Template{}, err
	}
	if err := s.store.UpdateComposeTemplate(ctx, row); err != nil {
		return Template{}, err
	}
	return merged, nil
}

// DeleteTemplate removes a template (and, via ON DELETE CASCADE, its instances).
func (s *Service) DeleteTemplate(ctx context.Context, id string) error {
	if s.store == nil {
		return errors.New("store is required")
	}
	if err := s.store.DeleteComposeTemplate(ctx, id); err != nil {
		if errors.Is(err, store.ErrComposeTemplateNotFound) {
			return ErrTemplateNotFound
		}
		return err
	}
	return nil
}

// ListInstances returns the recorded instantiations for a template.
func (s *Service) ListInstances(ctx context.Context, templateID string) ([]TemplateInstance, error) {
	if s.store == nil {
		return []TemplateInstance{}, nil
	}
	rows, err := s.store.ListComposeTemplateInstances(ctx, templateID)
	if err != nil {
		return nil, err
	}
	out := make([]TemplateInstance, 0, len(rows))
	for i := range rows {
		inst, err := fromStoreInstance(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, inst)
	}
	return out, nil
}

// RenderPreview resolves the compose YAML for a template (or an inline draft)
// with the given values applied, without deploying anything.
func (s *Service) RenderPreview(ctx context.Context, templateID string, inline *Template, values map[string]string) (string, error) {
	tpl := inline
	if templateID != "" {
		loaded, err := s.GetTemplate(ctx, templateID)
		if err != nil {
			return "", err
		}
		tpl = &loaded
	}
	if tpl == nil {
		return "", errors.New("either templateId or a compose draft is required")
	}
	return Render(tpl.ComposeYAML, tpl.Parameters, values), nil
}

// InstantiateTemplate renders the template and deploys the result as a new
// compose stack, then records the instantiation.
func (s *Service) InstantiateTemplate(ctx context.Context, templateID string, values map[string]string, name string, nodeID string, userID string) (*InstantiateResult, error) {
	if s.store == nil {
		return nil, errors.New("store is required")
	}
	if s.composeSvc == nil {
		return nil, errors.New("compose service is not configured")
	}
	tpl, err := s.GetTemplate(ctx, templateID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("stack name is required")
	}
	if userID == "" {
		userID = "system"
	}

	rendered := Render(tpl.ComposeYAML, tpl.Parameters, values)
	if err := validateRendered(tpl.Parameters, values); err != nil {
		return nil, err
	}

	stack, err := s.composeSvc.DeployComposeStack(ctx, compose.DeployComposeRequest{
		UserID:      userID,
		Name:        name,
		NodeID:      nodeID,
		ComposeYAML: rendered,
		ComposeType: "docker-compose",
		SourceType:  "raw",
	})
	if err != nil {
		return nil, err
	}

	inst := TemplateInstance{
		ID:         uuid.NewString(),
		TemplateID: tpl.ID,
		StackID:    stack.ID,
		Values:     sanitizeValues(tpl.Parameters, values),
		CreatedAt:  time.Now().UTC(),
	}
	row, err := toStoreInstance(inst)
	if err != nil {
		return nil, err
	}
	if err := s.store.CreateComposeTemplateInstance(ctx, row); err != nil {
		return nil, err
	}

	return &InstantiateResult{Instance: inst, Stack: stack}, nil
}

// Render substitutes ${KEY} placeholders in the compose YAML. Effective values
// start from each parameter's default and are overridden by caller-provided
// values; placeholders with no matching value (and no default) are left intact so
// docker-compose can resolve them at runtime.
func Render(composeYAML string, params []TemplateParameter, values map[string]string) string {
	effective := make(map[string]string, len(params)+len(values))
	for _, p := range params {
		if p.Default != "" {
			effective[p.Key] = p.Default
		}
	}
	for k, v := range values {
		effective[k] = v
	}
	return paramVarPattern.ReplaceAllStringFunc(composeYAML, func(match string) string {
		key := match[2 : len(match)-1]
		if v, ok := effective[key]; ok {
			return v
		}
		return match
	})
}

func normalizeAndValidate(t *Template) error {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(t.ComposeYAML) == "" {
		return errors.New("composeYaml is required")
	}
	if t.Parameters == nil {
		t.Parameters = []TemplateParameter{}
	}
	seen := map[string]bool{}
	for i := range t.Parameters {
		p := &t.Parameters[i]
		p.Key = strings.TrimSpace(p.Key)
		if p.Key == "" || !identifierPattern.MatchString(p.Key) {
			return fmt.Errorf("parameter key %q must be a letter or underscore followed by letters, digits or underscores", p.Key)
		}
		if seen[p.Key] {
			return fmt.Errorf("duplicate parameter key %q", p.Key)
		}
		seen[p.Key] = true
		if p.Label == "" {
			p.Label = p.Key
		}
		if p.Type == "" {
			p.Type = ParamTypeText
		}
		if !validParamType(p.Type) {
			return fmt.Errorf("parameter %q has unsupported type %q", p.Key, p.Type)
		}
		if p.Type == ParamTypeSelect && len(p.Options) == 0 {
			return fmt.Errorf("select parameter %q requires at least one option", p.Key)
		}
	}
	switch t.Visibility {
	case "":
		t.Visibility = VisibilityPrivate
	case VisibilityPrivate, VisibilityPublic:
	default:
		return fmt.Errorf("visibility must be %q or %q", VisibilityPrivate, VisibilityPublic)
	}
	return nil
}

func validParamType(t string) bool {
	switch t {
	case ParamTypeText, ParamTypeNumber, ParamTypeSelect, ParamTypePassword, ParamTypeEnvFile:
		return true
	}
	return false
}

// validateRendered rejects rendering only for genuinely unusable input: a
// required parameter that ended up empty. Unknown placeholders are permitted and
// passed through untouched.
func validateRendered(params []TemplateParameter, values map[string]string) error {
	effective := make(map[string]string, len(params)+len(values))
	for _, p := range params {
		if p.Default != "" {
			effective[p.Key] = p.Default
		}
	}
	for k, v := range values {
		effective[k] = v
	}
	for _, p := range params {
		if p.Required && strings.TrimSpace(effective[p.Key]) == "" {
			return fmt.Errorf("required parameter %q is missing a value", p.Key)
		}
	}
	return nil
}

// sanitizeValues drops secret parameter values from the recorded instance so we
// never persist plaintext secrets in the audit row; non-secret choices are kept.
func sanitizeValues(params []TemplateParameter, values map[string]string) map[string]string {
	secretKeys := map[string]bool{}
	for _, p := range params {
		if p.Secret || p.Type == ParamTypePassword {
			secretKeys[p.Key] = true
		}
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		if secretKeys[k] {
			continue
		}
		out[k] = v
	}
	return out
}

func mergeTemplate(existing, patch Template) Template {
	merged := existing
	if patch.Name != "" {
		merged.Name = patch.Name
	}
	if patch.Description != "" {
		merged.Description = patch.Description
	}
	if patch.Category != "" {
		merged.Category = patch.Category
	}
	if patch.LogoURL != "" {
		merged.LogoURL = patch.LogoURL
	}
	if patch.ComposeYAML != "" {
		merged.ComposeYAML = patch.ComposeYAML
	}
	if patch.Parameters != nil {
		merged.Parameters = patch.Parameters
	}
	if patch.Visibility != "" {
		merged.Visibility = patch.Visibility
	}
	return merged
}

func fromStoreTemplate(row *store.ComposeTemplate) (Template, error) {
	var params []TemplateParameter
	if len(row.Parameters) > 0 {
		if err := json.Unmarshal(row.Parameters, &params); err != nil {
			return Template{}, fmt.Errorf("decode template parameters: %w", err)
		}
	}
	if params == nil {
		params = []TemplateParameter{}
	}
	return Template{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		Category:    row.Category,
		LogoURL:     row.LogoURL,
		ComposeYAML: row.ComposeYAML,
		Parameters:  params,
		Visibility:  row.Visibility,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}, nil
}

func toStoreTemplate(t Template) (*store.ComposeTemplate, error) {
	params := t.Parameters
	if params == nil {
		params = []TemplateParameter{}
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("encode template parameters: %w", err)
	}
	return &store.ComposeTemplate{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		Category:    t.Category,
		LogoURL:     t.LogoURL,
		ComposeYAML: t.ComposeYAML,
		Parameters:  encoded,
		Visibility:  t.Visibility,
		CreatedBy:   t.CreatedBy,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}, nil
}

func fromStoreInstance(row *store.ComposeTemplateInstance) (TemplateInstance, error) {
	values := map[string]string{}
	if len(row.ValuesJSON) > 0 {
		if err := json.Unmarshal(row.ValuesJSON, &values); err != nil {
			return TemplateInstance{}, fmt.Errorf("decode instance values: %w", err)
		}
	}
	return TemplateInstance{
		ID:         row.ID,
		TemplateID: row.TemplateID,
		StackID:    row.StackID,
		Values:     values,
		CreatedAt:  row.CreatedAt,
	}, nil
}

func toStoreInstance(in TemplateInstance) (*store.ComposeTemplateInstance, error) {
	values := in.Values
	if values == nil {
		values = map[string]string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("encode instance values: %w", err)
	}
	return &store.ComposeTemplateInstance{
		ID:         in.ID,
		TemplateID: in.TemplateID,
		StackID:    in.StackID,
		ValuesJSON: encoded,
		CreatedAt:  in.CreatedAt,
	}, nil
}
