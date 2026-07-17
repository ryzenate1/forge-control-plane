# Goal (Architecture / Stack Decisions)

Modern prototype game panel (Pterodactyl-inspired) optimized for:
- low memory usage
- realtime workflows (console/stats/logs)
- infra-focused learning + internship showcase
- AI-assisted development and fast iteration

## Final stack
- Frontend: Next.js + TypeScript + Tailwind + shadcn/ui + Zustand
- Backend API: Go + Fiber
- Daemon: Go (standard library HTTP/WebSocket) + Docker SDK behind a runtime interface
- Container runtime: Docker first (abstract later for containerd/Podman)
- Database: PostgreSQL
- Cache/queue/pubsub: Redis
- Web IDE: Monaco
- File transfers: streaming + chunked uploads; SFTP + rsync over SSH/SFTP
- Monitoring: Prometheus + Grafana
- Reverse proxy: Nginx (optional for production)
- Python: microservices only (installers/automation/analytics/AI helpers), not core orchestration

## File system principle
Frontend never touches host FS directly.\n+Flow: Frontend → API → Daemon → jailed server directory / container filesystem