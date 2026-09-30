package envmanifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// errEmptyManifest is returned when the provided YAML parses to nothing.
var errEmptyManifest = errors.New("manifest is empty")

// Parse decodes a YAML (or JSON) EnvManifest with strict field checking and
// validates its semantic invariants. gopkg.in/yaml.v3 is used because it is
// already in the module's go.mod; no go.mod changes are required.
func Parse(data []byte) (EnvManifest, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return EnvManifest{}, errEmptyManifest
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var m EnvManifest
	if err := dec.Decode(&m); err != nil {
		return EnvManifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return EnvManifest{}, err
	}
	return m, nil
}

// Validate checks semantic invariants of a parsed manifest.
func (m EnvManifest) Validate() error {
	if m.Name == "" {
		return errors.New("manifest name is required")
	}
	if len(m.Services) == 0 {
		return errors.New("manifest must declare at least one service")
	}

	seen := map[string]bool{}
	for i := range m.Services {
		s := &m.Services[i]
		name := strings.TrimSpace(s.Name)
		if name == "" {
			return fmt.Errorf("services[%d]: name is required", i)
		}
		if seen[name] {
			return fmt.Errorf("services[%d]: duplicate service name %q", i, name)
		}
		seen[name] = true

		if s.Replicas < 0 {
			return fmt.Errorf("services[%d]: replicas must be non-negative", i)
		}
		if s.Replicas == 0 {
			s.Replicas = 1
		}
		for j, p := range s.Ports {
			if p.ContainerPort < 1 || p.ContainerPort > 65535 {
				return fmt.Errorf("services[%d].ports[%d]: invalid containerPort %d", i, j, p.ContainerPort)
			}
		}
		switch strings.ToLower(s.DesiredState) {
		case "", "running", "stopped":
		default:
			return fmt.Errorf("services[%d]: unsupported desiredState %q", i, s.DesiredState)
		}
	}

	envSeen := map[string]bool{}
	for i, e := range m.Env {
		if strings.TrimSpace(e.Key) == "" {
			return fmt.Errorf("env[%d]: key is required", i)
		}
		if e.Service != "" && !seen[e.Service] {
			return fmt.Errorf("env[%d]: step targets unknown service %q", i, e.Service)
		}
		key := e.Service + "." + e.Key
		if envSeen[key] {
			return fmt.Errorf("env[%d]: duplicate key %q within scope %q", i, e.Key, e.Service)
		}
		envSeen[key] = true
	}
	return nil
}

// JSONSchema is the JSON-schema document describing the EnvManifest wire
// format. Served by Export so editors can validate against it client-side.
const JSONSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "EnvManifest",
  "description": "Service-definition manifest for a gamepanel environment",
  "type": "object",
  "required": ["name", "services"],
  "properties": {
    "version": { "type": "string" },
    "name": { "type": "string", "minLength": 1 },
    "domain": { "type": "string" },
    "ports": { "type": "array", "items": { "$ref": "#/$defs/port" } },
    "services": { "type": "array", "items": { "$ref": "#/$defs/service" } },
    "env": { "type": "array", "items": { "$ref": "#/$defs/envStep" } }
  },
  "$defs": {
    "port": {
      "type": "object",
      "required": ["containerPort"],
      "properties": {
        "containerPort": { "type": "integer", "minimum": 1, "maximum": 65535 },
        "hostPort": { "type": "integer", "minimum": 1, "maximum": 65535 },
        "protocol": { "type": "string", "enum": ["http", "https", "tcp", "udp"] }
      }
    },
    "service": {
      "type": "object",
      "required": ["name"],
      "properties": {
        "name": { "type": "string", "minLength": 1 },
        "image": { "type": "string" },
        "composeService": { "type": "string" },
        "replicas": { "type": "integer", "minimum": 0 },
        "ports": { "type": "array", "items": { "$ref": "#/$defs/port" } },
        "env": { "type": "object", "additionalProperties": { "type": "string" } },
        "groups": { "type": "array", "items": { "type": "string" } },
        "dependsOn": { "type": "array", "items": { "type": "string" } },
        "desiredState": { "type": "string", "enum": ["running", "stopped"] }
      }
    },
    "envStep": {
      "type": "object",
      "required": ["key"],
      "properties": {
        "key": { "type": "string", "minLength": 1 },
        "value": { "type": "string" },
        "service": { "type": "string" },
        "sensitive": { "type": "boolean" }
      }
    }
  }
}`

// Schema returns the JSONSchema document for the EnvManifest wire format.
func Schema() string { return JSONSchema }

// Raw re-serializes an EnvManifest to JSON (for persistence/rendering).
func Raw(m EnvManifest) ([]byte, error) {
	return json.Marshal(m)
}
