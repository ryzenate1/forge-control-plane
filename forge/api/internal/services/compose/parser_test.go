package compose

import (
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

// TestComposeParser tests the basic functionality of the Compose parser
func TestComposeParser(t *testing.T) {
	parser := NewComposeParser("")

	// Test parsing a simple Compose file
	composeContent := `
version: '3.8'
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
    environment:
      - NGINX_ENV=production
  db:
    image: postgres:13
    volumes:
      - db_data:/var/lib/postgresql/data

volumes:
  db_data:
`

	project, err := parser.ParseComposeString(composeContent, "test.yaml")
	if err != nil {
		t.Fatalf("Failed to parse compose: %v", err)
	}

	if project == nil {
		t.Fatal("Project is nil")
	}

	// Check that we have the expected services
	if len(project.Services) != 2 {
		t.Errorf("Expected 2 services, got %d", len(project.Services))
	}

	// Check web service
	webSvc, exists := project.Services["web"]
	if !exists {
		t.Error("web service not found")
	} else {
		if webSvc.Image != "nginx:latest" {
			t.Errorf("Expected web image to be nginx:latest, got %s", webSvc.Image)
		}
	}

	// Check db service
	dbSvc, exists := project.Services["db"]
	if !exists {
		t.Error("db service not found")
	} else {
		if dbSvc.Image != "postgres:13" {
			t.Errorf("Expected db image to be postgres:13, got %s", dbSvc.Image)
		}
	}

	// Check volumes
	if len(project.Volumes) != 1 {
		t.Errorf("Expected 1 volume, got %d", len(project.Volumes))
	}
}

// TestNormalizeToForgeModels tests the normalization of Compose to Forge models
func TestNormalizeToForgeModels(t *testing.T) {
	parser := NewComposeParser("")

	composeContent := `
version: '3.8'
name: test-project
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
    environment:
      - NGINX_ENV=production
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost"]
      interval: 30s
      timeout: 10s
      retries: 3
    deploy:
      replicas: 2
      resources:
        limits:
          cpus: '0.5'
          memory: 512M
`

	project, err := parser.ParseComposeString(composeContent, "test.yaml")
	if err != nil {
		t.Fatalf("Failed to parse compose: %v", err)
	}

	parsed, err := parser.NormalizeToForgeModels(project)
	if err != nil {
		t.Fatalf("Failed to normalize: %v", err)
	}

	if parsed == nil {
		t.Fatal("Parsed config is nil")
	}

	if parsed.Name != "test-project" {
		t.Errorf("Expected project name to be test-project, got %s", parsed.Name)
	}

	if parsed.Version != "3.8" {
		t.Errorf("Expected version to be 3.8, got %s", parsed.Version)
	}

	if len(parsed.Services) != 1 {
		t.Errorf("Expected 1 service, got %d", len(parsed.Services))
	}

	webSvc := parsed.Services[0]
	if webSvc.Name != "web" {
		t.Errorf("Expected service name to be web, got %s", webSvc.Name)
	}

	if webSvc.Image != "nginx:latest" {
		t.Errorf("Expected image to be nginx:latest, got %s", webSvc.Image)
	}

	// Check health check
	if webSvc.HealthCheck == nil {
		t.Error("HealthCheck is nil")
	} else {
		if webSvc.HealthCheck.Interval != "30s" {
			t.Errorf("Expected health check interval to be 30s, got %s", webSvc.HealthCheck.Interval)
		}
	}

	// Check deploy
	if webSvc.Deploy == nil {
		t.Error("Deploy is nil")
	} else {
		if webSvc.Deploy.Replicas != 2 {
			t.Errorf("Expected replicas to be 2, got %d", webSvc.Deploy.Replicas)
		}
	}
}

// TestServiceMappings tests the service mapping functionality
func TestServiceMappings(t *testing.T) {
	// This test would require the store and other dependencies
	// For now, we'll just test the parser functionality
	parser := NewComposeParser("")

	composeContent := `
version: '3.8'
services:
  web_server:
    image: nginx:latest
  database:
    image: postgres:13
  cache:
    image: redis:latest
`

	project, err := parser.ParseComposeString(composeContent, "test.yaml")
	if err != nil {
		t.Fatalf("Failed to parse compose: %v", err)
	}

	serviceNames := parser.GetServiceNames(project)
	if len(serviceNames) != 3 {
		t.Errorf("Expected 3 service names, got %d", len(serviceNames))
	}

	// Check that all expected services are present
	expectedServices := map[string]bool{
		"web_server": false,
		"database":   false,
		"cache":      false,
	}

	for _, name := range serviceNames {
		if _, exists := expectedServices[name]; exists {
			expectedServices[name] = true
		}
	}

	for name, found := range expectedServices {
		if !found {
			t.Errorf("Service %s not found in parsed services", name)
		}
	}
}

// TestComposeVersionCompatibility tests version compatibility checking
func TestComposeVersionCompatibility(t *testing.T) {
	parser := NewComposeParser("")

	// Test supported versions
	supportedVersions := []string{"2.4", "3.8", "3.0", ""}
	for _, version := range supportedVersions {
		err := parser.CheckComposeVersionCompatibility(version)
		if err != nil {
			t.Errorf("Version %s should be supported, but got error: %v", version, err)
		}
	}

	// Test unsupported version
	unsupportedVersion := "1.0"
	err := parser.CheckComposeVersionCompatibility(unsupportedVersion)
	if err == nil {
		t.Errorf("Version %s should not be supported, but no error was returned", unsupportedVersion)
	}
}

// TestGetServiceByName tests retrieving a specific service
func TestGetServiceByName(t *testing.T) {
	parser := NewComposeParser("")

	composeContent := `
version: '3.8'
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
  db:
    image: postgres:13
`

	project, err := parser.ParseComposeString(composeContent, "test.yaml")
	if err != nil {
		t.Fatalf("Failed to parse compose: %v", err)
	}

	// Get web service
	webSvc, err := parser.GetServiceByName(project, "web")
	if err != nil {
		t.Fatalf("Failed to get web service: %v", err)
	}

	if webSvc.Image != "nginx:latest" {
		t.Errorf("Expected web service image to be nginx:latest, got %s", webSvc.Image)
	}

	// Try to get non-existent service
	_, err = parser.GetServiceByName(project, "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent service, but got none")
	}
}

