package queue

import "context"

func (c *Client[TTx]) JobDeleteMany(ctx context.Context, params *JobDeleteManyParams) ([]*JobRow, error) {
	exec := c.driver.GetExecutor()
	return exec.JobDeleteMany(ctx, params)
}

func (c *Client[TTx]) JobGetByIDMany(ctx context.Context, params *JobGetByIDManyParams) ([]*JobRow, error) {
	exec := c.driver.GetExecutor()
	return exec.JobGetByIDMany(ctx, params)
}
