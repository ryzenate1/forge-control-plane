// Package beacon adapts Forge's application deployment contract to the
// authenticated Beacon Docker runtime. It deliberately reuses the existing
// daemon client instead of opening a second control channel.
package beacon

import (
	"context"
	"errors"
	"sort"
	"strings"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/modules/apphosting/ports"
	"gamepanel/forge/internal/store"
)

type NodeCredentials interface {
	GetNodeDaemonCredential(context.Context, string) (string, error)
	GetNode(context.Context, string) (store.Node, error)
}

type Client interface {
	CreateServer(context.Context, string, string, daemon.CreateRequest) (daemon.CreateResponse, error)
	DeleteServer(context.Context, string, string, string) (daemon.PowerResponse, error)
}

type Runtime struct {
	nodes  NodeCredentials
	client Client
}

func NewRuntime(nodes NodeCredentials, client Client) (*Runtime, error) {
	if nodes == nil || client == nil {
		return nil, errors.New("node credentials and daemon client are required")
	}
	return &Runtime{nodes: nodes, client: client}, nil
}

func (r *Runtime) Deploy(ctx context.Context, request ports.DeploymentRequest) error {
	node, err := r.nodes.GetNode(ctx, request.NodeID)
	if err != nil {
		return err
	}
	if node.Maintenance || node.Draining {
		return errors.New("selected Beacon node is unavailable for placement")
	}
	if strings.EqualFold(strings.TrimSpace(node.ActualState), "offline") {
		return errors.New("selected Beacon node is offline")
	}
	if strings.TrimSpace(node.BaseURL) == "" {
		return errors.New("node has no Beacon endpoint")
	}
	credential, err := r.nodes.GetNodeDaemonCredential(ctx, request.NodeID)
	if err != nil {
		return err
	}
	env := make([]string, 0, len(request.Environment))
	for key, value := range request.Environment {
		env = append(env, key+"="+value)
	}
	sort.Strings(env)
	_, err = r.client.CreateServer(ctx, node.BaseURL, credential, daemon.CreateRequest{
		ServerID: request.WorkloadID, Image: request.Image, Command: request.Command, Env: env,
		MemoryMB: request.MemoryMB, CPUPercent: request.CPUPercent, DiskMB: request.DiskMB,
		// Reuse Beacon's provisioned managed network for the initial image
		// deployment path. A dedicated application network will be owned by the
		// networking module once its lifecycle is available.
		NetworkName: "gamepanel", Start: true,
	})
	return err
}

func (r *Runtime) Delete(ctx context.Context, workloadID, nodeID string) error {
	node, err := r.nodes.GetNode(ctx, nodeID)
	if err != nil {
		return err
	}
	credential, err := r.nodes.GetNodeDaemonCredential(ctx, nodeID)
	if err != nil {
		return err
	}
	_, err = r.client.DeleteServer(ctx, node.BaseURL, credential, workloadID)
	return err
}

var _ ports.Runtime = (*Runtime)(nil)
