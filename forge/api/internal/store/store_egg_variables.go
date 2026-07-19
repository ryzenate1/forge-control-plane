package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var eggVariableNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

type EggVariable struct {
	ID           string    `json:"id"`
	EggID        string    `json:"eggId"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	EnvVariable  string    `json:"envVariable"`
	DefaultValue string    `json:"defaultValue"`
	UserViewable bool      `json:"userViewable"`
	UserEditable bool      `json:"userEditable"`
	Rules        string    `json:"rules"`
	Sort         int       `json:"sort"`
	CreatedAt    time.Time `json:"createdAt"`
}

type EggVariableRequest struct {
	Name         string
	Description  string
	EnvVariable  string
	DefaultValue string
	UserViewable bool
	UserEditable bool
	Rules        string
	Sort         int
}

func (s *Store) ListEggVariables(ctx context.Context, eggID string) ([]EggVariable, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, egg_id::text, name, description, env_variable, default_value,
		       user_viewable, user_editable, rules, sort, created_at
		FROM egg_variables WHERE egg_id = $1 ORDER BY sort, name, id
	`, eggID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	variables := []EggVariable{}
	for rows.Next() {
		var variable EggVariable
		if err := rows.Scan(&variable.ID, &variable.EggID, &variable.Name, &variable.Description,
			&variable.EnvVariable, &variable.DefaultValue, &variable.UserViewable,
			&variable.UserEditable, &variable.Rules, &variable.Sort, &variable.CreatedAt); err != nil {
			return nil, err
		}
		variables = append(variables, variable)
	}
	return variables, rows.Err()
}

func (s *Store) CreateEggVariable(ctx context.Context, eggID string, req EggVariableRequest, actorID *string) (EggVariable, error) {
	if err := validateEggVariableRequest(req); err != nil {
		return EggVariable{}, err
	}
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO egg_variables (id, egg_id, name, description, env_variable, default_value,
		                           user_viewable, user_editable, rules, sort)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, id, eggID, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description),
		strings.TrimSpace(req.EnvVariable), req.DefaultValue, req.UserViewable, req.UserEditable, strings.TrimSpace(req.Rules),
		req.Sort)
	if err != nil {
		return EggVariable{}, fmt.Errorf("create egg variable: %w", err)
	}
	_ = s.AppendAudit(ctx, actorID, "egg variable created", "egg", &eggID, fmt.Sprintf(`{"variable":"%s"}`, req.EnvVariable))
	return s.getEggVariable(ctx, eggID, id)
}

func (s *Store) UpdateEggVariable(ctx context.Context, eggID, variableID string, req EggVariableRequest, actorID *string) (EggVariable, error) {
	if err := validateEggVariableRequest(req); err != nil {
		return EggVariable{}, err
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE egg_variables
		SET name = $1, description = $2, env_variable = $3, default_value = $4,
		    user_viewable = $5, user_editable = $6, rules = $7, sort = $8
		WHERE id = $9 AND egg_id = $10
	`, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), strings.TrimSpace(req.EnvVariable),
		req.DefaultValue, req.UserViewable, req.UserEditable, strings.TrimSpace(req.Rules),
		req.Sort, variableID, eggID)
	if err != nil {
		return EggVariable{}, fmt.Errorf("update egg variable: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return EggVariable{}, errors.New("egg variable not found")
	}
	_ = s.AppendAudit(ctx, actorID, "egg variable updated", "egg", &eggID, fmt.Sprintf(`{"variable":"%s"}`, req.EnvVariable))
	return s.getEggVariable(ctx, eggID, variableID)
}

func (s *Store) DeleteEggVariable(ctx context.Context, eggID, variableID string, actorID *string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM egg_variables WHERE id = $1 AND egg_id = $2`, variableID, eggID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("egg variable not found")
	}
	return s.AppendAudit(ctx, actorID, "egg variable deleted", "egg", &eggID, fmt.Sprintf(`{"variableId":"%s"}`, variableID))
}

func (s *Store) getEggVariable(ctx context.Context, eggID, variableID string) (EggVariable, error) {
	var variable EggVariable
	err := s.db.QueryRow(ctx, `
		SELECT id::text, egg_id::text, name, description, env_variable, default_value,
		       user_viewable, user_editable, rules, sort, created_at
		FROM egg_variables WHERE id = $1 AND egg_id = $2
	`, variableID, eggID).Scan(&variable.ID, &variable.EggID, &variable.Name, &variable.Description,
		&variable.EnvVariable, &variable.DefaultValue, &variable.UserViewable,
		&variable.UserEditable, &variable.Rules, &variable.Sort, &variable.CreatedAt)
	return variable, err
}

func validateEggVariableRequest(req EggVariableRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}
	if !eggVariableNamePattern.MatchString(strings.TrimSpace(req.EnvVariable)) {
		return errors.New("envVariable must start with an uppercase letter and contain only A-Z, 0-9, and underscore")
	}
	if strings.TrimSpace(req.Rules) == "" {
		return errors.New("rules are required")
	}
	return validateVariableValue(req.DefaultValue, req.Rules)
}

func validateVariableValue(value, rules string) error {
	for _, rule := range strings.Split(rules, "|") {
		rule = strings.TrimSpace(rule)
		name, arg, _ := strings.Cut(rule, ":")
		switch name {
		case "", "nullable", "string":
		case "required":
			if value == "" {
				return errors.New("value is required")
			}
		case "max", "min":
			limit, err := strconv.Atoi(arg)
			if err != nil || limit < 0 {
				return fmt.Errorf("invalid %s validation rule", name)
			}
			length := len([]rune(value))
			if name == "max" && length > limit {
				return fmt.Errorf("value must be at most %d characters", limit)
			}
			if name == "min" && length < limit {
				return fmt.Errorf("value must be at least %d characters", limit)
			}
		case "in":
			allowed := strings.Split(arg, ",")
			found := false
			for _, candidate := range allowed {
				if value == candidate {
					found = true
					break
				}
			}
			if !found {
				return errors.New("value is not in the allowed set")
			}
		case "regex":
			pattern, err := regexp.Compile(arg)
			if err != nil {
				return errors.New("invalid regex validation rule")
			}
			if !pattern.MatchString(value) {
				return errors.New("value does not match the required pattern")
			}
		default:
			return fmt.Errorf("unsupported validation rule %q", name)
		}
	}
	return nil
}
