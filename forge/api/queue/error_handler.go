package queue

import (
	"context"
	"log/slog"
)

type ErrorHandler interface {
	HandleError(ctx context.Context, job *JobRow, err error) *ErrorHandlerResult
	HandlePanic(ctx context.Context, job *JobRow, panicVal any, trace string) *ErrorHandlerResult
}

type ErrorHandlerResult struct {
	SetCancelled bool
}

type DefaultErrorHandler struct {
	Logger *slog.Logger
}

func NewDefaultErrorHandler(logger *slog.Logger) *DefaultErrorHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &DefaultErrorHandler{Logger: logger}
}

func (h *DefaultErrorHandler) HandleError(ctx context.Context, job *JobRow, err error) *ErrorHandlerResult {
	h.Logger.ErrorContext(ctx, "job error",
		"job_id", job.ID,
		"kind", job.Kind,
		"attempt", job.Attempt,
		"error", err,
	)
	return &ErrorHandlerResult{}
}

func (h *DefaultErrorHandler) HandlePanic(ctx context.Context, job *JobRow, panicVal any, trace string) *ErrorHandlerResult {
	h.Logger.ErrorContext(ctx, "job panic",
		"job_id", job.ID,
		"kind", job.Kind,
		"panic", panicVal,
		"trace", trace,
	)
	return &ErrorHandlerResult{}
}
