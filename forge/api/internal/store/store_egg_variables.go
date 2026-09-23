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

func splitValidationRules(rules string) []string {
	var out []string
	var cur strings.Builder
	inRegex := false
	inBracket := false
	escaped := false
	for i := 0; i < len(rules); i++ {
		c := rules[i]
		if escaped {
			cur.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' {
			cur.WriteByte(c)
			escaped = true
			continue
		}
		// Detect regex start: "regex:" prefix
		if !inRegex && cur.Len() >= 6 && cur.String()[cur.Len()-6:] == "regex:" {
			// Check if next char is '/' starting pattern
			if c == '/' {
				inRegex = true
				cur.WriteByte(c)
				continue
			}
		}
		if inRegex {
			cur.WriteByte(c)
			if c == '[' && !inBracket {
				inBracket = true
			} else if c == ']' && inBracket {
				inBracket = false
			} else if c == '/' && !inBracket {
				// Potential end of regex pattern, check for flags or pipe
				// Look ahead: if next char is '|' or end, it's the end
				// Also handle flags like /i
				j := i + 1
				for j < len(rules) && ((rules[j] >= 'a' && rules[j] <= 'z') || (rules[j] >= 'A' && rules[j] <= 'Z')) {
					cur.WriteByte(rules[j])
					j++
				}
				i = j - 1
				inRegex = false
			}
			continue
		}
		if c == '[' {
			inBracket = true
			cur.WriteByte(c)
		} else if c == ']' {
			inBracket = false
			cur.WriteByte(c)
		} else if c == '|' && !inBracket {
			out = append(out, cur.String())
			cur.Reset()
		} else {
			cur.WriteByte(c)
		}
	}
	out = append(out, cur.String())
	return out
}

func validateVariableValue(value, rules string) error {
	isInteger := false
	isNullable := false
	for _, r := range splitValidationRules(rules) {
		if strings.TrimSpace(r) == "integer" {
			isInteger = true
		}
		if strings.TrimSpace(r) == "nullable" {
			isNullable = true
		}
	}
	if value == "" && isNullable {
		return nil
	}
	for _, rule := range splitValidationRules(rules) {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		name, arg, _ := strings.Cut(rule, ":")
		name = strings.TrimSpace(name)
		arg = strings.TrimSpace(arg)
		switch name {
		case "", "nullable", "string", "integer", "boolean":
		case "required":
			if value == "" {
				return errors.New("value is required")
			}
		case "max", "min":
			limit, err := strconv.Atoi(arg)
			if err != nil || limit < 0 {
				return fmt.Errorf("invalid %s validation rule", name)
			}
			if isInteger && value != "" {
				// Numeric comparison for integer rules
				intVal, err := strconv.Atoi(value)
				if err != nil {
					return errors.New("value must be an integer")
				}
				if name == "max" && intVal > limit {
					return fmt.Errorf("value must be at most %d", limit)
				}
				if name == "min" && intVal < limit {
					return fmt.Errorf("value must be at least %d", limit)
				}
			} else {
				length := len([]rune(value))
				if name == "max" && length > limit {
					return fmt.Errorf("value must be at most %d characters", limit)
				}
				if name == "min" && length < limit {
					return fmt.Errorf("value must be at least %d characters", limit)
				}
			}
		case "between":
			parts := strings.Split(arg, ",")
			if len(parts) != 2 {
				return fmt.Errorf("invalid between rule")
			}
			low, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			high, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil {
				return fmt.Errorf("invalid between rule")
			}
			if isInteger {
				intVal, err := strconv.Atoi(value)
				if err != nil {
					return errors.New("value must be an integer")
				}
				if intVal < low || intVal > high {
					return fmt.Errorf("value must be between %d and %d", low, high)
				}
			}
		case "in":
			allowed := strings.Split(arg, ",")
			found := false
			for _, candidate := range allowed {
				if value == strings.TrimSpace(candidate) {
					found = true
					break
				}
			}
			if !found {
				return errors.New("value is not in the allowed set")
			}
		case "regex":
			pattern := arg
			flags := ""
			// Strip slash delimiters /pattern/flags
			if len(pattern) >= 2 && pattern[0] == '/' {
				// Find closing slash
				end := -1
				escaped := false
				inBracket := false
				for idx := 1; idx < len(pattern); idx++ {
					c := pattern[idx]
					if escaped {
						escaped = false
						continue
					}
					if c == '\\' {
						escaped = true
						continue
					}
					if c == '[' && !inBracket {
						inBracket = true
					} else if c == ']' && inBracket {
						inBracket = false
					} else if c == '/' && !inBracket {
						end = idx
						break
					}
				}
				if end != -1 {
					flags = pattern[end+1:]
					pattern = pattern[1:end]
					// Validate trailing flags. Only i (case-insensitive), m
					// (multiline) and s (dotall) map onto Go inline flags. Any
					// other trailing character (e.g. the JS-only g/x flags) is not
					// a valid Go regexp flag, so the rule must be rejected rather
					// than silently compiling a different pattern.
					flagPrefix := ""
					seenFlag := map[byte]bool{}
					for k := 0; k < len(flags); k++ {
						switch f := flags[k]; f {
						case 'i', 'm', 's':
							if !seenFlag[f] {
								seenFlag[f] = true
								flagPrefix += string(f)
							}
						default:
							return errors.New("invalid regex validation rule: unsupported flag")
						}
					}
					if flagPrefix != "" {
						pattern = "(?" + flagPrefix + ")" + pattern
					}
				}
			}
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				return errors.New("invalid regex validation rule")
			}
			if !compiled.MatchString(value) {
				return errors.New("value does not match the required pattern")
			}
		default:
			return fmt.Errorf("unsupported validation rule %q", name)
		}
	}
	return nil
}
