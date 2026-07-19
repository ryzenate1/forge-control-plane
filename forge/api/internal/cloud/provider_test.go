package cloud

import (
	"context"
	"testing"
)

type testProvider struct {
	kind ProviderKind
}

func (p testProvider) Kind() ProviderKind { return p.kind }
func (p testProvider) Name() string       { return string(p.kind) }
func (p testProvider) Info() ProviderInfo {
	return ProviderInfo{Kind: p.kind, Name: p.Name(), Region: "test-1"}
}
func (p testProvider) ListInstances(context.Context) ([]Instance, error) { return nil, nil }
func (p testProvider) CreateInstance(context.Context, CreateInstanceRequest) (*Instance, error) {
	return nil, nil
}
func (p testProvider) DeleteInstance(context.Context, string) error { return nil }
func (p testProvider) GetInstance(context.Context, string) (*Instance, error) {
	return nil, nil
}
func (p testProvider) StartInstance(context.Context, string) error { return nil }
func (p testProvider) StopInstance(context.Context, string) error  { return nil }
func (p testProvider) ListRegions(context.Context) ([]string, error) {
	return nil, nil
}
func (p testProvider) ListInstanceTypes(context.Context, string) ([]string, error) {
	return nil, nil
}

func TestCreateInstanceRequestValidate(t *testing.T) {
	valid := CreateInstanceRequest{Name: "node-1", Region: "test-1", InstanceType: "small", Image: "image-1"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	for _, request := range []CreateInstanceRequest{
		{Region: valid.Region, InstanceType: valid.InstanceType, Image: valid.Image},
		{Name: valid.Name, InstanceType: valid.InstanceType, Image: valid.Image},
		{Name: valid.Name, Region: valid.Region, Image: valid.Image},
		{Name: valid.Name, Region: valid.Region, InstanceType: valid.InstanceType},
	} {
		if err := request.Validate(); err == nil {
			t.Fatalf("Validate() accepted incomplete request: %+v", request)
		}
	}
}

func TestManagerListProvidersIsSorted(t *testing.T) {
	manager := NewManager()
	manager.RegisterProvider(testProvider{kind: ProviderHetzner})
	manager.RegisterProvider(testProvider{kind: ProviderAWS})

	providers := manager.ListProviders()
	if len(providers) != 2 || providers[0].Kind != ProviderAWS || providers[1].Kind != ProviderHetzner {
		t.Fatalf("ListProviders() = %#v, want sorted providers", providers)
	}
}
