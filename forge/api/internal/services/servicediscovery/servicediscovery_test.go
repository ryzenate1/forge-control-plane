package servicediscovery

import (
	"context"
	"net/netip"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterEndpoint_AddsEndpoint(t *testing.T) {
	svc := newTestService()

	ep := ServiceEndpoint{
		ServiceName:  "nginx",
		ServiceID:    "svc-1",
		NodeID:       "node-1",
		NodeName:     "alpha",
		Address:      netip.MustParseAddr("10.0.0.1"),
		Port:         80,
		Protocol:     ProtocolTCP,
		ReplicaIndex: 0,
	}
	created, err := svc.RegisterEndpoint(context.Background(), ep)
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, EndpointStatusUnknown, created.Status)
	assert.Equal(t, "nginx", created.ServiceName)

	endpoints := svc.ListEndpoints(context.Background(), EndpointFilter{})
	assert.Len(t, endpoints, 1)
}

func TestRegisterEndpoint_ValidatesFields(t *testing.T) {
	svc := newTestService()

	tests := []struct {
		name string
		ep   ServiceEndpoint
	}{
		{"empty service name", ServiceEndpoint{NodeID: "n1", Address: netip.MustParseAddr("10.0.0.1"), Port: 80}},
		{"empty node ID", ServiceEndpoint{ServiceName: "svc", Address: netip.MustParseAddr("10.0.0.1"), Port: 80}},
		{"invalid address", ServiceEndpoint{ServiceName: "svc", NodeID: "n1", Port: 80}},
		{"zero port", ServiceEndpoint{ServiceName: "svc", NodeID: "n1", Address: netip.MustParseAddr("10.0.0.1"), Port: 0}},
		{"port too high", ServiceEndpoint{ServiceName: "svc", NodeID: "n1", Address: netip.MustParseAddr("10.0.0.1"), Port: 70000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.RegisterEndpoint(context.Background(), tt.ep)
			assert.Error(t, err)
		})
	}
}

func TestRegisterEndpoint_DuplicateServiceName(t *testing.T) {
	svc := newTestService()

	ep1 := ServiceEndpoint{
		ServiceName:  "web",
		ServiceID:    "svc-web",
		NodeID:       "node-1",
		NodeName:     "alpha",
		Address:      netip.MustParseAddr("10.0.0.1"),
		Port:         80,
		ReplicaIndex: 0,
	}
	ep2 := ServiceEndpoint{
		ServiceName:  "web",
		ServiceID:    "svc-web",
		NodeID:       "node-2",
		NodeName:     "beta",
		Address:      netip.MustParseAddr("10.0.0.2"),
		Port:         80,
		ReplicaIndex: 1,
	}

	_, err := svc.RegisterEndpoint(context.Background(), ep1)
	require.NoError(t, err)
	_, err = svc.RegisterEndpoint(context.Background(), ep2)
	require.NoError(t, err)

	services := svc.ListServices(context.Background())
	require.Len(t, services, 1)
	assert.Equal(t, "web", services[0].ServiceName)
	assert.Len(t, services[0].Endpoints, 2)
}

func TestRemoveEndpoint_RemovesEndpoint(t *testing.T) {
	svc := newTestService()

	ep, err := svc.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "redis",
		ServiceID:   "svc-redis",
		NodeID:      "node-1",
		Address:     netip.MustParseAddr("10.0.0.5"),
		Port:        6379,
	})
	require.NoError(t, err)

	err = svc.RemoveEndpoint(context.Background(), ep.ID)
	require.NoError(t, err)

	_, err = svc.GetEndpoint(context.Background(), ep.ID)
	assert.Error(t, err)

	endpoints := svc.ListEndpoints(context.Background(), EndpointFilter{})
	assert.Len(t, endpoints, 0)
}

