// Package compose validates and summarizes Compose manifests with the official
// Compose Specification loader. It deliberately does not execute a stack: a
// validated manifest becomes deployable only after the Beacon stack runtime
// has explicit network, volume, secret, and rollback semantics.
package compose

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	composetypes "github.com/compose-spec/compose-go/v2/types"
)

const maxManifestBytes = 512 * 1024

type Service struct {
	Name      string   `json:"name"`
	Image     string   `json:"image,omitempty"`
	Build     bool     `json:"build"`
	DependsOn []string `json:"dependsOn,omitempty"`
	Ports     int      `json:"ports"`
}

type Result struct {
	Name     string    `json:"name"`
	Services []Service `json:"services"`
	Networks []string  `json:"networks"`
	Volumes  []string  `json:"volumes"`
	Warnings []string  `json:"warnings,omitempty"`
}

func Validate(ctx context.Context, content string, environment map[string]string) (Result, error) {
	if strings.TrimSpace(content) == "" {
		return Result{}, errors.New("compose content is required")
	}
	if len(content) > maxManifestBytes {
		return Result{}, errors.New("compose content exceeds 512 KiB")
	}
	// Includes and env_file need a real source directory. They are rejected in
	// this API rather than resolved against an arbitrary panel filesystem path.
	if strings.Contains(content, "\ninclude:") || strings.HasPrefix(content, "include:") {
		return Result{}, errors.New("compose include is not supported for inline manifests")
	}
	details := composetypes.ConfigDetails{
		WorkingDir:  "/forge/compose",
		ConfigFiles: []composetypes.ConfigFile{{Filename: "compose.yaml", Content: []byte(content)}},
		Environment: composetypes.Mapping(environment),
	}
	project, err := loader.LoadWithContext(ctx, details, func(options *loader.Options) {
		options.ResolvePaths = false
		options.SkipInclude = true
	})
	if err != nil {
		return Result{}, err
	}
	result := Result{Name: project.Name, Networks: project.NetworkNames(), Volumes: project.VolumeNames()}
	for _, name := range project.ServiceNames() {
		service := project.Services[name]
		dependencies := make([]string, 0, len(service.DependsOn))
		for dependency := range service.DependsOn {
			dependencies = append(dependencies, dependency)
		}
		sort.Strings(dependencies)
		result.Services = append(result.Services, Service{Name: name, Image: service.Image, Build: service.Build != nil, DependsOn: dependencies, Ports: len(service.Ports)})
		if service.Privileged {
			result.Warnings = append(result.Warnings, "service "+name+" requests privileged mode and will require an explicit policy")
		}
		if service.NetworkMode == "host" {
			result.Warnings = append(result.Warnings, "service "+name+" requests host networking and will require an explicit policy")
		}
		if len(service.EnvFiles) > 0 {
			result.Warnings = append(result.Warnings, "service "+name+" references env_file; provide its values through Forge environment configuration before deployment")
		}
	}
	return result, nil
}
