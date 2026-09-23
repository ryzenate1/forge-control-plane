# Reference Clustering Rationale

**Repo:** `/Users/riyaz/project/gamepanel/reference`
**Date:** 2026-08-23
**Coordinator:** research-coordinator (Phase 0 inventory)
**Verification:** all 27 checkouts enumerated via `ls reference/*/*`, `git log --oneline -1`, `git rev-parse HEAD`, `go.mod`/`package.json`, `README.md`, `wc -l`

## Inventory (Phase 0 source-verified)

| # | Project | Path | Language | Commit | Branch | Files | Purpose | maturity |
|---|---------|------|----------|--------|--------|-------|---------|----------|
|1|1panel|app-platforms/1panel|Go+Vue|b02cdbc|dev-v2|1703|Single-host VPS panel (sites, apps, DBs, AI/MCP)|2M users, active|
|2|caprover|app-platforms/caprover|Node|56343fc|master|222|Simplest PaaS (CaptainDefinition one-click)|active|
|3|coolify|app-platforms/coolify|PHP/Laravel+Next|e7dff30|v4.x|3013|Self-host Heroku/Nixpacks+Dkr|very active|
|4|docker-compose|app-platforms/docker-compose|Go|efb63f2|main|743|Compose spec impl (not PaaS)|active|
|5|dokku|app-platforms/dokku|Bash+Go|730fa85|master|1616|Mini-Heroku plugins|active|
|6|dokploy|app-platforms/dokploy|TS+Go gRPC|df3965a|canary|1424|PaaS w/ traefik native|very active|
|7|komodo|app-platforms/komodo|Rust+Svelte|5c30102|main|915|Multi-server deploy via periphery agent|active|
|8|portainer|app-platforms/portainer|Go+Angular|a27627d|develop|5286|Largest container management (CE/BE split)|active|
|9|uncloud|app-platforms/uncloud|Go|b99abb7|main|542|Quorum-less WG mesh + p2p clustering|active|
|10|pelican-panel|game-hosting/pelican-panel|PHP/Laravel|b9f0eff|main|2004|Pterodactyl fork panel|active|
|11|pelican-wings|game-hosting/pelican-wings|Go|70f3344|main|195|Pelican daemon (Wings fork)|active|
|12|pterodactyl-panel|game-hosting/pterodactyl-panel|PHP/Laravel|c39a7be|1.0-develop|1518|Canonical game panel|active|
|13|pterodactyl-wings|game-hosting/pterodactyl-wings|Go|e771816|develop|198|Canonical daemon|active|
|14|pufferpanel|game-hosting/pufferpanel|Go|4776a57|v3|1486|Daemon-per-node + JSON templates|maintenance|
|15|pufferpanel-templates|game-hosting/pufferpanel-templates|JSON|3ad7c11|v3|112|Template library|archived|
|16|kopia|backup/kopia|Go|0c15687|master|1281|Encrypted chunk dedup backup|active|
|17|restic|backup/restic|Go|d4088aa|master|1419|Repo packing + index backup|active|
|18|caddy|networking/caddy|Go|93c0721|master|663|Extensible HTTPS-first server (certmagic)|active|
|19|nginx-proxy-manager|networking/nginx-proxy-manager|Node+Vue|a62c2a6|develop|877|Nginx template + certbot 1-page editor|maintenance|
|20|traefik|networking/traefik|Go|14bc52d|master|2282|Dynamic provider+middleware router|very active|
|21|longhorn|large-systems/longhorn|Go|d5d522b|master|424|K8s distributed block store CNCF|incubating|
|22|rancher|large-systems/rancher|Go+Vue|76c28ec|main|3041|K8s cluster fleet manager|active SUSE|
|23|river|operations/river|Go|7cf399a|master|434 (82 top)|Postgres durable queue (pgx)|very active|
|24|incus|orchestration/incus|Go|f51548a|main|1832|System containers+VMs clustering|active LXD fork|
|25|netbird|orchestration/netbird|Go|92a5ed1|main|2486|WireGuard overlay + management/relay|very active|
|26|nomad|orchestration/nomad|Go|d4a17d2|main|6013|Largest orchestrator (job→group→alloc) BUSL|active|
|27|docker-compose|app-platforms/docker-compose|Go|efb63f2|main|743|Spec lib already counted|—|

## Clustering Principle

Goal: **MAXIMUM COMPARATIVE VALUE**, not arbitrary triples. Group by `capability overlap + architecture similarity + Forge relevance`. Two projects solving same problem with divergent architecture in same cluster is valuable (reveals design tradeoffs). Single-purpose outlier gets paired with nearest Forge subsystem.

Chosen clustering (4 phases + 1 large-systems/orchestration merge to avoid thin phase):

