package compose

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A1 - Parse valid Compose v2.x YAML

func TestA1_ParseComposeV2x(t *testing.T) {
	content := `
version: "3.8"
name: test-stack
services:
  web:
    image: nginx:latest
    ports:
      - "8080:80"
    environment:
      NODE_ENV: production
      DEBUG: "true"
    depends_on:
      - db
    restart: unless-stopped
    command: ["nginx", "-g", "daemon off;"]
    entrypoint: ["/docker-entrypoint.sh"]
  db:
    image: postgres:16
    volumes:
      - pgdata:/var/lib/postgresql/data
    environment:
      POSTGRES_PASSWORD: secret
networks:
  backend:
    driver: bridge
volumes:
  pgdata:
    driver: local
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Equal(t, "3.8", parsed.Version)
	assert.Equal(t, "test-stack", parsed.Name)
	assert.Len(t, parsed.Services, 2)
	assert.Len(t, parsed.Networks, 1)
	assert.Len(t, parsed.Volumes, 1)

	var web, db *ServiceSummary
	for i := range parsed.Services {
		switch parsed.Services[i].Name {
		case "web":
			web = &parsed.Services[i]
		case "db":
			db = &parsed.Services[i]
		}
	}
	require.NotNil(t, web)
	require.NotNil(t, db)
	assert.Equal(t, "nginx:latest", web.Image)
	assert.NotEmpty(t, web.Ports)
	assert.Equal(t, "production", web.Environment["NODE_ENV"])
	assert.Equal(t, "true", web.Environment["DEBUG"])
	assert.Equal(t, []string{"db"}, web.DependsOn)
	assert.Equal(t, "unless-stopped", web.Restart)
	assert.NotEmpty(t, web.Command)
	assert.NotEmpty(t, web.Entrypoint)
	assert.Equal(t, "postgres:16", db.Image)
	assert.NotEmpty(t, db.Volumes)
}

func TestA1_ParseV2xWithBuildContext(t *testing.T) {
	content := `
version: "2.4"
services:
  app:
    build:
      context: ./src
      dockerfile: Dockerfile.prod
      args:
        NODE_ENV: production
      target: builder
    ports:
      - "3000:3000"
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	require.NotNil(t, parsed.Services[0].Build)
	assert.Equal(t, "./src", parsed.Services[0].Build.Context)
	assert.Equal(t, "Dockerfile.prod", parsed.Services[0].Build.Dockerfile)
	assert.Equal(t, "production", parsed.Services[0].Build.Args["NODE_ENV"])
	assert.Equal(t, "builder", parsed.Services[0].Build.Target)
}

// A1 - Validation of required fields

func TestA1_ValidateMissingImageAndBuild(t *testing.T) {
	content := `
services:
  nosource:
    restart: always
    ports:
      - "8080:80"
`
	svc := New(nil)
	result := svc.Validate([]byte(content), "/app")
	assert.False(t, result.Valid)
	require.NotEmpty(t, result.Errors)
	found := false
	for _, e := range result.Errors {
		if e.Field == "services.nosource" && e.Message == "service must specify either 'image' or 'build'" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected validation error for missing image/build")
}

func TestA1_ValidateNoServices(t *testing.T) {
	content := `
version: "3.8"
networks:
  net1:
    driver: bridge
volumes:
  vol1:
    driver: local
`
	svc := New(nil)
	result := svc.Validate([]byte(content), "/app")
	assert.False(t, result.Valid)
	require.NotEmpty(t, result.Errors)
	assert.Equal(t, "services", result.Errors[0].Field)
	assert.Contains(t, result.Errors[0].Message, "at least one service")
}

func TestA1_ValidateEmptyYAML(t *testing.T) {
	svc := New(nil)
	result := svc.Validate([]byte(""), "/app")
	assert.False(t, result.Valid)
	require.NotEmpty(t, result.Errors)
	assert.Equal(t, "services", result.Errors[0].Field)
}

func TestA1_ValidateMalformedYAML(t *testing.T) {
	svc := New(nil)
	result := svc.Validate([]byte(`services: [[malformed`), "/app")
	assert.False(t, result.Valid)
	require.NotEmpty(t, result.Errors)
	assert.Equal(t, "yaml", result.Errors[0].Field)
}

// A1 - Includes

func TestA1_ParseWithInclude(t *testing.T) {
	content := `
include:
  - path: ./common.yml
  - path: ./networks.yml
    project_directory: /shared
  - env_file: ./env/common.env
services:
  app:
    image: myapp:latest
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Services, 1)
	assert.Equal(t, "app", parsed.Services[0].Name)
}

func TestA1_ParseWithIncludeComplex(t *testing.T) {
	content := `
include:
  - path:
      - ./base.yml
      - ./override.yml
services:
  web:
    image: nginx:latest
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Services, 1)
	assert.Equal(t, "web", parsed.Services[0].Name)
}

