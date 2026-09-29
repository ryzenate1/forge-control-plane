package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	stdruntime "runtime"
	"strconv"
	"strings"
	"time"

	"gamepanel/beacon/internal/backup"
)

type CapabilityType string

const (
	CapabilityRuntime  CapabilityType = "runtime"
	CapabilityBuild    CapabilityType = "build"
	CapabilityCompose  CapabilityType = "compose"
	CapabilityStorage  CapabilityType = "storage"
	CapabilityGateway  CapabilityType = "gateway"
	CapabilityDatabase CapabilityType = "database"
)

type CapabilityReport struct {
	NodeID        string              `json:"nodeId,omitempty"`
	BeaconVersion string              `json:"beaconVersion"`
	OS            string              `json:"os"`
	Architecture  string              `json:"architecture"`
	CPUThreads    int                 `json:"cpuThreads"`
	MemoryMB      uint64              `json:"memoryMb"`
	DiskMB        uint64              `json:"diskMb"`
	UptimeSeconds int64               `json:"uptimeSeconds"`
	Capabilities  []CapabilityEntry   `json:"capabilities"`
	RuntimeInfo   *RuntimeCapability  `json:"runtimeInfo,omitempty"`
	BuildInfo     *BuildCapability    `json:"buildInfo,omitempty"`
	ComposeInfo   *ComposeCapability  `json:"composeInfo,omitempty"`
	StorageInfo   *StorageCapability  `json:"storageInfo,omitempty"`
	GatewayInfo   *GatewayCapability  `json:"gatewayInfo,omitempty"`
	DatabaseInfo  *DatabaseCapability `json:"databaseInfo,omitempty"`
	FetchedAt     string              `json:"fetchedAt"`
}

type CapabilityEntry struct {
	Type    CapabilityType `json:"type"`
	Version string         `json:"version,omitempty"`
	Status  string         `json:"status"`
}

type RuntimeCapability struct {
	DockerVersion   string `json:"dockerVersion,omitempty"`
	DockerAvailable bool   `json:"dockerAvailable"`
	DockerStatus    string `json:"dockerStatus"`
	RuntimeProvider string `json:"runtimeProvider,omitempty"`
}

type BuildCapability struct {
	DockerBuildEnabled bool `json:"dockerBuildEnabled"`
	NixpacksEnabled    bool `json:"nixpacksEnabled"`
}

type ComposeCapability struct {
	ComposeVersion string `json:"composeVersion,omitempty"`
	ComposeEnabled bool   `json:"composeEnabled"`
	StackCount     int    `json:"stackCount"`
}

type StorageCapability struct {
	BackupAdapters  []string `json:"backupAdapters"`
	LocalBackups    bool     `json:"localBackups"`
	S3Backups       bool     `json:"s3Backups"`
	TransferEnabled bool     `json:"transferEnabled"`
}

type GatewayCapability struct {
	SFTPEnabled      bool `json:"sftpEnabled"`
	WebSocketEnabled bool `json:"webSocketEnabled"`
	ConsoleEnabled   bool `json:"consoleEnabled"`
}

type DatabaseCapability struct {
	ProvisioningEnabled bool     `json:"provisioningEnabled"`
	SupportedEngines    []string `json:"supportedEngines,omitempty"`
}

