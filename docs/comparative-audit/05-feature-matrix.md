# Feature Matrix

| Feature | GamePanel | Pterodactyl | Pelican | PufferPanel | Wings | Best | Evidence |
|---------|:---------:|:-----------:|:-------:|:-----------:|:-----:|:----:|----------|
| **Authentication** |
| Email/password login | Complete | Complete | Complete | Complete | N/A | Tie | All panels implement |
| OAuth2 (client credentials) | Complete | Missing | Missing | Complete | N/A | GP/PP | forge/api/handlers_oauth2.go |
| Session versioning | Complete | Missing | Missing | Missing | N/A | **GP** | store.go session_version |
| Encrypted TOTP secrets | Complete | Missing | Missing | Missing | N/A | **GP** | secrets/keyring.go |
| WebAuthn/Passkeys | Missing | Missing | Missing | Complete | N/A | **PP** | pufferpanel/oauth2/ |
| SSH key management | Complete | Missing | Missing | Missing | N/A | **GP** | migration 018 |
| 2FA (TOTP) | Complete | Complete | Complete | Complete | N/A | Tie | |
| Account sessions (view/revoke) | Complete | Missing | Missing | Missing | N/A | **GP** | migration 053 |
| Password reset tokens | Complete | Complete | Complete | Complete | N/A | Tie | |
| **Server Management** |
| Create/start/stop/restart/delete | Complete | Complete | Complete | Complete | N/A | Tie | |
| Server transfer (node-to-node) | Complete | Missing | Missing | Missing | N/A | **GP** | migrations 004-006, 045 |
| Server templates | Complete | Complete (Eggs) | Complete (Eggs) | Complete (Eggs) | N/A | Pterodactyl | Community ecosystem |
| Startup variables | Complete | Complete | Complete | Complete | N/A | Tie | migration 013 |
| Docker labels | Missing | Complete | Complete | Missing | N/A | Pterodactyl | |
| Install scripts | Partial | Complete | Complete | Complete | N/A | Pterodactyl | Beacon installer basic |
| File denylist | Missing | Complete | Complete | Missing | N/A | Pterodactyl | |
| Soft deletes | Missing | Complete | Complete | Complete | N/A | Pterodactyl/PP | |
| Subuser system | Complete | Complete | Complete | Complete | N/A | Tie | migration 016 |
| Subuser invitations | Complete | Missing | Missing | Missing | N/A | **GP** | migration 050 |
| **Node Management** |
| Node registration | Complete | Complete | Complete | N/A | N/A | Tie | |
| Heartbeat monitoring | Complete | Basic | Basic | N/A | N/A | **GP** | migration 010, heartbeatmonitor |
| Heartbeat expiry engine | Complete | Missing | Missing | N/A | N/A | **GP** | migration 025 |
| Placement reservations | Complete | Missing | Missing | N/A | N/A | **GP** | migration 026 |
| Node evacuation | Complete | Missing | Missing | N/A | N/A | **GP** | migration 022 |
| Recovery coordinator | Complete | Missing | Missing | N/A | N/A | **GP** | migration 027 |
| Migration engine | Complete | Missing | Missing | N/A | N/A | **GP** | migration 023 |
| Multi-region | Complete | Missing | Missing | N/A | N/A | **GP** | migration 020 |
| **Database** |
| Database provisioning | Complete | Complete | Complete | Complete | N/A | Tie | |
| Multiple DB hosts | Complete | Complete | Complete | Complete | N/A | Tie | |
| Multi-database (SQLite/MySQL/PG) | PostgreSQL only | MySQL | MySQL | Multi | N/A | **PP** | |
| Schema migrations | Complete (53 SQL) | Complete (195 PHP) | Complete (242 PHP) | Partial (GORM) | N/A | GP/Pterodactyl | |
| Migration CI validation | Complete | Missing | Missing | Missing | N/A | **GP** | validate-api-migrations.sh |
| **File Management** |
| Upload/download | Complete | Complete | Complete | Complete | N/A | Tie | |
| SFTP | Complete | Complete | Complete | Complete | Complete | Tie | |
| File copy/chmod/archive | Complete | Missing | Missing | Missing | N/A | **GP** | handlers_servers.go |
| Remote file pull | Complete | Missing | Missing | Missing | N/A | **GP** | handlers_remote.go |
| File staging with rollback | Complete | Missing | Missing | Missing | N/A | **GP** | secure_files.go |
| Disk quota | Complete | Missing | Missing | Missing | N/A | **GP** | store_servers.go |
| Archive validation (zip bombs) | Complete | Missing | Missing | Missing | N/A | **GP** | secure_files.go |
| Download tickets | Complete | Missing | Missing | Missing | N/A | **GP** | handlers_file_download.go |
| **WebSocket/Console** |
| Console log streaming | Complete | Complete | Complete | Complete | Complete | Tie | |
| WS ticket auth | Complete | Missing | Missing | Missing | N/A | **GP** | handlers_ws_ticket.go |
| Console throttle | Complete | Missing | Missing | Missing | N/A | **GP** | console.go |
| Console replay buffer | Complete | Missing | Missing | Missing | N/A | **GP** | console.go (128 entries/256KB) |
| Startup detection (regex) | Missing | Complete | Complete | Missing | Complete | Pterodactyl/Wings | |
| **Backups** |
| Local backups | Complete | Complete | Complete | Complete | Complete | Tie | |
| S3 backups | Complete | Complete | Complete | Missing | N/A | GP/Pterodactyl | |
| Backup locking | Complete | Missing | Missing | Missing | N/A | **GP** | migration 049 |
| Auto backup retention | Complete | Missing | Missing | Missing | N/A | **GP** | store_backups.go |
| Crash recovery journal | Complete | Missing | Missing | Missing | N/A | **GP** | backup/local.go |
| Backup download tickets | Complete | Missing | Missing | Missing | N/A | **GP** | handlers_file_download.go |
| Backup rate limiting | Missing | Missing | Missing | Missing | Complete | **Wings** | Wings backup limiter |
| **Scheduling** |
| Cron/task scheduling | Complete | Complete | Complete | Complete | N/A | Tie | |
| Lease-based execution | Complete | Missing | Missing | Missing | N/A | **GP** | schedule_runner.go |
| Schedule run history | Complete | Missing | Missing | Missing | N/A | **GP** | migration 009 |
| Timezone support | Complete | Complete | Complete | Missing | N/A | GP/Pterodactyl | migration 052 |
| **Security** |
| CSRF protection | Complete | Complete | Complete | Missing | N/A | GP/Pterodactyl | middleware_csrf.go |
| Rate limiting | Complete | Complete | Complete | Missing | Complete | GP/Pterodactyl | middleware_ratelimit.go |
| IP access control | Complete | Missing | Missing | Missing | N/A | **GP** | middleware_ipaccess.go |
| CSP headers | Complete | Missing | Missing | Missing | N/A | **GP** | middleware_security.go |
| DNS pinning (SSRF) | Complete | Missing | Missing | Missing | N/A | **GP** | handlers_remote.go |
| Encryption at rest | Complete | Missing | Missing | Missing | N/A | **GP** | secrets/keyring.go |
| Archive bomb protection | Complete | Missing | Missing | Missing | N/A | **GP** | secure_files.go |
| Panic recovery | Missing | N/A | N/A | N/A | Complete | **Wings** | wings router middleware |
| CORS | Missing | Complete | Complete | Complete | N/A | Pterodactyl | |
| **Observability** |
| Prometheus metrics | Complete | Missing | Missing | Missing | N/A | **GP** | infra/prometheus.yml |
| Grafana dashboards | Complete | Missing | Missing | Missing | N/A | **GP** | infra/grafana/ |
| Alertmanager | Complete | Missing | Missing | Missing | N/A | **GP** | infra/alertmanager.yml |
| Audit logging | Complete | Complete | Complete | Missing | N/A | GP/Pterodactyl | store_audit.go |
| **Deployment** |
| Docker | Complete | Complete | Complete | Complete | Complete | Tie | |
| Docker Compose | Complete | Complete | Complete | Missing | Complete | GP/Pterodactyl | |
| Docker HEALTHCHECK | Missing | Missing | Complete | Missing | N/A | **Pelican** | Pelican Dockerfile |
| CI/CD pipeline | Complete | Complete | Complete | Missing | Complete | GP/Pterodactyl | |
| Migration validation in CI | Complete | Missing | Missing | Missing | N/A | **GP** | validate-api-migrations.sh |
| Non-root Docker | Missing | Complete | Complete | Complete | Complete | Pterodactyl+ | |
