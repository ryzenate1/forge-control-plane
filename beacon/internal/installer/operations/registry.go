package operations

import (
	"context"
	"encoding/json"
	"fmt"
)

type Step struct {
	Type      string          `json:"type"`
	Args      json.RawMessage `json:"args,omitempty"`
	Condition *Condition      `json:"condition,omitempty"`
}

func ExecuteSteps(ctx context.Context, serverDir string, steps []Step) error {
	for i, step := range steps {
		factory, ok := GetFactory(step.Type)
		if !ok {
			return fmt.Errorf("step %d: unknown operation type %q", i, step.Type)
		}
		op, err := factory(step.Args)
		if err != nil {
			return fmt.Errorf("step %d (%s): build: %w", i, step.Type, err)
		}
		ok, err = step.Condition.ShouldExecute(serverDir)
		if err != nil {
			return fmt.Errorf("step %d (%s): condition: %w", i, step.Type, err)
		}
		if !ok {
			continue
		}
		if err := op.Execute(ctx, serverDir); err != nil {
			return fmt.Errorf("step %d (%s): %w", i, step.Type, err)
		}
	}
	return nil
}

func StepsFromJSON(data []byte) ([]Step, error) {
	var steps []Step
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil, fmt.Errorf("parse steps: %w", err)
	}
	return steps, nil
}
