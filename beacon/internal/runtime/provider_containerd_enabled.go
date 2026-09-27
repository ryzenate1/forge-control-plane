//go:build containerd

package runtime

func init() { supportedProviders[ProviderContainerd] = true }

func createContainerdRuntime(config ContainerdConfig) (Runtime, error) {
	return NewContainerdRuntime(config)
}
