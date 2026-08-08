package buildpack

import (
	"context"
	"fmt"
	"strings"
	"time"

	buildsvc "gamepanel/forge/internal/services/build"
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
	store   *store.Store
	builder *buildsvc.Service
}

func NewService(st *store.Store, builders ...*buildsvc.Service) *Service {
	service := &Service{store: st}
	if len(builders) > 0 {
		service.builder = builders[0]
	}
	return service
}

func (s *Service) Start(ctx context.Context) error {
	if s.builder == nil {
		return fmt.Errorf("build executor is unavailable")
	}
	builds, err := s.store.ListActiveAppBuilds(ctx)
	if err != nil {
		return fmt.Errorf("list active app builds: %w", err)
	}
	for _, appBuild := range builds {
		records, err := s.builder.ListBuilds(ctx, appBuild.ID)
		if err != nil {
			return fmt.Errorf("list durable builds for %s: %w", appBuild.ID, err)
		}
		if len(records) == 0 {
			if err := s.store.UpdateAppBuildStatus(ctx, appBuild.ID, string(failedStatus), "build dispatch was interrupted before a durable build was created", ""); err != nil {
				return err
			}
			continue
		}
		record := records[0]
		for _, candidate := range records[1:] {
			if candidate.StartedAt.After(record.StartedAt) {
				record = candidate
			}
		}
		switch buildsvc.BuildStatus(record.Status) {
		case buildsvc.BuildSucceeded:
			if err := s.store.UpdateAppBuildStatus(ctx, appBuild.ID, string(succeededStatus), record.BuildLog, appBuild.ImageTag); err != nil {
				return err
			}
		case buildsvc.BuildFailed, buildsvc.BuildCanceled, buildsvc.BuildAbandoned:
			if err := s.store.UpdateAppBuildStatus(ctx, appBuild.ID, string(failedStatus), buildFailureLog(record), ""); err != nil {
				return err
			}
		default:
			go s.monitorBuild(appBuild.ID, record.ID, appBuild.ImageTag)
		}
	}
	return nil
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
	if s.builder == nil {
		return nil, fmt.Errorf("build executor is unavailable")
	}
	if buildpackID != nil {
		if _, err := s.store.GetBuildpack(ctx, *buildpackID); err != nil {
			return nil, fmt.Errorf("resolve buildpack: %w", err)
		}
	}
	nodeID, err := s.store.ServerNodeID(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("resolve build node: %w", err)
	}
	imageTag := "forge/server-" + serverID + ":build-" + build.ID
	record, err := s.builder.StartBuild(ctx, build.ID, buildsvc.BuilderNixpacks, buildsvc.BuildOptions{
		SourceDir:           "server:" + serverID,
		ImageName:           imageTag,
		Tags:                []string{imageTag},
		NodeID:              nodeID,
		BuildTimeout:        1800,
		BuildIdempotencyKey: "app-build:" + build.ID,
	}, nil)
	if err != nil {
		failLog := "[buildpack] dispatch failed: " + err.Error()
		_ = s.store.UpdateAppBuildStatus(ctx, build.ID, string(failedStatus), failLog, "")
		return nil, fmt.Errorf("dispatch build: %w", err)
	}
	build.Status = string(runningStatus)
	build.ImageTag = imageTag
	build.BuildLog = "[buildpack] dispatched as durable build " + record.ID + "\n"
	if err := s.store.UpdateAppBuildStatus(ctx, build.ID, build.Status, build.BuildLog, imageTag); err != nil {
		return nil, fmt.Errorf("persist dispatched build: %w", err)
	}
	go s.monitorBuild(build.ID, record.ID, imageTag)
	return build, nil
}

func (s *Service) monitorBuild(appBuildID, recordID, imageTag string) {
	ctx, cancel := context.WithTimeout(context.Background(), 31*time.Minute)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		record, err := s.builder.GetBuild(ctx, recordID)
		if err == nil {
			switch buildsvc.BuildStatus(record.Status) {
			case buildsvc.BuildSucceeded:
				_ = s.store.UpdateAppBuildStatus(ctx, appBuildID, string(succeededStatus), record.BuildLog, imageTag)
				return
			case buildsvc.BuildFailed, buildsvc.BuildCanceled, buildsvc.BuildAbandoned:
				_ = s.store.UpdateAppBuildStatus(ctx, appBuildID, string(failedStatus), buildFailureLog(record), "")
				return
			}
		}
		select {
		case <-ctx.Done():
			_ = s.store.UpdateAppBuildStatus(context.Background(), appBuildID, string(failedStatus), "build status monitor timed out", "")
			return
		case <-ticker.C:
		}
	}
}

func buildFailureLog(record *store.BuildRecord) string {
	log := record.BuildLog
	if record.ErrorMessage != "" {
		log += "\n" + record.ErrorMessage
	}
	return strings.TrimSpace(log)
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
