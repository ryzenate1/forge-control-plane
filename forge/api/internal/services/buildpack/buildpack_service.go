package buildpack

import (
	"context"
	"fmt"
	"strings"

	"gamepanel/forge/internal/store"
)

type LanguageInfo struct {
	Language   string            `json:"language"`
	Confidence float64           `json:"confidence"`
	Buildpacks []store.Buildpack `json:"buildpacks,omitempty"`
}

type DetectRequest struct {
	Files []string `json:"files"`
	Repo  string   `json:"repo,omitempty"`
}

type Service struct {
	store *store.Store
}

func NewService(st *store.Store) *Service {
	return &Service{store: st}
}

func (s *Service) DetectLanguage(files []string) LanguageInfo {
	scores := map[string]float64{}
	indicators := map[string]map[string]float64{
		"Node.js": {
			"package.json": 1.0, "yarn.lock": 0.9, "pnpm-lock.yaml": 0.9,
			"bun.lockb": 0.8, "package-lock.json": 0.8, "tsconfig.json": 0.6,
			".nvmrc": 0.4, "lerna.json": 0.5,
		},
		"Python": {
			"requirements.txt": 1.0, "Pipfile": 0.9, "pyproject.toml": 0.9,
			"setup.py": 0.9, "setup.cfg": 0.7, "Pipfile.lock": 0.8,
			"poetry.lock": 0.8, "MANIFEST.in": 0.3,
		},
		"Ruby": {
			"Gemfile": 1.0, "Gemfile.lock": 0.9, "Rakefile": 0.6,
			"config.ru": 0.7, ".ruby-version": 0.5, ".gemspec": 0.8,
		},
		"Java": {
			"pom.xml": 1.0, "build.gradle": 0.9, "build.gradle.kts": 0.9,
			"gradlew": 0.7, "settings.gradle": 0.6, "mvnw": 0.7,
		},
		"Go": {
			"go.mod": 1.0, "go.sum": 0.9, "Gopkg.toml": 0.8, "Gopkg.lock": 0.7,
		},
		"PHP": {
			"composer.json": 1.0, "composer.lock": 0.9, ".php": 0.3,
		},
		"Rust": {
			"Cargo.toml": 1.0, "Cargo.lock": 0.9, "rust-toolchain": 0.5,
			"rust-toolchain.toml": 0.5,
		},
		"Elixir": {
			"mix.exs": 1.0, "mix.lock": 0.9, ".credo.exs": 0.4,
		},
	}

	for _, f := range files {
		name := f
		if idx := strings.LastIndex(f, "/"); idx >= 0 {
			name = f[idx+1:]
		}
		for lang, indicators := range indicators {
			if score, ok := indicators[name]; ok {
				scores[lang] += score
			}
		}
		ext := ""
		if idx := strings.LastIndex(f, "."); idx >= 0 {
			ext = f[idx:]
		}
		switch ext {
		case ".js", ".jsx", ".ts", ".tsx", ".mjs":
			scores["Node.js"] += 0.2
		case ".py":
			scores["Python"] += 0.2
		case ".rb":
			scores["Ruby"] += 0.2
		case ".java", ".kt", ".kts":
			scores["Java"] += 0.2
		case ".go":
			scores["Go"] += 0.2
		case ".php":
			scores["PHP"] += 0.2
		case ".rs":
			scores["Rust"] += 0.2
		case ".ex", ".exs":
			scores["Elixir"] += 0.2
		}
	}

	lang := "Unknown"
	confidence := 0.0
	for l, s := range scores {
		if s > confidence {
			confidence = s
			lang = l
		}
	}

	if confidence > 1.0 {
		confidence = 1.0
	}

	return LanguageInfo{Language: lang, Confidence: confidence}
}

func (s *Service) DetectBuildpack(ctx context.Context, files []string) (*LanguageInfo, error) {
	langInfo := s.DetectLanguage(files)

	bps, err := s.store.ListBuildpacks(ctx)
	if err != nil {
		return &langInfo, nil
	}

	var compatible []store.Buildpack
	langToType := map[string]string{
		"Node.js": "herokuish", "Python": "herokuish", "Ruby": "herokuish",
		"Java": "herokuish", "Go": "herokuish", "PHP": "herokuish",
		"Rust": "herokuish", "Elixir": "herokuish",
	}

	preferredType := langToType[langInfo.Language]
	for _, bp := range bps {
		if preferredType != "" && bp.BuilderType == preferredType {
			compatible = append(compatible, bp)
		}
	}
	if len(compatible) == 0 {
		compatible = bps
	}

	langInfo.Buildpacks = compatible
	return &langInfo, nil
}

func (s *Service) TriggerBuild(ctx context.Context, serverID string, buildpackID *string) (*store.AppBuild, error) {
	req := store.CreateAppBuildRequest{BuildpackID: buildpackID}
	build, err := s.store.CreateAppBuild(ctx, serverID, req)
	if err != nil {
		return nil, fmt.Errorf("create build: %w", err)
	}

	go func() {
		build.Status = string(runningStatus)
		_ = s.store.UpdateAppBuildStatus(context.Background(), build.ID, string(runningStatus), build.BuildLog, build.ImageTag)

		imageTag := fmt.Sprintf("forge-%s-%s", serverID, build.ID[:8])
		log := simulateBuild(build.ID)

		_ = s.store.UpdateAppBuildStatus(context.Background(), build.ID, string(succeededStatus), log, imageTag)
	}()

	return build, nil
}

type BuildStatus string

const (
	pendingStatus   BuildStatus = "pending"
	runningStatus   BuildStatus = "running"
	succeededStatus BuildStatus = "succeeded"
	failedStatus    BuildStatus = "failed"
	canceledStatus  BuildStatus = "canceled"
)

func (s *Service) GetBuildStatus(ctx context.Context, buildID string) (*store.AppBuild, error) {
	return s.store.GetAppBuild(ctx, buildID)
}

func simulateBuild(buildID string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[buildpack] Starting build %s\n", buildID))
	sb.WriteString("[buildpack] Detected Node.js application\n")
	sb.WriteString("[buildpack] Installing dependencies...\n")
	sb.WriteString("[buildpack] Running build scripts...\n")
	sb.WriteString("[buildpack] Build complete\n")
	return sb.String()
}
