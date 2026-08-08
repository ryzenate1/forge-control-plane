# Infrastructure as Code

Production deployment configurations for GamePanel.

## 📁 Configuration Files

- [Caddyfile](Caddyfile) - Development Caddy configuration
- [Caddyfile.production](Caddyfile.production) - Production Caddy configuration
- [Caddyfile.production.example](Caddyfile.production.example) - Example production Caddy configuration
- [compose.yml](compose.yml) - Base Docker Compose
- [compose.production.yml](compose.production.yml) - Production overrides
- [compose.beacon.yml](compose.beacon.yml) - Beacon service configuration
- [compose.caddy.production.yml](compose.caddy.production.yml) - Caddy production configuration
- [compose.logging.yml](compose.logging.yml) - Logging configuration
- [compose.override.yml](compose.override.yml) - Development overrides
- [compose.realtime.yml](compose.realtime.yml) - Real-time services configuration
- [compose.secrets.yml](compose.secrets.yml) - Secrets configuration
- [compose.security.yml](compose.security.yml) - Security configuration
- [compose.smoke.yml](compose.smoke.yml) - Smoke test configuration
- [compose.tls.yml](compose.tls.yml) - TLS configuration

## 🏗️ Components

- [alertmanager/](alertmanager/) - Alert management configuration
- [alertmanager.yml](alertmanager.yml) - Alertmanager configuration file
- [ci/](ci/) - Continuous integration configurations
- [grafana/](grafana/) - Monitoring dashboards
- [prometheus/](prometheus/) - Metrics collection
- [prometheus.yml](prometheus.yml) - Prometheus configuration
- [secrets/](secrets/) - Secret management
- [ship/](ship/) - Deployment scripts
- [traefik/](traefik/) - Reverse proxy configuration
- [nginx.conf](nginx.conf) - Nginx configuration

## 🚀 Quick Start

### Development
```bash
docker compose -f compose.yml up -d
```

### Production
```bash
docker compose -f compose.yml -f compose.production.yml up -d
```

### With Caddy (Production)
```bash
docker compose -f compose.yml -f compose.production.yml -f compose.caddy.production.yml up -d
```