// A1 - Profiles

func TestA1_ParseWithProfiles(t *testing.T) {
	content := `
services:
  frontend:
    image: nginx:latest
    profiles:
      - production
      - web
  worker:
    image: python:3.12
    profiles:
      - production
      - background
  redis:
    image: redis:7
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 3)

	profilesByService := map[string][]string{}
	for _, s := range parsed.Services {
		profilesByService[s.Name] = s.Profiles
	}
	assert.Equal(t, []string{"production", "web"}, profilesByService["frontend"])
	assert.Equal(t, []string{"production", "background"}, profilesByService["worker"])
	assert.Nil(t, profilesByService["redis"])
}

// A1 - Env interpolation

func TestA1_EnvInterpolation(t *testing.T) {
	os.Setenv("APP_PORT", "8080")
	os.Setenv("DB_HOST", "postgres.local")
	defer os.Unsetenv("APP_PORT")
	defer os.Unsetenv("DB_HOST")

	content := `
services:
  app:
    image: myapp:latest
    ports:
      - "${APP_PORT}:80"
    environment:
      DB_HOST: ${DB_HOST}
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	assert.Equal(t, "myapp:latest", parsed.Services[0].Image)
}

func TestA1_EnvInterpolationDefaultValues(t *testing.T) {
	os.Setenv("DEFINED_VAR", "resolved-value")
	defer os.Unsetenv("DEFINED_VAR")

	content := `
services:
  app:
    image: ${DEFINED_VAR}:latest
    environment:
      LOG_LEVEL: ${ENV_LOG_LEVEL:-info}
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	assert.Equal(t, "resolved-value:latest", parsed.Services[0].Image)
}

// A1 - Normalization (port format, environment, volumes)

func TestA1_PortNormalization(t *testing.T) {
	content := `
services:
  app:
    image: nginx:latest
    ports:
      - "8080:80"
      - 3000
      - "0.0.0.0:443:443"
      - 9090.0
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	assert.Len(t, parsed.Services[0].Ports, 4)
	assert.Contains(t, parsed.Services[0].Ports, "8080:80")
	assert.Contains(t, parsed.Services[0].Ports, "3000")
	assert.Contains(t, parsed.Services[0].Ports, "0.0.0.0:443:443")
	assert.Contains(t, parsed.Services[0].Ports, "9090")
}

func TestA1_EnvironmentNormalization(t *testing.T) {
	content := `
services:
  app:
    image: nginx:latest
    environment:
      PLAIN: value1
      EQUALS: val=with=equals
      INT: 42
      BOOL: "true"
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	assert.Equal(t, "value1", parsed.Services[0].Environment["PLAIN"])
	assert.Equal(t, "val=with=equals", parsed.Services[0].Environment["EQUALS"])
	assert.Equal(t, "42", parsed.Services[0].Environment["INT"])
	assert.Equal(t, "true", parsed.Services[0].Environment["BOOL"])
}

func TestA1_EnvironmentListNormalization(t *testing.T) {
	content := `
services:
  app:
    image: nginx:latest
    environment:
      - KEY1=VALUE1
      - KEY2=VALUE2
      - KEY3
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	assert.Equal(t, "VALUE1", parsed.Services[0].Environment["KEY1"])
	assert.Equal(t, "VALUE2", parsed.Services[0].Environment["KEY2"])
	assert.Empty(t, parsed.Services[0].Environment["KEY3"])
}