func TestGetEndpoint_NotFound(t *testing.T) {
	svc := newTestService()
	_, err := svc.GetEndpoint(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestResolve_ReturnsHealthyEndpoints(t *testing.T) {
	svc := newTestService()

	for i := 0; i < 3; i++ {
		ep := ServiceEndpoint{
			ServiceName:  "api",
			ServiceID:    "svc-api",
			NodeID:       "node-" + string(rune('0'+i)),
			Address:      netip.MustParseAddr("10.0.0." + string(rune('1'+i))),
			Port:         8080,
			Status:       EndpointStatusHealthy,
			ReplicaIndex: i,
		}
		_, err := svc.RegisterEndpoint(context.Background(), ep)
		require.NoError(t, err)
	}

	endpoints, err := svc.Resolve(context.Background(), "api", "")
	require.NoError(t, err)
	assert.Len(t, endpoints, 3)

	for _, ep := range endpoints {
		assert.Equal(t, EndpointStatusHealthy, ep.Status)
	}
}

func TestResolve_FiltersUnhealthy(t *testing.T) {
	svc := newTestService()

	_, _ = svc.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "api", ServiceID: "svc-api",
		NodeID: "n1", Address: netip.MustParseAddr("10.0.0.1"), Port: 8080,
		Status: EndpointStatusHealthy, ReplicaIndex: 0,
	})
	_, _ = svc.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "api", ServiceID: "svc-api",
		NodeID: "n2", Address: netip.MustParseAddr("10.0.0.2"), Port: 8080,
		Status: EndpointStatusUnhealthy, ReplicaIndex: 1,
	})

	healthy, err := svc.Resolve(context.Background(), "api", "")
	require.NoError(t, err)
	assert.Len(t, healthy, 1)
	assert.Equal(t, EndpointStatusHealthy, healthy[0].Status)
}

func TestResolve_UnknownService(t *testing.T) {
	svc := newTestService()
	_, err := svc.Resolve(context.Background(), "nonexistent", "")
	assert.Error(t, err)
}

func TestCrossNode_ServiceConnection(t *testing.T) {
	svc := newTestService()

	_, err := svc.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "db", ServiceID: "svc-db",
		NodeID: "node-a", Address: netip.MustParseAddr("10.0.0.10"), Port: 5432,
		Status: EndpointStatusHealthy,
	})
	require.NoError(t, err)
	_, err = svc.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "db", ServiceID: "svc-db",
		NodeID: "node-b", Address: netip.MustParseAddr("10.0.0.11"), Port: 5432,
		Status: EndpointStatusHealthy,
	})
	require.NoError(t, err)

	endpoints := svc.ResolveAll(context.Background(), "db", "")
	assert.Len(t, endpoints, 2)

	nodeAEndpoints := svc.ListEndpoints(context.Background(), EndpointFilter{NodeID: "node-a"})
	assert.Len(t, nodeAEndpoints, 1)
	assert.Equal(t, "node-a", nodeAEndpoints[0].NodeID)
}

func TestNodeOffline_MarksEndpoints(t *testing.T) {
	svc := newTestService()

	ep, err := svc.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "cache", ServiceID: "svc-cache",
		NodeID: "node-1", Address: netip.MustParseAddr("10.0.0.99"), Port: 11211,
		Status: EndpointStatusHealthy,
	})
	require.NoError(t, err)

	err = svc.UpdateEndpointStatus(context.Background(), ep.ID, EndpointStatusUnhealthy)
	require.NoError(t, err)

	updated, err := svc.GetEndpoint(context.Background(), ep.ID)
	require.NoError(t, err)
	assert.Equal(t, EndpointStatusUnhealthy, updated.Status)
}

func TestStaleEndpoint_MarkedUnhealthy(t *testing.T) {
	registry := NewRegistry(nil, nil)
	frozenNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	registry.now = func() time.Time { return frozenNow }

	ep, err := registry.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "stale-svc", ServiceID: "svc-stale",
		NodeID: "n1", Address: netip.MustParseAddr("10.0.0.50"), Port: 9000,
		Status: EndpointStatusHealthy,
	})
	require.NoError(t, err)

	reaper := NewStaleEndpointReaper(registry)
	reaper.heartbeatTTL = 5 * time.Minute
	reaper.now = func() time.Time { return frozenNow.Add(10 * time.Minute) }

	reaper.reap(context.Background())

	updated, ok := registry.GetEndpoint(ep.ID)
	require.True(t, ok)
	assert.Equal(t, EndpointStatusUnhealthy, updated.Status)
}

