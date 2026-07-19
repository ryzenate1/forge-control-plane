package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types/mount"
)

func TestBuildContainerMountsUsesCanonicalRootBind(t *testing.T) {
	root := t.TempDir()
	customSource := t.TempDir()
	mounts, err := buildContainerMounts(root, []Mount{{
		Source:   customSource,
		Target:   "/mnt/custom",
		ReadOnly: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 2 {
		t.Fatalf("expected two mounts, got %d", len(mounts))
	}
	canonical := mounts[0]
	if canonical.Type != mount.TypeBind || canonical.Source != filepath.Clean(root) || canonical.Target != serverContainerRoot || canonical.ReadOnly {
		t.Fatalf("unexpected canonical mount: %+v", canonical)
	}
	if mounts[1].Type != mount.TypeBind || mounts[1].Source != customSource || mounts[1].Target != "/mnt/custom" || !mounts[1].ReadOnly {
		t.Fatalf("unexpected custom mount: %+v", mounts[1])
	}
}

func TestValidateRootDirRejectsInvalidRoots(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(filePath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing")

	for name, root := range map[string]string{
		"empty":    "",
		"relative": "relative/path",
		"missing":  missing,
		"file":     filePath,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateRootDir(root); err == nil {
				t.Fatalf("expected root %q to be rejected", root)
			}
		})
	}
}

func TestBuildContainerMountsRejectsCanonicalTargetOverride(t *testing.T) {
	for _, target := range []string{"/home/container", "/home/container/", "/home/./container"} {
		t.Run(target, func(t *testing.T) {
			_, err := buildContainerMounts(t.TempDir(), []Mount{{Source: t.TempDir(), Target: target}})
			if err == nil {
				t.Fatalf("expected target %q to be rejected", target)
			}
		})
	}
}

func TestBuildHostConfigDoesNotEnableDockerRestartPolicy(t *testing.T) {
	hostConfig := buildHostConfig(CreateRequest{}, nil, nil)
	if hostConfig.RestartPolicy.Name != "" {
		t.Fatalf("expected no Docker restart policy, got %q", hostConfig.RestartPolicy.Name)
	}
}

func TestDockerResourceSemantics(t *testing.T) {
	req := CreateRequest{MemoryMB: 1024, SwapMB: 512, CPUShares: 512, CPUPercent: 250, CPUSet: "0-1", IOWeight: 600, OOMKillDisabled: true, PIDLimit: 128}
	resources := buildResources(req)
	if resources.CPUPeriod != 100000 || resources.CPUQuota != 250000 {
		t.Fatalf("unexpected CPU quota/period: %d/%d", resources.CPUQuota, resources.CPUPeriod)
	}
	if resources.CPUShares != 512 {
		t.Fatalf("CPU shares were not preserved: %d", resources.CPUShares)
	}
	if resources.MemorySwap != 1536*1024*1024 {
		t.Fatalf("unexpected total memory+swap: %d", resources.MemorySwap)
	}
	if resources.BlkioWeight != 600 || resources.PidsLimit == nil || *resources.PidsLimit != 128 || resources.OomKillDisable == nil || !*resources.OomKillDisable {
		t.Fatalf("unexpected resources: %+v", resources)
	}
}

func TestValidateCreateRequestRejectsInvalidCombinations(t *testing.T) {
	base := CreateRequest{ServerID: "server", Image: "image", RootDir: t.TempDir(), NetworkName: "gamepanel"}
	invalid := []CreateRequest{base, base, base, base}
	invalid[0].SwapMB = 1
	invalid[1].CPUShares = 1
	invalid[2].IOWeight = 9
	invalid[3].Ports = []PortBinding{{HostIP: "not-an-ip", HostPort: 25565, Protocol: "tcp"}}
	for i, req := range invalid {
		if err := validateCreateRequest(req); err == nil {
			t.Fatalf("case %d should fail", i)
		}
	}
}

func TestDockerPortsPreserveTCPAndUDP(t *testing.T) {
	exposed, bindings, err := dockerPorts([]PortBinding{{HostIP: "127.0.0.1", HostPort: 25565, ContainerPort: 25565, Protocol: "tcp"}, {HostIP: "127.0.0.1", HostPort: 19132, ContainerPort: 19132, Protocol: "udp"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := exposed["25565/tcp"]; !ok {
		t.Fatal("missing TCP exposure")
	}
	if _, ok := exposed["19132/udp"]; !ok {
		t.Fatal("missing UDP exposure")
	}
	if len(bindings["19132/udp"]) != 1 {
		t.Fatal("missing UDP publication")
	}
}

func TestRegistryAuthIsScopedAndExcludedFromConfigHash(t *testing.T) {
	first := CreateRequest{ServerID: "s", Image: "private/image", NetworkName: "gamepanel", RegistryAuth: &RegistryAuth{Username: "u", Password: "one"}}
	second := first
	second.RegistryAuth = &RegistryAuth{Username: "u", Password: "two"}
	a, _ := createRequestHash(first)
	b, _ := createRequestHash(second)
	if a != b {
		t.Fatal("registry credentials must not influence persisted config hash")
	}
	options, err := imagePullOptions(first.RegistryAuth)
	if err != nil || options.RegistryAuth == "" {
		t.Fatalf("expected scoped pull auth: %v", err)
	}
}
