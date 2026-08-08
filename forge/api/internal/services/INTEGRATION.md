# Integration Instructions for Lead Agent

## 1. Backup Provider Wiring (in main.go)

```go
import (
    "gamepanel/forge/internal/services/backup"
)

// Initialize backup service
backupSvc := backup.New(store)

// Register storage adapters
switch cfg.Backup.Provider {
case "s3":
    adapter, err := backup.NewS3Adapter(ctx, cfg.S3.Region, cfg.S3.Endpoint, cfg.S3.Bucket,
        cfg.S3.Prefix, cfg.S3.AccessKeyID, cfg.S3.SecretAccessKey, cfg.S3.UsePathStyle)
    if err != nil { /* handle */ }
    backupSvc.RegisterAdapter(adapter)

case "gcs":
    adapter, err := backup.NewGCSAdapter(cfg.GCS.Bucket, cfg.GCS.KeyFile)
    if err != nil { /* handle */ }
    backupSvc.RegisterAdapter(adapter)

case "azure":
    adapter, err := backup.NewAzureAdapter(cfg.Azure.Container, cfg.Azure.ConnectionString)
    if err != nil { /* handle */ }
    backupSvc.RegisterAdapter(adapter)
}

// Alternatively, use ProviderFactory pattern:
backup.RegisterProvider("s3", func(config map[string]string) (backup.StorageAdapter, error) {
    return backup.NewS3Adapter(ctx, config["region"], config["endpoint"], config["bucket"],
        config["prefix"], config["accessKey"], config["secretKey"], true)
})
adapter, err := backup.GetProvider("s3", cfg.Backup.S3)
if err != nil { /* handle */ }
backupSvc.RegisterAdapter(adapter)

// Start backup worker
backupWorker := backup.NewWorker(store, backupSvc, daemonClient)
backupWorker.Start(ctx)
```

## 2. Service Discovery Wiring (in main.go)

```go
import (
    "gamepanel/forge/internal/services/servicediscovery"
)

// Initialize service discovery (depends on store)
discoverySvc := servicediscovery.New(store, eventPublisher)
discoverySvc.Start(ctx)
defer discoverySvc.Stop()

// Register endpoints when nodes come online
discoverySvc.RegisterEndpoint(ctx, servicediscovery.ServiceEndpoint{
    ServiceName:  "nginx",
    ServiceID:    "svc-1",
    NodeID:       "node-1",
    NodeName:     "alpha",
    Address:      netip.MustParseAddr("10.0.0.1"),
    Port:         80,
    Protocol:     servicediscovery.ProtocolTCP,
    Status:       servicediscovery.EndpointStatusHealthy,
})

// Resolve healthy endpoints for a service
endpoints, err := discoverySvc.Resolve(ctx, "nginx", "tenant-1")
```

## 3. Cross-Node Routing Wiring (in main.go)

```go
import (
    "gamepanel/forge/internal/services/crossnode"
    "gamepanel/forge/internal/services/servicediscovery"
    "gamepanel/forge/internal/services/trafficmanager"
)

// Initialize cross-node routing
resolver := crossnode.NewResolver(store)
healthFilter := crossnode.NewHealthFilter(3, 30*time.Second)

// Connect resolver to service discovery for node resolution
resolver.SetServiceDiscovery(discoverySvc)

// Create ingress synchronizer with gateway adapter
var gateway gatewayAdapter // e.g., trafficmanager.NewTraefikReverseProxy()
syncer := crossnode.NewIngressSynchronizer(gateway, resolver, healthFilter, eventPublisher)

// Start periodic ingress sync
syncer.Start(ctx, 30*time.Second)

// Start stale health entry cleanup
healthFilter.StartReaper(ctx, 5*time.Minute)

// Set routing rules
syncer.SetRules(rules)
syncer.SetPolicies(policies)

// Clean shutdown
defer syncer.Stop()
defer healthFilter.StopReaper()
```

## 4. Startup Ordering

1. Store (database connection)
2. Event publisher
3. Daemon client
4. **Service Discovery** (starts reaper goroutine)
5. **Cross-node resolver** (attached to discovery)
6. **Cross-node health filter + reaper**
7. **Backup service** (register adapters)
8. **Backup worker** (scheduled backup loop)
9. **Cross-node ingress synchronizer** (starts sync loop)
10. HTTP server / other services

Shutdown order is reverse of startup.

## 5. Key Types and Interfaces

### Backup
- `backup.StorageAdapter` — interface all providers implement
- `backup.ProviderFactory` — `func(config map[string]string) (StorageAdapter, error)`
- `backup.RegisterProvider(name, factory)` — global provider registry
- `backup.GetProvider(name, config)` — construct adapter from registry

### Service Discovery
- `servicediscovery.Service` — main service with Start/Stop, RegisterEndpoint, Resolve, etc.
- `servicediscovery.Registry` — in-memory endpoint store
- `servicediscovery.StaleEndpointReaper` — marks endpoints unhealthy after TTL
- `servicediscovery.ReachabilityVerifier` — TCP dial-based health check

### Cross-Node
- `crossnode.Resolver` — resolves server/node to target host (can use discovery)
- `crossnode.HealthFilter` — circuit-breaker style health tracking with periodic stale reaper
- `crossnode.IngressSynchronizer` — syncs routing rules to gateway with Start/Stop
- `crossnode.RouteGroup` — groups rules by domain+path for backend merging