func TestStaleEndpointReaper_NoopForDraining(t *testing.T) {
	registry := NewRegistry(nil, nil)
	frozenNow := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	registry.now = func() time.Time { return frozenNow }

	_, err := registry.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "draining-svc", ServiceID: "svc-drain",
		NodeID: "n1", Address: netip.MustParseAddr("10.0.0.60"), Port: 9001,
		Status: EndpointStatusDraining,
	})
	require.NoError(t, err)

	reaper := NewStaleEndpointReaper(registry)
	reaper.heartbeatTTL = 1 * time.Minute
	reaper.now = func() time.Time { return frozenNow.Add(30 * time.Minute) }

	reaper.reap(context.Background())

	endpoints := registry.ListEndpoints(EndpointFilter{})
	require.Len(t, endpoints, 1)
	assert.Equal(t, EndpointStatusDraining, endpoints[0].Status)
}

func TestMultiReplica_EndpointSet(t *testing.T) {
	svc := newTestService()

	ips := []string{"10.0.0.100", "10.0.0.101", "10.0.0.102", "10.0.0.103", "10.0.0.104"}
	for i := 0; i < 5; i++ {
		_, err := svc.RegisterEndpoint(context.Background(), ServiceEndpoint{
			ServiceName: "web", ServiceID: "svc-web",
			NodeID: "node-" + itoa(i), Address: netip.MustParseAddr(ips[i]),
			Port: 3000, Status: EndpointStatusHealthy,
			ReplicaIndex: i,
		})
		require.NoError(t, err)
	}

	services := svc.ListServices(context.Background())
	require.Len(t, services, 1)
	assert.Len(t, services[0].Endpoints, 5)

	for i, ep := range services[0].Endpoints {
		assert.Equal(t, i, ep.ReplicaIndex)
	}
}

func TestTenantIsolation(t *testing.T) {
	svc := newTestService()

	_, err := svc.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "app", ServiceID: "svc-app",
		NodeID: "n1", Address: netip.MustParseAddr("10.0.0.1"), Port: 8080,
		TenantID: "tenant-alpha",
	})
	require.NoError(t, err)

	_, err = svc.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "app", ServiceID: "svc-app",
		NodeID: "n2", Address: netip.MustParseAddr("10.0.0.2"), Port: 8080,
		TenantID: "tenant-beta",
	})
	require.NoError(t, err)

	tenantsA := svc.ListEndpoints(context.Background(), EndpointFilter{TenantID: "tenant-alpha"})
	require.Len(t, tenantsA, 1)
	assert.Equal(t, "tenant-alpha", tenantsA[0].TenantID)

	tenantsB := svc.ListEndpoints(context.Background(), EndpointFilter{TenantID: "tenant-beta"})
	require.Len(t, tenantsB, 1)
	assert.Equal(t, "tenant-beta", tenantsB[0].TenantID)
}