func TestA1_VolumeNormalization(t *testing.T) {
	content := `
services:
  app:
    image: nginx:latest
    volumes:
      - /host/data:/container/data
      - named_vol:/container/named
      - type: tmpfs
        target: /tmpfs
      - source: /host/config
        target: /container/config
        read_only: true
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	vols := parsed.Services[0].Volumes
	assert.Len(t, vols, 4)
	assert.Contains(t, vols, "/host/data:/container/data")
	assert.Contains(t, vols, "named_vol:/container/named")
	assert.Contains(t, vols, "tmpfs:/tmpfs")
	foundRo := false
	for _, v := range vols {
		if strings.Contains(v, "/host/config:/container/config") && strings.Contains(v, "ro") {
			foundRo = true
			break
		}
	}
	assert.True(t, foundRo, "expected read_only volume containing /host/config:/container/config and 'ro'")
}

func TestA1_DependsOnNormalization(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
    depends_on:
      - db
      - redis
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	assert.Equal(t, []string{"db", "redis"}, parsed.Services[0].DependsOn)
}

func TestA1_DependsOnLongSyntax(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
    depends_on:
      db:
        condition: service_healthy
      redis:
        condition: service_started
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	assert.Contains(t, parsed.Services[0].DependsOn, "db")
	assert.Contains(t, parsed.Services[0].DependsOn, "redis")
	assert.Len(t, parsed.Services[0].DependsOn, 2)
}

func TestA1_RelativePathResolution(t *testing.T) {
	content := `
services:
  app:
    build:
      context: ./backend
      dockerfile: Dockerfile
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/home/project")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	assert.NotNil(t, parsed.Services[0].Build)
	assert.Equal(t, "./backend", parsed.Services[0].Build.Context)
}

// G1 - Profile activation/deactivation

func TestG1_ProfileActivation(t *testing.T) {
	content := `
services:
  frontend:
    image: nginx:latest
  debug-tool:
    image: busybox:latest
    profiles:
      - debug
  monitoring:
    image: prom/prometheus:latest
    profiles:
      - monitoring
      - production
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 3)

	var frontend, debugTool, monitoring *ServiceSummary
	for i := range parsed.Services {
		switch parsed.Services[i].Name {
		case "frontend":
			frontend = &parsed.Services[i]
		case "debug-tool":
			debugTool = &parsed.Services[i]
		case "monitoring":
			monitoring = &parsed.Services[i]
		}
	}
	require.NotNil(t, frontend)
	require.NotNil(t, debugTool)
	require.NotNil(t, monitoring)

	assert.Nil(t, frontend.Profiles)
	assert.Equal(t, []string{"debug"}, debugTool.Profiles)
	assert.Equal(t, []string{"monitoring", "production"}, monitoring.Profiles)
}

// G1 - Include resolution

func TestG1_IncludePathsPreserved(t *testing.T) {
	content := `
include:
  - path: ./docker-compose.base.yml
  - path: ./docker-compose.override.yml
    project_directory: /app/overrides
services:
  app:
    image: myapp:latest
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Services, 1)
}

// G1 - Include failure (missing file is not a parser error - it's a deployment concern)

func TestG1_IncludeWithEnvFile(t *testing.T) {
	content := `
include:
  - env_file: .env.production
services:
  web:
    image: nginx:latest
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Services, 1)
}

// O1 - Secrets parsing

func TestO1_ParseSecretsFileReference(t *testing.T) {
	content := `
services:
  db:
    image: postgres:16
    secrets:
      - db_password
      - db_root_password
secrets:
  db_password:
    file: ./secrets/db_password.txt
  db_root_password:
    file: ./secrets/db_root_password.txt
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Secrets, 2)

	secretsByName := map[string]SecretSummary{}
	for _, s := range parsed.Secrets {
		secretsByName[s.Name] = s
	}
	assert.Equal(t, "./secrets/db_password.txt", secretsByName["db_password"].File)
	assert.Equal(t, "./secrets/db_root_password.txt", secretsByName["db_root_password"].File)
	assert.False(t, secretsByName["db_password"].External)
}

