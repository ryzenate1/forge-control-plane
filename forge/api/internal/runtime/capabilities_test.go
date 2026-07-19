package runtime

import (
	"testing"
)

func TestCapabilitiesUnion(t *testing.T) {
	a := Capabilities{Containers: true, Snapshots: true}
	b := Capabilities{Containers: false, Migration: true, ResourceLimits: true}
	got := a.Union(b)
	if !got.Containers {
		t.Error("Union should have Containers=true (from a)")
	}
	if !got.Snapshots {
		t.Error("Union should have Snapshots=true (from a)")
	}
	if !got.Migration {
		t.Error("Union should have Migration=true (from b)")
	}
	if !got.ResourceLimits {
		t.Error("Union should have ResourceLimits=true (from b)")
	}
	if got.MicroVM {
		t.Error("Union should have MicroVM=false (neither set)")
	}
}

func TestCapabilitiesUnionAllBools(t *testing.T) {
	all := Capabilities{
		Containers:     true,
		Snapshots:      true,
		Migration:      true,
		LiveMigration:  true,
		Checkpoints:    true,
		ResourceLimits: true,
		MicroVM:        true,
		Kubernetes:     true,
		GPUPassthrough: true,
		VirtIOFS:       true,
		Seccomp:        true,
		NativeExec:     true,
		MultiArch:      true,
	}
	none := Capabilities{}
	got := all.Union(none)
	if got != all {
		t.Errorf("Union with zero-value should equal the original: %+v", got)
	}
}

func TestCapabilitiesUnionClosedUnderIdempotent(t *testing.T) {
	x := Capabilities{Containers: true, Seccomp: true}
	got := x.Union(x)
	if got != x {
		t.Errorf("Union with itself should be equal: %+v", got)
	}
}

func TestCapabilitiesSupportsContainers(t *testing.T) {
	c := Capabilities{Containers: true}
	if !c.Supports(CapabilityContainers) {
		t.Error("Supports(Containers) should be true")
	}
	c2 := Capabilities{}
	if c2.Supports(CapabilityContainers) {
		t.Error("Supports(Containers) should be false")
	}
}

func TestCapabilitiesSupportsSnapshots(t *testing.T) {
	c := Capabilities{Snapshots: true}
	if !c.Supports(CapabilitySnapshots) {
		t.Error("Supports(Snapshots) should be true")
	}
}

func TestCapabilitiesSupportsMigration(t *testing.T) {
	c := Capabilities{Migration: true}
	if !c.Supports(CapabilityMigration) {
		t.Error("Supports(Migration) should be true")
	}
}

func TestCapabilitiesSupportsLiveMigration(t *testing.T) {
	c := Capabilities{LiveMigration: true}
	if !c.Supports(CapabilityLiveMigration) {
		t.Error("Supports(LiveMigration) should be true")
	}
}

func TestCapabilitiesSupportsCheckpoints(t *testing.T) {
	c := Capabilities{Checkpoints: true}
	if !c.Supports(CapabilityCheckpoints) {
		t.Error("Supports(Checkpoints) should be true")
	}
}

func TestCapabilitiesSupportsResourceLimits(t *testing.T) {
	c := Capabilities{ResourceLimits: true}
	if !c.Supports(CapabilityResourceLimits) {
		t.Error("Supports(ResourceLimits) should be true")
	}
}

func TestCapabilitiesSupportsMicroVM(t *testing.T) {
	c := Capabilities{MicroVM: true}
	if !c.Supports(CapabilityMicroVM) {
		t.Error("Supports(MicroVM) should be true")
	}
}

func TestCapabilitiesSupportsKubernetes(t *testing.T) {
	c := Capabilities{Kubernetes: true}
	if !c.Supports(CapabilityKubernetes) {
		t.Error("Supports(Kubernetes) should be true")
	}
}

func TestCapabilitiesSupportsGPUPassthrough(t *testing.T) {
	c := Capabilities{GPUPassthrough: true}
	if !c.Supports(CapabilityGPUPassthrough) {
		t.Error("Supports(GPUPassthrough) should be true")
	}
}

func TestCapabilitiesSupportsVirtIOFS(t *testing.T) {
	c := Capabilities{VirtIOFS: true}
	if !c.Supports(CapabilityVirtIOFS) {
		t.Error("Supports(VirtIOFS) should be true")
	}
}

func TestCapabilitiesSupportsSeccomp(t *testing.T) {
	c := Capabilities{Seccomp: true}
	if !c.Supports(CapabilitySeccomp) {
		t.Error("Supports(Seccomp) should be true")
	}
}

func TestCapabilitiesSupportsNativeExec(t *testing.T) {
	c := Capabilities{NativeExec: true}
	if !c.Supports(CapabilityNativeExec) {
		t.Error("Supports(NativeExec) should be true")
	}
}

func TestCapabilitiesSupportsMultiArch(t *testing.T) {
	c := Capabilities{MultiArch: true}
	if !c.Supports(CapabilityMultiArch) {
		t.Error("Supports(MultiArch) should be true")
	}
}

func TestCapabilitiesSupportsUnknownReturnsFalse(t *testing.T) {
	c := Capabilities{Containers: true}
	if c.Supports(Capability("unknown_capability")) {
		t.Error("Supports(unknown) should return false")
	}
}

func TestDockerCapabilities(t *testing.T) {
	c := DockerCapabilities()
	if !c.Containers || !c.ResourceLimits || !c.NativeExec {
		t.Error("Docker capabilities should have Containers, ResourceLimits, NativeExec")
	}
	if c.Snapshots || c.Migration || c.MicroVM || c.Kubernetes {
		t.Error("Docker capabilities should not have uncommon fields")
	}
}

func TestContainerdCapabilities(t *testing.T) {
	c := ContainerdCapabilities()
	if !c.Containers || !c.ResourceLimits || !c.Snapshots || !c.NativeExec || !c.Seccomp || !c.MultiArch {
		t.Error("Containerd capabilities should have Containers, ResourceLimits, Snapshots, NativeExec, Seccomp, MultiArch")
	}
}

func TestPodmanCapabilities(t *testing.T) {
	c := PodmanCapabilities()
	if !c.Containers || !c.ResourceLimits || !c.Checkpoints || !c.NativeExec || !c.GPUPassthrough || !c.Seccomp {
		t.Error("Podman capabilities should have Containers, ResourceLimits, Checkpoints, NativeExec, GPUPassthrough, Seccomp")
	}
}

func TestFirecrackerCapabilities(t *testing.T) {
	c := FirecrackerCapabilities()
	if !c.MicroVM || !c.ResourceLimits || !c.Snapshots || !c.Seccomp {
		t.Error("Firecracker capabilities should have MicroVM, ResourceLimits, Snapshots, Seccomp")
	}
}

func TestKubernetesCapabilities(t *testing.T) {
	c := KubernetesCapabilities()
	if !c.Containers || !c.ResourceLimits || !c.Migration || !c.Kubernetes || !c.MultiArch {
		t.Error("Kubernetes capabilities should have Containers, ResourceLimits, Migration, Kubernetes, MultiArch")
	}
}