func TestPrivateNetworkPolicy_Classify(t *testing.T) {
	policy := NewPrivateNetworkPolicy()

	tests := []struct {
		addr string
		want NetworkAccess
	}{
		{"10.0.0.1", NetworkAccessPrivate},
		{"192.168.1.1", NetworkAccessPrivate},
		{"172.16.0.1", NetworkAccessPrivate},
		{"8.8.8.8", NetworkAccessPublic},
		{"1.1.1.1", NetworkAccessPublic},
	}
	for _, tc := range tests {
		t.Run(tc.addr, func(t *testing.T) {
			ep := ServiceEndpoint{Address: netip.MustParseAddr(tc.addr)}
			got := policy.ClassifyEndpoint(ep)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPrivateNetworkPolicy_AddCustomCIDR(t *testing.T) {
	policy := NewPrivateNetworkPolicy()

	err := policy.AddPrivateCIDR("100.64.0.0/10")
	require.NoError(t, err)

	ep := ServiceEndpoint{Address: netip.MustParseAddr("100.64.0.1")}
	assert.Equal(t, NetworkAccessPrivate, policy.ClassifyEndpoint(ep))

	ep2 := ServiceEndpoint{Address: netip.MustParseAddr("100.127.255.255")}
	assert.Equal(t, NetworkAccessPrivate, policy.ClassifyEndpoint(ep2))

	ep3 := ServiceEndpoint{Address: netip.MustParseAddr("100.128.0.1")}
	assert.Equal(t, NetworkAccessPublic, policy.ClassifyEndpoint(ep3))

	err = policy.RemovePrivateCIDR("100.64.0.0/10")
	require.NoError(t, err)
	assert.Equal(t, NetworkAccessPublic, policy.ClassifyEndpoint(ep))
}

func TestHealthBased_EndpointFiltering(t *testing.T) {
	svc := newTestService()
	_ = setupTestEndpoints(svc)

	all := svc.ListEndpoints(context.Background(), EndpointFilter{})
	assert.Len(t, all, 4)

	healthy := svc.ListEndpoints(context.Background(), EndpointFilter{HealthyOnly: true})
	for _, ep := range healthy {
		assert.Equal(t, EndpointStatusHealthy, ep.Status)
	}

	filterStatus := svc.ListEndpoints(context.Background(), EndpointFilter{Status: EndpointStatusUnhealthy})
	for _, ep := range filterStatus {
		assert.Equal(t, EndpointStatusUnhealthy, ep.Status)
	}
}

func TestNetworkVisibilityView(t *testing.T) {
	svc := newTestService()
	_ = setupTestEndpoints(svc)

	view := svc.NetworkVisibility(context.Background())
	assert.Greater(t, view.TotalEndpoints, 0)
	assert.Equal(t, view.HealthyCount+view.UnhealthyCount, view.TotalEndpoints)
	assert.Greater(t, view.NodesCount, 0)
}

func TestNodeNetworkView(t *testing.T) {
	svc := newTestService()
	_ = setupTestEndpoints(svc)

	view := svc.NodeNetworkView(context.Background(), "node-1")
	assert.Equal(t, "node-1", view.NodeID)
	assert.NotEmpty(t, view.Endpoints)
}

func TestReachabilityVerifier_Sweep(t *testing.T) {
	svc := newTestService()
	_ = setupTestEndpoints(svc)

	svc.verifier.SetTimeout(50 * time.Millisecond)

	done := make(chan struct{})
	var results []ReachabilityResult
	go func() {
		results = svc.SweepReachability(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("sweep timed out")
	}

	assert.NotEmpty(t, results)
}

func TestSelectNodeAddress(t *testing.T) {
	node := fakeNode{
		fqdn:       "node1.example.com",
		publicHost: "203.0.113.1",
		allowedIPs: []string{"10.0.0.1", "192.168.1.1"},
	}

	addr := SelectNodeAddress(node)
	assert.Equal(t, "10.0.0.1", addr.String())

	node2 := fakeNode{
		fqdn:       "10.0.0.5",
		publicHost: "",
		allowedIPs: nil,
	}
	addr2 := SelectNodeAddress(node2)
	assert.True(t, addr2.IsValid())
	assert.Equal(t, "10.0.0.5", addr2.String())

	node3 := fakeNode{
		fqdn:       "",
		publicHost: "",
		allowedIPs: nil,
	}
	addr3 := SelectNodeAddress(node3)
	assert.False(t, addr3.IsValid())
}

func TestStaleReaper_Stats(t *testing.T) {
	reaper := NewStaleEndpointReaper(NewRegistry(nil, nil))
	reaper.heartbeatTTL = 5 * time.Minute

	lastRun, reaped, interval := reaper.Stats()
	assert.True(t, lastRun.IsZero())
	assert.Equal(t, 0, reaped)
	assert.Equal(t, 30*time.Second, interval)
}

func TestServiceDiscovery_StartStop(t *testing.T) {
	svc := newTestService()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.Start(ctx)
	svc.Stop()
}

func TestPrivateNetworkPolicy_AllowPort(t *testing.T) {
	policy := NewPrivateNetworkPolicy()
	policy.AllowPort("mysql", 3306)
	policy.AllowPort("mysql", 33060)

	assert.True(t, policy.IsPortAllowed("mysql", 3306))
	assert.True(t, policy.IsPortAllowed("mysql", 33060))
	assert.False(t, policy.IsPortAllowed("mysql", 8080))
	assert.False(t, policy.IsPortAllowed("redis", 6379))

	ports := policy.AllowedPorts("mysql")
	assert.Len(t, ports, 2)

	policy.RevokePort("mysql", 3306)
	assert.False(t, policy.IsPortAllowed("mysql", 3306))
	assert.True(t, policy.IsPortAllowed("mysql", 33060))
}

func TestEndpointRegistry_RebuildFromEndpoints(t *testing.T) {
	registry := NewRegistry(nil, nil)

	endpoints := []ServiceEndpoint{
		{ID: "e1", ServiceName: "svc1", ServiceID: "s1", NodeID: "n1", Address: netip.MustParseAddr("10.0.0.1"), Port: 80, Status: EndpointStatusHealthy, ReplicaIndex: 0},
		{ID: "e2", ServiceName: "svc1", ServiceID: "s1", NodeID: "n2", Address: netip.MustParseAddr("10.0.0.2"), Port: 80, Status: EndpointStatusHealthy, ReplicaIndex: 1},
		{ID: "e3", ServiceName: "svc2", ServiceID: "s2", NodeID: "n1", Address: netip.MustParseAddr("10.0.0.3"), Port: 443, Status: EndpointStatusUnhealthy, ReplicaIndex: 0},
	}
	registry.RebuildFromEndpoints(endpoints)

	assert.Equal(t, 3, registry.EndpointCount())
	assert.Equal(t, 2, registry.ServiceCount())

	svc1 := registry.GetServiceEndpoints("svc1", "")
	assert.Len(t, svc1, 2)

	svc2 := registry.GetServiceEndpoints("svc2", "")
	assert.Len(t, svc2, 1)
	assert.Equal(t, EndpointStatusUnhealthy, svc2[0].Status)
}

func TestEndpointRegistry_TouchHeartbeat(t *testing.T) {
	registry := NewRegistry(nil, nil)
	frozen := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	registry.now = func() time.Time { return frozen }

	ep, err := registry.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "svc", ServiceID: "sid", NodeID: "n1",
		Address: netip.MustParseAddr("10.0.0.1"), Port: 8080,
	})
	require.NoError(t, err)

	newTime := frozen.Add(1 * time.Hour)
	registry.now = func() time.Time { return newTime }
	err = registry.TouchHeartbeat(context.Background(), ep.ID)
	require.NoError(t, err)

	updated, ok := registry.GetEndpoint(ep.ID)
	require.True(t, ok)
	assert.Equal(t, newTime, updated.LastHeartbeat)
}

func TestVisibility_BuildNetworkVisibility(t *testing.T) {
	registry := NewRegistry(nil, nil)
	policy := NewPrivateNetworkPolicy()

	_, _ = registry.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "public-svc", ServiceID: "ps1", NodeID: "n1",
		Address: netip.MustParseAddr("8.8.8.8"), Port: 80, Status: EndpointStatusHealthy,
	})
	_, _ = registry.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "private-svc", ServiceID: "ps2", NodeID: "n2",
		Address: netip.MustParseAddr("10.0.0.1"), Port: 8080, Status: EndpointStatusHealthy,
	})
	_, _ = registry.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "private-svc", ServiceID: "ps2", NodeID: "n3",
		Address: netip.MustParseAddr("10.0.0.2"), Port: 8080, Status: EndpointStatusUnhealthy,
	})

	view := BuildNetworkVisibility(registry, policy)

	assert.Equal(t, 3, view.TotalEndpoints)
	assert.Equal(t, 2, view.HealthyCount)
	assert.Equal(t, 1, view.UnhealthyCount)
	assert.Equal(t, 3, view.NodesCount)

	assert.Len(t, view.Services, 2)

	for _, svc := range view.Services {
		if svc.ServiceName == "public-svc" {
			assert.Equal(t, NetworkAccessPublic, svc.Access)
			assert.Len(t, svc.Endpoints, 1)
		} else {
			assert.Equal(t, NetworkAccessPrivate, svc.Access)
			assert.Len(t, svc.Endpoints, 2)
		}
	}
}