### Phase 1 — Application Platform Cluster (9 projects)
**Members:** coolify, dokploy, dokku, caprover, komodo, portainer, 1panel, uncloud, docker-compose (spec as control)
**Why:** All solve `app → container(s) → proxy → domain` lifecycle on own infra. Overlap >70% on application lifecycle, build, container ops, proxy. Architectural divergence is extreme (single-host bash-Dokku vs multi-server Rust-Komodo vs distributed Go-Uncloud vs minimal Node-CapRover vs largest Go-Portainer). This divergence reveals Forge's strongest differentiator (game+app unified scheduling) vs each platform's narrow tradeoff. Highest Forge relevance: this is the app-hosting subsystem (`services/apphosting|compose|deployment|build`).
**Excluded from Phase 1:** nothing — this cluster is Phase 1 because Forge's app-hosting is the most user-visible gap after game hosting.

### Phase 2 — Game Hosting Cluster (6 projects)
**Members:** pelican-panel, pelican-wings, pterodactyl-panel, pterodactyl-wings, pufferpanel, pufferpanel-templates
**Why:** Directly isomorphic to Forge+Beacon (Panel ↔ Forge API, Wings ↔ Beacon). Every game capability (egg/template, allocation, SFTP, console, backup, transfer, schedule, subuser) has a 1:1 Forge mapping to audit for parity/better/worse. Pufferpanel-templates maps to `packages/game-templates`. This cluster is the canonical correctness check for `beacon/internal/server/server.go` vs `wings/server`.
**Size justification:** 6 projects but 2 are panels and 2 are wings (paired), plus templates — effectively 3 systems. Kept together to diff Pterodactyl→Pelican evolution vs Forge.

### Phase 3 — Backup Cluster (2 projects)
**Members:** kopia, restic
**Why:** Only two projects but both are deep Go implementations of the *same* problem (chunked encrypted deduplicated repository) with fundamentally different designs (Kopia: content-addressed blobs+index, Restic: pack files+locks+index). Both map to Forge's `services/backup` + `beacon/internal/backup` (already duplicated inside Forge: `backup/service.go` vs `backup/main_service.go`). 2-project depth is valuable (not breadth) — reveals Forge's missing dedup and required adapter abstraction. Grouped alone to go deep on chunking/encryption/retention/verification/GC/resumability.

### Phase 4 — Networking Cluster (3 projects)
**Members:** caddy, traefik, nginx-proxy-manager
**Why:** Triple solve `domain → cert → proxy → upstream` with three patterns: Caddy=automatic HTTPS+caddyfile, NPM=one-page nginx+certbot, Traefik=dynamic providers+middlewares. Forge scatters the same problem across 7 pages (`endpoints/firewall/load-balancer/traffic/domains/certificates/dns`) and 6 services (`trafficmanager/servicediscovery/crossnode/loadbalancer/domains/acme`). Clustering the three reveals the strongest gateway abstraction (Traefik provider+middleware, Caddy certmagic, NPM UX).

### Phase 5 — Orchestration & Operations Cluster (7 projects)
**Members:** nomad, incus, netbird, longhorn, rancher, river
**Why:** Merged because each is unique and thin alone, but together they test Forge's deepest systems: Nomad→placement/scheduler/autoscaler/drain (6 files comparison), Incus→runtime LXC/KVM + clustering, NetBird→overlay mesh vs Forge's lack of overlay, Longhorn→storage locality/replica vs Forge mounts, Rancher→fleet/cluster vs Forge regions/nodes, River→durable queue vs Forge's duplicated queue+operation. This phase is Architecture/Security/Reliability heavy. NetBird could pair with networking, but its value is higher alongside orchestration’s placement decisions (where overlay changes scheduling). Rancher+Longhorn together test whether Forge should ever become k8s (answer: no, but placement health concept borrowable).

### Rejected alternative groupings

- Splitting 9 app platforms into two phases (e.g., PaaS vs container-mgmt) would *reduce* comparative value: the PaaS-vs-container-mgmt tradeoff is exactly the insight (Forge's docker page vs Portainer). Kept together.
- Pairing Uncloud+NetBird as “mesh” cluster would duplicate networking; NetBird more valuable next to Nomad's placement (where mesh changes node reachability).
- Isolating Docker-Compose as its own phase low value; used as spec-control inside app-platform phase.
- Merging Longhorn+Rancher with app platforms would dilute focus; kept with orchestrators where storage/cluster semantics matter.

## Phase ordering

1. App Platform (largest overlap, most user-visible integration debt, fastest win via Forgefile/gateway)
2. Game Hosting (correctness parity; validates Beacon vs Wings before deeper)
3. Backup (deep technical; needs game-hosting context for artifact sizing)
4. Networking (needs backup context for cert/artifact domain handling)
5. Orchestration+Operations (deepest arch; benefits from all prior capability maps)

Each phase spawns 5 dimension-specialized subagents (lifecycle/git/build/runtime/ux/arch split in Phase 1; adapted per phase).
