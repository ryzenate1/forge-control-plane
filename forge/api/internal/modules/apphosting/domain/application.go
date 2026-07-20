package domain

import (
	"errors"
	"net/url"
	"path"
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
	Branch          string            `json:"branch,omitempty"`
	BaseDirectory   string            `json:"baseDirectory,omitempty"`
	DockerfilePath  string            `json:"dockerfilePath,omitempty"`
	BuildArgs       map[string]string `json:"buildArgs,omitempty"`
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
		if err := validateGitSource(a.RepositoryURL, a.BaseDirectory, a.DockerfilePath); err != nil {
			return err
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
// source. Image and public Git/Dockerfile sources have execution drivers;
// Compose remains a validated import until stack execution is available.
func (a Application) DeployableNow() error {
	switch a.Source {
	case SourceImage, SourceGit:
		return nil
	case SourceCompose:
		return errors.New("compose applications must be imported and deployed as a compose project")
	default:
		return errors.New("unsupported application source")
	}
}

func validateGitSource(repositoryURL, baseDirectory, dockerfilePath string) error {
	parsed, err := url.Parse(strings.TrimSpace(repositoryURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("git applications require an HTTPS repository URL without embedded credentials")
	}
	if err := validateRelativePath(baseDirectory, "base directory"); err != nil {
		return err
	}
	return validateRelativePath(dockerfilePath, "dockerfile path")
}

func validateRelativePath(value, field string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if path.IsAbs(value) || value == ".." || strings.HasPrefix(value, "../") || strings.Contains(value, "\\") {
		return errors.New(field + " must be a relative path inside the repository")
	}
	return nil
}