func TestIPv4AndIPv6Behavior(t *testing.T) {
	svc := newTestService()

	_, err := svc.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "ipv4-svc", ServiceID: "s1", NodeID: "n1",
		Address: netip.MustParseAddr("10.0.0.1"), Port: 80,
	})
	require.NoError(t, err)

	_, err = svc.RegisterEndpoint(context.Background(), ServiceEndpoint{
		ServiceName: "ipv6-svc", ServiceID: "s2", NodeID: "n2",
		Address: netip.MustParseAddr("fd00::1"), Port: 80,
	})
	require.NoError(t, err)

	ipv4 := svc.ListEndpoints(context.Background(), EndpointFilter{ServiceName: "ipv4-svc"})
	require.Len(t, ipv4, 1)
	assert.True(t, ipv4[0].Address.Is4())

	ipv6 := svc.ListEndpoints(context.Background(), EndpointFilter{ServiceName: "ipv6-svc"})
	require.Len(t, ipv6, 1)
	assert.True(t, ipv6[0].Address.Is6())

	policy := NewPrivateNetworkPolicy()

	ipv4Ep := ServiceEndpoint{Address: netip.MustParseAddr("10.0.0.1")}
	assert.Equal(t, NetworkAccessPrivate, policy.ClassifyEndpoint(ipv4Ep))

	ipv6Private := ServiceEndpoint{Address: netip.MustParseAddr("fd00::1")}
	assert.Equal(t, NetworkAccessPrivate, policy.ClassifyEndpoint(ipv6Private))

	ipv6Public := ServiceEndpoint{Address: netip.MustParseAddr("2001:db8::1")}
	assert.Equal(t, NetworkAccessPublic, policy.ClassifyEndpoint(ipv6Public))
}