func (s *Server) collectCapabilities() CapabilityReport {
	runtimeProvider := s.runtimeProvider()
	if runtimeProvider == "unknown" {
		runtimeProvider = ""
	}
	// A wired runtime is not necessarily a usable one: mock mode installs
	// UnavailableRuntime, which answers every workload call with an error. The
	// capability report has to say "not available" rather than advertise a
	// runtime that can build nothing.
	rtAvailable := runtimeAvailable(s.runtime)
	// dockerStatus pings the engine and reports "unknown" when the wired runtime
	// cannot be pinged at all. It used to fall back to a snapshot taken once at
	// construction, which kept reading "ok" for the lifetime of the process after
	// the engine behind it died; the panel placed workloads on that reading.
	runtimeStatus := s.dockerStatus()
	// runtimeUsable is the only claim of usability this report makes: a runtime
	// that is wired up but does not answer is not usable, and a runtime that
	// cannot be asked at all is unknown rather than working.
	runtimeUsable := rtAvailable && runtimeStatus == "ok"

	_, dockerBuildErr := exec.LookPath("docker")
	_, nixpacksErr := exec.LookPath("nixpacks")
	dockerBuildEnabled := runtimeUsable && dockerBuildErr == nil
	nixpacksEnabled := runtimeUsable && nixpacksErr == nil
	buildStatus := "error"
	if dockerBuildEnabled || nixpacksEnabled {
		buildStatus = "ok"
	}

	// "compose" is not the stack bookkeeping this process keeps in memory - any
	// node has that. It is the ability to run `docker compose`, which needs both
	// a usable engine and the docker CLI the compose subcommand rides on.
	composeEnabled := s.composeStacks != nil && dockerBuildEnabled
	var stackCount int
	if s.composeStacks != nil {
		stackCount = s.composeStacks.count()
	}
	transferEnabled := s.transferProtocol != nil
	backupsEnabled := s.backups != nil
	consoleEnabled := s.consoles != nil && runtimeUsable
	gatewayEnabled := s.consoles != nil || s.SFTPEnabled()
	databaseEnabled := runtimeUsable

	// Every status below is derived from something this process actually wired
	// up. They used to be a row of literal "ok" values, which told the panel the
	// node could compose, back up, serve databases and open consoles whether it
	// could or not.
	capabilities := []CapabilityEntry{
		{Type: CapabilityRuntime, Status: runtimeStatus},
		{Type: CapabilityBuild, Status: buildStatus},
		{Type: CapabilityCompose, Status: capabilityStatus(composeEnabled)},
		{Type: CapabilityStorage, Status: capabilityStatus(backupsEnabled || transferEnabled)},
		{Type: CapabilityGateway, Status: capabilityStatus(gatewayEnabled)},
		{Type: CapabilityDatabase, Status: capabilityStatus(databaseEnabled)},
	}

	buildInfo := &BuildCapability{
		DockerBuildEnabled: dockerBuildEnabled,
		NixpacksEnabled:    nixpacksEnabled,
	}

	composeInfo := &ComposeCapability{
		ComposeEnabled: composeEnabled,
		StackCount:     stackCount,
	}

	storageInfo := &StorageCapability{
		TransferEnabled: transferEnabled,
	}
	if backupsEnabled {
		// The adapter names itself. Reporting "local" unconditionally told the
		// panel that backups landed on this node's disk when the operator
		// configured the S3 adapter, and S3Backups never turned true.
		switch adapter := s.backups.Type(); adapter {
		case backup.LocalAdapter:
			storageInfo.LocalBackups = true
			storageInfo.BackupAdapters = append(storageInfo.BackupAdapters, string(adapter))
		case backup.S3Adapter:
			storageInfo.S3Backups = true
			storageInfo.BackupAdapters = append(storageInfo.BackupAdapters, string(adapter))
		default:
			// An adapter this build does not recognise is reported as itself
			// rather than as one of the two known kinds.
			if name := strings.TrimSpace(string(adapter)); name != "" {
				storageInfo.BackupAdapters = append(storageInfo.BackupAdapters, name)
			}
		}
	}

	gatewayInfo := &GatewayCapability{
		SFTPEnabled:      s.SFTPEnabled(),
		WebSocketEnabled: gatewayEnabled,
		ConsoleEnabled:   consoleEnabled,
	}

	databaseInfo := &DatabaseCapability{
		ProvisioningEnabled: databaseEnabled,
	}
	if databaseEnabled {
		databaseInfo.SupportedEngines = provisionableDatabaseEngines()
	}

	return CapabilityReport{
		BeaconVersion: s.version,
		OS:            stdruntime.GOOS,
		Architecture:  stdruntime.GOARCH,
		CPUThreads:    stdruntime.NumCPU(),
		MemoryMB:      totalSystemMemoryMB(),
		// Disk is probed from the data directory's filesystem. A report that
		// omitted it left the panel reading a node with zero disk capacity.
		DiskMB:        totalDiskMB(s.dataDir),
		UptimeSeconds: int64(time.Since(s.started).Seconds()),
		Capabilities:  capabilities,
		// Only an engine that answers a live ping is advertised as available.
		// Reporting "wired, therefore available" told the control plane a node
		// could run workloads after its socket went away, and it kept placing
		// servers on a daemon that refuses every create. The capability-gated
		// placeholder runtimes report nothing but "unknown" for the same reason.
		RuntimeInfo:  &RuntimeCapability{DockerAvailable: runtimeUsable, DockerStatus: runtimeStatus, RuntimeProvider: runtimeProvider},
		BuildInfo:    buildInfo,
		ComposeInfo:  composeInfo,
		StorageInfo:  storageInfo,
		GatewayInfo:  gatewayInfo,
		DatabaseInfo: databaseInfo,
		FetchedAt:    time.Now().UTC().Format(time.RFC3339),
	}
}

