package build

import (
	"os"
	"syscall"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"
)

// BuilderType selects the build strategy executed for a source.
type BuilderType string

const (
	BuilderDockerfile BuilderType = "dockerfile"
	BuilderNixpacks   BuilderType = "nixpacks"
)

// BuildStatus mirrors the values persisted in the builds table.
type BuildStatus string

const (
	BuildRunning   BuildStatus = "running"
	BuildSucceeded BuildStatus = "succeeded"
	BuildFailed    BuildStatus = "failed"
	BuildCanceled  BuildStatus = "canceled"
	BuildAbandoned BuildStatus = "abandoned"
)

// BuildStage tracks progress within a running build.
type BuildStage string

const (
	BuildStageQueued    BuildStage = "queued"
	BuildStageCloning   BuildStage = "cloning"
	BuildStageBuilding  BuildStage = "building"
	BuildStageBuilt     BuildStage = "built"
	BuildStagePushing   BuildStage = "pushing"
	BuildStageVerifying BuildStage = "verifying_digest"
)

type BuildLogEntry struct {
	BuildID   string    `json:"buildId"`
	Line      string    `json:"line"`
	Timestamp time.Time `json:"timestamp"`
}

type BuildOptions struct {
	SourceDir           string
	Dockerfile          string
	ImageName           string
	BuildArgs           []string
	Labels              []string
	Tags                []string
	NoCache             bool
	CacheFrom           []string
	CacheTo             []string
	Platform            string
	NodeID              string
	Registry            string
	BuildTimeout        int
	BuildIdempotencyKey string
	RegistryAuth        *daemon.RegistryAuth
	NixpacksPlan        map[string]any
	CommitSHA           string
	CommitRef           string
}

var terminalStatuses = map[BuildStatus]bool{
	BuildSucceeded: true,
	BuildFailed:    true,
	BuildCanceled:  true,
	BuildAbandoned: true,
}

// IsTerminal reports whether a build has reached a final state.
func IsTerminal(status BuildStatus) bool {
	return terminalStatuses[status]
}

// DisplayBuildStatus reports a running build whose local process has died as
// abandoned, so callers never wait on a build that cannot finish.
func DisplayBuildStatus(record *store.BuildRecord) BuildStatus {
	if record == nil {
		return BuildFailed
	}
	status := BuildStatus(record.Status)
	if status == BuildRunning && record.PID != nil && !pidAlive(*record.PID) {
		return BuildAbandoned
	}
	return status
}

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