func TestO1_ParseSecretsExternalFlag(t *testing.T) {
	content := `
secrets:
  app_cert:
    external: true
  vault_token:
    external: true
services:
  app:
    image: myapp:latest
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Secrets, 2)

	secretsByName := map[string]SecretSummary{}
	for _, s := range parsed.Secrets {
		secretsByName[s.Name] = s
	}
	assert.True(t, secretsByName["app_cert"].External)
	assert.True(t, secretsByName["vault_token"].External)
}

func TestO1_ParseSecretsEnvironment(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
    secrets:
      - source: db_password
        target: /run/secrets/db_pass
        uid: "1000"
        gid: "1000"
        mode: 0400
secrets:
  db_password:
    file: ./secrets/db_password.txt
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Secrets, 1)
	assert.Equal(t, "db_password", parsed.Secrets[0].Name)
}

func TestO1_ParseSecretsMultiple(t *testing.T) {
	content := `
services:
  api:
    image: myapp:latest
    secrets:
      - api_key
      - db_password
      - jwt_secret
secrets:
  api_key:
    file: ./secrets/api.key
  db_password:
    file: ./secrets/db.pass
  jwt_secret:
    external: true
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Secrets, 3)

	names := map[string]bool{}
	for _, s := range parsed.Secrets {
		names[s.Name] = true
	}
	assert.True(t, names["api_key"])
	assert.True(t, names["db_password"])
	assert.True(t, names["jwt_secret"])
}

// O2 - Configs parsing

func TestO2_ParseConfigsFileReference(t *testing.T) {
	content := `
services:
  app:
    image: nginx:latest
    configs:
      - source: nginx_config
        target: /etc/nginx/conf.d/default.conf
configs:
  nginx_config:
    file: ./configs/nginx.conf
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Configs, 1)
	assert.Equal(t, "nginx_config", parsed.Configs[0].Name)
	assert.Equal(t, "./configs/nginx.conf", parsed.Configs[0].File)
}

func TestO2_ParseConfigsExternalFlag(t *testing.T) {
	content := `
configs:
  shared_config:
    external: true
services:
  app:
    image: myapp:latest
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Configs, 1)
	assert.Equal(t, "shared_config", parsed.Configs[0].Name)
	assert.True(t, parsed.Configs[0].External)
}

func TestO2_ParseConfigsMountPath(t *testing.T) {
	content := `
services:
  web:
    image: nginx:latest
    configs:
      - source: site_config
        target: /etc/nginx/conf.d/site.conf
        mode: 0440
      - source: ssl_certs
        target: /etc/nginx/ssl
configs:
  site_config:
    file: ./nginx/site.conf
  ssl_certs:
    file: ./nginx/ssl
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Configs, 2)

	configsByName := map[string]ConfigSummary{}
	for _, c := range parsed.Configs {
		configsByName[c.Name] = c
	}
	assert.Equal(t, "./nginx/site.conf", configsByName["site_config"].File)
	assert.Equal(t, "./nginx/ssl", configsByName["ssl_certs"].File)
	assert.False(t, configsByName["site_config"].External)
	assert.False(t, configsByName["ssl_certs"].External)
}

func TestO2_ParseConfigsMultiple(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
    configs:
      - app_config
      - logging_config
      - shared_config
configs:
  app_config:
    file: ./configs/app.yml
  logging_config:
    file: ./configs/logging.yml
  shared_config:
    external: true
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Configs, 3)

	names := map[string]bool{}
	externals := map[string]bool{}
	for _, c := range parsed.Configs {
		names[c.Name] = true
		if c.External {
			externals[c.Name] = true
		}
	}
	assert.True(t, names["app_config"])
	assert.True(t, names["logging_config"])
	assert.True(t, names["shared_config"])
	assert.True(t, externals["shared_config"])
	assert.False(t, externals["app_config"])
}

// O3 - Health check parsing

func TestO3_ParseHealthCheckCMD(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 5s
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	require.NotNil(t, parsed.Services[0].HealthCheck)
	hc := parsed.Services[0].HealthCheck
	assert.Equal(t, []string{"CMD", "curl", "-f", "http://localhost/health"}, hc.Test)
	assert.Equal(t, "30s", hc.Interval)
	assert.Equal(t, "10s", hc.Timeout)
	assert.Equal(t, 3, hc.Retries)
	assert.Equal(t, "5s", hc.StartPeriod)
	assert.False(t, hc.Disable)
}

func TestO3_ParseHealthCheckCMDSHELL(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
    healthcheck:
      test: "curl -f http://localhost/health || exit 1"
      interval: 1m
      timeout: 5s
      retries: 5
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	require.NotNil(t, parsed.Services[0].HealthCheck)
	hc := parsed.Services[0].HealthCheck
	assert.Equal(t, []string{"CMD-SHELL", "curl -f http://localhost/health || exit 1"}, hc.Test)
	assert.Equal(t, "1m", hc.Interval)
	assert.Equal(t, "5s", hc.Timeout)
	assert.Equal(t, 5, hc.Retries)
	assert.Empty(t, hc.StartPeriod)
}

func TestO3_ParseHealthCheckDisable(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
    healthcheck:
      disable: true
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	require.NotNil(t, parsed.Services[0].HealthCheck)
	assert.True(t, parsed.Services[0].HealthCheck.Disable)
}

func TestO3_ParseHealthCheckNginx(t *testing.T) {
	content := `
services:
  web:
    image: nginx:latest
    healthcheck:
      test: ["CMD", "service", "nginx", "status"]
      interval: 10s
      timeout: 3s
      retries: 2
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	require.NotNil(t, parsed.Services[0].HealthCheck)
	hc := parsed.Services[0].HealthCheck
	assert.Equal(t, []string{"CMD", "service", "nginx", "status"}, hc.Test)
	assert.Equal(t, "10s", hc.Interval)
	assert.Equal(t, "3s", hc.Timeout)
	assert.Equal(t, 2, hc.Retries)
}