// provisionableDatabaseEngines reports exactly the engines this daemon's
// provisioning path accepts, filtered through the same guard the provision
// handler uses. It used to be a hardcoded pair, which both omitted the engines
// the node really can serve and would have kept advertising them if support
// were dropped.
func provisionableDatabaseEngines() []string {
	candidates := []string{"postgresql", "mysql", "mariadb", "mongodb", "redis"}
	engines := make([]string, 0, len(candidates))
	for _, engine := range candidates {
		if validDatabaseEngine(engine) {
			engines = append(engines, engine)
		}
	}
	return engines
}

// capabilityStatus names a subsystem's real wiring instead of a hardcoded "ok".
func capabilityStatus(enabled bool) string {
	if enabled {
		return "ok"
	}
	return "unavailable"
}

// systemCapabilities lists what this daemon can actually do, read from the same
// wiring as the capability report. File and stats routes are served by this
// process whenever it is up, so they are always present; everything else has to
// earn its place. A node with no usable runtime no longer advertises docker,
// and one with no backups adapter no longer advertises backups.
func (s *Server) systemCapabilities() []string {
	report := s.collectCapabilities()
	capabilities := []string{"files", "stats"}
	// The provider this process actually resolved, or nothing. Falling back to a
	// literal "docker" here advertised a Docker node behind a Podman, containerd
	// or Kubernetes engine, which is the phantom-provider bug in the report the
	// panel places with.
	if report.RuntimeInfo != nil && report.RuntimeInfo.DockerAvailable {
		if provider := report.RuntimeInfo.RuntimeProvider; provider != "" {
			capabilities = append(capabilities, provider)
		}
	}
	if report.BuildInfo != nil && (report.BuildInfo.DockerBuildEnabled || report.BuildInfo.NixpacksEnabled) {
		capabilities = append(capabilities, "build")
	}
	if report.ComposeInfo != nil && report.ComposeInfo.ComposeEnabled {
		capabilities = append(capabilities, "compose")
	}
	if report.StorageInfo != nil {
		if len(report.StorageInfo.BackupAdapters) > 0 {
			capabilities = append(capabilities, "backups")
		}
		if report.StorageInfo.TransferEnabled {
			capabilities = append(capabilities, "transfers")
		}
	}
	if report.GatewayInfo != nil {
		if report.GatewayInfo.SFTPEnabled {
			capabilities = append(capabilities, "sftp")
		}
		if report.GatewayInfo.ConsoleEnabled {
			capabilities = append(capabilities, "console")
		}
	}
	if report.DatabaseInfo != nil && report.DatabaseInfo.ProvisioningEnabled {
		capabilities = append(capabilities, "database")
	}
	return capabilities
}

func (s *Server) handleGetCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	report := s.collectCapabilities()
	writeJSON(w, http.StatusOK, report)
}

type CapabilityDelta struct {
	NodeID    string            `json:"nodeId,omitempty"`
	Added     []CapabilityEntry `json:"added,omitempty"`
	Removed   []CapabilityEntry `json:"removed,omitempty"`
	Changed   []CapabilityEntry `json:"changed,omitempty"`
	Unchanged []CapabilityEntry `json:"unchanged,omitempty"`
	FetchedAt string            `json:"fetchedAt"`
}

func (s *Server) handlePostCapabilitiesHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var externalReport CapabilityReport
	if err := decodeJSONBody(w, r, capabilityReportLimit, &externalReport); err != nil {
		// decodeJSONBody has already written the response, and it distinguishes
		// an oversized payload (413) from a malformed one (400) instead of
		// reporting both as bad JSON.
		return
	}
	externalReport.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	forwarded := false
	if s.panelClient != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := s.panelClient.SendCapabilityReport(ctx, externalReport); err != nil {
			// The whole point of this endpoint is to get the report onto the
			// panel. Acknowledging it locally while the forward failed told the
			// caller a capability update landed when it did not, so the failure
			// is returned as a bad gateway instead of being logged and dropped.
			log.Printf("[beacon] failed to send capability report: %v", err)
			writeError(w, http.StatusBadGateway, "capability report not delivered to panel: "+err.Error())
			return
		}
		forwarded = true
	}
	delta := s.computeCapabilityDeltaFromExternal(externalReport)
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "forwarded": forwarded, "delta": delta})
}

