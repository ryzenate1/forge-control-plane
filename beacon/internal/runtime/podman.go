package runtime

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/client"
)

type PodmanRuntime struct {
	DockerRuntime
}

func NewPodmanRuntime(cfg PodmanConfig) (*PodmanRuntime, error) {
	if cfg.URI == "" {
		cfg.URI = "unix:///run/podman/podman.sock"
	}
	if err := validateDockerEndpoint(cfg.URI); err != nil {
		return nil, fmt.Errorf("invalid Podman endpoint: %w", err)
	}

	cli, err := client.NewClientWithOpts(
		client.WithHost(cfg.URI),
		client.WithAPIVersionNegotiation(),
		client.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to podman: %w", err)
	}

	networkName := strings.TrimSpace(os.Getenv("DAEMON_DOCKER_NETWORK"))
	if networkName == "" {
		networkName = "gamepanel"
	}

	return &PodmanRuntime{
		DockerRuntime: DockerRuntime{
			client:         cli,
			defaultNetwork: networkName,
			hostSettings:   loadDockerHostSettings(),
		},
	}, nil
}

func (r *PodmanRuntime) Provider() string {
	return ProviderPodman
}

func (r *PodmanRuntime) Ping(ctx context.Context) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("podman runtime is not initialized")
	}
	_, err := r.client.Ping(ctx)
	return err
}
