package compose

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const basicCompose = `
version: "3.8"
name: test-stack
services:
  web:
    image: nginx:latest
    ports:
      - "8080:80"
    environment:
      NODE_ENV: production
    depends_on:
      - api
  api:
    image: myapp:latest
    build:
      context: .
      dockerfile: Dockerfile
    restart: always
    profiles:
      - production
networks:
  frontend:
    driver: bridge
volumes:
  data:
    driver: local
`

const minimalCompose = `
services:
  app:
    image: alpine:latest
`

const invalidCompose = `
services:
  broken:
`

const composeWithHealthCheck = `
services:
  app:
    image: myapp:latest
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 5s
`

const composeWithDeploy = `
services:
  app:
    image: myapp:latest
    deploy:
      mode: replicated
      replicas: 3
      resources:
        limits:
          cpus: "2.0"
          memory: 512M
`

const composeWithSecretsConfigs = `
services:
  app:
    image: myapp:latest
secrets:
  db_password:
    file: ./secrets/db_password.txt
configs:
  app_config:
    file: ./configs/app.yml
`

const composeWithInclude = `
include:
  - path: ./common.yml
services:
  app:
    image: myapp:latest
`

func TestParse_BasicCompose(t *testing.T) {
	svc, _ := New(nil)
	parsed, err := svc.Parse([]byte(basicCompose), "/app")
	require.NoError(t, err)
	assert.Equal(t, "3.8", parsed.Version)
	assert.Equal(t, "test-stack", parsed.Name)
	assert.Len(t, parsed.Services, 2)
	assert.Len(t, parsed.Networks, 1)
	assert.Len(t, parsed.Volumes, 1)

	var web, api *ServiceSummary
	for i := range parsed.Services {
		switch parsed.Services[i].Name {
		case "web":
			web = &parsed.Services[i]
		case "api":
			api = &parsed.Services[i]
		}
	}
	require.NotNil(t, web)
	require.NotNil(t, api)

	assert.Equal(t, "nginx:latest", web.Image)
	assert.Equal(t, []string{"8080:80"}, web.Ports)
	assert.Equal(t, "production", web.Environment["NODE_ENV"])
	assert.Equal(t, []string{"api"}, web.DependsOn)

	assert.Equal(t, "myapp:latest", api.Image)
	assert.NotNil(t, api.Build)
	assert.Equal(t, ".", api.Build.Context)
	assert.Equal(t, "Dockerfile", api.Build.Dockerfile)
	assert.Equal(t, "always", api.Restart)
	assert.Equal(t, []string{"production"}, api.Profiles)

	assert.Equal(t, "frontend", parsed.Networks[0].Name)
	assert.Equal(t, "bridge", parsed.Networks[0].Driver)

	assert.Equal(t, "data", parsed.Volumes[0].Name)
	assert.Equal(t, "local", parsed.Volumes[0].Driver)
}

func TestParse_MinimalCompose(t *testing.T) {
	svc, _ := New(nil)
	parsed, err := svc.Parse([]byte(minimalCompose), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Services, 1)
	assert.Equal(t, "app", parsed.Services[0].Name)
	assert.Equal(t, "alpine:latest", parsed.Services[0].Image)
}

func TestValidate_ValidCompose(t *testing.T) {
	svc, _ := New(nil)
	result := svc.Validate([]byte(basicCompose), "/app")
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
	assert.NotNil(t, result.Summary)
}

func TestValidate_InvalidCompose_NoImageOrBuild(t *testing.T) {
	content := `
services:
  broken-svc:
    restart: always
`
	svc, _ := New(nil)
	result := svc.Validate([]byte(content), "/app")
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)
}

func TestValidate_InvalidCompose_NoServices(t *testing.T) {
	content := `
version: "3.8"
networks:
  net1:
    driver: bridge
`
	svc, _ := New(nil)
	result := svc.Validate([]byte(content), "/app")
	assert.False(t, result.Valid)
}

func TestValidate_HealthCheckWarning(t *testing.T) {
	svc, _ := New(nil)
	result := svc.Validate([]byte(composeWithHealthCheck), "/app")
	assert.True(t, result.Valid)
	found := false
	for _, w := range result.Warnings {
		if w.Message == "healthcheck support is not yet implemented in Forge" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected healthcheck warning")
}

func TestValidate_DeployWarning(t *testing.T) {
	svc, _ := New(nil)
	result := svc.Validate([]byte(composeWithDeploy), "/app")
	assert.True(t, result.Valid)
	found := false
	for _, w := range result.Warnings {
		if w.Message == "deploy section is not yet fully supported in Forge" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected deploy warning")
}

func TestValidate_ProfileWarning(t *testing.T) {
	svc, _ := New(nil)
	result := svc.Validate([]byte(basicCompose), "/app")
	assert.True(t, result.Valid)
	found := false
	for _, w := range result.Warnings {
		if w.Message == "profiles are not currently enforced by Forge" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected profiles warning")
}

func TestParse_InvalidYAML(t *testing.T) {
	svc, _ := New(nil)
	_, err := svc.Parse([]byte(`{{invalid-yaml`), "/app")
	assert.Error(t, err)
}

func TestParse_SecretsAndConfigs(t *testing.T) {
	svc, _ := New(nil)
	parsed, err := svc.Parse([]byte(composeWithSecretsConfigs), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Secrets, 1)
	assert.Equal(t, "db_password", parsed.Secrets[0].Name)
	assert.Equal(t, "./secrets/db_password.txt", parsed.Secrets[0].File)
	assert.Len(t, parsed.Configs, 1)
	assert.Equal(t, "app_config", parsed.Configs[0].Name)
}

func TestParse_Include(t *testing.T) {
	svc, _ := New(nil)
	parsed, err := svc.Parse([]byte(composeWithInclude), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Services, 1)
	assert.Equal(t, "app", parsed.Services[0].Name)
}

func TestJSONRoundTrip(t *testing.T) {
	svc, _ := New(nil)
	parsed, err := svc.Parse([]byte(basicCompose), "/app")
	require.NoError(t, err)

	data, err := json.Marshal(parsed)
	require.NoError(t, err)

	var restored ParsedCompose
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)
	assert.Equal(t, parsed.Version, restored.Version)
	assert.Equal(t, len(parsed.Services), len(restored.Services))
}
