package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gamepanel/forge/internal/services/envvars"
	"gamepanel/forge/internal/store"
)

// envVarsForKind returns the environment variable names a catalog kind's
// connection string is injected as, plus the attach-link var prefix.
func envVarsForKind(entryKey string) ([]string, string) {
	switch entryKey {
	case "postgres", "mysql", "mariadb", "mongodb", "clickhouse":
		return []string{"DATABASE_URL"}, "DATABASE"
	case "redis", "valkey":
		return []string{"REDIS_URL"}, "REDIS"
	case "redis-queue":
		return []string{"REDIS_URL", "REDIS_QUEUE_URL"}, "REDIS"
	case "rabbitmq":
		return []string{"AMQP_URL", "RABBITMQ_URL"}, "AMQP"
	case "nats":
		return []string{"NATS_URL"}, "NATS"
	case "memcached":
		return []string{"MEMCACHED_URL"}, "MEMCACHED"
	default:
		return []string{"DATABASE_URL"}, "DATABASE"
	}
}

// resolveConnString extracts the connection string from any provisioning
// result type the catalog produces.
func resolveConnString(kind string, instance any) (string, error) {
	switch v := instance.(type) {
	case store.CatalogInstance:
		if v.ConnString == "" {
			return "", errors.New("catalog instance has no connection string yet")
		}
		return v.ConnString, nil
	case *store.CatalogInstance:
		if v == nil || v.ConnString == "" {
			return "", errors.New("catalog instance has no connection string yet")
		}
		return v.ConnString, nil
	case store.DBContainer:
		return v.ConnectionString, nil
	case *store.DBContainer:
		if v == nil {
			return "", errors.New("nil database container")
		}
		return v.ConnectionString, nil
	default:
		return "", fmt.Errorf("cannot build connection string for kind %q from %T", kind, instance)
	}
}

// writeConnVars converges the sensitive connection variables for a connection
// string into one environment scope, creating or updating as needed.
func (s *Service) writeConnVars(ctx context.Context, entryKey, connStr, envID string) error {
	if s.envSvc == nil {
		return errors.New("catalog: environment variable service is not configured")
	}
	varNames, _ := envVarsForKind(entryKey)

	existing := map[string]store.EnvironmentVariable{}
	if vars, listErr := s.envSvc.List(ctx, "environment", envID); listErr == nil {
		for _, v := range vars {
			existing[v.Key] = v
		}
	}

	actorID := "catalog"
	for _, name := range varNames {
		if v, ok := existing[name]; ok {
			if _, err := s.envSvc.Update(ctx, v.ID, envvars.UpdateEnvVarInput{
				Value:       connStr,
				IsSensitive: true,
				Actor:       &actorID,
			}); err != nil {
				return fmt.Errorf("update env var %s: %w", name, err)
			}
			continue
		}
		if _, err := s.envSvc.Create(ctx, envvars.CreateEnvVarInput{
			EnvironmentID: &envID,
			Scope:         "environment",
			Key:           name,
			Value:         connStr,
			IsSensitive:   true,
			Actor:         &actorID,
		}); err != nil {
			return fmt.Errorf("create env var %s: %w", name, err)
		}
	}
	return nil
}

// InjectAndInjectConnector writes the connection-string env group for a
// freshly provisioned instance into the target environment. kind selects the
// conventional variable names (DATABASE_URL, REDIS_URL, AMQP_URL, ...) and
// instance may be any provision result the catalog produces (DBContainer or
// CatalogInstance). Existing variables are updated so the operation is
// idempotent.
//
// The connection values are stored as sensitive environment variables in the
// environment scope so ResolveEnvironmentVariables serves them to running
// workloads.
func (s *Service) InjectAndInjectConnector(ctx context.Context, kind string, instance any, environmentID string) error {
	if strings.TrimSpace(environmentID) == "" {
		return errors.New("environment id is required")
	}
	connStr, err := resolveConnString(kind, instance)
	if err != nil {
		return err
	}
	entryKey := kind
	if entryKey == "postgresql" {
		entryKey = "postgres"
	}
	return s.writeConnVars(ctx, entryKey, connStr, environmentID)
}

// Attach injects the instance's connection variables into an environment and
// records a catalog_attach_links row. Safe to call repeatedly.
func (s *Service) Attach(ctx context.Context, instanceID, envID string) (*store.CatalogAttachLink, error) {
	if strings.TrimSpace(envID) == "" {
		return nil, errors.New("environment id is required")
	}
	inst, err := s.store.GetCatalogInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get catalog instance: %w", err)
	}
	if inst.ConnString == "" {
		return nil, errors.New("catalog instance has no connection string yet; provision may still be in flight")
	}
	if err := s.writeConnVars(ctx, inst.EntryKey, inst.ConnString, envID); err != nil {
		return nil, err
	}
	_, prefix := envVarsForKind(inst.EntryKey)
	link, err := s.store.CreateCatalogAttachLink(ctx, instanceID, envID, prefix)
	if err != nil {
		return nil, fmt.Errorf("record attach link: %w", err)
	}
	return &link, nil
}

// AttachLinks lists the environments a catalog instance is attached to.
func (s *Service) AttachLinks(ctx context.Context, instanceID string) ([]store.CatalogAttachLink, error) {
	links, err := s.store.ListCatalogAttachLinks(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("list attach links: %w", err)
	}
	return links, nil
}