func TestO3_HealthCheckDisableStoredCorrectly(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
    healthcheck:
      disable: true
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	require.NotNil(t, parsed.Services[0].HealthCheck)
	assert.True(t, parsed.Services[0].HealthCheck.Disable)
	assert.Nil(t, parsed.Services[0].HealthCheck.Test)

	result := svc.Validate([]byte(content), "/app")
	assert.True(t, result.Valid)
	found := false
	for _, w := range result.Warnings {
		if w.Field == "services.app.healthcheck" {
			found = true
			break
		}
	}
	assert.True(t, found, "healthcheck section (including disable) triggers a warning")
}

func TestO3_HealthCheckJSONRoundTrip(t *testing.T) {
	svc := New(nil)
	parsed, err := svc.Parse([]byte(composeWithHealthCheck), "/app")
	require.NoError(t, err)
	require.NotNil(t, parsed.Services[0].HealthCheck)

	data, err := json.Marshal(parsed)
	require.NoError(t, err)

	var restored ParsedCompose
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)
	require.Len(t, restored.Services, 1)
	require.NotNil(t, restored.Services[0].HealthCheck)
	assert.Equal(t, parsed.Services[0].HealthCheck.Test, restored.Services[0].HealthCheck.Test)
	assert.Equal(t, parsed.Services[0].HealthCheck.Interval, restored.Services[0].HealthCheck.Interval)
	assert.Equal(t, parsed.Services[0].HealthCheck.Retries, restored.Services[0].HealthCheck.Retries)
}

// O4 - Deploy section parsing

func TestO4_ParseDeployResources(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
    deploy:
      resources:
        limits:
          cpus: "2.0"
          memory: 512M
        reservations:
          cpus: "1.0"
          memory: 256M
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	require.NotNil(t, parsed.Services[0].Deploy)
	d := parsed.Services[0].Deploy
	require.NotNil(t, d.Resources)
	assert.Equal(t, "2.0", d.Resources.Limits["cpus"])
	assert.Equal(t, "512M", d.Resources.Limits["memory"])
	assert.Equal(t, "1.0", d.Resources.Reservations["cpus"])
	assert.Equal(t, "256M", d.Resources.Reservations["memory"])
}

func TestO4_ParseDeployReplicas(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
    deploy:
      mode: replicated
      replicas: 3
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	require.NotNil(t, parsed.Services[0].Deploy)
	assert.Equal(t, "replicated", parsed.Services[0].Deploy.Mode)
	assert.Equal(t, 3, parsed.Services[0].Deploy.Replicas)
}

func TestO4_ParseDeployGlobalMode(t *testing.T) {
	content := `
services:
  agent:
    image: datadog/agent:latest
    deploy:
      mode: global
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	require.NotNil(t, parsed.Services[0].Deploy)
	assert.Equal(t, "global", parsed.Services[0].Deploy.Mode)
	assert.Equal(t, 0, parsed.Services[0].Deploy.Replicas)
}

func TestO4_ParseDeployCPUMemory(t *testing.T) {
	content := `
services:
  worker:
    image: worker:latest
    deploy:
      resources:
        limits:
          cpus: "4"
          memory: 2G
        reservations:
          cpus: "2"
          memory: 1G
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	require.NotNil(t, parsed.Services[0].Deploy)
	require.NotNil(t, parsed.Services[0].Deploy.Resources)
	assert.Equal(t, "4", parsed.Services[0].Deploy.Resources.Limits["cpus"])
	assert.Equal(t, "2G", parsed.Services[0].Deploy.Resources.Limits["memory"])
}

