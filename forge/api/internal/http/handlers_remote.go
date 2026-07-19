package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/services/noderegistry"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

var (
	errHMACMismatch   = errors.New("HMAC signature mismatch")
	errHMACTimestamp  = errors.New("missing or invalid HMAC timestamp")
	errHMACBodyRead   = errors.New("failed to read request body for HMAC verification")
	errHMACMethod     = errors.New("HMAC verification requires HTTP method")
	errHMACRequestURI = errors.New("HMAC verification requires request URI")
)

func requestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// verifyRemoteHMAC checks X-Panel-Signature and X-Panel-Timestamp when present.
// When HMAC headers are absent (legacy daemon), authentication falls through to
// the bearer token check.  When present, both checks must pass.
func verifyRemoteHMAC(c *fiber.Ctx, nodeToken string) error {
	signature := c.Get("X-Panel-Signature")
	timestamp := c.Get("X-Panel-Timestamp")
	if signature == "" && timestamp == "" {
		return nil // legacy daemon – no HMAC headers
	}
	if timestamp == "" {
		return errHMACTimestamp
	}
	method := c.Method()
	if method == "" {
		return errHMACMethod
	}
	requestURI := c.Request().URI().RequestURI()
	if len(requestURI) == 0 {
		return errHMACRequestURI
	}
	var body []byte
	if c.Request().Body() != nil {
		var err error
		body, err = io.ReadAll(c.Request().BodyStream())
		if err != nil {
			return errHMACBodyRead
		}
		// Re-arm the body for downstream handlers.
		c.Request().SetBody(body)
	}
	expected := signHMAC(nodeToken, string(method), string(requestURI), timestamp, body)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return errHMACMismatch
	}
	return nil
}

func signHMAC(token, method, requestURI, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(method))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(requestURI))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func remoteNodeMiddleware(cfg Config, nodeRegistry *noderegistry.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		header := c.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return fiber.NewError(fiber.StatusUnauthorized, "missing daemon bearer token")
		}
		bearerToken := strings.TrimPrefix(header, "Bearer ")
		ctx, cancel := requestContext()
		defer cancel()
		node, err := nodeRegistry.AuthenticateRemoteNode(ctx, bearerToken)
		if err != nil {
			return fiber.NewError(fiber.StatusForbidden, "invalid daemon bearer token")
		}
		// HMAC verification – backward-compatible with legacy daemons.
		// Uses the bearer token itself as the HMAC key so the panel does not
		// need to store the raw daemon secret in the response Node struct.
		if err := verifyRemoteHMAC(c, bearerToken); err != nil {
			return fiber.NewError(fiber.StatusForbidden, err.Error())
		}
		c.Locals("remoteNode", node)
		return c.Next()
	}
}

func remoteServerPayload(target store.ServerProvisionTarget) fiber.Map {
	return remoteServerPayloadDTO(target.ToDTO())
}

func remoteServerPayloadDTO(target store.ServerProvisionTargetDTO) fiber.Map {
	settings := remoteServerSettingsDTO(target)
	return fiber.Map{
		"uuid":                  target.ServerID,
		"settings":              settings,
		"process_configuration": remoteProcessConfigurationDTO(target),
		"suspended":             target.Suspended,
		"is_installing":         target.Status == "installing",
		"installed":             target.Installed,
		"status":                target.Status,
	}
}

func remoteServerSettings(target store.ServerProvisionTarget) fiber.Map {
	return remoteServerSettingsDTO(target.ToDTO())
}

func remoteServerSettingsDTO(target store.ServerProvisionTargetDTO) fiber.Map {
	environment := fiber.Map{
		"STARTUP":           target.StartupCommand,
		"P_SERVER_UUID":     target.ServerID,
		"SERVER_MEMORY":     fmt.Sprintf("%d", target.MemoryMB),
		"SERVER_IP":         target.AllocationIP,
		"SERVER_PORT":       fmt.Sprintf("%d", target.AllocationPort),
		"P_SERVER_LOCATION": "local",
	}
	for key, value := range target.Environment {
		environment[key] = value
	}
	mappings := fiber.Map{}
	if target.AllocationIP != "" && target.AllocationPort > 0 {
		mappings[target.AllocationIP] = []int{target.AllocationPort}
	}
	denylist := []string{}
	if strings.TrimSpace(target.FileDenylist) != "" {
		_ = json.Unmarshal([]byte(target.FileDenylist), &denylist)
	}
	return fiber.Map{
		"uuid": target.ServerID,
		"meta": fiber.Map{
			"name":        target.Name,
			"description": "",
		},
		"suspended":        target.Suspended,
		"environment":      environment,
		"invocation":       target.StartupCommand,
		"skip_egg_scripts": false,
		"build": fiber.Map{
			"memory_limit": target.MemoryMB,
			"swap":         target.SwapMB,
			"io_weight":    target.IOWeight,
			"cpu_limit":    firstNonZeroInt64(target.CPULimit, target.CPUShares),
			"threads":      target.Threads,
			"disk_space":   target.DiskMB,
			"oom_disabled": target.OOMDisabled,
		},
		"container": fiber.Map{
			"image":            target.Image,
			"oom_disabled":     false,
			"requires_rebuild": false,
		},
		"allocations": fiber.Map{
			"force_outgoing_ip": false,
			"default": fiber.Map{
				"ip":   target.AllocationIP,
				"port": target.AllocationPort,
			},
			"mappings": mappings,
			"ports":    target.Allocations,
		},
	}
}

