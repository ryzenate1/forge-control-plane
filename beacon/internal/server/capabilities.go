package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	stdruntime "runtime"
	"sync"
	"time"

	"gamepanel/beacon/internal/runtime"
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
	var mem stdruntime.MemStats
	stdruntime.ReadMemStats(&mem)

	runtimeStatus := "unknown"
	runtimeAvailable := false
	runtimeProvider := ""
	if s.runtime != nil {
		runtimeStatus = s.dockerStatus()
		runtimeAvailable = true
		if pinger, ok := s.runtime.(runtime.Pinger); ok {
			if err := pinger.Ping(context.Background()); err == nil {
				runtimeStatus = "ok"
			} else {
				runtimeStatus = "error"
			}
		}
	}

	capabilities := []CapabilityEntry{
		{Type: CapabilityRuntime, Status: runtimeStatus},
		{Type: CapabilityBuild, Status: "ok"},
		{Type: CapabilityCompose, Status: "ok"},
		{Type: CapabilityStorage, Status: "ok"},
		{Type: CapabilityGateway, Status: "ok"},
		{Type: CapabilityDatabase, Status: "ok"},
	}

	buildInfo := &BuildCapability{
		DockerBuildEnabled: runtimeAvailable,
		NixpacksEnabled:    false,
	}

	var composeEnabled bool
	var stackCount int
	if s.composeStacks != nil {
		composeEnabled = true
	}
	composeInfo := &ComposeCapability{
		ComposeEnabled: composeEnabled,
		StackCount:     stackCount,
	}

	storageInfo := &StorageCapability{
		TransferEnabled: s.transferProtocol != nil,
	}
	if s.backups != nil {
		storageInfo.LocalBackups = true
		storageInfo.BackupAdapters = append(storageInfo.BackupAdapters, "local")
	}

	gatewayInfo := &GatewayCapability{
		SFTPEnabled:      true,
		WebSocketEnabled: true,
		ConsoleEnabled:   s.runtime != nil && s.consoles != nil,
	}

	databaseInfo := &DatabaseCapability{
		ProvisioningEnabled: runtimeAvailable,
		SupportedEngines:    []string{"mysql", "postgresql"},
	}

	return CapabilityReport{
		BeaconVersion: "beacon-dev",
		OS:            stdruntime.GOOS,
		Architecture:  stdruntime.GOARCH,
		CPUThreads:    stdruntime.NumCPU(),
		MemoryMB:      mem.Alloc / (1024 * 1024),
		UptimeSeconds: int64(time.Since(beaconStart).Seconds()),
		Capabilities:  capabilities,
		RuntimeInfo:   &RuntimeCapability{DockerAvailable: runtimeAvailable, DockerStatus: runtimeStatus, RuntimeProvider: runtimeProvider},
		BuildInfo:     buildInfo,
		ComposeInfo:   composeInfo,
		StorageInfo:   storageInfo,
		GatewayInfo:   gatewayInfo,
		DatabaseInfo:  databaseInfo,
		FetchedAt:     time.Now().UTC().Format(time.RFC3339),
	}
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

var (
	previousCapabilities *CapabilityReport
	capDeltaMu           sync.Mutex
)

func (s *Server) handlePostCapabilitiesHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var externalReport CapabilityReport
	if err := json.NewDecoder(r.Body).Decode(&externalReport); err != nil {
		writeError(w, http.StatusBadRequest, "invalid capabilities payload")
		return
	}
	externalReport.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	if s.panelClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.panelClient.SendCapabilityReport(ctx, externalReport); err != nil {
			log.Printf("[beacon] failed to send capability report: %v", err)
		}
	}
	delta := s.computeCapabilityDeltaFromExternal(externalReport)
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "delta": delta})
}

func (s *Server) handleGetCapabilitiesDelta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, s.collectCapabilities())
}

func (s *Server) computeCapabilityDeltaFromExternal(current CapabilityReport) CapabilityDelta {
	delta := CapabilityDelta{
		NodeID:    current.NodeID,
		FetchedAt: current.FetchedAt,
	}

	capDeltaMu.Lock()
	if previousCapabilities == nil {
		previousCapabilities = &current
		capDeltaMu.Unlock()
		delta.Added = current.Capabilities
		return delta
	}
	prev := *previousCapabilities
	capDeltaMu.Unlock()

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

	capDeltaMu.Lock()
	previousCapabilities = &current
	capDeltaMu.Unlock()

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
	minBeacon := "1.0.0"
	compatible := true
	var message string
	if beaconVersion == "" {
		compatible = false
		message = "beacon version is unknown"
	} else if apiVersion != "" && beaconVersion < minBeacon {
		compatible = false
		message = fmt.Sprintf("beacon version %s is below minimum %s", beaconVersion, minBeacon)
	}
	return VersionCompatibility{
		BeaconVersion:    beaconVersion,
		APIVersion:       apiVersion,
		Compatible:       compatible,
		MinBeaconVersion: minBeacon,
		Message:          message,
	}
}