func TestO4_DeployJSONRoundTrip(t *testing.T) {
	svc := New(nil)
	parsed, err := svc.Parse([]byte(composeWithDeploy), "/app")
	require.NoError(t, err)
	require.NotNil(t, parsed.Services[0].Deploy)

	data, err := json.Marshal(parsed)
	require.NoError(t, err)

	var restored ParsedCompose
	err = json.Unmarshal(data, &restored)
	require.NoError(t, err)
	require.Len(t, restored.Services, 1)
	require.NotNil(t, restored.Services[0].Deploy)
	assert.Equal(t, parsed.Services[0].Deploy.Mode, restored.Services[0].Deploy.Mode)
	assert.Equal(t, parsed.Services[0].Deploy.Replicas, restored.Services[0].Deploy.Replicas)
}

// O5 - Unsupported fields and warning collection

func TestO5_UnsupportedFieldsGenerateWarnings(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
    healthcheck:
      test: ["CMD", "curl", "http://localhost"]
    deploy:
      resources:
        limits:
          memory: 512M
    profiles:
      - production
`
	svc := New(nil)
	result := svc.Validate([]byte(content), "/app")
	assert.True(t, result.Valid)

	expectedWarnings := map[string]bool{
		"healthcheck support is not yet implemented in Forge": false,
		"deploy section is not yet fully supported in Forge":  false,
		"profiles are not currently enforced by Forge":        false,
	}
	for _, w := range result.Warnings {
		if _, ok := expectedWarnings[w.Message]; ok {
			expectedWarnings[w.Message] = true
		}
	}
	for msg, found := range expectedWarnings {
		assert.True(t, found, "expected warning: %s", msg)
	}
}

func TestO5_WarningsDontBlockDeployment(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
    healthcheck:
      test: ["CMD", "echo", "ok"]
    deploy:
      replicas: 2
    profiles:
      - staging
`
	svc := New(nil)
	result := svc.Validate([]byte(content), "/app")
	assert.True(t, result.Valid, "warnings should not block validation")
	assert.NotNil(t, result.Summary)
	assert.GreaterOrEqual(t, len(result.Warnings), 1)
	assert.Empty(t, result.Errors)
}

func TestO5_WarningCollectionMultipleServices(t *testing.T) {
	content := `
services:
  web:
    image: nginx:latest
    healthcheck:
      test: ["CMD", "nginx", "-t"]
    deploy:
      resources:
        limits:
          memory: 256M
  api:
    image: api:latest
    profiles:
      - production
    healthcheck:
      test: ["CMD", "curl", "http://localhost"]
  worker:
    image: worker:latest
    deploy:
      replicas: 3
`
	svc := New(nil)
	result := svc.Validate([]byte(content), "/app")
	assert.True(t, result.Valid)

	hasHealthWarning := false
	hasDeployWarning := false
	hasProfileWarning := false
	for _, w := range result.Warnings {
		switch {
		case w.Field == "services.web.healthcheck" || w.Field == "services.api.healthcheck" || w.Field == "services.worker.healthcheck":
			hasHealthWarning = true
		case w.Field == "services.web.deploy" || w.Field == "services.worker.deploy":
			hasDeployWarning = true
		case w.Field == "services.api.profiles":
			hasProfileWarning = true
		}
	}
	assert.True(t, hasHealthWarning)
	assert.True(t, hasDeployWarning)
	assert.True(t, hasProfileWarning)
}