// TestParseComposeWithMultipleFiles tests parsing multiple Compose files
func TestParseComposeWithMultipleFiles(t *testing.T) {
	parser := NewComposeParser("")

	files := map[string][]byte{
		"docker-compose.yml": []byte(`
version: '3.8'
services:
  web:
    image: nginx:latest
`),
		"docker-compose.override.yml": []byte(`
version: '3.8'
services:
  web:
    ports:
      - "80:80"
`),
	}

	project, err := parser.ParseComposeWithMultipleFiles(files)
	if err != nil {
		t.Fatalf("Failed to parse multiple files: %v", err)
	}

	if project == nil {
		t.Fatal("Project is nil")
	}

	// Check that the web service exists
	webSvc, exists := project.Services["web"]
	if !exists {
		t.Error("web service not found")
	} else {
		if webSvc.Image != "nginx:latest" {
			t.Errorf("Expected web image to be nginx:latest, got %s", webSvc.Image)
		}
	}
}

// BenchmarkComposeParsing benchmarks the performance of Compose parsing
func BenchmarkComposeParsing(b *testing.B) {
	parser := NewComposeParser("")

	composeContent := `
version: '3.8'
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
    environment:
      - NGINX_ENV=production
    volumes:
      - ./data:/var/www/html
    depends_on:
      - db
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost"]
      interval: 30s
      timeout: 10s
      retries: 3
    deploy:
      replicas: 2
      resources:
        limits:
          cpus: '0.5'
          memory: 512M
        reservations:
          cpus: '0.25'
          memory: 256M

  db:
    image: postgres:13
    volumes:
      - db_data:/var/lib/postgresql/data
    environment:
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=mydb

  cache:
    image: redis:latest
    ports:
      - "6379:6379"

volumes:
  db_data:

networks:
  app_network:
    driver: bridge
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parser.ParseComposeString(composeContent, "test.yaml")
		if err != nil {
			b.Fatalf("Failed to parse compose: %v", err)
		}
	}
}

// Ensure types.Project is used to avoid import issues
var _ types.Project