func remoteProcessConfiguration(target store.ServerProvisionTarget) fiber.Map {
	return remoteProcessConfigurationDTO(target.ToDTO())
}

func remoteProcessConfigurationDTO(target store.ServerProvisionTargetDTO) fiber.Map {
	var stopType, stopValue string
	if target.ConfigJSON != "" {
		var parsed struct {
			Startup struct {
				Done []string `json:"done"`
			} `json:"startup"`
			Stop struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"stop"`
		}
		if err := json.Unmarshal([]byte(target.ConfigJSON), &parsed); err == nil {
			stopType = parsed.Stop.Type
			stopValue = parsed.Stop.Value
		}
	}
	if stopType == "" {
		stopType = "command"
	}
	if stopValue == "" {
		stopValue = "^C"
	}
	var startup fiber.Map
	if target.ConfigJSON != "" {
		var parsed struct {
			Startup fiber.Map `json:"startup"`
		}
		if err := json.Unmarshal([]byte(target.ConfigJSON), &parsed); err == nil {
			startup = parsed.Startup
		}
	}
	if startup == nil {
		startup = fiber.Map{
			"done":       []string{"*"},
			"strip_ansi": false,
		}
	}
	return fiber.Map{
		"startup": startup,
		"stop":    fiber.Map{"type": stopType, "value": stopValue},
	}
}

func buildDaemonServerConfiguration(target store.ServerProvisionTargetDTO) daemon.ServerConfiguration {
	config := map[string]any{}
	if strings.TrimSpace(target.ConfigJSON) != "" {
		_ = json.Unmarshal([]byte(target.ConfigJSON), &config)
	}
	var denylist []string
	if strings.TrimSpace(target.FileDenylist) != "" {
		_ = json.Unmarshal([]byte(target.FileDenylist), &denylist)
	}
	environment := map[string]string{
		"SERVER_MEMORY": fmt.Sprintf("%d", target.MemoryMB),
		"SERVER_IP":     target.AllocationIP,
		"SERVER_PORT":   fmt.Sprintf("%d", target.AllocationPort),
	}
	for key, value := range target.Environment {
		environment[key] = value
	}
	allocations := map[string]any{
		"default": map[string]any{
			"ip":    target.AllocationIP,
			"port":  target.AllocationPort,
			"notes": "",
		},
		"mappings": map[string][]int{},
	}
	mappings := map[string][]int{}
	for _, allocation := range target.Allocations {
		mappings[allocation.IP] = append(mappings[allocation.IP], allocation.Port)
	}
	allocations["mappings"] = mappings
	allocations["ports"] = target.Allocations
	return daemon.ServerConfiguration{
		UUID:        target.ServerID,
		Name:        target.Name,
		Suspended:   target.Suspended,
		Environment: environment,
		Invocation:  target.StartupCommand,
		DockerImage: target.Image,
		Egg: map[string]any{
			"id":           target.EggID,
			"fileDenylist": denylist,
		},
		Build: map[string]any{
			"memoryLimit": target.MemoryMB,
			"diskSpace":   target.DiskMB,
			"cpuShares":   target.CPUShares,
			"cpuLimit":    firstNonZeroInt64(target.CPULimit, target.CPUShares),
			"ioWeight":    target.IOWeight,
			"swapMb":      target.SwapMB,
			"threads":     target.Threads,
			"oomDisabled": target.OOMDisabled,
		},
		Allocations: allocations,
		Config:      config,
		Mounts:      daemonMounts(target.Mounts),
	}
}

func daemonMounts(mounts []store.ServerMount) []daemon.Mount {
	out := make([]daemon.Mount, 0, len(mounts))
	for _, mount := range mounts {
		out = append(out, daemon.Mount{Source: mount.Source, Target: mount.Target, ReadOnly: mount.ReadOnly})
	}
	return out
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
