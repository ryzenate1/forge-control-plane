package runtime

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/docker/docker/client"
)

type PodmanRuntime struct {
	DockerRuntime
}

func NewPodmanRuntime(cfg PodmanConfig) (*PodmanRuntime, error) {
	if cfg.URI == "" {
		cfg.URI = "unix:///run/podman/podman.sock"
	}

	apiVersion := os.Getenv("PODMAN_API_VERSION")
	if apiVersion == "" {
		apiVersion = "5.0.0"
	}

	cli, err := client.NewClientWithOpts(
		client.WithHost(cfg.URI),
		client.WithAPIVersionNegotiation(),
		client.WithHTTPClient(&http.Client{}),
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
