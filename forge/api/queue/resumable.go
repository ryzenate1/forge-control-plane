package queue

import (
	"context"
	"encoding/json"
	"fmt"
)

type ResumableState struct {
	CompletedSteps []string         `json:"completed_steps"`
	Cursors        map[string]any  `json:"cursors,omitempty"`
	Err            string          `json:"err,omitempty"`
}

type ResumableHelper struct {
	jobRow *JobRow
	state  *ResumableState
}

func NewResumableHelper(jobRow *JobRow) *ResumableHelper {
	state := &ResumableState{
		Cursors: make(map[string]any),
	}
	if len(jobRow.Metadata) > 0 {
		_ = json.Unmarshal(jobRow.Metadata, &state)
	}
	return &ResumableHelper{
		jobRow: jobRow,
		state:  state,
	}
}

func (h *ResumableHelper) IsStepCompleted(name string) bool {
	for _, s := range h.state.CompletedSteps {
		if s == name {
			return true
		}
	}
	return false
}

func (h *ResumableHelper) Step(ctx context.Context, name string, stepFunc func(context.Context) error) error {
	if h.IsStepCompleted(name) {
		return nil
	}

	if err := stepFunc(ctx); err != nil {
		h.state.Err = err.Error()
		_ = h.saveMetadata()
		return err
	}

	h.state.CompletedSteps = append(h.state.CompletedSteps, name)
	_ = h.saveMetadata()
	return nil
}

func (h *ResumableHelper) SetCursor(name string, cursor any) {
	if h.state.Cursors == nil {
		h.state.Cursors = make(map[string]any)
	}
	h.state.Cursors[name] = cursor
	_ = h.saveMetadata()
}

func (h *ResumableHelper) GetCursor(name string) (any, bool) {
	v, ok := h.state.Cursors[name]
	return v, ok
}

func (h *ResumableHelper) AllCompletedSteps() []string {
	return h.state.CompletedSteps
}

func (h *ResumableHelper) saveMetadata() error {
	data, err := json.Marshal(h.state)
	if err != nil {
		return fmt.Errorf("failed to marshal resumable state: %w", err)
	}
	h.jobRow.Metadata = data
	return nil
}
