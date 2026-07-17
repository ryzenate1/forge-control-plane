package runtime

import (
	"context"
	"fmt"
)

type Factory struct {
	config RuntimeConfig
}

func NewFactory(config RuntimeConfig) *Factory {
	return &Factory{config: config}
}

func (f *Factory) CreateRuntime(ctx context.Context) (Runtime, error) {
	switch f.config.Provider {
	case ProviderDocker:
		return NewDockerRuntime()
	case ProviderPodman:
		return NewPodmanRuntime(f.config.Podman)
	case ProviderKubernetes:
		return NewKubernetesRuntime(f.config.Kubernetes)
	case ProviderContainerd:
		return nil, fmt.Errorf("containerd runtime requires additional build tags: go build -tags containerd")
	case ProviderFirecracker:
		return nil, fmt.Errorf("firecracker runtime requires additional build tags: go build -tags firecracker")
	default:
		return nil, fmt.Errorf("unsupported runtime provider: %s", f.config.Provider)
	}
}

func (f *Factory) AvailableProviders() []string {
	return []string{
		ProviderDocker,
		ProviderContainerd,
		ProviderPodman,
		ProviderFirecracker,
		ProviderKubernetes,
	}
}
