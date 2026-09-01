//go:build !firecracker

package runtime

import "fmt"

func createFirecrackerRuntime(FirecrackerConfig) (Runtime, error) {
	return nil, fmt.Errorf("firecracker runtime requires additional build tags: go build -tags firecracker")
}
