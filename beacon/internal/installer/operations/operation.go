package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

type Operation interface {
	Execute(ctx context.Context, serverDir string) error
}

type Condition struct {
	FileExists  *string `json:"fileExists,omitempty"`
	FileMissing *string `json:"fileMissing,omitempty"`
}

func (c *Condition) ShouldExecute(serverDir string) (bool, error) {
	if c == nil {
		return true, nil
	}
	if c.FileExists != nil {
		ok, err := FileExists(serverDir, *c.FileExists)
		if err != nil {
			return false, err
		}
		return ok, nil
	}
	if c.FileMissing != nil {
		ok, err := FileExists(serverDir, *c.FileMissing)
		if err != nil {
			return false, err
		}
		return !ok, nil
	}
	return true, nil
}

type OperationFactory func(args json.RawMessage) (Operation, error)

var (
	registryMu sync.RWMutex
	registry   = map[string]OperationFactory{}
)

func Register(name string, factory OperationFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, ok := registry[name]; ok {
		panic(fmt.Sprintf("operation %q already registered", name))
	}
	registry[name] = factory
}

func GetFactory(name string) (OperationFactory, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := registry[name]
	return f, ok
}

func ListRegistered() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