func TestListServices_Sorted(t *testing.T) {
	svc := newTestService()

	names := []string{"z-svc", "m-svc", "a-svc"}
	for _, name := range names {
		_, err := svc.RegisterEndpoint(context.Background(), ServiceEndpoint{
			ServiceName: name, ServiceID: "svc-" + name,
			NodeID: "n1", Address: netip.MustParseAddr("10.0.0.1"), Port: 8080,
		})
		require.NoError(t, err)
	}

	services := svc.ListServices(context.Background())
	require.Len(t, services, 3)
	assert.True(t, sort.SliceIsSorted(services, func(i, j int) bool {
		return services[i].ServiceName < services[j].ServiceName
	}))
}

func TestNewTestService(t *testing.T) {
	svc := newTestService()
	require.NotNil(t, svc)
	require.NotNil(t, svc.discovery)
	require.NotNil(t, svc.registry)
	require.NotNil(t, svc.reaper)
	require.NotNil(t, svc.verifier)
	require.NotNil(t, svc.policy)
}

func newTestService() *Service {
	svc := New(nil, nil)
	svc.SetNetworkAdapter(nil)
	return svc
}

func setupTestEndpoints(svc *Service) []string {
	var ids []string

	eps := []ServiceEndpoint{
		{ServiceName: "api", ServiceID: "svc-api", NodeID: "node-1", NodeName: "Alpha", Address: netip.MustParseAddr("10.0.0.1"), Port: 8080, Status: EndpointStatusHealthy, ReplicaIndex: 0},
		{ServiceName: "api", ServiceID: "svc-api", NodeID: "node-2", NodeName: "Beta", Address: netip.MustParseAddr("10.0.0.2"), Port: 8080, Status: EndpointStatusHealthy, ReplicaIndex: 1},
		{ServiceName: "web", ServiceID: "svc-web", NodeID: "node-1", NodeName: "Alpha", Address: netip.MustParseAddr("10.0.0.3"), Port: 3000, Status: EndpointStatusHealthy, ReplicaIndex: 0},
		{ServiceName: "web", ServiceID: "svc-web", NodeID: "node-3", NodeName: "Gamma", Address: netip.MustParseAddr("10.0.0.4"), Port: 3000, Status: EndpointStatusUnhealthy, ReplicaIndex: 1},
	}

	for _, ep := range eps {
		created, err := svc.RegisterEndpoint(context.Background(), ep)
		if err == nil && created != nil {
			ids = append(ids, created.ID)
		}
	}

	return ids
}

func itoa(i int) string {
	return string(rune('0'+i%10)) + string(rune('0'+i/10))
}

type fakeNode struct {
	fqdn       string
	publicHost string
	allowedIPs []string
}

func (f fakeNode) GetFQDN() string           { return f.fqdn }
func (f fakeNode) GetPublicHostname() string { return f.publicHost }
func (f fakeNode) GetAllowedIPs() []string   { return f.allowedIPs }
