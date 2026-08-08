package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type JobSetStateIfRunningManyParams struct {
	Jobs   []*JobSetStateIfRunningParams
	Schema string
}

type Client[TTx any] struct {
	driver    Driver[TTx]
	config    *Config
	producers map[string]*producer
	started   bool
	mu        sync.Mutex
	cancel    context.CancelFunc
	clientID  string
}

func NewClient[TTx any](driver Driver[TTx], config *Config) (*Client[TTx], error) {
	if config == nil {
		return nil, errors.New("config is required")
	}
	if config.Workers == nil {
		return nil, errors.New("workers must be provided")
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.RetryPolicy == nil {
		config.RetryPolicy = &DefaultClientRetryPolicy{}
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 25
	}
	if config.FetchPollInterval <= 0 {
		config.FetchPollInterval = 1 * time.Second
	}
	if config.FetchCooldown <= 0 {
		config.FetchCooldown = 100 * time.Millisecond
	}
	if config.Schema == "" {
		config.Schema = "public"
	}

	return &Client[TTx]{
		driver:    driver,
		config:    config,
		producers: make(map[string]*producer),
		clientID:  fmt.Sprintf("client-%d", time.Now().UnixNano()),
	}, nil
}

func (c *Client[TTx]) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return errors.New("client already started")
	}

	ctx, c.cancel = context.WithCancel(ctx)
	exec := c.driver.GetExecutor()

	for queueName, queueCfg := range c.config.Queues {
		if queueCfg.MaxWorkers <= 0 {
			queueCfg.MaxWorkers = 1
		}
		prod := newProducer(queueName, queueCfg, c.config, exec, c.clientID)
		c.producers[queueName] = prod
		prod.Start(ctx)
	}

	c.started = true
	return nil
}

func (c *Client[TTx]) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	if c.cancel != nil {
		c.cancel()
	}

	for _, prod := range c.producers {
		prod.Stop(ctx)
	}

	for k := range c.producers {
		delete(c.producers, k)
	}
	c.started = false
	return nil
}

func (c *Client[TTx]) Insert(ctx context.Context, args JobArgs, opts *InsertOpts) (*JobRow, error) {
	exec := c.driver.GetExecutor()
	params, err := c.buildInsertParams(args, opts)
	if err != nil {
		return nil, err
	}
	return exec.JobInsertFull(ctx, params)
}

func (c *Client[TTx]) InsertMany(ctx context.Context, params []InsertManyParams) ([]*JobRow, error) {
	exec := c.driver.GetExecutor()
	var full []*JobInsertFullParams
	for _, p := range params {
		fp, err := c.buildInsertParams(p.Args, p.Opts)
		if err != nil {
			return nil, err
		}
		full = append(full, fp)
	}
	return exec.JobInsertFullMany(ctx, &JobInsertFullManyParams{
		Jobs:   full,
		Schema: c.config.Schema,
	})
}

func (c *Client[TTx]) JobCancel(ctx context.Context, id int64) error {
	exec := c.driver.GetExecutor()
	_, err := exec.JobCancel(ctx, &JobCancelParams{
		ID:     id,
		Schema: c.config.Schema,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (c *Client[TTx]) JobRetry(ctx context.Context, id int64) error {
	exec := c.driver.GetExecutor()
	now := time.Now()
	_, err := exec.JobRetry(ctx, &JobRetryParams{
		ID:     id,
		Now:    &now,
		Schema: c.config.Schema,
	})
	return err
}

func (c *Client[TTx]) JobList(ctx context.Context, params *JobListParams) ([]*JobRow, error) {
	exec := c.driver.GetExecutor()
	return exec.JobList(ctx, params)
}

func (c *Client[TTx]) JobGet(ctx context.Context, id int64) (*JobRow, error) {
	exec := c.driver.GetExecutor()
	row, err := exec.JobGetByID(ctx, &JobGetByIDParams{
		ID:     id,
		Schema: c.config.Schema,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return row, nil
}

func (c *Client[TTx]) buildInsertParams(args JobArgs, opts *InsertOpts) (*JobInsertFullParams, error) {
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal job args: %w", err)
	}

	queue := args.Kind()
	maxAttempts := c.config.MaxAttempts
	var priority int
	var scheduledAt *time.Time
	var tags []string
	var metadata []byte

	if opts != nil {
		if opts.Queue != "" {
			queue = opts.Queue
		}
		if opts.MaxAttempts > 0 {
			maxAttempts = opts.MaxAttempts
		}
		priority = opts.Priority
		tags = opts.Tags
		if !opts.ScheduledAt.IsZero() {
			scheduledAt = &opts.ScheduledAt
		}
		if len(opts.Metadata) > 0 {
			metadata, err = json.Marshal(opts.Metadata)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal metadata: %w", err)
			}
		}
	}

	var uniqueKey []byte
	var uniqueStates byte
	if opts != nil && opts.UniqueOpts != nil {
		uniqueKey, uniqueStates = uniqueKeyWithQueue(args.Kind(), encodedArgs, queue, opts.UniqueOpts)
	}

	return &JobInsertFullParams{
		EncodedArgs:  encodedArgs,
		Kind:         args.Kind(),
		MaxAttempts:  maxAttempts,
		Priority:     priority,
		Queue:        queue,
		ScheduledAt:  scheduledAt,
		State:        JobStateAvailable,
		Tags:         tags,
		UniqueKey:    uniqueKey,
		UniqueStates: uniqueStates,
		Schema:       c.config.Schema,
		Metadata:     metadata,
	}, nil
}

func buildUniqueStates(states []JobState) byte {
	if len(states) == 0 {
		states = UniqueOptsByStateDefault()
	}
	return UniqueStatesToBitmask(states)
}