func TestO5_ExternalSecretsWarning(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
secrets:
  vault_secret:
    external: true
  file_secret:
    file: ./secrets/secret.txt
`
	svc := New(nil)
	result := svc.Validate([]byte(content), "/app")
	assert.True(t, result.Valid)

	found := false
	for _, w := range result.Warnings {
		if w.Field == "secrets.vault_secret" && w.Message == "external secrets are not yet supported in Forge" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected external secrets warning")
}

func TestO5_ExternalConfigsWarning(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
configs:
  remote_config:
    external: true
  local_config:
    file: ./configs/local.yml
`
	svc := New(nil)
	result := svc.Validate([]byte(content), "/app")
	assert.True(t, result.Valid)

	found := false
	for _, w := range result.Warnings {
		if w.Field == "configs.remote_config" && w.Message == "external configs are not yet supported in Forge" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected external configs warning")
}

func TestO5_WarningsIncludeErrorCount(t *testing.T) {
	content := `
services:
  app:
    image: myapp:latest
    profiles:
      - warning-only
`
	svc := New(nil)
	result := svc.Validate([]byte(content), "/app")
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
	assert.NotEmpty(t, result.Warnings)
}

// Additional edge case tests

func TestParse_CommandAsList(t *testing.T) {
	content := `
services:
  app:
    image: node:20
    command: ["node", "server.js", "--port", "3000"]
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	assert.Equal(t, "node server.js --port 3000", parsed.Services[0].Command)
}

func TestParse_CommandAsString(t *testing.T) {
	content := `
services:
  app:
    image: node:20
    command: node server.js --port 3000
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	assert.Equal(t, "node server.js --port 3000", parsed.Services[0].Command)
}

func TestParse_ExternalNetworks(t *testing.T) {
	content := `
services:
  app:
    image: nginx:latest
networks:
  shared:
    external: true
  internal:
    driver: overlay
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Networks, 2)

	netByName := map[string]NetworkSummary{}
	for _, n := range parsed.Networks {
		netByName[n.Name] = n
	}
	assert.True(t, netByName["shared"].External)
	assert.False(t, netByName["internal"].External)
}

func TestParse_ExternalVolumes(t *testing.T) {
	content := `
services:
  db:
    image: postgres:16
volumes:
  pgdata:
    external: true
  logs:
    driver: local
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Volumes, 2)

	volByName := map[string]VolumeSummary{}
	for _, v := range parsed.Volumes {
		volByName[v.Name] = v
	}
	assert.True(t, volByName["pgdata"].External)
	assert.False(t, volByName["logs"].External)
}

func TestParse_EntrypointAsList(t *testing.T) {
	content := `
services:
  app:
    image: python:3.12
    entrypoint: ["python", "-m", "uvicorn"]
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	assert.Equal(t, "python -m uvicorn", parsed.Services[0].Entrypoint)
}

func TestValidate_ComposeWithImageOnlyIsValid(t *testing.T) {
	content := `
services:
  app:
    image: alpine:latest
    command: echo hi
`
	svc := New(nil)
	result := svc.Validate([]byte(content), "/app")
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestValidate_ComposeWithBuildOnlyIsValid(t *testing.T) {
	content := `
services:
  app:
    build:
      context: .
`
	svc := New(nil)
	result := svc.Validate([]byte(content), "/app")
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
}

func TestParse_EmptyServices(t *testing.T) {
	content := `
version: "3.8"
name: empty-project
services: {}
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Services, 0)
}

func TestParse_NetworkLabels(t *testing.T) {
	content := `
services:
  app:
    image: nginx:latest
networks:
  custom:
    driver: overlay
    labels:
      com.example.env: production
      com.example.team: platform
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Networks, 1)
	assert.Equal(t, "production", parsed.Networks[0].Labels["com.example.env"])
	assert.Equal(t, "platform", parsed.Networks[0].Labels["com.example.team"])
}

func TestParse_MinimalValidV2_0(t *testing.T) {
	content := `
version: "2.0"
services:
  app:
    image: alpine:3.21
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Equal(t, "2.0", parsed.Version)
	assert.Len(t, parsed.Services, 1)
}

