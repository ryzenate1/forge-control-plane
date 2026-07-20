package domain

import "errors"

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
	WorkloadID      string
	EnvironmentID   string
	Name            string
	Source          SourceKind
	Image           string
	RepositoryURL   string
	ComposeFile     string
	Deployment      Strategy
	HealthCheckPath string
	HealthCheckPort int
}

func (a Application) Validate() error {
	if a.EnvironmentID == "" || a.Name == "" {
		return errors.New("environment id and application name are required")
	}
	switch a.Source {
	case SourceImage:
		if a.Image == "" {
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
