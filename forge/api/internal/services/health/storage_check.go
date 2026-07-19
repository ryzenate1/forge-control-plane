package health

import (
	"context"
)

type StorageCheck struct {
	label     string
	checkFunc func(context.Context) (bool, string, map[string]any)
}

func NewStorageCheck(label string, checkFunc func(context.Context) (bool, string, map[string]any)) *StorageCheck {
	return &StorageCheck{
		label:     label,
		checkFunc: checkFunc,
	}
}

func (c *StorageCheck) Name() string  { return "storage" }
func (c *StorageCheck) Label() string { return c.label }

func (c *StorageCheck) Run(ctx context.Context) CheckResult {
	result := CheckResult{
		Name:    c.Name(),
		Label:   c.Label(),
		Status:  StatusOK,
		Message: "Storage accessible",
	}

	if c.checkFunc == nil {
		result.Status = StatusWarning
		result.Message = "Storage check not configured"
		return result
	}

	ok, message, details := c.checkFunc(ctx)
	if !ok {
		result.Status = StatusFailed
		result.Message = message
	} else {
		result.Status = StatusOK
		result.Message = message
	}
	result.Details = details

	return result
}