func TestParse_MinimalValidV3_0(t *testing.T) {
	content := `
version: "3.0"
services:
  app:
    image: alpine:3.21
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	assert.Equal(t, "3.0", parsed.Version)
	assert.Len(t, parsed.Services, 1)
}

// Test that ParseSummary exported helper works
func TestParseSummary_ExportedHelper(t *testing.T) {
	parsed, err := ParseSummary([]byte(minimalCompose), "/app")
	require.NoError(t, err)
	assert.Len(t, parsed.Services, 1)
	assert.Equal(t, "app", parsed.Services[0].Name)
}

func TestValidateSummary_ExportedHelper(t *testing.T) {
	result := ValidateSummary([]byte(minimalCompose), "/app")
	assert.True(t, result.Valid)
	assert.NotNil(t, result.Summary)
}

// Test compose service with environment using both map and list
func TestParse_EnvironmentMixedMapAndList(t *testing.T) {
	content := `
services:
  web:
    image: nginx:latest
    environment:
      - NODE_ENV=production
      - DEBUG=true
      - KEY_ONLY
  api:
    image: node:20
    environment:
      DB_HOST: postgres.local
      DB_PORT: "5432"
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 2)

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

	assert.Equal(t, "production", web.Environment["NODE_ENV"])
	assert.Equal(t, "true", web.Environment["DEBUG"])
	assert.Empty(t, web.Environment["KEY_ONLY"])

	assert.Equal(t, "postgres.local", api.Environment["DB_HOST"])
	assert.Equal(t, "5432", api.Environment["DB_PORT"])
}

func TestParse_ProjectNameFromFilepath(t *testing.T) {
	content := `
services:
  app:
    image: alpine:latest
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/home/user/projects/my-app")
	require.NoError(t, err)
	assert.Equal(t, "my-app", parsed.Name)
}

func TestParse_ProjectNameFromYAMLTakesPrecedence(t *testing.T) {
	content := `
name: explicit-name
services:
  app:
    image: alpine:latest
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/home/user/projects/some-project")
	require.NoError(t, err)
	assert.Equal(t, "explicit-name", parsed.Name)
}

// Port protocol parsing
func TestParse_PortWithProtocol(t *testing.T) {
	content := `
services:
  app:
    image: nginx:latest
    ports:
      - "80:80/tcp"
      - "443:443/udp"
      - "127.0.0.1:8080:8080/tcp"
`
	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)
	require.Len(t, parsed.Services, 1)
	assert.Len(t, parsed.Services[0].Ports, 3)
	assert.Contains(t, parsed.Services[0].Ports, "80:80/tcp")
	assert.Contains(t, parsed.Services[0].Ports, "443:443/udp")
	assert.Contains(t, parsed.Services[0].Ports, "127.0.0.1:8080:8080/tcp")
}

// Test compose with multiple advanced features simultaneously
func TestParse_FullFeaturedCompose(t *testing.T) {
	content := `
version: "3.9"
name: full-stack
services:
  frontend:
    image: nginx:alpine
    ports:
      - "80:80"
      - "443:443"
    depends_on:
      - backend
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost"]
      interval: 10s
      timeout: 3s
      retries: 3
    deploy:
      replicas: 2
      resources:
        limits:
          cpus: "0.5"
          memory: 256M
    profiles:
      - production
    restart: unless-stopped
  backend:
    image: node:20-alpine
    command: ["node", "server.js"]
    environment:
      - NODE_ENV=production
      - DB_HOST=db
    depends_on:
      db:
        condition: service_healthy
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost:3000/health"]
      interval: 15s
      timeout: 5s
      retries: 5
      start_period: 10s
    deploy:
      resources:
        limits:
          cpus: "1.0"
          memory: 512M
    volumes:
      - uploads:/app/uploads
      - type: tmpfs
        target: /tmp
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: myapp
      POSTGRES_USER: app
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD", "pg_isready"]
      interval: 10s
      timeout: 5s
      retries: 5
    secrets:
      - db_password
    restart: unless-stopped
networks:
  frontend:
    driver: bridge
    labels:
      tier: web
  backend:
    driver: overlay
    external: true
volumes:
  pgdata:
    driver: local
  uploads:
    external: true
secrets:
  db_password:
    file: ./secrets/db_password.txt
configs:
  nginx_conf:
    file: ./configs/nginx.conf
`

	svc := New(nil)
	parsed, err := svc.Parse([]byte(content), "/app")
	require.NoError(t, err)

	assert.Equal(t, "3.9", parsed.Version)
	assert.Equal(t, "full-stack", parsed.Name)
	assert.Len(t, parsed.Services, 3)
	assert.Len(t, parsed.Networks, 2)
	assert.Len(t, parsed.Volumes, 2)
	assert.Len(t, parsed.Secrets, 1)
	assert.Len(t, parsed.Configs, 1)

	names := map[string]bool{}
	for _, s := range parsed.Services {
		names[s.Name] = true
	}
	assert.True(t, names["frontend"])
	assert.True(t, names["backend"])
	assert.True(t, names["db"])
}
