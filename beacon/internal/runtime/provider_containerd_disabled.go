//go:build !containerd

package runtime

import "fmt"

func createContainerdRuntime(ContainerdConfig) (Runtime, error) {
	return nil, fmt.Errorf("containerd runtime requires additional build tags: go build -tags containerd")
}
