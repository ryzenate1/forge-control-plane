package runtime

import (
	"errors"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestNewKubernetesRuntimeFailsInCluster(t *testing.T) {
	_, err := NewKubernetesRuntime(KubernetesConfig{InCluster: true})
	if err == nil {
		t.Fatal("expected in-cluster config to fail outside a cluster")
	}
}

func TestNewKubernetesRuntimeFailsBadKubeconfig(t *testing.T) {
	badPath := filepath.Join(t.TempDir(), "nonexistent-config")
	_, err := NewKubernetesRuntime(KubernetesConfig{KubeconfigPath: badPath})
	if err == nil {
		t.Fatal("expected bad kubeconfig path to fail")
	}
}

func TestKubernetesProvider(t *testing.T) {
	r := &KubernetesRuntime{}
	if got := r.Provider(); got != ProviderKubernetes {
		t.Fatalf("Provider() = %q, want %q", got, ProviderKubernetes)
	}
}

func TestKubePodName(t *testing.T) {
	if got := kubePodName("abc123"); got != "forge-abc123" {
		t.Fatalf("kubePodName = %q", got)
	}
}

func TestEnvVarsFromSlice(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		want int
	}{
		{"nil input", nil, 0},
		{"empty input", []string{}, 0},
		{"valid pairs", []string{"A=1", "B=2"}, 2},
		{"malformed ignored", []string{"A=1", "NOEQUALS"}, 1},
		{"empty value", []string{"A="}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			vars := envVarsFromSlice(tc.env)
			if len(vars) != tc.want {
				t.Fatalf("got %d vars, want %d", len(vars), tc.want)
			}
		})
	}
}

func TestContainerPorts(t *testing.T) {
	ports := containerPorts([]PortBinding{
		{ContainerPort: 25565, HostPort: 25565, HostIP: "0.0.0.0", Protocol: "tcp"},
		{ContainerPort: 19132, HostPort: 19132, Protocol: "udp"},
	})
	if len(ports) != 2 {
		t.Fatalf("got %d ports, want 2", len(ports))
	}
	if ports[0].Protocol != "TCP" || ports[0].ContainerPort != 25565 {
		t.Fatalf("unexpected first port: %+v", ports[0])
	}
	if ports[1].Protocol != "UDP" || ports[1].ContainerPort != 19132 {
		t.Fatalf("unexpected second port: %+v", ports[1])
	}
}

func TestBuildResourceLimits(t *testing.T) {
	req := CreateRequest{CPUPercent: 250, CPUShares: 512, MemoryMB: 1024}
	limits := buildResourceLimits(req)
	if limits.Cpu().Cmp(resource.MustParse("2500m")) != 0 {
		t.Fatalf("CPU limit = %v, want 2500m", limits.Cpu())
	}
	if limits.Memory().Cmp(resource.MustParse("1024Mi")) != 0 {
		t.Fatalf("memory limit = %v, want 1024Mi", limits.Memory())
	}

	req2 := CreateRequest{CPUShares: 512}
	limits2 := buildResourceLimits(req2)
	if limits2.Cpu().Cmp(resource.MustParse("512m")) != 0 {
		t.Fatalf("CPU limit from shares = %v, want 512m", limits2.Cpu())
	}
}

func TestBuildResourceRequests(t *testing.T) {
	req := CreateRequest{CPUPercent: 250, CPUShares: 512, MemoryMB: 2048}
	requests := buildResourceRequests(req)
	if requests.Cpu().Cmp(resource.MustParse("2500m")) != 0 {
		t.Fatalf("CPU request = %v, want 2500m", requests.Cpu())
	}
	if requests.Memory().Cmp(resource.MustParse("2048Mi")) != 0 {
		t.Fatalf("memory request = %v, want 2048Mi", requests.Memory())
	}

	empty := buildResourceRequests(CreateRequest{})
	if len(empty) != 0 {
		t.Fatalf("expected empty resource requests, got %v", empty)
	}
}

func TestIsNotFound(t *testing.T) {
	if isNotFound(nil) {
		t.Fatal("nil should not be not-found")
	}
	if !isNotFound(errors.New("not found")) {
		t.Fatal("'not found' message should match")
	}
	if !isNotFound(errors.New(`"not found"`)) {
		t.Fatal("quoted 'not found' message should match")
	}
	if isNotFound(errors.New("other error")) {
		t.Fatal("other errors should not match")
	}
}

func TestSignalToNumber(t *testing.T) {
	tests := []struct {
		signal string
		num    int
		ok     bool
	}{
		{"SIGTERM", 15, true},
		{"SIGKILL", 9, true},
		{"SIGINT", 2, true},
		{"SIGQUIT", 3, true},
		{"SIGHUP", 1, true},
		{"SIGUSR1", 10, true},
		{"SIGUSR2", 12, true},
		{"sigterm", 15, true},
		{"SIGUNKNOWN", 0, false},
		{"", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.signal, func(t *testing.T) {
			num, ok := signalToNumber(tc.signal)
			if num != tc.num || ok != tc.ok {
				t.Fatalf("signalToNumber(%q) = (%d, %v), want (%d, %v)", tc.signal, num, ok, tc.num, tc.ok)
			}
		})
	}
}

func TestEncodeBase64(t *testing.T) {
	if got := encodeBase64("hello"); got != "aGVsbG8=" {
		t.Fatalf("encodeBase64 = %q", got)
	}
}

func TestKubernetesValidateCreate(t *testing.T) {
	r := &KubernetesRuntime{}
	tests := []struct {
		name string
		req  CreateRequest
		fail bool
	}{
		{"valid", CreateRequest{ServerID: "s1", Image: "nginx"}, false},
		{"missing server id", CreateRequest{Image: "nginx"}, true},
		{"missing image", CreateRequest{ServerID: "s1"}, true},
		{"negative memory", CreateRequest{ServerID: "s1", Image: "nginx", MemoryMB: -1}, true},
		{"negative swap", CreateRequest{ServerID: "s1", Image: "nginx", SwapMB: -1}, true},
		{"swap without memory", CreateRequest{ServerID: "s1", Image: "nginx", SwapMB: 512}, true},
		{"swap with memory", CreateRequest{ServerID: "s1", Image: "nginx", MemoryMB: 1024, SwapMB: 512}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := r.validateCreate(tc.req)
			if tc.fail && err == nil {
				t.Fatal("expected error")
			}
			if !tc.fail && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
