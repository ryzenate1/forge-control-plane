package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}

func formatInt(value int) string {
	return strconv.Itoa(value)
}

func formatUint(value uint64) string {
	return strconv.FormatUint(value, 10)
}

func diskLimitMBFromConfiguration(payload map[string]any) int64 {
	build, ok := payload["build"].(map[string]any)
	if !ok {
		return -1
	}
	for _, key := range []string{"disk_space", "diskSpace", "diskMb"} {
		switch value := build[key].(type) {
		case float64:
			return int64(value)
		case int64:
			return value
		case int:
			return int64(value)
		case json.Number:
			parsed, err := value.Int64()
			if err == nil {
				return parsed
			}
		}
	}
	return -1
}

func memoryMBFromConfiguration(payload map[string]any) int64 {
	build, _ := payload["build"].(map[string]any)
	if len(build) == 0 {
		return 0
	}
	return int64(firstNumber(build, "memory_limit", "memoryLimit", "memoryMb", "memory_mb"))
}

func allocationIPFromConfiguration(payload map[string]any) string {
	allocations, _ := payload["allocations"].(map[string]any)
	if len(allocations) == 0 {
		return ""
	}
	defaultAlloc, _ := allocations["default"].(map[string]any)
	if len(defaultAlloc) == 0 {
		return ""
	}
	if value, ok := defaultAlloc["ip"].(string); ok {
		return value
	}
	return ""
}

func allocationPortFromConfiguration(payload map[string]any) int {
	allocations, _ := payload["allocations"].(map[string]any)
	if len(allocations) == 0 {
		return 0
	}
	defaultAlloc, _ := allocations["default"].(map[string]any)
	if len(defaultAlloc) == 0 {
		return 0
	}
	return firstNumber(defaultAlloc, "port")
}

func stopTypeFromConfiguration(payload map[string]any) string {
	processConfiguration, _ := payload["process_configuration"].(map[string]any)
	if len(processConfiguration) == 0 {
		return ""
	}
	stop, _ := processConfiguration["stop"].(map[string]any)
	if len(stop) == 0 {
		return ""
	}
	value, _ := stop["type"].(string)
	return value
}

func stopValueFromConfiguration(payload map[string]any) string {
	processConfiguration, _ := payload["process_configuration"].(map[string]any)
	if len(processConfiguration) == 0 {
		return ""
	}
	stop, _ := processConfiguration["stop"].(map[string]any)
	if len(stop) == 0 {
		return ""
	}
	value, _ := stop["value"].(string)
	return value
}

func stopTimeoutFromConfiguration(payload map[string]any) time.Duration {
	processConfiguration, _ := payload["process_configuration"].(map[string]any)
	stop, _ := processConfiguration["stop"].(map[string]any)
	for _, key := range []string{"timeout", "timeout_seconds"} {
		if value, ok := stop[key].(float64); ok && value > 0 {
			return time.Duration(value) * time.Second
		}
	}
	return 30 * time.Second
}

func firstNumber(values map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case float32:
			return int(typed)
		case int:
			return typed
		case int64:
			return int(typed)
		case int32:
			return int(typed)
		case json.Number:
			if parsed, err := typed.Int64(); err == nil {
				return int(parsed)
			}
		}
	}
	return 0
}

func (s *Server) applyConfigurationFiles(serverID string, payload map[string]any) error {
	config, _ := payload["config"].(map[string]any)
	if len(config) == 0 {
		return nil
	}
	files, _ := config["files"].([]any)
	if len(files) == 0 {
		return nil
	}
	env := map[string]string{}
	if rawEnv, ok := payload["environment"].(map[string]any); ok {
		for key, value := range rawEnv {
			env[key] = fmt.Sprint(value)
		}
	}
	for _, raw := range files {
		fileConfig, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		pathValue, _ := fileConfig["path"].(string)
		if pathValue == "" {
			pathValue, _ = fileConfig["file"].(string)
		}
		if pathValue == "" {
			continue
		}
		content := renderTemplate(fmt.Sprint(fileConfig["content"]), env)
		if properties, ok := fileConfig["properties"].(map[string]any); ok {
			lines := []string{}
			for key, value := range properties {
				lines = append(lines, key+"="+renderTemplate(fmt.Sprint(value), env))
			}
			content = strings.Join(lines, "\n") + "\n"
		}
		if jsonValue, ok := fileConfig["json"]; ok {
			body, err := json.MarshalIndent(jsonValue, "", "  ")
			if err != nil {
				return err
			}
			content = renderTemplate(string(body), env)
		}
		target, err := s.safePath(serverID, pathValue)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		if err := s.manager.HasSpaceForWrite(serverID, int64(len(content))); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(content), 0o640); err != nil {
			return err
		}
	}
	return nil
}

func renderTemplate(input string, env map[string]string) string {
	output := input
	for key, value := range env {
		output = strings.ReplaceAll(output, "{{"+key+"}}", value)
		output = strings.ReplaceAll(output, "{{ "+key+" }}", value)
		output = strings.ReplaceAll(output, "{{env."+key+"}}", value)
		output = strings.ReplaceAll(output, "{{ env."+key+" }}", value)
		output = strings.ReplaceAll(output, "{{"+key+"|default:''}}", value)
	}
	return output
}

func (s *Server) effectiveEnvList(serverID string, explicit []string) []string {
	merged := map[string]string{}
	state := s.manager.State(serverID)
	state.mu.Lock()
	if strings.TrimSpace(state.StartupCommand) != "" {
		merged["STARTUP"] = state.StartupCommand
	}
	if state.MemoryMB > 0 {
		merged["SERVER_MEMORY"] = strconv.FormatInt(state.MemoryMB, 10)
	}
	if strings.TrimSpace(state.AllocationIP) != "" {
		merged["SERVER_IP"] = state.AllocationIP
	}
	if state.AllocationPort > 0 {
		merged["SERVER_PORT"] = strconv.Itoa(state.AllocationPort)
	}
	for key, value := range state.EnvVars {
		if key != "" && !strings.Contains(key, "=") {
			merged[key] = value
		}
	}
	state.mu.Unlock()
	for _, entry := range explicit {
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		merged[parts[0]] = parts[1]
	}
	out := make([]string, 0, len(merged))
	for key, value := range merged {
		out = append(out, key+"="+value)
	}
	return out
}

func (s *Server) effectiveEnvMapList(serverID string, explicit map[string]string) []string {
	merged := map[string]string{}
	state := s.manager.State(serverID)
	state.mu.Lock()
	if strings.TrimSpace(state.StartupCommand) != "" {
		merged["STARTUP"] = state.StartupCommand
	}
	if state.MemoryMB > 0 {
		merged["SERVER_MEMORY"] = strconv.FormatInt(state.MemoryMB, 10)
	}
	if strings.TrimSpace(state.AllocationIP) != "" {
		merged["SERVER_IP"] = state.AllocationIP
	}
	if state.AllocationPort > 0 {
		merged["SERVER_PORT"] = strconv.Itoa(state.AllocationPort)
	}
	for key, value := range state.EnvVars {
		if key != "" && !strings.Contains(key, "=") {
			merged[key] = value
		}
	}
	state.mu.Unlock()
	for key, value := range explicit {
		if key == "" || strings.Contains(key, "=") {
			continue
		}
		merged[key] = value
	}
	out := make([]string, 0, len(merged))
	for key, value := range merged {
		out = append(out, key+"="+value)
	}
	return out
}
