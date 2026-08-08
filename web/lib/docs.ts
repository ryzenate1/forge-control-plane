export type DocCallout = { tone: "note" | "warning" | "danger" | "success"; title: string; body: string };
export type DocSection = { heading: string; body?: string[]; code?: { label: string; value: string }; callout?: DocCallout; bullets?: string[]; table?: { headers: string[]; rows: string[][] } };
export type DocPage = { slug: string; group: string; title: string; summary: string; status?: string; sections: DocSection[] };

export const docs: DocPage[] = [
  {
    slug: "introduction", group: "Getting Started", title: "Introduction to Forge Plane", summary: "Forge Plane is a modern, self-hosted control plane for managing game servers, applications, databases, and infrastructure across one or more nodes.",
    sections: [
      { heading: "What is in the control plane", body: ["Forge Web is the browser interface. Forge API persists desired state, runs API-facing services, and coordinates operations. PostgreSQL is the durable source of truth. Redis supports runtime coordination. The supplied Compose stack also includes monitoring, backups, and a reverse proxy."] },
      { heading: "What runs on a node", body: ["Beacon is the per-node execution agent. It receives authenticated work from Forge, uses the local Docker socket, stores workload files in its data directory, serves SFTP, reports health and observations, and retains no independent placement authority."] },
      { heading: "Choose a deployment", table: { headers: ["Model", "Use it when", "Reality"], rows: [["One VPS", "Learning, small installations, first TCP game server", "Web, API, PostgreSQL, Redis, Caddy, and one Beacon share one host."], ["Control plane + nodes", "Production workloads or recovery", "Keep Forge state on one host and deploy a Beacon Compose stack to every workload host."], ["Multiple nodes", "Evacuation or restore testing", "At least two healthy Beacons plus a shared backup destination are required."]] } },
      { heading: "Project status", callout: { tone: "warning", title: "Operate from verified capabilities", body: "The dashboard exposes a broad platform surface. Treat load balancing, evacuation, recovery, and cloud provisioning as operational features that need a tested topology and shared storage; do not assume automatic failover from their presence in the interface." } },
      { heading: "What is Forge Plane?", body: ["Forge Plane is a complete control plane solution that provides everything you need to manage game servers, applications, and infrastructure at scale.", "The system consists of two main components: the Forge Control Plane (web dashboard + API) and Beacon (node agent). Together, they provide a unified interface for deploying, managing, and monitoring workloads across your infrastructure."] },
      { heading: "Core Components", table: { headers: ["Component", "Technology", "Purpose"], rows: [["Forge Web", "Next.js 15 + React 19", "Responsive browser interface for administration"], ["Forge API", "Go 1.26 + Fiber", "REST API, authentication, orchestration, and persistence"], ["PostgreSQL", "PostgreSQL 16", "Durable source of truth for all control plane data"], ["Redis", "Redis 7", "Runtime coordination, caching, and session management"], ["Beacon", "Go 1.26", "Per-node agent for Docker workload execution"], ["Caddy", "Go (Caddy 2)", "TLS termination and reverse proxy"]] } },
      { heading: "What Forge Plane Manages", bullets: ["Game servers (Minecraft, Valheim, CS2, and any Docker-based game)", "Container applications with Git-based deployments", "Docker Compose stacks", "Managed databases (PostgreSQL, MySQL, MongoDB, Redis, etc.)", "Network allocations (TCP/UDP ports)", "Load balancing and traffic routing", "Backups (local and S3-compatible)", "Cloud node provisioning (AWS EC2)", "Multi-node orchestration and failover"] },
      { heading: "Deployment Models", table: { headers: ["Model", "Description", "Use Case"], rows: [["All-in-One", "Control plane + Beacon + PostgreSQL + Redis on one host", "Development, testing, small deployments"], ["Control Plane + Nodes", "Separate control plane host with multiple Beacon nodes", "Production workloads with high availability"], ["Multi-Node Cluster", "Control plane with multiple Beacon nodes across different locations", "Large-scale deployments with evacuation and recovery capabilities"], ["Cloud Provisioning", "Automatic AWS EC2 node provisioning with Beacon bootstrap", "Cloud-native deployments with auto-scaling"]] } },
      { heading: "Project Status", callout: { tone: "success", title: "Production Ready", body: "Forge Plane is production-ready with Docker Compose deployment. TCP/UDP allocations, integrated L4 proxy, multi-node evacuation, shared-backup recovery, and AWS Beacon bootstrap are fully implemented and tested." } }
    ],
  },
  {
    slug: "quick-start", group: "Getting Started", title: "Quick Start Guide", summary: "Get Forge Plane running in minutes with our automated installer.",
    sections: [
      { heading: "Prerequisites", bullets: ["Ubuntu 22.04 LTS, 24.04 LTS, or Debian 12 (recommended)", "Docker Engine 24.0+ and Docker Compose v2 plugin (2.24.4+)", "Git and curl", "A domain name with DNS control", "Minimum: 2 vCPU, 2 GiB RAM, 20 GiB free disk"] },
      { heading: "One-Command Installation", code: { label: "Automated production installation", value: `git clone https://github.com/ryzenate1/forge-control-plane.git
cd forge-control-plane
./scripts/install.sh` } },
      { heading: "Manual Installation", code: { label: "Step-by-step setup", value: `git clone https://github.com/ryzenate1/forge-control-plane.git
cd forge-control-plane
cp infra/.env.example infra/.env
chmod 600 infra/.env
cd infra
docker compose -f compose.yml -f compose.production.yml --env-file .env up -d --build` } },
      { heading: "Access Forge Plane", body: ["After installation, access the web dashboard at your configured domain.", "The first-run setup will guide you through creating an administrator account."], table: { headers: ["Service", "Access Point"], rows: [["Web Dashboard", "https://your-domain.com"], ["API", "https://your-domain.com/api/v1"], ["PostgreSQL", "localhost:5432 (internal)"], ["Redis", "localhost:6379 (internal)"]] } },
      { heading: "Verify Installation", code: { label: "Health check", value: `./scripts/healthcheck.sh
cd infra && docker compose -f compose.yml -f compose.production.yml --env-file .env ps
curl -k https://localhost/api/health` } }
    ],
  },
  {
    slug: "requirements", group: "Getting Started", title: "System Requirements", summary: "Complete hardware, software, and network requirements for Forge Plane.",
    sections: [
      { heading: "Platform support", table: { headers: ["Platform", "Status", "Evidence"], rows: [["Ubuntu 22.04 LTS", "Supported", "Listed by scripts/install.sh."], ["Ubuntu 24.04 LTS", "Supported", "Listed by scripts/install.sh."], ["Debian 12", "Supported", "Listed by scripts/install.sh."], ["macOS 14-15", "Development only", "Recognized by the installer; do not use as a production Beacon host."], ["Other Linux", "Not tested", "No installer claim is made."], ["Windows", "Not supported for host deployment", "No supported installer path is supplied."]] } },
      { heading: "Host prerequisites", bullets: ["Docker Engine and the Docker Compose v2 plugin.", "A non-root operator account able to run Docker commands.", "A domain and public DNS control for public TLS.", "Outbound HTTPS access for image pulls, certificate issuance, Git, and optional object storage.", "A persistent disk for PostgreSQL and Beacon workload files."] },
      { heading: "Check Docker before installation", code: { label: "Host verification", value: `docker info
docker compose version` } },
      { heading: "Security boundary", callout: { tone: "danger", title: "Do not publish internal services", body: "PostgreSQL, Redis, the Beacon API, monitoring dashboards, and Caddy's admin API are not public panel endpoints. In the production override they bind to loopback; expose only the proxy and deliberately allocated game ports." } },
      { heading: "Operating System Support", table: { headers: ["OS", "Version", "Architectures", "Docker", "Status"], rows: [["Ubuntu", "24.04 LTS (Noble)", "amd64, arm64", "24.0+", "Fully Supported"], ["Ubuntu", "22.04 LTS (Jammy)", "amd64, arm64", "24.0+", "Fully Supported"], ["Debian", "12 (Bookworm)", "amd64, arm64", "24.0+", "Fully Supported"], ["macOS", "14+ (Sonoma)", "amd64, arm64", "Docker Desktop", "Development Only"], ["Ubuntu", "20.04 LTS (Focal)", "amd64", "24.0+", "Untested"]] } },
      { heading: "Control Plane Requirements", table: { headers: ["Deployment Type", "vCPU", "RAM", "Storage", "Notes"], rows: [["Small all-in-one (panel + few servers)", "4", "8 GiB", "80 GiB SSD", "Good for testing and small deployments"], ["Dedicated control plane", "2-4", "4-8 GiB", "40 GiB SSD", "Control plane only, no game workloads"], ["Beacon game node", "Varies", "1 GiB + game needs", "Depends on games", "Reserve 1 GiB for OS and Beacon overhead"]] } },
      { heading: "Software Dependencies", bullets: ["Docker Engine 24.0 or newer", "Docker Compose v2 plugin (2.24.4+)", "Git 2.x", "curl and ca-certificates"] },
      { heading: "Development Environment", bullets: ["Go 1.26 or newer", "Node.js 20 LTS or newer", "npm (included with Node.js)", "Docker Desktop (macOS/Windows) or Docker Engine (Linux)", "Recommended: 4 CPU cores, 8 GiB RAM, 20 GiB free disk"] },
      { heading: "Network Requirements", bullets: ["Public domain with A/AAAA DNS records", "Ports 80 and 443 open for web traffic", "Game ports (TCP/UDP) as configured in allocations", "Outbound HTTPS for Docker image pulls, certificate issuance, and backups", "Internal ports: 8080 (API), 3000 (Web), 5432 (PostgreSQL), 6379 (Redis), 9090 (Beacon), 2022 (SFTP)"] },
      { heading: "Storage Considerations", body: ["Game workloads consume most memory and storage. Size your nodes for peak player load, backup storage requirements, world growth over time, container image cache, and log retention."] },
      { heading: "Verify Your System", code: { label: "Pre-installation checks", value: `docker --version
docker compose version
free -h
df -h
uname -m` } }
    ],
  },
  {
    slug: "installation-methods", group: "Installation", title: "Installation Methods", summary: "Multiple ways to install Forge Plane based on your needs.",
    sections: [
      { heading: "Automated Installer", body: ["The recommended way to install Forge Plane is the automated installer script. It handles OS detection, dependency verification, configuration generation, and service startup."], code: { label: "Automated install", value: `git clone https://github.com/ryzenate1/forge-control-plane.git
cd forge-control-plane
./scripts/install.sh` } },
      { heading: "Docker Compose Manual Install", body: ["For advanced users who want full control over the installation process, manual Docker Compose deployment provides visibility into every configuration step."], code: { label: "Manual Compose deploy", value: `git clone https://github.com/ryzenate1/forge-control-plane.git
cd forge-control-plane
cp infra/.env.example infra/.env
chmod 600 infra/.env
cd infra
docker compose -f compose.yml -f compose.production.yml --env-file .env up -d --build` } },
      { heading: "Standalone Beacon Installation", body: ["To add a worker node to an existing control plane, deploy the standalone Beacon Compose stack on the target host. This requires a pre-registered node ID and token from the dashboard."], code: { label: "Beacon node install", value: `git clone https://github.com/ryzenate1/forge-control-plane.git
cd forge-control-plane/infra
cp ../beacon/.env.example .env
chmod 600 .env
./bootstrap-beacon.sh` } },
      { heading: "Cloud Provisioning", body: ["Forge Plane can automatically provision AWS EC2 instances and bootstrap them as Beacon nodes. This feature requires AWS credentials configured in the dashboard and an EC2 key pair."], table: { headers: ["Provider", "Status", "Requirements"], rows: [["AWS EC2", "Implemented", "AWS credentials, key pair, security group, subnet"], ["DigitalOcean", "Planned", "N/A"], ["Hetzner", "Planned", "N/A"], ["Linode", "Planned", "N/A"]] } },
      { heading: "Post-Installation Steps", bullets: ["Complete the first-run setup wizard and create your administrator account", "Register a Beacon node from the dashboard", "Configure DNS records and TLS certificates", "Configure backup storage (local or S3-compatible)", "Create your first game server or application"] }
    ],
  },
  {
    slug: "panel-installation", group: "Installation", title: "Panel Installation Guide", summary: "Separate the control plane installation from Beacon — start here to deploy Forge API, Web, database, and proxy.",
    sections: [
      { heading: "What the Panel Includes", body: ["The control plane panel consists of Forge Web (Next.js 15), Forge API (Go + Fiber), PostgreSQL 16, Redis 7, Caddy reverse proxy, Prometheus/Grafana monitoring, and the postgres-backup sidecar. This is the administrative hub."] },
      { heading: "Prerequisites", bullets: ["Ubuntu 22.04 LTS, Ubuntu 24.04 LTS, or Debian 12", "Docker Engine 24.0+ with Compose v2 plugin (2.24.4+)", "A domain with DNS pointing to the server IP", "Minimum 2 vCPU, 4 GiB RAM, 40 GiB SSD", "Ports 80 and 443 reachable from the internet"] },
      { heading: "Automated Panel Installation", code: { label: "Run the installer", value: `git clone https://github.com/ryzenate1/forge-control-plane.git
cd forge-control-plane
./scripts/install.sh` } },
      { heading: "Manual Panel Installation", code: { label: "Step-by-step", value: `git clone https://github.com/ryzenate1/forge-control-plane.git
cd forge-control-plane
cp infra/.env.example infra/.env
chmod 600 infra/.env
nano infra/.env
cd infra
docker compose -f compose.yml -f compose.production.yml --env-file .env up -d --build` } },
      { heading: "Generated Configuration", body: ["The installer generates an infra/.env file with all required secrets, including database passwords, API keys, encryption master keys, and Beacon tokens. The Beacon daemon runs as a service alongside the panel in all-in-one mode."] },
      { heading: "Required Environment Variables", table: { headers: ["Variable", "Purpose", "Generated By"], rows: [["POSTGRES_PASSWORD", "PostgreSQL authentication", "install.sh or manual"], ["DATABASE_URL", "PostgreSQL connection string", "install.sh or manual"], ["REDIS_PASSWORD", "Redis authentication", "install.sh or manual"], ["API_AUTH_SECRET", "JWT signing secret", "install.sh or manual"], ["APP_KEY", "Application encryption key", "install.sh or manual"], ["FORGE_MASTER_KEY", "At-rest encryption master key", "install.sh or manual"], ["DAEMON_NODE_TOKEN", "Built-in Beacon credential", "install.sh or manual"], ["DAEMON_NODE_ID", "Built-in Beacon node UUID", "install.sh or manual"]] } },
      { heading: "First-Run Setup", body: ["After installation, navigate to https://your-domain.com/setup. The setup wizard will prompt you to create the administrator account and configure the panel name and timezone."] },
      { heading: "Verifying Panel Health", code: { label: "Check services", value: `cd infra && docker compose -f compose.yml -f compose.production.yml --env-file .env ps
./scripts/healthcheck.sh
curl http://127.0.0.1:8080/api/v1/health/ready` } }
    ],
  },
  {
    slug: "docker-compose", group: "Installation", title: "Docker Compose Configuration", summary: "Understand the Docker Compose files and how to customize them.",
    sections: [
      { heading: "Compose File Structure", body: ["Forge Plane uses a layered Docker Compose configuration. The base compose.yml defines all services, while override files add production settings, TLS, and Beacon-only deployment."], table: { headers: ["File", "Purpose"], rows: [["compose.yml", "Base configuration with all services defined"], ["compose.production.yml", "Production overrides: loopback bindings, restart policies, port ranges"], ["compose.beacon.yml", "Standalone Beacon node deployment for worker hosts"], ["compose.caddy.production.yml", "Caddy reverse proxy TLS configuration"], ["compose.tls.yml", "Traefik ACME TLS termination (alternative)"]] } },
      { heading: "Environment Configuration", body: ["All runtime configuration is stored in infra/.env. This file contains database credentials, API secrets, master keys, Beacon tokens, and deployment URLs. It must be kept secure and never committed to version control."], code: { label: "Environment file setup", value: `cp infra/.env.example infra/.env
chmod 600 infra/.env` } },
      { heading: "Service Dependencies", body: ["Services are started in dependency order. Forge API waits for PostgreSQL and Redis to be healthy. Forge Web waits for the API. Beacon operates independently."], code: { label: "Dependency graph", value: `PostgreSQL -> Redis -> Forge API -> Forge Web
                         -> Beacon (standalone or built-in)` } },
      { heading: "Resource Limits", table: { headers: ["Service", "CPU Limit", "Memory Limit"], rows: [["Forge API", "2 cores", "512 MiB"], ["Forge Web", "1 core", "512 MiB"], ["PostgreSQL", "2 cores", "1 GiB"], ["Redis", "1 core", "256 MiB"], ["Beacon", "2 cores", "256 MiB"], ["Caddy", "0.5 cores", "256 MiB"]] } },
      { heading: "Production Port Bindings", body: ["In compose.production.yml, all internal services bind to 127.0.0.1 (loopback). Caddy binds to 0.0.0.0:80 and 0.0.0.0:443 for public access. The load balancer range (30000-30100) is published on all interfaces."] }
    ],
  },
  {
    slug: "tls-configuration", group: "Installation", title: "TLS and Reverse Proxy Configuration", summary: "Configure HTTPS and reverse proxy for secure access to Forge Plane.",
    sections: [
      { heading: "DNS Prerequisites", body: ["Before configuring TLS, ensure your domain DNS is properly configured. Create an A record pointing to your control plane server's IPv4 address."], code: { label: "Verify DNS resolution", value: `dig +short panel.example.com A` } },
      { heading: "Caddy Configuration", body: ["Caddy is the default reverse proxy with automatic HTTPS via Let's Encrypt. Configure your domain in the Caddyfile."], code: { label: "Enable Caddy", value: `cp infra/Caddyfile.production.example infra/Caddyfile.production
nano infra/Caddyfile.production
cd infra
docker compose -f compose.yml -f compose.production.yml -f compose.caddy.production.yml --env-file .env up -d` } },
      { heading: "Traefik Configuration", body: ["As an alternative, Traefik can be used with the compose.tls.yml override."], code: { label: "Enable Traefik TLS", value: `cd infra
docker compose -f compose.yml -f compose.production.yml -f compose.tls.yml --env-file .env up -d` } },
      { heading: "Firewall Configuration", table: { headers: ["Port", "Protocol", "Purpose", "Required"], rows: [["80", "TCP", "HTTP (redirect + ACME challenge)", "Yes"], ["443", "TCP", "HTTPS (web dashboard + API)", "Yes"], ["2022", "TCP", "SFTP (Beacon file access)", "Optional"], ["9090", "TCP", "Beacon API (private)", "No (keep internal)"]] } },
      { heading: "Certificate Troubleshooting", callout: { tone: "warning", title: "HTTP-01 needs a reachable port 80", body: "Allow TCP 80 and 443 to the proxy. A failed certificate is commonly caused by incorrect DNS, an occupied port 80, an unreachable IPv6 address, or a CDN/proxy mode that prevents the challenge from reaching your server." } }
    ],
  },
  {
    slug: "architecture", group: "Architecture", title: "Architecture Deep Dive", summary: "Understand how Forge Plane components work together to manage your infrastructure.",
    sections: [
      { heading: "Complete System Architecture", body: ["A browser reaches Forge Web through the public proxy. Web calls Forge API. The API stores state and migrations in PostgreSQL, uses Redis where configured, and coordinates commands with Beacon. Beacon controls the Docker runtime and its local workload files. Backup artifacts can be local or S3-compatible; multi-node restore needs a repository reachable from every eligible node."] },
      { heading: "Request and Command Flows", table: { headers: ["Flow", "Path"], rows: [["Browser request", "Browser -> Caddy -> Forge Web; /api traffic is proxied to Forge API."], ["Game start", "Dashboard/API -> stored operation -> Beacon request -> Docker/container/files -> observation returned to Forge."], ["Application deployment", "Dashboard/API -> deployment record -> Beacon build -> health observation -> deployment status."], ["Backup and restore", "Beacon reads workload data -> backup adapter -> local or S3-compatible repository -> destination Beacon restores."]] } },
      { heading: "Text Topology", code: { label: "Architecture at a glance", value: `Browser
  | HTTPS 443
  v
Caddy --- Forge Web :3000
  | /api
  v
Forge API :8080 --- PostgreSQL :5432 (private)
  |                 Redis :6379 (private)
  | authenticated commands / observations
  v
Beacon :9090 --- Docker socket --- workload containers
  \`-- SFTP :2022 --- node data directory` } },
      { heading: "Data Flow", body: ["State changes flow from the dashboard through the API to PostgreSQL. Beacon agents poll for pending work, execute it via Docker, and report results back."], table: { headers: ["Direction", "Data", "Protocol"], rows: [["Dashboard -> API", "Configuration, deployment requests", "HTTPS (REST JSON)"], ["API -> PostgreSQL", "Persistent state, migrations", "TCP (PostgreSQL protocol)"], ["Beacon -> API", "Health, status observations", "HTTPS (authenticated)"], ["API -> Beacon", "Work commands via polling", "HTTPS"], ["Beacon -> Docker", "Container lifecycle operations", "Unix socket (Docker API)"]] } },
      { heading: "Security Architecture", bullets: ["All external communication uses HTTPS", "API authentication via JWT tokens and API keys", "Beacon authentication via pre-shared node tokens", "Internal services (PostgreSQL, Redis, Beacon API) bind to loopback in production", "Secrets stored in environment files with restricted permissions (chmod 600)"] },
      { heading: "High Availability Design", body: ["Forge Plane achieves high availability through stateless API and Web services that can be scaled horizontally behind the proxy. PostgreSQL provides the durable state layer. Beacon nodes operate independently and reconnect automatically after API interruptions."] }
    ],
  },
  {
    slug: "beacon-architecture", group: "Architecture", title: "Beacon Node Agent Architecture", summary: "Deep dive into how Beacon works as the execution agent on each node.",
    sections: [
      { heading: "Beacon Responsibilities", bullets: ["Execute Docker containers for game servers, applications, and databases", "Manage workload files via SFTP and direct file operations", "Report node health, resource usage, and workload status to Forge API", "Run scheduled tasks and cron jobs locally", "Execute backup and restore operations using configured adapters"] },
      { heading: "Internal Architecture", body: ["Beacon is a single Go binary built from cmd/daemon/main.go. It exposes an HTTP API for command and control, an SFTP server for file access, and communicates with the local Docker socket (via docker-socket-proxy for security) for container management.", "The binary accepts one CLI flag: --healthcheck for Docker health checks. All configuration comes from environment variables (DAEMON_* prefix) and an optional YAML config file specified by DAEMON_CONFIG_FILE."], code: { label: "Beacon directory structure", value: `/srv/game-panel/servers/
  <server-uuid>/
    .config/
      server.json      # Workload configuration
    <files>             # Workload files mounted into containers
  .beacon/
    backups/            # Local backup artifacts
    beacon.db           # Local state database` } },
      { heading: "Work Execution Flow", table: { headers: ["Step", "Description"], rows: [["1. Poll", "Beacon polls Forge API for pending work assignments"], ["2. Receive", "Beacon receives work details including image, environment, and mounts"], ["3. Prepare", "Beacon pulls images and prepares files and configuration"], ["4. Execute", "Beacon creates or updates Docker containers"], ["5. Observe", "Beacon monitors container health and resource usage"], ["6. Report", "Beacon sends observations back to Forge API"]] } },
      { heading: "Authentication and Security", body: ["Beacon authenticates to Forge API using a pre-shared node token (DAEMON_NODE_TOKEN). All communication is over HTTPS. The Beacon API itself binds to a configurable address (DAEMON_ADDR, default :9090) and should not be exposed publicly.", "Legacy Wings environment variables (WINGS_TOKEN, WINGS_TOKEN_ID, WINGS_NODE_ID, WINGS_PANEL_URL) are supported as fallbacks."], callout: { tone: "warning", title: "Treat the Docker socket as root-equivalent", body: "Beacon mounts /var/run/docker.sock (read-only via proxy). Limit who can administer its host and do not expose its API publicly." } },
      { heading: "Heartbeat and Edge Connectivity", body: ["Beacon runs a heartbeat loop every 30 seconds, reporting the node's OS, architecture, CPU count, memory, disk space, runtime status, and uptime to the panel. An additional edge agent maintains a persistent connection with 15-second heartbeats and exponential backoff reconnection (1s initial, 60s max)."], code: { label: "Heartbeat payload fields", value: `Version, OS, Architecture, CPUThreads, MemoryMB, DiskMB,
RuntimeStatus, RuntimeProvider, Error, Uptime, LoadAverage` } },
      { heading: "When Forge Is Unavailable", body: ["Already-running containers continue under Docker. New control-plane actions, state updates, and centralized observations cannot complete until the API and PostgreSQL return. A node cannot perform control-plane scheduling on its own. The edge agent will attempt reconnection with exponential backoff."] }
    ],
  },
  {
    slug: "first-steps", group: "Administration", title: "First Steps After Installation", summary: "Complete these essential tasks to get started with Forge Plane.",
    sections: [
      { heading: "Complete First-Run Setup", body: ["After accessing the web dashboard for the first time, you will be guided through the initial setup wizard. This creates your administrator account and configures basic system settings."], bullets: ["Set your admin email and password", "Configure the panel name and timezone", "Review and confirm system settings"] },
      { heading: "Register Your First Node", body: ["Before deploying any workloads, register at least one Beacon node. From the dashboard, navigate to Nodes and create a new node record."], code: { label: "Node registration steps", value: `1. Go to Administration > Nodes > Create Node
2. Enter a name and select a region
3. Copy the generated Node ID and Token
4. SSH into your node and run the Beacon bootstrap
5. Verify the node shows as Healthy in the dashboard` } },
      { heading: "Configure DNS and TLS", bullets: ["Ensure your domain A record points to your control plane IP", "Verify port 80 and 443 are reachable from the internet", "Confirm TLS certificates are issued and valid"] },
      { heading: "Set Up Backup Storage", body: ["Configure backup storage to protect your data. Local backups work for single-node setups. For multi-node recovery, configure S3-compatible storage accessible from all nodes."], table: { headers: ["Backend", "Configuration"], rows: [["Local", "POSTGRES_BACKUP_HOST_DIR"], ["S3", "S3_BUCKET, S3_REGION, S3_ACCESS_KEY_ID, S3_SECRET_ACCESS_KEY"]] } },
      { heading: "Create Your First Server", bullets: ["Navigate to Servers > Create Server", "Select a nest and egg for your game type", "Configure resource limits and environment variables", "Assign a network allocation (port)", "Select a node for deployment", "Start the server and verify connectivity"] },
      { heading: "Review Security Settings", callout: { tone: "success", title: "Security checklist", body: "Verify: TLS is active, default passwords are changed, internal services are not publicly exposed, backup storage is configured and tested, and only authorized users have admin access." } }
    ],
  },
  {
    slug: "servers", group: "Administration", title: "Game Server Management", summary: "Complete guide to creating, managing, and troubleshooting game servers.",
    sections: [
      { heading: "Understanding Nests and Eggs", body: ["Forge Plane uses a nested service hierarchy. Nests are collections of related game services. Eggs are individual service definitions that contain Docker images, startup commands, environment variables, and configuration templates."], table: { headers: ["Concept", "Description", "Example"], rows: [["Nest", "A collection of related game eggs", "Minecraft, Source Engine"], ["Egg", "A specific game service definition", "Minecraft Java, CS2, Valheim"], ["Variable", "A configurable egg parameter", "Server Memory, Game Port"], ["Allocation", "A port assignment for the server", "TCP 27015, UDP 27015"]] } },
      { heading: "Creating a Game Server", bullets: ["Select the appropriate nest and egg for your game", "Configure server name, description, and owner", "Select a node with available capacity", "Assign network allocations (ports)", "Configure resource limits (CPU, memory, disk)", "Set environment variables specific to the game"] },
      { heading: "Managing Server Files", body: ["Access server files through the web-based file manager or SFTP."], code: { label: "SFTP connection details", value: `Host: your-domain.com
Port: 2022
Username: <server-id>.<node-id>
Password: <sftp-password>` } },
      { heading: "Server Status and Monitoring", table: { headers: ["Status", "Meaning", "Action Required"], rows: [["Running", "Server is operational", "None"], ["Starting", "Server is initializing", "Wait for startup to complete"], ["Stopped", "Server is not running", "Start from dashboard"], ["Suspended", "Server suspended by admin", "Reactivate from dashboard"], ["Error", "Server encountered a failure", "Check console and logs"]] } },
      { heading: "Backup and Restore", body: ["Create manual backups or configure automatic backup schedules. Backups can be stored locally on the node or in S3-compatible storage for cross-node recovery."] }
    ],
  },
  {
    slug: "applications", group: "Administration", title: "Application Management", summary: "Deploy and manage container applications with Git-based workflows.",
    sections: [
      { heading: "Application Overview", body: ["Forge Plane's application management system supports deploying containerized applications from Git repositories. It automates the build, deployment, and lifecycle management of applications across your Beacon nodes."] },
      { heading: "Git Provider Integration", table: { headers: ["Provider", "Authentication", "Webhook Support"], rows: [["GitHub", "OAuth / Personal Access Token", "Push, Pull Request events"], ["GitLab", "OAuth / Personal Access Token", "Push, Merge Request events"], ["Bitbucket", "OAuth / App Password", "Push, Pull Request events"]] } },
      { heading: "Deployment Workflow", code: { label: "Deployment lifecycle", value: `1. Source code pushed to Git repository
2. Webhook triggers Forge API
3. API creates deployment record
4. Beacon pulls source and builds Docker image
5. Beacon starts new container
6. Forge verifies health endpoint
7. Deployment marked complete or rolled back` } },
      { heading: "Environment and Secrets", bullets: ["Per-application environment variables", "Encrypted secret storage at rest", "Per-deployment variable overrides"] }
    ],
  },
  {
    slug: "databases", group: "Administration", title: "Managed Databases", summary: "Provision and manage databases for your applications and game servers.",
    sections: [
      { heading: "Database Overview", body: ["Forge Plane's database management system allows you to provision and manage databases as workloads on your Beacon nodes. Supported engines include PostgreSQL, MySQL, MariaDB, MongoDB, and Redis."] },
      { heading: "Supported Database Engines", table: { headers: ["Engine", "Use Case"], rows: [["PostgreSQL 15, 16", "Relational data, applications"], ["MySQL 8.0, 8.4", "Web applications, CMS"], ["MariaDB 10.11, 11.4", "MySQL-compatible workloads"], ["MongoDB 7.0", "Document storage"], ["Redis 7.0, 7.2", "Caching, session store"]] } },
      { heading: "Backup and Restore", table: { headers: ["Engine", "Backup Method"], rows: [["PostgreSQL", "pg_dump / pg_restore"], ["MySQL/MariaDB", "mysqldump / mysql"], ["MongoDB", "mongodump / mongorestore"], ["Redis", "SAVE / RDB file copy"]] } },
      { heading: "Security", callout: { tone: "warning", title: "Do not grant workloads access to Forge PostgreSQL", body: "An application or game server should receive only its own database host, database, and least-privilege user. Forge's internal PostgreSQL is not intended for workload use." } }
    ],
  },
  {
    slug: "compose", group: "Administration", title: "Docker Compose Stacks", summary: "Deploy and manage multi-container applications with Docker Compose.",
    sections: [
      { heading: "Compose Stack Overview", body: ["Forge Plane supports deploying multi-container applications as Docker Compose stacks. You can run complex applications with multiple services, networks, and volumes defined in a single Compose file."] },
      { heading: "Stack Lifecycle Management", table: { headers: ["Action", "Description"], rows: [["Deploy", "Start the stack for the first time"], ["Update", "Apply changes to the stack"], ["Scale", "Change service replica count"], ["Restart", "Restart all services"], ["Stop", "Stop all services"], ["Remove", "Remove the stack and its resources"]] } },
      { heading: "Service Configuration", bullets: ["Per-service resource limits (CPU, memory)", "Port mappings and network allocations", "Volume mounts and persistent storage", "Environment variables and secrets", "Health check configurations"] }
    ],
  },
  {
    slug: "networking", group: "Administration", title: "Networking and Allocations", summary: "Manage network allocations, load balancing, and traffic routing.",
    sections: [
      { heading: "Network Allocations", body: ["Network allocations are port assignments that map public ports to your workloads. Each allocation consists of a port number, protocol (TCP/UDP), and optional IP binding."] },
      { heading: "Allocation Types", table: { headers: ["Type", "Protocol", "Use Case"], rows: [["Single Port", "TCP", "HTTP APIs, SSH, database connections"], ["Single Port", "UDP", "Game server queries, voice chat"], ["Port Range", "TCP+UDP", "Multiplayer game servers, streaming"], ["Load Balanced", "TCP", "High-availability services"]] } },
      { heading: "Load Balancer", body: ["The integrated L4 load balancer distributes traffic across multiple workload instances. The default allocation range is 30000-30100 (TCP+UDP)."] },
      { heading: "Firewall and Security", callout: { tone: "danger", title: "Security best practices", body: "Only expose the ports required for your workloads. Keep Beacon API (:9090), PostgreSQL (:5432), and Redis (:6379) on internal networks." } }
    ],
  },
  {
    slug: "storage", group: "Administration", title: "Storage and Backups", summary: "Manage persistent storage, mounts, and comprehensive backup solutions.",
    sections: [
      { heading: "Storage Overview", body: ["Forge Plane supports multiple storage backends for workload data. Persistent storage is managed through mounts, volumes, and backup adapters that can store data locally or in S3-compatible object storage."] },
      { heading: "Mounts and Volumes", table: { headers: ["Type", "Description", "Use Case"], rows: [["Bind Mount", "Host directory mounted into container", "Game server files, configuration"], ["Docker Volume", "Managed by Docker", "Database data, persistent state"], ["tmpfs", "In-memory filesystem", "Cache, temporary files"]] } },
      { heading: "Backup Adapter Configuration", code: { label: "S3 backup configuration", value: `BACKUP_ADAPTER=s3
S3_BUCKET=my-forge-backups
S3_REGION=us-east-1
S3_ACCESS_KEY_ID=your-access-key
S3_SECRET_ACCESS_KEY=your-secret-key
S3_ENDPOINT=https://s3.us-east-1.amazonaws.com
S3_PREFIX=forge/beacon-1
S3_USE_PATH_STYLE=true` } },
      { heading: "Local Storage Paths", table: { headers: ["Path", "Purpose"], rows: [["/srv/game-panel/servers", "Workload files and configuration"], ["/srv/game-panel/servers/.beacon/backups", "Local backup artifacts"], ["/var/backups/gamepanel/postgres", "PostgreSQL database dumps"]] } }
    ],
  },
  {
    slug: "beacon", group: "Beacon Node Guide", title: "Beacon Node Agent Overview", summary: "Understand what Beacon is, how it connects to the panel, and how it manages workloads.",
    sections: [
      { heading: "What Beacon Does", body: ["Beacon is Forge's per-node execution agent. It runs Docker actions, file management, console sessions, SFTP, backups, and node reporting. It is not another web panel — it is the worker that executes what the control plane orchestrates."] },
      { heading: "Deployment Models", table: { headers: ["Model", "Beacon Location", "Use"], rows: [["All in one", "The daemon service in infra/compose.yml", "A first host where control plane and workloads share a VPS."], ["Standalone node", "infra/compose.beacon.yml on the workload VPS", "The supported node-specific Compose path."], ["Several nodes", "One standalone Beacon deployment per workload VPS", "Required for workload movement and destination recovery."]] } },
      { heading: "How Beacon Connects", body: ["Beacon needs DAEMON_NODE_ID (a UUID), DAEMON_NODE_TOKEN (a credential in the format <id>.<secret>), and a PANEL_API_URL reachable from that node. The control plane uses the node credential for daemon calls; Beacon's panel synchronization uses the panel API URL.", "Legacy Wings environment variables (WINGS_NODE_ID, WINGS_TOKEN, WINGS_TOKEN_ID, WINGS_PANEL_URL) are supported as fallbacks."] },
      { heading: "How Beacon Accesses Docker", body: ["Beacon does not mount the Docker socket directly in production. Instead, it connects through docker-socket-proxy (tecnativa/docker-socket-proxy:v0.4.2), a hardened TCP proxy that exposes only necessary Docker API endpoints (CONTAINERS, IMAGES, NETWORKS, VOLUMES, INFO, BUILD, EXEC, PING). The actual socket is mounted read-only into the proxy container."] },
      { heading: "Heartbeats", body: ["Beacon sends a heartbeat to the panel every 30 seconds. The heartbeat payload includes the node's OS, architecture, CPU count, memory, disk space, runtime status (Docker ping result), runtime provider, uptime, and load average. An additional edge agent maintains connectivity with 15-second heartbeats and exponential backoff reconnection (1s initial, 60s max)."] },
      { heading: "Capability Reporting", body: ["At startup and on each heartbeat, Beacon reports its runtime provider and status. The runtime provider defaults to docker but can be set to kubernetes or podman via DAEMON_RUNTIME_PROVIDER. The panel uses this information for scheduling decisions."] },
      { heading: "Runtime Providers", table: { headers: ["Provider", "Status", "Env Var"], rows: [["docker", "Default, production-ready", "DAEMON_RUNTIME_PROVIDER=docker"], ["kubernetes", "Experimental", "DAEMON_RUNTIME_PROVIDER=kubernetes"], ["podman", "Experimental", "DAEMON_RUNTIME_PROVIDER=podman"]] } },
      { heading: "When Forge Is Unavailable", body: ["Already-running containers continue under Docker. New control-plane actions, state updates, and centralized observations cannot complete until the API and PostgreSQL return. A node cannot perform control-plane scheduling on its own. The edge agent will attempt reconnection with exponential backoff."] },
      { heading: "Quick Configuration Reference", table: { headers: ["Variable", "Description", "Default"], rows: [["DAEMON_NODE_ID", "Unique node UUID", "Required"], ["DAEMON_NODE_TOKEN", "Authentication token (<id>.<secret>)", "Required"], ["PANEL_API_URL", "Forge API base URL", "Required"], ["DAEMON_ADDR", "API listen address", ":9090"], ["DAEMON_SFTP_ADDR", "SFTP listen address", ":2022"], ["DAEMON_DATA_DIR", "Workload data directory", "/srv/game-panel/servers"], ["DAEMON_BACKUP_DIR", "Backup storage path", "/srv/game-panel/servers/.beacon/backups"], ["BACKUP_ADAPTER", "Backup storage adapter", "local"]] } }
    ],
  },
  {
    slug: "beacon-install", group: "Beacon Node Guide", title: "Beacon Linux Installation", summary: "Production-quality installation guide for Beacon on Linux servers.",
    sections: [
      { heading: "Purpose", body: ["Beacon is the Forge node agent. It executes Docker workloads, manages files via SFTP, runs backups, reports node health, and performs scheduled tasks. Every server that runs game workloads needs a Beacon instance."] },
      { heading: "Supported Operating Systems", table: { headers: ["OS", "Version", "Status"], rows: [["Ubuntu", "22.04 LTS (Jammy)", "Fully supported"], ["Ubuntu", "24.04 LTS (Noble)", "Fully supported"], ["Debian", "12 (Bookworm)", "Fully supported"], ["Ubuntu", "20.04 LTS (Focal)", "Untested but may work"]] } },
      { heading: "Supported Architectures", body: ["Beacon supports amd64 (x86_64) and arm64 (aarch64) architectures. The Docker build process uses Alpine multi-stage builds with architecture-specific binary downloads for nixpacks."] },
      { heading: "Runtime Requirements", bullets: ["Docker Engine 24.0 or newer", "Docker Compose v2 plugin (2.24.4+)", "Minimum 1 GiB RAM reserved for Beacon (game workloads additional)", "Persistent storage for workload data (DAEMON_DATA_DIR)", "Outbound HTTPS access to the Forge API server", "Port 9090 available for Beacon API (internal only)", "Port 2022 available for SFTP (or proxy through SSH)"] },
      { heading: "How Beacon Accesses Docker", body: ["Beacon connects to Docker through docker-socket-proxy, a hardened TCP proxy that exposes only necessary Docker API endpoints. The actual Docker socket is mounted read-only into the proxy container. This eliminates direct root-level socket access from Beacon.", "The proxy restricts which Docker API endpoints are accessible: containers, images, networks, volumes, info, version, ping, build, and exec operations are allowed; auth, secrets, services, swarm, and system operations are disabled.", "In the standalone Compose deployment, Beacon connects via DOCKER_HOST=tcp://127.0.0.1:2375 pointing at the local docker-socket-proxy. In all-in-one mode, it connects via tcp://docker-proxy:2375."] },
      { heading: "Directory Layout", code: { label: "Data directory structure", value: `/srv/game-panel/servers/          # DAEMON_DATA_DIR
  <server-uuid-1>/
    .config/
      server.json                # Workload config synced from panel
    <files>                      # Game/app files
  <server-uuid-2>/
    ...
  .beacon/
    backups/                     # Local backup artifacts (DAEMON_BACKUP_DIR)
    beacon.db                    # Local state database` } },
      { heading: "Prerequisites", bullets: ["A supported Ubuntu or Debian host with Docker Engine and Compose v2.", "A node record created in the Forge dashboard (generates DAEMON_NODE_ID and DAEMON_NODE_TOKEN).", "A PANEL_API_URL that the node can reach over HTTPS.", "A persistent directory for workload data (default: /srv/game-panel/servers)."] },
      { heading: "Installation via Docker Compose (Primary Method)", code: { label: "Standalone Beacon node", value: `git clone https://github.com/ryzenate1/forge-control-plane.git
cd forge-control-plane/infra
cp ../beacon/.env.example .env
chmod 600 .env

# Edit .env with your node details:
#   DAEMON_NODE_ID=<uuid-from-dashboard>
#   DAEMON_NODE_TOKEN=<token-from-dashboard>
#   PANEL_API_URL=https://panel.example.com/api/v1
#   GAME_SERVERS_HOST_DIR=/srv/game-panel/servers

mkdir -p /srv/game-panel/servers
./bootstrap-beacon.sh` } },
      { heading: "What bootstrap-beacon.sh Does", body: ["1. Sources infra/.env for required variables", "2. Validates PANEL_API_URL is an absolute HTTP(S) URL", "3. Creates the data directory if missing", "4. Validates the Compose configuration", "5. Starts the docker-proxy and Beacon containers with --build", "6. Shows the container status"] },
      { heading: "Configuration Reference", table: { headers: ["Variable", "Required", "Default", "Description"], rows: [["DAEMON_NODE_ID", "Yes", "-", "Node UUID created in the Forge dashboard"], ["DAEMON_NODE_TOKEN", "Yes", "-", "Panel-issued credential (<id>.<secret>)"], ["PANEL_API_URL", "Yes", "-", "Reachable Forge API URL (e.g. https://panel.example.com/api/v1)"], ["DAEMON_ADDR", "No", "127.0.0.1:9090", "Beacon API listen address (loopback in Compose)"], ["DAEMON_SFTP_ADDR", "No", ":2022", "Advertised SFTP address"], ["DAEMON_SFTP_BIND_ADDR", "No", "0.0.0.0:2022", "Container SFTP bind address (host publishing stays loopback-only)"], ["DAEMON_DATA_DIR", "No", "/srv/game-panel/servers", "Workload data root"], ["DAEMON_BACKUP_DIR", "No", "/srv/game-panel/servers/.beacon/backups", "Backup staging directory"], ["BACKUP_ADAPTER", "No", "local", "Backup adapter: local or s3"], ["DAEMON_ALLOW_MOCK_RUNTIME", "No", "false", "Allow mock runtime in development"], ["DAEMON_RUNTIME_PROVIDER", "No", "docker", "Runtime provider: docker, kubernetes, or podman"], ["DAEMON_DETECT_CLEAN_EXIT_AS_CRASH", "No", "false", "Treat clean exits as crashes"], ["DAEMON_SFTP_READ_ONLY", "No", "false", "Restrict SFTP to read-only"], ["DAEMON_SFTP_IDLE_TIMEOUT", "No", "15m", "SFTP idle connection timeout"], ["DAEMON_SFTP_MAX_CONNECTIONS", "No", "128", "Max SFTP connections"], ["DAEMON_SFTP_MAX_SESSIONS_PER_USER", "No", "8", "Max sessions per SFTP user"]] } },
      { heading: "Node Token Format", body: ["The node token uses the format <token-id>.<token-secret>, for example: a1b2c3d4e5f6g7h8.9i8u7y6t5r4e3w2q1z2x3c4v5b6n7m8. The token ID identifies which credential is used; the secret is the authentication secret. When using legacy Wings env vars, WINGS_TOKEN contains the secret and WINGS_TOKEN_ID is prepended automatically."] },
      { heading: "Panel URL", body: ["PANEL_API_URL must be the full URL to the Forge API, including the /api/v1 base path. For example: https://panel.example.com/api/v1. The URL must use http or https scheme and include a hostname. Beacon validates this at startup and will refuse to start with an invalid URL."] },
      { heading: "Bind Address and Port", body: ["DAEMON_ADDR controls where Beacon's HTTP API listens. In the standalone Compose deployment, the default is 127.0.0.1:9090 (loopback only). In all-in-one mode, it defaults to :9090. The API should never be exposed publicly.", "For SFTP, DAEMON_SFTP_ADDR is the advertised endpoint and DAEMON_SFTP_BIND_ADDR is the container listener. Compose binds the container listener to 0.0.0.0 so port publishing works, while the published host port remains restricted to 127.0.0.1."] },
      { heading: "HTTP vs HTTPS (TLS)", body: ["By default, Beacon serves plain HTTP. TLS can be configured in two ways:", "1. Manual TLS: Set DAEMON_TLS_CERT_FILE and DAEMON_TLS_KEY_FILE to PEM file paths.", "2. Auto TLS (ACME): Set DAEMON_AUTO_TLS_HOSTNAME to enable automatic certificate issuance via ACME. Certificates are cached in the data directory under .tls-cache/.", "In most deployments, TLS is handled by the reverse proxy (Caddy/Traefik) and Beacon communicates internally over HTTP."] },
      { heading: "Data Directory", body: ["DAEMON_DATA_DIR defaults to /srv/game-panel/servers. This directory stores all workload files, configuration, and local backup artifacts. The host path is configured via GAME_SERVERS_HOST_DIR in the .env file.", "Each workload gets its own subdirectory named by UUID. Inside, a .config/server.json file stores the workload configuration synced from the panel. The data directory should be backed up regularly."] },
      { heading: "Log Directory", body: ["Beacon logs to stdout/stderr (captured by Docker). For native deployments, Beacon generates a logrotate configuration at /var/log/beacon with the log name beacon. The Docker Compose setup uses the json-file log driver with max-size=10m and max-file=3."] },
      { heading: "Authentication", body: ["Beacon authenticates to the Forge API using DAEMON_NODE_TOKEN. All API calls include the token in the Authorization header as a Bearer token. The API validates the token against the node record.", "For development, DAEMON_ALLOW_INSECURE_NO_AUTH=true disables authentication. This must never be used in production. The daemon refuses to start with a development token (dev-node-token) in production mode."] },
      { heading: "Heartbeats", body: ["Beacon sends a heartbeat every 30 seconds via the heartbeatLoop. Each heartbeat reports: OS, architecture, CPU count, memory (MB), disk (MB), runtime status, runtime provider, uptime (seconds), and load average.", "An edge agent maintains panel connectivity with 15-second heartbeats and handles reconnection with exponential backoff (1s initial, 60s max, 2x factor with jitter). The panel detects nodes as offline if no heartbeat is received within 30 seconds."] },
      { heading: "Capability Reporting", body: ["Each heartbeat includes the runtime provider (docker, kubernetes, or podman) and a runtime status (ok or error with description). The panel uses this data to determine node eligibility for workload placement. If the runtime is unavailable (e.g., Docker daemon not responding), the node is marked as unhealthy."] },
      { heading: "Systemd Service (Alternative to Docker Compose)", body: ["If you prefer to manage Beacon as a native systemd service instead of Docker Compose, use the following unit file. This requires building the Beacon binary separately (go build ./cmd/daemon) or extracting it from the official Docker image."], code: { label: "/etc/systemd/system/beacon.service", value: `[Unit]
Description=Forge Beacon Node Agent
Documentation=https://forge-plane.dev
After=docker.service network-online.target
Wants=network-online.target
Requires=docker.service

[Service]
Type=simple
User=beacon
Group=beacon
ExecStart=/usr/local/bin/beacon
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RestartSec=10
StartLimitBurst=5
StartLimitIntervalSec=300
EnvironmentFile=-/etc/forge/beacon.env
ExecStartPre=/usr/bin/docker info
StandardOutput=journal
StandardError=journal
TimeoutStopSec=60

[Install]
WantedBy=multi-user.target` } },
      { heading: "Systemd Environment File", code: { label: "/etc/forge/beacon.env", value: `DAEMON_NODE_ID=22222222-2222-2222-2222-222222222222
DAEMON_NODE_TOKEN=aaaaaaaa.bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
PANEL_API_URL=https://panel.example.com/api/v1
DAEMON_ADDR=127.0.0.1:9090
DAEMON_SFTP_ADDR=:2022
DAEMON_DATA_DIR=/srv/game-panel/servers
DAEMON_BACKUP_DIR=/srv/game-panel/servers/.beacon/backups
BACKUP_ADAPTER=local
DAEMON_ALLOW_MOCK_RUNTIME=false
DAEMON_ALLOW_INSECURE_NO_AUTH=false
APP_ENV=production` } },
      { heading: "Systemd Setup Commands", code: { label: "Install the service", value: `sudo groupadd --system beacon
sudo useradd --system --gid beacon --create-home --home-dir /var/lib/beacon beacon
sudo mkdir -p /etc/forge /srv/game-panel/servers
sudo cp beacon /usr/local/bin/beacon
sudo cp beacon.env /etc/forge/beacon.env
sudo chmod 600 /etc/forge/beacon.env
sudo chown -R beacon:beacon /srv/game-panel/servers /etc/forge
sudo systemctl daemon-reload
sudo systemctl enable --now beacon
sudo systemctl status beacon` } },
      { heading: "Startup Verification", code: { label: "Verify Beacon is running", value: `# Docker Compose method
cd /opt/forge/infra
docker compose -f compose.beacon.yml --env-file .env ps
docker compose -f compose.beacon.yml --env-file .env exec beacon /daemon --healthcheck

# Direct health check
curl http://127.0.0.1:9090/health

# Expected response:
# {"status":"ok","node_id":"22222222-2222-2222-2222-222222222222"}

# From the dashboard:
# Administration > Nodes > [your node] should show "Healthy"` } },
      { heading: "Upgrading Beacon", code: { label: "Update the standalone node", value: `cd /opt/forge/infra
git pull origin main
docker compose -f compose.beacon.yml --env-file .env pull
docker compose -f compose.beacon.yml --env-file .env up -d --build
docker compose -f compose.beacon.yml --env-file .env logs --tail 50

# Verify health after upgrade
docker compose -f compose.beacon.yml --env-file .env exec beacon /daemon --healthcheck` } },
      { heading: "Removing Beacon", code: { label: "Clean removal", value: `# Stop and remove Beacon container
cd /opt/forge/infra
docker compose -f compose.beacon.yml --env-file .env down -v

# Remove data directory (backup first if needed!)
# rm -rf /srv/game-panel/servers

# Remove the node from the dashboard:
# Administration > Nodes > [your node] > Delete

# If using systemd:
# sudo systemctl stop beacon
# sudo systemctl disable beacon
# sudo rm /etc/systemd/system/beacon.service
# sudo userdel beacon
# sudo rm -rf /etc/forge` } },
      { heading: "Troubleshooting", table: { headers: ["Symptom", "Likely Cause", "Check"], rows: [["Beacon won't connect", "Wrong DAEMON_NODE_ID or DAEMON_NODE_TOKEN", "Verify the values match the dashboard node record"], ["Panel URL unreachable", "PANEL_API_URL is incorrect or firewall blocks it", "curl the panel URL from the Beacon host"], ["Authentication errors", "Node token does not match the panel record", "Regenerate the token from the dashboard"], ["Health check fails", "Docker daemon not accessible", "Check docker-proxy status and DOCKER_HOST setting"], ["Port conflicts", "9090 or 2022 already in use", "ss -tlnp | grep -E '9090|2022'"], ["Container won't start", "Image pull failure or disk full", "Inspect Beacon logs and Docker state"], ["SFTP refused", "Bind address or firewall restriction", "Check DAEMON_SFTP_BIND_ADDR and DAEMON_SFTP_ADDR"]] } },
      { heading: "Log Inspection", code: { label: "View Beacon logs", value: `# Docker Compose
cd /opt/forge/infra
docker compose -f compose.beacon.yml --env-file .env logs -f beacon
docker compose -f compose.beacon.yml --env-file .env logs --tail 200 beacon

# Systemd
sudo journalctl -u beacon -f
sudo journalctl -u beacon --since "5 minutes ago"` } },
      { heading: "Important Restrictions", callout: { tone: "danger", title: "Security boundaries", body: "DAEMON_NODE_TOKEN must be a production secret (not dev-node-token) when APP_ENV=production. DAEMON_ALLOW_INSECURE_NO_AUTH must be false in production. Do not expose the Beacon API or docker-socket-proxy port (2375) publicly." } }
    ],
  },
  {
    slug: "beacon-macos", group: "Beacon Node Guide", title: "macOS Development Node", summary: "Use macOS as a development Beacon node for local testing. Not for production use.",
    sections: [
      { heading: "Important Disclaimer", callout: { tone: "warning", title: "Development setup only", body: "This guide is for running Beacon as a development node on macOS. It is not a supported production configuration. Use Ubuntu or Debian Linux for production Beacon hosts." } },
      { heading: "Prerequisites", bullets: ["macOS 14 (Sonoma) or macOS 15 (Sequoia)", "Docker Desktop for Mac (required — Docker Engine is not available natively on macOS)", "Go 1.26 or newer (if running Beacon directly)", "Node.js 20 LTS or newer (if running Forge Web)", "Git"] },
      { heading: "Docker Desktop Requirement", body: ["macOS does not support Docker Engine directly. Docker Desktop is required and provides the Docker daemon inside a Linux VM managed by macOS. The installer script (scripts/install.sh) recognizes macOS and will offer to install Docker Desktop via Homebrew."], code: { label: "Install Docker Desktop via Homebrew", value: `brew install --cask docker
open -a Docker` } },
      { heading: "Docker Context Detection", body: ["Docker Desktop creates a default context named 'desktop-linux'. Verify it is active."], code: { label: "Check Docker context", value: `docker context show
# Expected: desktop-linux

docker info
# Shows Docker Desktop version and platform=macOS
# Server: Docker Desktop (architecture, OS type: linux)` } },
      { heading: "Docker Socket Differences from Linux", body: ["On Linux, the Docker socket is at /var/run/docker.sock. On macOS, Docker Desktop runs in a Linux VM and the socket is managed transparently. The docker-proxy container still works the same way since Docker Desktop handles socket forwarding.", "When referencing the socket from the host (not inside a container), Docker Desktop provides compatibility. Inside containers, /var/run/docker.sock works as on Linux."] },
      { heading: "Finding the LAN IP", body: ["macOS assigns IP addresses to network interfaces dynamically. Find the IP of the interface that other machines (or Docker containers) use to reach your Mac."], code: { label: "Get your LAN IP", value: `# Ethernet (most common for desktop Macs)
ipconfig getifaddr en0

# Wi-Fi (MacBooks, wireless desktops)
ipconfig getifaddr en1

# Default route interface
route get default | grep interface

# Listen address for Beacon (use the non-localhost IP)
# Example output for en0: 192.168.1.42` } },
      { heading: "Binding Beacon to a Reachable Interface", body: ["When the Forge API runs in Docker on macOS, it cannot reach Beacon at 127.0.0.1 because each Docker container has its own loopback interface. Beacon must bind to 0.0.0.0 or the specific LAN IP so the API container can reach it via the Docker bridge network or host.docker.internal."], code: { label: "Start Beacon on all interfaces", value: `export DAEMON_ADDR=0.0.0.0:9090
go run ./cmd/daemon` } },
      { heading: "Using HTTP Locally", body: ["For local development, HTTP is acceptable. Do not expose Beacon's HTTP port to the internet. The DAEMON_ALLOW_INSECURE_NO_AUTH=true flag can be used in development to disable token authentication, but keep this off unless absolutely necessary."] },
      { heading: "macOS Firewall Considerations", body: ["macOS has a built-in firewall (System Settings > Network > Firewall). When running Beacon on a non-loopback address, you may need to allow incoming connections or add an exception.", "For Docker Desktop, ensure that port forwarding is enabled in Docker Desktop settings (Settings > General > Allow the default Docker socket to be used)."] },
      { heading: "Why 127.0.0.1 Does Not Work When API Runs in Docker", body: ["When the Forge API runs inside a Docker container, its loopback interface (127.0.0.1) is isolated from the host's loopback. If Beacon binds to 127.0.0.1 on the macOS host, the API container cannot reach it.", "Use host.docker.internal to reach the macOS host from Docker containers on macOS:", "PANEL_API_URL=http://host.docker.internal:8080/api/v1", "Or bind Beacon to 0.0.0.0 and use the actual LAN IP."] },
      { heading: "API Container Reaching the Host", body: ["When running the Forge API in Docker on macOS, it can reach services on the host using the host.docker.internal DNS name. Configure Beacon's PANEL_API_URL accordingly."], code: { label: "Beacon env for macOS development", value: `DAEMON_ADDR=0.0.0.0:9090
DAEMON_NODE_ID=22222222-2222-2222-2222-222222222222
DAEMON_NODE_TOKEN=dev-node-token
PANEL_API_URL=http://host.docker.internal:8080/api/v1
DAEMON_DATA_DIR=/tmp/beacon-data
DAEMON_ALLOW_MOCK_RUNTIME=false
DAEMON_ALLOW_INSECURE_NO_AUTH=true
DAEMON_SFTP_ADDR=:2022
BACKUP_ADAPTER=local` } },
      { heading: "Running Beacon Directly (go run)", code: { label: "Start Beacon from source", value: `cd forge-control-plane
export DAEMON_ADDR=0.0.0.0:9090
export DAEMON_NODE_ID=22222222-2222-2222-2222-222222222222
export DAEMON_NODE_TOKEN=dev-node-token
export PANEL_API_URL=http://host.docker.internal:8080/api/v1
export DAEMON_DATA_DIR=/tmp/beacon-data
export DAEMON_ALLOW_INSECURE_NO_AUTH=true
go run ./cmd/daemon` } },
      { heading: "Running Beacon as a LaunchAgent", body: ["macOS supports user-level background services via LaunchAgents. This keeps Beacon running in the background across sessions."], code: { label: "~/Library/LaunchAgents/com.forge.beacon.plist", value: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.forge.beacon</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/beacon</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>DAEMON_ADDR</key>
        <string>0.0.0.0:9090</string>
        <key>DAEMON_NODE_ID</key>
        <string>22222222-2222-2222-2222-222222222222</string>
        <key>DAEMON_NODE_TOKEN</key>
        <string>dev-node-token</string>
        <key>PANEL_API_URL</key>
        <string>http://host.docker.internal:8080/api/v1</string>
        <key>DAEMON_DATA_DIR</key>
        <string>/tmp/beacon-data</string>
        <key>DAEMON_ALLOW_INSECURE_NO_AUTH</key>
        <string>true</string>
        <key>APP_ENV</key>
        <string>development</string>
    </dict>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/beacon.stdout.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/beacon.stderr.log</string>
</dict>
</plist>` } },
      { heading: "Load and Start the LaunchAgent", code: { label: "LaunchAgent commands", value: `# Load the agent
launchctl load ~/Library/LaunchAgents/com.forge.beacon.plist

# Start immediately (if already loaded)
launchctl start com.forge.beacon

# Check status
launchctl list | grep forge

# Unload (stop and remove)
launchctl unload ~/Library/LaunchAgents/com.forge.beacon.plist` } },
      { heading: "Viewing Logs", code: { label: "macOS log commands", value: `# Follow Beacon stdout
tail -f /tmp/beacon.stdout.log

# Follow Beacon stderr
tail -f /tmp/beacon.stderr.log

# View via Docker logs (if running in Docker)
docker compose -f compose.beacon.yml logs -f beacon

# View via go run output (if running in terminal)
# Logs appear directly in the terminal window` } },
      { heading: "Restarting Beacon", code: { label: "Restart commands", value: `# Docker Compose
docker compose -f compose.beacon.yml --env-file .env restart beacon

# LaunchAgent
launchctl stop com.forge.beacon
launchctl start com.forge.beacon

# Direct (go run)
# Press Ctrl+C in the terminal and re-run the go run command` } },
      { heading: "Testing Connectivity", code: { label: "Verify Beacon responds", value: `# Health check
curl http://localhost:9090/health

# From another machine on the LAN (if bound to 0.0.0.0)
curl http://192.168.1.42:9090/health

# Expected response:
# {"status":"ok","node_id":"22222222-2222-2222-2222-222222222222"}

# Test panel connectivity
curl http://host.docker.internal:8080/api/v1/health/ready` } },
      { heading: "macOS Limitations", callout: { tone: "warning", title: "Not for production", body: "Docker Desktop on macOS has networking differences from Linux: no host networking mode, Docker runs in a VM, and file I/O performance is lower. Use this setup only for development and testing. Never run game servers for real users on a macOS Beacon host." } }
    ],
  },
  {
    slug: "operations", group: "Operations", title: "Platform Operations", summary: "Advanced operations including scheduling, failover, evacuation, and recovery.",
    sections: [
      { heading: "Scheduling Overview", body: ["Forge Plane's scheduler places workloads on eligible nodes based on resource availability, location, and configured constraints. The scheduler considers CPU, memory, disk, and port availability when making placement decisions."] },
      { heading: "Node Management", bullets: ["View node health, resource usage, and workload count", "Mark nodes as available, unavailable, or draining", "Configure node regions and locations", "Set resource limits and allocation rules per node"] },
      { heading: "Workload Scheduling Strategies", table: { headers: ["Strategy", "Description", "Use Case"], rows: [["First Available", "Place on the first eligible node", "Simple deployments"], ["Least Loaded", "Place on the node with most free resources", "Balanced resource utilization"], ["Most Loaded", "Place on the most utilized node first", "Consolidation scenarios"], ["Manual", "Operator selects the target node", "Controlled deployments"]] } },
      { heading: "Evacuation", body: ["Evacuation moves workloads off a node in preparation for maintenance or decommissioning."], code: { label: "Safe evacuation sequence", value: `1. Mark the source node unavailable for new placement.
2. Verify a second Beacon is healthy and has capacity.
3. Create and verify a backup in shared storage.
4. Reserve the destination allocation and storage path.
5. Restore or redeploy at the destination.
6. Verify health, data, and public allocations.
7. Retire the source only after verification.` } },
      { heading: "Failover and High Availability", callout: { tone: "warning", title: "Automatic failover requires preparation", body: "Configure shared backup storage on all nodes before relying on automatic failover. Test failover scenarios with non-critical workloads first. Verify workload health after recovery." } },
      { heading: "Activity and Audit Logging", table: { headers: ["Event Type", "Logged Data"], rows: [["Deployments", "User, timestamp, application, status"], ["Node Events", "Node, event type, details"], ["Configuration Changes", "User, target, before/after"], ["Backup Operations", "Workload, size, status, storage path"], ["Authentication", "User, IP, action, result"]] } }
    ],
  },
  {
    slug: "backup-recovery", group: "Operations", title: "Backup and Disaster Recovery", summary: "Comprehensive backup and disaster recovery guide.",
    sections: [
      { heading: "Backup Design", body: ["Local backups help one node. Cross-node recovery requires every eligible Beacon to reach the same S3-compatible repository and have the credentials and encryption context needed to restore."] },
      { heading: "Backup Types", table: { headers: ["Type", "Scope", "Frequency", "Storage"], rows: [["Control Plane DB", "PostgreSQL dump", "Daily (scheduled)", "Local + optional S3"], ["Workload Data", "Game server files, application data", "Configurable per workload", "Local or S3"], ["Docker Images", "Application images", "On build", "Registry"]] } },
      { heading: "Control Plane Backup", code: { label: "PostgreSQL backup", value: `./scripts/upgrade.sh
./scripts/rollback.sh list
./scripts/rollback.sh db /var/backups/gamepanel/postgres/gamepanel-<timestamp>.dump` } },
      { heading: "Workload Backup Configuration", code: { label: "S3 backup adapter setup", value: `BACKUP_ADAPTER=s3
S3_BUCKET=my-forge-backups
S3_REGION=us-east-1
S3_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
S3_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
S3_ENDPOINT=https://s3.us-east-1.amazonaws.com
S3_PREFIX=forge/beacon-node-1
S3_USE_PATH_STYLE=true` } },
      { heading: "Evacuation Runbook", code: { label: "Safe sequence", value: `1. Mark the source node unavailable for new placement.
2. Verify a second Beacon is healthy and has capacity.
3. Create and verify a backup in shared storage.
4. Reserve the destination allocation and storage path.
5. Restore or redeploy at the destination.
6. Verify health, data, and public allocations.
7. Retire the source only after verification.` } },
      { heading: "Recovery Limits", callout: { tone: "warning", title: "Restore-oriented recovery", body: "The current configuration provides backup and restore paths. It is not evidence of automatic ownership failover for a running stateful workload. Practice the runbook with a non-critical workload." } }
    ],
  },
  {
    slug: "cloud", group: "Operations", title: "Cloud Provisioning", summary: "Automatically provision and manage cloud instances with Beacon.",
    sections: [
      { heading: "Cloud Provisioning Overview", body: ["Forge Plane can automatically provision cloud instances and bootstrap them as Beacon nodes. Currently supports AWS EC2 with plans to support additional providers."] },
      { heading: "AWS Configuration", bullets: ["AWS Access Key ID and Secret Access Key with EC2 permissions", "EC2 key pair for SSH access to provisioned instances", "Security group with required ports (Beacon API, SFTP, game ports)", "VPC subnet for instance placement", "AMI selection (Ubuntu 22.04 or 24.04 recommended)"] },
      { heading: "Provisioning a Cloud Node", code: { label: "Dashboard workflow", value: `1. Navigate to Nodes > Cloud Provisioning
2. Select provider: AWS EC2
3. Configure: region, instance type, AMI, subnet, security group
4. Set Beacon node ID and token for the new instance
5. Launch instance
6. Verify Beacon connects automatically after boot` } },
      { heading: "Instance Types and Sizing", table: { headers: ["Workload Type", "Recommended Instance", "vCPU", "RAM"], rows: [["Light game servers", "t3.medium / t3.large", "2-4", "4-8 GiB"], ["Medium workloads", "c6i.large / c6i.xlarge", "2-4", "4-8 GiB"], ["Heavy game servers", "c6i.2xlarge / c6i.4xlarge", "8-16", "16-32 GiB"]] } },
      { heading: "Cost Management", callout: { tone: "note", title: "Cloud cost considerations", body: "Monitor cloud instance costs through your provider's billing dashboard. Use cost allocation tags for tracking. Terminate unused instances to avoid unnecessary charges." } }
    ],
  },
  {
    slug: "reference", group: "Reference", title: "Configuration Reference", summary: "Complete reference for all configuration options, ports, and settings.",
    sections: [
      { heading: "Ports", table: { headers: ["Port", "Service", "Production Exposure"], rows: [["80, 443/TCP", "Caddy proxy", "Public when serving the panel and TLS."], ["8080/TCP", "Forge API", "Loopback in compose.production.yml."], ["3000/TCP", "Forge Web", "Loopback in compose.production.yml."], ["5432/TCP", "PostgreSQL", "Loopback in compose.production.yml; never public."], ["6379/TCP", "Redis", "Loopback in compose.production.yml; never public."], ["9090/TCP", "Beacon API", "Loopback all-in-one; loopback in standalone Compose."], ["2022/TCP", "Beacon SFTP", "Restrict deliberately; default bind is loopback."], ["9091/TCP", "Prometheus", "Loopback in compose.production.yml."], ["9093/TCP", "Alertmanager", "Loopback in compose.production.yml."], ["3001/TCP", "Grafana", "Loopback in compose.production.yml."], ["30000-30100/TCP+UDP", "Load-balancer allocation range", "Published by compose.production.yml."]] } },
      { heading: "Important Files", table: { headers: ["Path", "Purpose"], rows: [["infra/.env", "Runtime secrets and deployment configuration."], ["infra/compose.yml", "Base control-plane Compose stack."], ["infra/compose.production.yml", "Production loopback bindings and restart policies."], ["infra/compose.beacon.yml", "Standalone Beacon Compose deployment."], ["infra/compose.caddy.production.yml", "Caddy reverse proxy override."], ["infra/compose.tls.yml", "Traefik ACME TLS override."], ["infra/Caddyfile", "Baseline Caddy configuration for local hostnames."], ["scripts/install.sh", "Automated production installer."], ["scripts/healthcheck.sh", "Compose, API, Beacon, Web, database checks."], ["scripts/rollback.sh", "Database/image rollback helper."]] } },
      { heading: "Feature Maturity", table: { headers: ["Area", "Current Position"], rows: [["One-host Compose stack", "Available"], ["Standalone Beacon Compose", "Available"], ["Node Docker and file execution", "Available"], ["Shared backup restore workflow", "Available when storage is configured"], ["Automatic stateful failover", "Not currently available"], ["Cloud provisioning bootstrap", "Experimental; verify node enrollment"], ["Caddy production configuration", "Requires operator adaptation"]] } },
      { heading: "Beacon Environment Variables", table: { headers: ["Variable", "Default", "Description"], rows: [["DAEMON_NODE_ID", "-", "Node UUID created in the Forge dashboard (fallback: WINGS_NODE_ID)"], ["DAEMON_NODE_TOKEN", "-", "Panel-issued credential in <id>.<secret> format (fallback: WINGS_TOKEN + WINGS_TOKEN_ID)"], ["PANEL_API_URL", "-", "Reachable Forge API URL (fallback: WINGS_PANEL_URL)"], ["DAEMON_ADDR", ":9090", "Beacon API listen address"], ["DAEMON_SFTP_ADDR", ":2022", "Advertised SFTP listen address"], ["DAEMON_SFTP_BIND_ADDR", "0.0.0.0:2022", "Container SFTP bind address (host publishing stays loopback-only)"], ["DAEMON_DATA_DIR", "/srv/game-panel/servers", "Workload data root"], ["DAEMON_BACKUP_DIR", "<data>/.beacon/backups", "Backup staging directory"], ["DAEMON_CONFIG_FILE", "-", "Path to optional YAML config file"], ["DAEMON_RUNTIME_PROVIDER", "docker", "Runtime: docker, kubernetes, or podman"], ["DAEMON_TLS_CERT_FILE", "-", "TLS certificate path (manual mode)"], ["DAEMON_TLS_KEY_FILE", "-", "TLS key path (manual mode)"], ["DAEMON_AUTO_TLS_HOSTNAME", "-", "Enable ACME auto-TLS for this hostname"], ["DAEMON_ALLOW_INSECURE_NO_AUTH", "false", "Disable authentication (development only)"], ["DAEMON_ALLOW_MOCK_RUNTIME", "false", "Allow mock runtime when Docker unavailable"], ["DAEMON_DETECT_CLEAN_EXIT_AS_CRASH", "false", "Treat clean exits as crashes"], ["DAEMON_SFTP_READ_ONLY", "false", "Restrict SFTP to read-only"], ["DAEMON_SFTP_IDLE_TIMEOUT", "15m", "SFTP idle connection timeout"], ["DAEMON_SFTP_MAX_CONNECTIONS", "128", "Max concurrent SFTP connections"], ["DAEMON_SFTP_MAX_SESSIONS_PER_USER", "8", "Max concurrent SFTP sessions per user"], ["BACKUP_ADAPTER", "local", "Backup adapter (local or s3)"], ["DOCKER_HOST", "unix:// or tcp://", "Docker daemon endpoint"]] } },
      { heading: "S3 Backup Environment Variables", table: { headers: ["Variable", "Required", "Description"], rows: [["S3_BUCKET", "Yes (s3 adapter)", "S3 bucket name"], ["S3_REGION", "Yes (s3 adapter)", "AWS region"], ["S3_ACCESS_KEY_ID", "Yes (s3 adapter)", "AWS access key"], ["S3_SECRET_ACCESS_KEY", "Yes (s3 adapter)", "AWS secret key"], ["S3_ENDPOINT", "No", "Custom endpoint for S3-compatible storage"], ["S3_PREFIX", "No", "Key prefix for backup objects"], ["S3_USE_PATH_STYLE", "No (default: true)", "Use path-style addressing"]] } },
      { heading: "API Environment Variables", table: { headers: ["Variable", "Required", "Description"], rows: [["API_ADDR", ":8080", "Forge API listen address"], ["API_AUTH_SECRET", "Yes", "JWT signing secret"], ["APP_KEY", "Yes", "Application encryption key"], ["APP_ENV", "development", "Runtime environment"], ["DATABASE_URL", "Yes", "PostgreSQL connection string"], ["FORGE_MASTER_KEY", "Yes", "At-rest encryption master key"], ["FORGE_MASTER_KEY_ID", "primary", "Active master key identifier"], ["REDIS_ADDR", "redis:6379", "Redis server address"], ["REDIS_PASSWORD", "-", "Redis authentication password"], ["PANEL_URL", "http://localhost:3000", "Forge Web URL"], ["LOAD_BALANCER_ENABLED", "false", "Enable L4 load balancer"], ["LOAD_BALANCER_PORT_MIN", "30000", "Load balancer port range start"], ["LOAD_BALANCER_PORT_MAX", "30100", "Load balancer port range end"]] } }
    ],
  },
  {
    slug: "troubleshooting", group: "Reference", title: "Troubleshooting Guide", summary: "Comprehensive troubleshooting for all Forge Plane components.",
    sections: [
      { heading: "First Response", code: { label: "Control-plane diagnosis", value: `./scripts/healthcheck.sh
cd infra && docker compose -f compose.yml -f compose.production.yml --env-file .env ps
cd infra && docker compose -f compose.yml -f compose.production.yml --env-file .env logs --tail 200 api daemon postgres redis web` } },
      { heading: "Common Failures", table: { headers: ["Symptom", "Likely Cause", "Check and Fix"], rows: [["Beacon offline", "Wrong ID/token, unreachable PANEL_API_URL, or Docker failure", "Check standalone Beacon logs, then verify the three required variables and curl the panel API from the node."], ["SFTP refused", "No listener or restrictive bind/firewall", "Inspect DAEMON_SFTP_ADDR and DAEMON_SFTP_BIND_ADDR; use ss -lnt and the Beacon logs."], ["API unhealthy", "PostgreSQL, migration, Redis, or secret configuration", "Run scripts/healthcheck.sh and inspect API/PostgreSQL logs."], ["PostgreSQL auth failure", "DATABASE_URL and POSTGRES credentials differ", "Compare infra/.env values and reconnect."], ["TLS issuance fails", "DNS/port 80/IPv6/proxy conflict", "Use dig, confirm 80/443 ownership, then inspect Caddy or Traefik logs."], ["Container will not start", "Image, allocation, disk, or Docker failure", "Inspect Beacon logs and Docker state on the assigned node."], ["Backup restore fails", "Destination cannot reach repository or lacks capacity", "Verify object-storage credentials from the node and the destination data path before retrying."]] } },
      { heading: "Diagnostic Tools", table: { headers: ["Tool", "Location", "Purpose"], rows: [["healthcheck.sh", "scripts/healthcheck.sh", "Comprehensive system health check"], ["install.sh", "scripts/install.sh", "Installation and upgrade automation"], ["Docker logs", "docker compose logs", "Per-service log inspection"]] } },
      { heading: "Log Collection", code: { label: "Log commands", value: `# All services
cd infra && docker compose -f compose.yml -f compose.production.yml --env-file .env logs --tail 200

# Specific service
cd infra && docker compose -f compose.yml -f compose.production.yml --env-file .env logs -f api

# Beacon node (standalone)
docker compose -f compose.beacon.yml --env-file .env logs --tail 100` } },
      { heading: "Escalate Safely", callout: { tone: "danger", title: "Do not delete data to make an error disappear", body: "Capture logs, check the last successful backup, and preserve the PostgreSQL and Beacon data volumes before destructive repair actions." } }
    ],
  },
  {
    slug: "development", group: "Development", title: "Development Guide", summary: "Set up a development environment and contribute to Forge Plane.",
    sections: [
      { heading: "Prerequisites", bullets: ["Go 1.26 or newer", "Node.js 20 LTS or newer", "npm (included with Node.js)", "Docker Desktop (macOS/Windows) or Docker Engine (Linux)", "Git"] },
      { heading: "Repository Structure", table: { headers: ["Directory", "Technology", "Purpose"], rows: [["web/", "Next.js 15 + React 19", "Web dashboard frontend"], ["forge/api/", "Go 1.26 + Fiber", "REST API server"], ["beacon/", "Go 1.26", "Node agent (daemon)"], ["infra/", "Docker Compose", "Deployment configuration"], ["scripts/", "Bash", "Installation and maintenance scripts"]] } },
      { heading: "Setting Up the Development Environment", code: { label: "Quick start for developers", value: `git clone https://github.com/ryzenate1/forge-control-plane.git
cd forge-control-plane

# Start infrastructure services (PostgreSQL, Redis)
cd infra
docker compose -f compose.yml --env-file ../.env.dev up -d postgres redis

# Start Forge API (from repository root)
cd ..
go run ./forge/api/cmd/api

# Start Forge Web (in a separate terminal)
cd web
npm install
npm run dev` } },
      { heading: "Running Tests", code: { label: "Test commands", value: `# API tests
cd forge/api && go test ./...

# Beacon tests
cd beacon && go test ./...

# Web tests
cd web && npm test

# Lint and typecheck
cd web && npm run lint && npm run typecheck` } },
      { heading: "Building for Production", code: { label: "Build commands", value: `# Build Forge API binary
cd forge/api && go build -o ../../build/api ./cmd/api

# Build Beacon binary
cd beacon && go build -o ../build/beacon ./cmd/daemon

# Build Web dashboard
cd web && npm run build` } }
    ],
  }
];

export const docGroups = Array.from(new Set(docs.map((page) => page.group)));
export const docBySlug = (slug: string) => docs.find((page) => page.slug === slug);
