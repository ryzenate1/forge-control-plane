package queue

import (
	"context"

	"gamepanel/forge/queue/queuetype"
)

type Hook interface {
	IsHook() bool
}

type HookDefaults struct{}

func (HookDefaults) IsHook() bool { return true }

type Middleware interface {
	IsMiddleware() bool
}

type MiddlewareDefaults struct{}

func (MiddlewareDefaults) IsMiddleware() bool { return true }

type Plugin interface {
	IsPlugin() bool
}

type PluginDefaults struct{}

func (PluginDefaults) IsPlugin() bool { return true }

type HookInsertBegin interface {
	Hook
	InsertBegin(ctx context.Context, params *queuetype.JobInsertParams) error
}

type HookWorkBegin interface {
	Hook
	WorkBegin(ctx context.Context, job *JobRow) error
}

type HookWorkEnd interface {
	Hook
	WorkEnd(ctx context.Context, job *JobRow, err error) error
}

type JobInsertMiddleware interface {
	Middleware
	InsertMany(ctx context.Context, manyParams []*queuetype.JobInsertParams, doInner func(context.Context) ([]*JobInsertResult, error)) ([]*JobInsertResult, error)
}

type WorkerMiddleware interface {
	Middleware
	Work(ctx context.Context, job *JobRow, doInner func(context.Context) error) error
}

type PluginLookup struct {
	hooks    []Hook
	middleware []Middleware
	plugins  []Plugin
}

func NewPluginLookup(plugins []Plugin, hooks []Hook, middleware []Middleware) *PluginLookup {
	return &PluginLookup{
		hooks:      hooks,
		middleware: middleware,
		plugins:    plugins,
	}
}

func (l *PluginLookup) WorkBeginHooks() []HookWorkBegin {
	var result []HookWorkBegin
	for _, h := range l.hooks {
		if wb, ok := h.(HookWorkBegin); ok {
			result = append(result, wb)
		}
	}
	for _, p := range l.plugins {
		if wb, ok := p.(HookWorkBegin); ok {
			result = append(result, wb)
		}
	}
	return result
}

func (l *PluginLookup) WorkEndHooks() []HookWorkEnd {
	var result []HookWorkEnd
	for _, h := range l.hooks {
		if we, ok := h.(HookWorkEnd); ok {
			result = append(result, we)
		}
	}
	for _, p := range l.plugins {
		if we, ok := p.(HookWorkEnd); ok {
			result = append(result, we)
		}
	}
	return result
}

func (l *PluginLookup) InsertBeginHooks() []HookInsertBegin {
	var result []HookInsertBegin
	for _, h := range l.hooks {
		if ib, ok := h.(HookInsertBegin); ok {
			result = append(result, ib)
		}
	}
	for _, p := range l.plugins {
		if ib, ok := p.(HookInsertBegin); ok {
			result = append(result, ib)
		}
	}
	return result
}

func (l *PluginLookup) WorkerMiddleware() []WorkerMiddleware {
	var result []WorkerMiddleware
	for _, m := range l.middleware {
		if wm, ok := m.(WorkerMiddleware); ok {
			result = append(result, wm)
		}
	}
	for _, p := range l.plugins {
		if wm, ok := p.(WorkerMiddleware); ok {
			result = append(result, wm)
		}
	}
	return result
}

func (l *PluginLookup) JobInsertMiddleware() []JobInsertMiddleware {
	var result []JobInsertMiddleware
	for _, m := range l.middleware {
		if jm, ok := m.(JobInsertMiddleware); ok {
			result = append(result, jm)
		}
	}
	for _, p := range l.plugins {
		if jm, ok := p.(JobInsertMiddleware); ok {
			result = append(result, jm)
		}
	}
	return result
}

func MiddlewareChain(middlewares []WorkerMiddleware, doInner func(context.Context) error, jobRow *JobRow) func(context.Context) error {
	chain := doInner
	for i := len(middlewares) - 1; i >= 0; i-- {
		mw := middlewares[i]
		inner := chain
		chain = func(ctx context.Context) error {
			return mw.Work(ctx, jobRow, inner)
		}
	}
	return chain
}

type JobInsertMiddlewareChain struct {
	middlewares []JobInsertMiddleware
}

func NewJobInsertMiddlewareChain(middlewares []JobInsertMiddleware) *JobInsertMiddlewareChain {
	return &JobInsertMiddlewareChain{middlewares: middlewares}
}

func (c *JobInsertMiddlewareChain) InsertMany(ctx context.Context, params []*queuetype.JobInsertParams, doInner func(context.Context) ([]*JobInsertResult, error)) ([]*JobInsertResult, error) {
	chain := doInner
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		mw := c.middlewares[i]
		inner := chain
		chain = func(ctx context.Context) ([]*JobInsertResult, error) {
			return mw.InsertMany(ctx, params, inner)
		}
	}
	return chain(ctx)
}

type HookInsertBeginFunc func(ctx context.Context, params *queuetype.JobInsertParams) error

func (f HookInsertBeginFunc) IsHook() bool { return true }
func (f HookInsertBeginFunc) InsertBegin(ctx context.Context, params *queuetype.JobInsertParams) error {
	return f(ctx, params)
}

type HookWorkBeginFunc func(ctx context.Context, job *JobRow) error

func (f HookWorkBeginFunc) IsHook() bool { return true }
func (f HookWorkBeginFunc) WorkBegin(ctx context.Context, job *JobRow) error {
	return f(ctx, job)
}

type HookWorkEndFunc func(ctx context.Context, job *JobRow, err error) error

func (f HookWorkEndFunc) IsHook() bool { return true }
func (f HookWorkEndFunc) WorkEnd(ctx context.Context, job *JobRow, err error) error {
	return f(ctx, job, err)
}

type WorkerMiddlewareFunc func(ctx context.Context, job *JobRow, doInner func(context.Context) error) error

func (f WorkerMiddlewareFunc) IsMiddleware() bool { return true }
func (f WorkerMiddlewareFunc) Work(ctx context.Context, job *JobRow, doInner func(context.Context) error) error {
	return f(ctx, job, doInner)
}
