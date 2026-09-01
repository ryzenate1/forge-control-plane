//go:build firecracker

package runtime

func init() { supportedProviders[ProviderFirecracker] = true }

func createFirecrackerRuntime(config FirecrackerConfig) (Runtime, error) {
	return NewFirecrackerRuntime(config)
}
