//go:build containerd

package runtime

func createContainerdRuntime(config ContainerdConfig) (Runtime, error) {
	return NewContainerdRuntime(config)
}
