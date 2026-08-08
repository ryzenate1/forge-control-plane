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
	var rt Runtime
	var err error
	switch f.config.Provider {
	case ProviderDocker:
		rt, err = NewDockerRuntime()
	case ProviderPodman:
		rt, err = NewPodmanRuntime(f.config.Podman)
	case ProviderKubernetes:
		rt, err = NewKubernetesRuntime(f.config.Kubernetes)
	case ProviderContainerd:
		rt, err = createContainerdRuntime(f.config.Containerd)
	case ProviderFirecracker:
		rt, err = createFirecrackerRuntime(f.config.Firecracker)
	default:
		return nil, fmt.Errorf("unsupported runtime provider: %s", f.config.Provider)
	}
	if err != nil {
		return nil, err
	}
	if pinger, ok := rt.(Pinger); ok {
		if err := pinger.Ping(ctx); err != nil {
			_ = rt.Close()
			return nil, fmt.Errorf("%s runtime health check: %w", f.config.Provider, err)
		}
	}
	return rt, nil
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
