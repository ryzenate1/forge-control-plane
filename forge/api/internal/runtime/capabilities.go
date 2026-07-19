package runtime

type Capability string

const (
	CapabilityContainers     Capability = "containers"
	CapabilitySnapshots      Capability = "snapshots"
	CapabilityMigration      Capability = "migration"
	CapabilityLiveMigration  Capability = "live_migration"
	CapabilityCheckpoints    Capability = "checkpoints"
	CapabilityResourceLimits Capability = "resource_limits"
	CapabilityMicroVM        Capability = "microvm"
	CapabilityKubernetes     Capability = "kubernetes"
	CapabilityGPUPassthrough Capability = "gpu_passthrough"
	CapabilityVirtIOFS       Capability = "virtio_fs"
	CapabilitySeccomp        Capability = "seccomp"
	CapabilityNativeExec     Capability = "native_exec"
	CapabilityMultiArch      Capability = "multi_arch"
)

type Capabilities struct {
	Containers     bool `json:"containers"`
	Snapshots      bool `json:"snapshots"`
	Migration      bool `json:"migration"`
	LiveMigration  bool `json:"live_migration"`
	Checkpoints    bool `json:"checkpoints"`
	ResourceLimits bool `json:"resource_limits"`
	MicroVM        bool `json:"microvm"`
	Kubernetes     bool `json:"kubernetes"`
	GPUPassthrough bool `json:"gpu_passthrough"`
	VirtIOFS       bool `json:"virtio_fs"`
	Seccomp        bool `json:"seccomp"`
	NativeExec     bool `json:"native_exec"`
	MultiArch      bool `json:"multi_arch"`
}

func (c Capabilities) Union(other Capabilities) Capabilities {
	return Capabilities{
		Containers:     c.Containers || other.Containers,
		Snapshots:      c.Snapshots || other.Snapshots,
		Migration:      c.Migration || other.Migration,
		LiveMigration:  c.LiveMigration || other.LiveMigration,
		Checkpoints:    c.Checkpoints || other.Checkpoints,
		ResourceLimits: c.ResourceLimits || other.ResourceLimits,
		MicroVM:        c.MicroVM || other.MicroVM,
		Kubernetes:     c.Kubernetes || other.Kubernetes,
		GPUPassthrough: c.GPUPassthrough || other.GPUPassthrough,
		VirtIOFS:       c.VirtIOFS || other.VirtIOFS,
		Seccomp:        c.Seccomp || other.Seccomp,
		NativeExec:     c.NativeExec || other.NativeExec,
		MultiArch:      c.MultiArch || other.MultiArch,
	}
}

func (c Capabilities) Supports(capability Capability) bool {
	switch capability {
	case CapabilityContainers:
		return c.Containers
	case CapabilitySnapshots:
		return c.Snapshots
	case CapabilityMigration:
		return c.Migration
	case CapabilityLiveMigration:
		return c.LiveMigration
	case CapabilityCheckpoints:
		return c.Checkpoints
	case CapabilityResourceLimits:
		return c.ResourceLimits
	case CapabilityMicroVM:
		return c.MicroVM
	case CapabilityKubernetes:
		return c.Kubernetes
	case CapabilityGPUPassthrough:
		return c.GPUPassthrough
	case CapabilityVirtIOFS:
		return c.VirtIOFS
	case CapabilitySeccomp:
		return c.Seccomp
	case CapabilityNativeExec:
		return c.NativeExec
	case CapabilityMultiArch:
		return c.MultiArch
	default:
		return false
	}
}

func DockerCapabilities() Capabilities {
	return Capabilities{
		Containers:     true,
		ResourceLimits: true,
		NativeExec:     true,
	}
}

func ContainerdCapabilities() Capabilities {
	return Capabilities{
		Containers:     true,
		ResourceLimits: true,
		Snapshots:      true,
		NativeExec:     true,
		Seccomp:        true,
		MultiArch:      true,
	}
}

func PodmanCapabilities() Capabilities {
	return Capabilities{
		Containers:     true,
		ResourceLimits: true,
		Checkpoints:    true,
		NativeExec:     true,
		GPUPassthrough: true,
		Seccomp:        true,
	}
}

func FirecrackerCapabilities() Capabilities {
	return Capabilities{
		MicroVM:        true,
		ResourceLimits: true,
		Snapshots:      true,
		Seccomp:        true,
	}
}

func KubernetesCapabilities() Capabilities {
	return Capabilities{
		Containers:     true,
		ResourceLimits: true,
		Migration:      true,
		Kubernetes:     true,
		MultiArch:      true,
	}
}
