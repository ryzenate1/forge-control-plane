package domain

import (
	"errors"
	"strings"
)

type SourceKind string

const (
	SourceImage   SourceKind = "image"
	SourceGit     SourceKind = "git"
	SourceCompose SourceKind = "compose"
)

type Strategy string

const (
	StrategyRolling   Strategy = "rolling"
	StrategyBlueGreen Strategy = "blue-green"
	StrategyRecreate  Strategy = "recreate"
)

type Application struct {
	WorkloadID      string            `json:"workloadId,omitempty"`
	EnvironmentID   string            `json:"environmentId"`
	NodeID          string            `json:"nodeId"`
	Name            string            `json:"name"`
	Source          SourceKind        `json:"source"`
	Image           string            `json:"image,omitempty"`
	RepositoryURL   string            `json:"repositoryUrl,omitempty"`
	ComposeFile     string            `json:"composeFile,omitempty"`
	Command         []string          `json:"command,omitempty"`
	Environment     map[string]string `json:"environment,omitempty"`
	MemoryMB        int64             `json:"memoryMb,omitempty"`
	CPUPercent      int64             `json:"cpuPercent,omitempty"`
	DiskMB          int64             `json:"diskMb,omitempty"`
	Deployment      Strategy          `json:"deployment"`
	HealthCheckPath string            `json:"healthCheckPath,omitempty"`
	HealthCheckPort int               `json:"healthCheckPort,omitempty"`
}

func (a Application) Validate() error {
	if strings.TrimSpace(a.EnvironmentID) == "" || strings.TrimSpace(a.NodeID) == "" || strings.TrimSpace(a.Name) == "" {
		return errors.New("environment id, node id, and application name are required")
	}
	switch a.Source {
	case SourceImage:
		if strings.TrimSpace(a.Image) == "" {
			return errors.New("image is required for image applications")
		}
	case SourceGit:
		if a.RepositoryURL == "" {
			return errors.New("repository URL is required for git applications")
		}
	case SourceCompose:
		if a.ComposeFile == "" {
			return errors.New("compose file is required for compose applications")
		}
	default:
		return errors.New("unsupported application source")
	}
	if a.MemoryMB < 0 || a.CPUPercent < 0 || a.DiskMB < 0 {
		return errors.New("application resources must not be negative")
	}
	if a.Deployment == "" {
		a.Deployment = StrategyRolling
	}
	switch a.Deployment {
	case StrategyRolling, StrategyBlueGreen, StrategyRecreate:
		return nil
	default:
		return errors.New("unsupported deployment strategy")
	}
}

// DeployableNow reports whether Forge has a complete runtime path for the
// source. Git, Dockerfile, and Compose stay represented in the domain but are
// rejected until their audited execution drivers are present.
func (a Application) DeployableNow() error {
	if a.Source != SourceImage {
		return errors.New("only image applications are deployable in this release")
	}
	return nil
}
