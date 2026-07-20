package compose

import (
	"context"
	"testing"
)

func TestValidateSummarizesNormalizedComposeProject(t *testing.T) {
	result, err := Validate(context.Background(), `name: sample
services:
  web:
    image: nginx:alpine
    ports: ["8080:80"]
    depends_on: [db]
  db:
    image: postgres:16
volumes:
  database: {}
networks:
  default: {}
`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "sample" || len(result.Services) != 2 || result.Services[1].Name != "web" || result.Services[1].Ports != 1 || result.Services[1].DependsOn[0] != "db" {
		t.Fatalf("unexpected compose result: %#v", result)
	}
}

func TestValidateRejectsInlineIncludes(t *testing.T) {
	if _, err := Validate(context.Background(), "include: other.yaml\nservices: {}\n", nil); err == nil {
		t.Fatal("expected include to be rejected")
	}
}