// capabilityReportLimit bounds an inbound capability report. The report is a
// fixed-shape summary, so anything past this is either abusive or corrupt.
const capabilityReportLimit = int64(1 << 20)

// handleGetCapabilitiesDelta reports how this node's capabilities moved since
// the last capability observation, which is what the route's name promises.
// It used to answer with the full report, so a caller comparing two polls
// received an object with no added/removed/changed keys at all and had to
// assume nothing had changed. Both capability routes share one baseline: they
// describe the same node, and the delta is always "since anyone last looked".
func (s *Server) handleGetCapabilitiesDelta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, s.computeCapabilityDeltaFromExternal(s.collectCapabilities()))
}

func (s *Server) computeCapabilityDeltaFromExternal(current CapabilityReport) CapabilityDelta {
	delta := CapabilityDelta{
		NodeID:    current.NodeID,
		FetchedAt: current.FetchedAt,
	}

	s.capabilitiesMu.Lock()
	if s.previousCaps == nil {
		s.previousCaps = &current
		s.capabilitiesMu.Unlock()
		delta.Added = current.Capabilities
		return delta
	}
	prev := *s.previousCaps
	s.capabilitiesMu.Unlock()

	prevMap := make(map[CapabilityType]CapabilityEntry)
	for _, c := range prev.Capabilities {
		prevMap[c.Type] = c
	}
	curMap := make(map[CapabilityType]CapabilityEntry)
	for _, c := range current.Capabilities {
		curMap[c.Type] = c
	}
	for _, c := range current.Capabilities {
		if old, ok := prevMap[c.Type]; !ok {
			delta.Added = append(delta.Added, c)
		} else if old.Status != c.Status || old.Version != c.Version {
			delta.Changed = append(delta.Changed, c)
		} else {
			delta.Unchanged = append(delta.Unchanged, c)
		}
	}
	for _, c := range prev.Capabilities {
		if _, ok := curMap[c.Type]; !ok {
			delta.Removed = append(delta.Removed, c)
		}
	}

	s.capabilitiesMu.Lock()
	s.previousCaps = &current
	s.capabilitiesMu.Unlock()

	return delta
}

type VersionCompatibility struct {
	BeaconVersion    string `json:"beaconVersion"`
	APIVersion       string `json:"apiVersion"`
	Compatible       bool   `json:"compatible"`
	MinBeaconVersion string `json:"minBeaconVersion"`
	Message          string `json:"message,omitempty"`
}

func CheckVersionCompatibility(beaconVersion, apiVersion string) VersionCompatibility {
	minBeacon := "0.1.0"
	compatible := false
	var message string
	if beaconVersion == "" {
		message = "beacon version is unknown"
	} else if comparison, err := compareReleaseVersions(beaconVersion, minBeacon); err != nil {
		message = fmt.Sprintf("beacon version %q is invalid", beaconVersion)
	} else if comparison < 0 {
		message = fmt.Sprintf("beacon version %s is below minimum %s", beaconVersion, minBeacon)
	} else {
		compatible = true
	}
	return VersionCompatibility{
		BeaconVersion:    beaconVersion,
		APIVersion:       apiVersion,
		Compatible:       compatible,
		MinBeaconVersion: minBeacon,
		Message:          message,
	}
}

func compareReleaseVersions(left, right string) (int, error) {
	parse := func(value string) ([3]int, error) {
		var result [3]int
		value = strings.TrimPrefix(strings.TrimSpace(value), "v")
		value = strings.SplitN(value, "+", 2)[0]
		value = strings.SplitN(value, "-", 2)[0]
		parts := strings.Split(value, ".")
		if len(parts) != 3 {
			return result, errors.New("version must contain three numeric components")
		}
		for index, part := range parts {
			number, err := strconv.Atoi(part)
			if err != nil || number < 0 || part == "" || len(part) > 1 && part[0] == '0' {
				return result, errors.New("invalid version component")
			}
			result[index] = number
		}
		return result, nil
	}
	a, err := parse(left)
	if err != nil {
		return 0, err
	}
	b, err := parse(right)
	if err != nil {
		return 0, err
	}
	for index := range a {
		if a[index] < b[index] {
			return -1, nil
		}
		if a[index] > b[index] {
			return 1, nil
		}
	}
	return 0, nil
}
