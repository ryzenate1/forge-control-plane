//go:build firecracker

package runtime

func createFirecrackerRuntime(config FirecrackerConfig) (Runtime, error) {
	return NewFirecrackerRuntime(config)
}
