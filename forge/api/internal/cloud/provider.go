package cloud

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type ProviderKind string

const (
	ProviderAWS     ProviderKind = "aws"
	ProviderGCP     ProviderKind = "gcp"
	ProviderAzure   ProviderKind = "azure"
	ProviderDO      ProviderKind = "digitalocean"
	ProviderHetzner ProviderKind = "hetzner"
)

type ProviderInfo struct {
	Kind   ProviderKind `json:"kind"`
	Name   string       `json:"name"`
	Region string       `json:"region,omitempty"`
}

type Instance struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Provider     ProviderKind      `json:"provider"`
	Region       string            `json:"region"`
	InstanceType string            `json:"instanceType"`
	PublicIP     string            `json:"publicIp"`
	PrivateIP    string            `json:"privateIp"`
	Status       string            `json:"status"`
	CPU          int               `json:"cpu"`
	MemoryMB     int               `json:"memoryMb"`
	DiskGB       int               `json:"diskGb"`
	CreatedAt    time.Time         `json:"createdAt"`
	Tags         map[string]string `json:"tags,omitempty"`
}

type CreateInstanceRequest struct {
	Name               string            `json:"name"`
	Region             string            `json:"region"`
	InstanceType       string            `json:"instanceType"`
	Image              string            `json:"image"`
	SSHKeys            []string          `json:"sshKeys,omitempty"`
	Tags               map[string]string `json:"tags,omitempty"`
	SubnetID           string            `json:"subnetId,omitempty"`
	SecurityGroupIDs   []string          `json:"securityGroupIds,omitempty"`
	IAMInstanceProfile string            `json:"iamInstanceProfile,omitempty"`
	DiskGB             int               `json:"diskGb,omitempty"`
	UserData           string            `json:"-"`
	BeaconImage        string            `json:"beaconImage,omitempty"`
}

func (r CreateInstanceRequest) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("instance name is required")
	}
	if strings.TrimSpace(r.Region) == "" {
		return errors.New("region is required")
	}
	if strings.TrimSpace(r.InstanceType) == "" {
		return errors.New("instance type is required")
	}
	if strings.TrimSpace(r.Image) == "" {
		return errors.New("image is required")
	}
	return nil
}

type Provider interface {
	Kind() ProviderKind
	Name() string
	Info() ProviderInfo
	ListInstances(ctx context.Context) ([]Instance, error)
	CreateInstance(ctx context.Context, req CreateInstanceRequest) (*Instance, error)
	DeleteInstance(ctx context.Context, id string) error
	GetInstance(ctx context.Context, id string) (*Instance, error)
	StartInstance(ctx context.Context, id string) error
	StopInstance(ctx context.Context, id string) error
	ListRegions(ctx context.Context) ([]string, error)
	ListInstanceTypes(ctx context.Context, region string) ([]string, error)
}

type Manager struct {
	providers map[ProviderKind]Provider
}

func NewManager() *Manager {
	return &Manager{providers: make(map[ProviderKind]Provider)}
}

func (m *Manager) RegisterProvider(p Provider) {
	if p != nil {
		m.providers[p.Kind()] = p
	}
}

func (m *Manager) GetProvider(kind ProviderKind) (Provider, error) {
	p, ok := m.providers[kind]
	if !ok {
		return nil, fmt.Errorf("cloud provider %q is not configured", kind)
	}
	return p, nil
}

func (m *Manager) ListProviders() []ProviderInfo {
	providers := make([]ProviderInfo, 0, len(m.providers))
	for _, provider := range m.providers {
		providers = append(providers, provider.Info())
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Kind < providers[j].Kind })
	return providers
}

func (m *Manager) ProvisionNode(ctx context.Context, kind ProviderKind, req CreateInstanceRequest) (*Instance, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	provider, err := m.GetProvider(kind)
	if err != nil {
		return nil, err
	}
	return provider.CreateInstance(ctx, req)
}

func (m *Manager) DeprovisionNode(ctx context.Context, kind ProviderKind, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return errors.New("instance ID is required")
	}
	provider, err := m.GetProvider(kind)
	if err != nil {
		return err
	}
	return provider.DeleteInstance(ctx, instanceID)
}

func (m *Manager) GetNodeInstance(ctx context.Context, kind ProviderKind, instanceID string) (*Instance, error) {
	provider, err := m.GetProvider(kind)
	if err != nil {
		return nil, err
	}
	return provider.GetInstance(ctx, instanceID)
}
