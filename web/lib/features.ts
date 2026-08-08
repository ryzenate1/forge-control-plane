export type FeatureGroup = {
  title: string;
  summary: string;
  items: {
    name: string;
    does: string;
    how: string;
    status?: "Planning only" | "Registry only" | "Experimental" | "Available";
  }[];
};

export type ManualDiagram = {
  label: string;
  nodes: string[];
  edges: string[];
  boundary: string;
  checks: string[];
};

export type ManualFeature = FeatureGroup["items"][number] & {
  slug: string;
  group: string;
  groupSummary: string;
  diagram: ManualDiagram;
  workflow: string[];
  boundary: string;
  verify: string[];
};

// ============================================================================
// FORGE PLANE COMPREHENSIVE FEATURE LIST
// ============================================================================

export const featureGroups: FeatureGroup[] = [
  // ========================================================================
  // OPERATIONS
  // ========================================================================
  {
    title: "Operations",
    summary: "Observe the control plane, understand changes, and coordinate safe work across your infrastructure.",
    items: [
      {
        name: "Overview Dashboard",
        does: "Shows the live control-plane summary with all critical metrics.",
        how: "Aggregates health, workload, node, and operation signals from all components.",
        status: "Available"
      },
      {
        name: "Monitoring",
        does: "Tracks platform and node health in real-time.",
        how: "Reads health observations from Beacon nodes and monitoring integrations.",
        status: "Available"
      },
      {
        name: "Cron Jobs",
        does: "Schedules administrative shell tasks and automated operations.",
        how: "Forge API loads enabled schedules from PostgreSQL and executes them in-process with configurable retry logic.",
        status: "Available"
      },
      {
        name: "Host Terminal",
        does: "Provides direct host system information and management.",
        how: "Uses privileged node and host capabilities through Beacon; restrict access to trusted operators only.",
        status: "Available"
      },
      {
        name: "Activity Log",
        does: "Shows human-readable audit history of all platform actions.",
        how: "Displays recorded control-plane activity with timestamps, user information, and action details.",
        status: "Available"
      },
      {
        name: "Migrations & Recovery",
        does: "Plans movement and recovery work with visual workflows.",
        how: "The registered UI provides planning metadata and coordination; actual workload execution is handled by Beacon nodes.",
        status: "Available"
      },
      {
        name: "Operations Center",
        does: "Centralized view of all running and completed operations.",
        how: "Aggregates operation status, progress, logs, and results from across the platform.",
        status: "Available"
      },
      {
        name: "Health Dashboard",
        does: "Real-time health status of all platform components.",
        how: "Combines health checks from Forge API, PostgreSQL, Redis, Beacon nodes, and all running workloads.",
        status: "Available"
      }
    ]
  },

  // ========================================================================
  // INFRASTRUCTURE
  // ========================================================================
  {
    title: "Infrastructure",
    summary: "Describe placement, networking, storage, and the nodes that execute workloads.",
    items: [
      {
        name: "Regions and Locations",
        does: "Organize infrastructure geography and placement policies.",
        how: "Associate nodes with regions and locations for intelligent placement context and failover coordination.",
        status: "Available"
      },
      {
        name: "Nodes",
        does: "Registers and manages Beacon hosts with comprehensive status.",
        how: "Stores node identity, capacity, health, runtime connection details, and coordinates all node-side operations.",
        status: "Available"
      },
      {
        name: "Allocations",
        does: "Reserves and manages workload ports across nodes.",
        how: "Maps TCP or UDP port assignments to eligible nodes with automatic or manual allocation strategies.",
        status: "Available"
      },
      {
        name: "Database Hosts",
        does: "Defines and manages workload-database provisioning targets.",
        how: "Keeps hosted database credentials and placement separate from Forge's own PostgreSQL; supports multiple database engines.",
        status: "Available"
      },
      {
        name: "Mounts",
        does: "Shares approved host paths with workload containers.",
        how: "Attaches controlled paths to selected node workloads with configurable permissions and read-only options.",
        status: "Available"
      },
      {
        name: "Files and Terminal",
        does: "Administers host files and provides shell access.",
        how: "Uses privileged node-side access for file operations and terminal sessions; grant only to trusted operators.",
        status: "Available"
      },
      {
        name: "Docker Management",
        does: "Complete Docker runtime management across all nodes.",
        how: "Inspects containers, images, networks, and volumes through Beacon's Docker socket access.",
        status: "Available"
      },
      {
        name: "Load Balancer",
        does: "Manages L4 load balancing with health-aware routing.",
        how: "Defines target groups, routes, and traffic policies with automatic health checks and failover.",
        status: "Available"
      },
      {
        name: "Traffic Management",
        does: "Coordinates traffic routing and proxy configuration.",
        how: "Manages routes, target groups, and traffic policies with support for WebSockets, gRPC, and TCP routing.",
        status: "Available"
      },
      {
        name: "Cloud Provisioning",
        does: "Automatically provisions cloud instances with Beacon.",
        how: "Connects provider accounts (AWS EC2) and provisions instances with automatic Beacon bootstrap via cloud-init.",
        status: "Available"
      },
      {
        name: "Autoscaler",
        does: "Automatically scales node capacity based on demand.",
        how: "Uses capacity metrics, placement constraints, and scaling policies to automatically provision new nodes when needed.",
        status: "Available"
      },
      {
        name: "Failover",
        does: "Manages automatic and manual failover procedures.",
        how: "Coordinates failover policies, crash detection, and recovery workflows across eligible nodes.",
        status: "Available"
      },
      {
        name: "Scheduler",
        does: "Intelligent workload placement and scheduling.",
        how: "Scores placements based on capacity, constraints, affinity, and policy configuration for optimal resource utilization.",
        status: "Available"
      },
      {
        name: "Reconciliation",
        does: "Ensures desired state matches actual state.",
        how: "Continuously compares desired state from the control plane with actual state from Beacon nodes and corrects discrepancies.",
        status: "Available"
      }
    ]
  },

  // ========================================================================
  // MANAGEMENT
  // ========================================================================
  {
    title: "Management",
    summary: "Create the workloads and identities your organization manages.",
    items: [
      {
        name: "Servers",
        does: "Manages complete game-server instances with full lifecycle.",
        how: "Combines a template (nest and egg), allocation, resource limits, storage, and Beacon runtime action for complete server management.",
        status: "Available"
      },
      {
        name: "Applications",
        does: "Manages container applications, Git sources, and Compose stacks.",
        how: "Creates deployments and revisions that Beacon builds or runs, with support for Git-based workflows, Docker images, and Nixpacks.",
        status: "Available"
      },
      {
        name: "Deployments",
        does: "Manages application deployments with history and rollback.",
        how: "Creates deployment records, revisions, and health-aware rollout work with support for blue-green and rolling deployments.",
        status: "Available"
      },
      {
        name: "Preview Deployments",
        does: "Creates temporary preview environments for testing.",
        how: "Deploys application revisions to isolated preview environments for testing before production rollout.",
        status: "Available"
      },
      {
        name: "Users and Roles",
        does: "Controls accounts, limits, and additional roles with RBAC.",
        how: "Applies administrative authorization, account configuration, and role-based permissions across all platform features.",
        status: "Available"
      },
      {
        name: "OAuth Clients",
        does: "Creates and manages user-owned OAuth clients.",
        how: "Stores client credentials, application authorization settings, and redirect URIs for third-party integrations.",
        status: "Available"
      },
      {
        name: "Organizations",
        does: "Manages multi-tenant organizations with isolated resources.",
        how: "Provides resource isolation, user management, and settings at the organization level for multi-tenant deployments.",
        status: "Available"
      },
      {
        name: "Projects",
        does: "Organizes workloads into logical projects.",
        how: "Groups servers, applications, databases, and other resources into projects for better organization and management.",
        status: "Available"
      },
      {
        name: "Environments",
        does: "Manages deployment environments (dev, staging, production).",
        how: "Provides environment-specific configurations, isolation, and promotion workflows for applications and services.",
        status: "Available"
      }
    ]
  },

  // ========================================================================
  // SERVICES
  // ========================================================================
  {
    title: "Services",
    summary: "Define repeatable templates and external integrations for workloads.",
    items: [
      {
        name: "Nests & Eggs",
        does: "Defines canonical game-server templates with variables.",
        how: "Supplies startup variables, Docker images, service definitions, and configuration templates for game servers.",
        status: "Available"
      },
      {
        name: "App Templates",
        does: "Provides reusable application deployment templates.",
        how: "Prepares common workload configuration for app creation with pre-configured settings and variables.",
        status: "Available"
      },
      {
        name: "Compatibility Templates",
        does: "Maintains legacy template compatibility for migration.",
        how: "Preserves older template shapes and configurations for migration from other platforms or import from existing deployments.",
        status: "Available"
      },
      {
        name: "Webhooks",
        does: "Delivers lifecycle events to external endpoints.",
        how: "Sends configured event notifications to external endpoints for CI/CD integration, monitoring, and automation workflows.",
        status: "Available"
      },
      {
        name: "Plugins",
        does: "Registers plugin manifests for extensibility.",
        how: "Stores metadata only; runtime plugin execution is not currently available but planned for future extensibility.",
        status: "Registry only"
      },
      {
        name: "API Keys",
        does: "Manages API access keys with configurable permissions.",
        how: "Creates, manages, and revokes API keys with granular permissions for programmatic access to the platform.",
        status: "Available"
      },
      {
        name: "Settings",
        does: "Configures application access and panel behavior.",
        how: "Stores credentials, operator-selected settings, and platform configuration for customization and optimization.",
        status: "Available"
      },
      {
        name: "Git Providers",
        does: "Integrates with external Git hosting services.",
        how: "Connects to GitHub, GitLab, Bitbucket, and other Git providers for application deployments and repository management.",
        status: "Available"
      },
      {
        name: "Container Registries",
        does: "Manages Docker image registry connections.",
        how: "Configures connections to Docker Hub, GitHub Container Registry, private registries, and other container image sources.",
        status: "Available"
      },
      {
        name: "Backup Providers",
        does: "Configures external backup storage providers.",
        how: "Manages connections to S3-compatible storage, local storage, and other backup destinations for disaster recovery.",
        status: "Available"
      }
    ]
  },

  // ========================================================================
  // ADVANCED
  // ========================================================================
  {
    title: "Advanced",
    summary: "Coordinate deployments, traffic, scaling, security, and provider integrations.",
    items: [
      {
        name: "Docker",
        does: "Inspects containers, images, networks, and volumes across all nodes.",
        how: "Uses the node runtime through Beacon and Docker access to provide comprehensive container management and inspection.",
        status: "Available"
      },
      {
        name: "Scheduler",
        does: "Scores placements and applies scheduling policies.",
        how: "Uses capacity, constraints, affinity, and policy configuration to determine optimal placement for new workloads.",
        status: "Available"
      },
      {
        name: "Auto-Scaler",
        does: "Automatically scales resources based on demand.",
        how: "Monitors resource usage and automatically adjusts allocations, scales services, or provisions new nodes based on configured policies.",
        status: "Available"
      },
      {
        name: "Deployments",
        does: "Runs rolling, blue-green, and preview delivery flows.",
        how: "Creates deployment records, revisions, and health-aware rollout work with configurable strategies and automatic rollback on failure.",
        status: "Available"
      },
      {
        name: "Preview Deployments",
        does: "Creates isolated preview environments for testing.",
        how: "Deploys application revisions to temporary, isolated environments for testing and validation before production deployment.",
        status: "Available"
      },
      {
        name: "Failover",
        does: "Defines failover policy and crash simulation.",
        how: "Requires a tested multi-node topology; coordinates automatic and manual failover with health checks and recovery procedures.",
        status: "Available"
      },
      {
        name: "Load Balancer",
        does: "Defines target groups, routes, and traffic policy.",
        how: "Selects and manages configured routing targets with health checks, load balancing algorithms, and traffic validation.",
        status: "Available"
      },
      {
        name: "Traffic",
        does: "Manages traffic routing and proxy configuration.",
        how: "Configures routes, target groups, and traffic policies with support for WebSockets, gRPC, TCP, and UDP routing.",
        status: "Available"
      },
      {
        name: "Domains",
        does: "Manages custom domains and DNS configuration.",
        how: "Connects domains, DNS configuration, and gateway certificate workflows for custom domain access to applications and services.",
        status: "Available"
      },
      {
        name: "Certificates",
        does: "Manages TLS certificates for custom domains.",
        how: "Provisions, manages, and renews TLS certificates using Let's Encrypt (HTTP-01, DNS-01) or custom certificates for secure access.",
        status: "Available"
      },
      {
        name: "Cloud",
        does: "Connects provider accounts and instance provisioning.",
        how: "Creates provider-side infrastructure with automatic Beacon bootstrap and node enrollment; validate Beacon hardening before production.",
        status: "Available"
      },
      {
        name: "Compose",
        does: "Imports and manages Docker Compose files.",
        how: "Parses stack definitions and coordinates lifecycle work through the platform with support for most Compose features.",
        status: "Available"
      },
      {
        name: "Social Login",
        does: "Configures identity providers for authentication.",
        how: "Integrates with OAuth providers (Google, GitHub, Discord, etc.) for user authentication and authorization.",
        status: "Available"
      },
      {
        name: "mTLS",
        does: "Configures mutual TLS materials for secure communication.",
        how: "Stores and applies dedicated certificate settings for mutual TLS authentication between components.",
        status: "Available"
      },
      {
        name: "Security",
        does: "Manages platform-wide security settings.",
        how: "Configures authentication, authorization, encryption, rate limiting, and other security policies for the entire platform.",
        status: "Available"
      }
    ]
  },

  // ========================================================================
  // BEACON
  // ========================================================================
  {
    title: "Beacon Node Agent",
    summary: "The per-node execution agent that powers all workload execution.",
    items: [
      {
        name: "Container Management",
        does: "Complete Docker container lifecycle management.",
        how: "Creates, starts, stops, restarts, and deletes Docker containers with comprehensive configuration and monitoring.",
        status: "Available"
      },
      {
        name: "Image Management",
        does: "Pulls, builds, and manages Docker images.",
        how: "Handles image pulls from registries, custom builds from Dockerfiles, and Nixpacks auto-build for applications without Dockerfiles.",
        status: "Available"
      },
      {
        name: "File Operations",
        does: "Comprehensive file management for workloads.",
        how: "Provides upload, download, edit, delete, and directory operations for server files through the dashboard and SFTP.",
        status: "Available"
      },
      {
        name: "Console Access",
        does: "Real-time terminal access to containers.",
        how: "Provides web-based terminal access with command execution, real-time output, and session management via WebSocket.",
        status: "Available"
      },
      {
        name: "SFTP Service",
        does: "Secure file transfer protocol for server files.",
        how: "Runs SFTP server on configurable port (default 2022) for bulk file operations and external file management tools.",
        status: "Available"
      },
      {
        name: "Backup Management",
        does: "Creates, restores, and manages backups.",
        how: "Handles local and S3-compatible backups with configurable retention, compression, and verification for all workloads.",
        status: "Available"
      },
      {
        name: "Health Reporting",
        does: "Reports node and container health to control plane.",
        how: "Monitors and reports health status, resource usage, and observations back to Forge API for centralized monitoring.",
        status: "Available"
      },
      {
        name: "Resource Monitoring",
        does: "Tracks CPU, memory, disk, and network usage.",
        how: "Collects and reports resource metrics for nodes and containers to the control plane for monitoring and alerting.",
        status: "Available"
      },
      {
        name: "Cron Execution",
        does: "Runs scheduled tasks on the node.",
        how: "Executes node-side scheduled tasks defined in the control plane with configurable schedules and retry logic.",
        status: "Available"
      },
      {
        name: "Upgrade Management",
        does: "Handles Beacon self-updates.",
        how: "Automatically or manually updates Beacon to new versions with minimal downtime and automatic rollback on failure.",
        status: "Available"
      },
      {
        name: "Git Deployments",
        does: "Clones, builds, and deploys from Git repositories.",
        how: "Handles Git repository cloning, branch selection, build processes, and deployment coordination for application workloads.",
        status: "Available"
      },
      {
        name: "Compose Execution",
        does: "Deploys and manages Docker Compose stacks.",
        how: "Parses and executes Compose files with support for multiple services, networks, volumes, and advanced Compose features.",
        status: "Available"
      },
      {
        name: "Database Provisioning",
        does: "Creates and manages database containers.",
        how: "Provisions database containers with persistent storage, credentials, and configuration for application workloads.",
        status: "Available"
      }
    ]
  },

  // ========================================================================
  // MONITORING
  // ========================================================================
  {
    title: "Monitoring & Observability",
    summary: "Comprehensive monitoring, logging, and alerting for your infrastructure.",
    items: [
      {
        name: "Health Dashboard",
        does: "Shows real-time health status of all platform components.",
        how: "Aggregates health checks from Forge API, PostgreSQL, Redis, Beacon nodes, and all running workloads into a unified dashboard.",
        status: "Available"
      },
      {
        name: "Node Monitoring",
        does: "Tracks node health, resources, and status.",
        how: "Monitors CPU, memory, disk, network, Docker status, and Beacon health for each node with configurable thresholds and alerts.",
        status: "Available"
      },
      {
        name: "Container Monitoring",
        does: "Monitors all running containers across all nodes.",
        how: "Tracks container status, health checks, resource usage, and logs for comprehensive container monitoring and troubleshooting.",
        status: "Available"
      },
      {
        name: "Service Monitoring",
        does: "Monitors all platform services.",
        how: "Tracks Forge API, Forge Web, PostgreSQL, Redis, and all supporting services with health checks and performance metrics.",
        status: "Available"
      },
      {
        name: "Prometheus Integration",
        does: "Collects and stores metrics for the entire platform.",
        how: "Scrapes metrics from all services, nodes, and workloads with configurable scrape intervals and retention policies.",
        status: "Available"
      },
      {
        name: "Grafana Dashboards",
        does: "Provides visual dashboards for metrics and monitoring.",
        how: "Offers pre-configured dashboards for platform overview, node monitoring, container metrics, and custom visualizations.",
        status: "Available"
      },
      {
        name: "Alertmanager",
        does: "Manages alerts and notifications.",
        how: "Handles alert routing, grouping, inhibition, and notification delivery to email, Slack, Discord, and other endpoints.",
        status: "Available"
      },
      {
        name: "Logging",
        does: "Aggregates and manages logs from all components.",
        how: "Collects, stores, and provides access to logs from all services, nodes, and workloads with filtering and export capabilities.",
        status: "Available"
      },
      {
        name: "Activity Log",
        does: "Maintains complete audit trail of all platform actions.",
        how: "Records all user actions, system events, and platform changes with timestamps, user information, and detailed descriptions.",
        status: "Available"
      },
      {
        name: "Metrics",
        does: "Provides platform and workload metrics.",
        how: "Collects and exposes CPU, memory, disk, network, and custom metrics for monitoring, alerting, and analysis.",
        status: "Available"
      }
    ]
  }
];

// ============================================================================
// MANUAL FEATURES (for detailed documentation pages)
// ============================================================================

// These are used for the manual/step-by-step documentation pages
export const manualFeatures: ManualFeature[] = [
  {
    slug: "create-server",
    group: "Getting Started",
    groupSummary: "Step-by-step guides for common tasks",
    name: "Create Your First Game Server",
    does: "Deploy a game server from a template",
    how: "Uses nest/egg templates, node selection, allocation assignment, and Beacon execution",
    diagram: {
      label: "Server Creation Flow",
      nodes: ["User", "Dashboard", "API", "Database", "Beacon", "Docker", "Container"],
      edges: [
        "User->Dashboard: Select template",
        "Dashboard->API: Create server request",
        "API->Database: Store server record",
        "API->Beacon: Execute creation",
        "Beacon->Docker: Create container",
        "Docker->Container: Start game server",
        "Container->Beacon: Health status",
        "Beacon->API: Report success",
        "API->Dashboard: Update UI"
      ],
      boundary: "Forge Control Plane",
      checks: ["Template exists", "Node has capacity", "Allocations available", "Docker running", "Healthy container"]
    },
    workflow: [
      "Navigate to Management > Servers",
      "Click 'Create Server'",
      "Select nest (game type)",
      "Select egg (version/template)",
      "Choose node with available resources",
      "Configure allocations (ports)",
      "Set resource limits",
      "Configure startup variables",
      "Review and create",
      "Monitor deployment progress"
    ],
    boundary: "Requires: Valid template, Node with capacity, Available allocations, Docker socket access",
    verify: [
      "Server appears in dashboard",
      "Server status changes to 'Installing'",
      "Server status changes to 'Running'",
      "Health checks pass",
      "Console is accessible",
      "Game is playable"
    ]
  },
  {
    slug: "add-node",
    group: "Infrastructure",
    groupSummary: "Infrastructure setup and management",
    name: "Add a Beacon Node",
    does: "Deploy Beacon to a new node and register it with the control plane",
    how: "Installs Beacon via Docker Compose, configures credentials, and establishes communication with Forge API",
    diagram: {
      label: "Node Addition Flow",
      nodes: ["Admin", "Dashboard", "API", "New Node", "Beacon", "Docker"],
      edges: [
        "Admin->Dashboard: Initiate node creation",
        "Dashboard->API: Create node record",
        "API->Database: Store node credentials",
        "Admin->New Node: Install Beacon",
        "New Node->Beacon: Start Beacon container",
        "Beacon->Docker: Access Docker socket",
        "Beacon->API: Register and authenticate",
        "API->Dashboard: Update node list",
        "Beacon->API: Send heartbeat"
      ],
      boundary: "Forge Control Plane + New Node",
      checks: ["Docker installed", "Node credentials valid", "API reachable", "Docker socket accessible", "Heartbeat successful"]
    },
    workflow: [
      "Navigate to Infrastructure > Nodes",
      "Click 'Create Node'",
      "Copy node ID and token",
      "SSH to new server",
      "Install Docker and Docker Compose",
      "Clone Forge Plane repository",
      "Configure Beacon environment",
      "Start Beacon service",
      "Verify node appears in dashboard",
      "Confirm heartbeat is healthy"
    ],
    boundary: "Requires: Linux server, Docker installed, Network connectivity to API, Valid credentials",
    verify: [
      "Node appears in Infrastructure > Nodes",
      "Node status is 'Healthy'",
      "Beacon container is running",
      "Heartbeat is active",
      "Docker socket is accessible"
    ]
  },
  {
    slug: "configure-backups",
    group: "Operations",
    groupSummary: "Operational workflows",
    name: "Configure Server Backups",
    does: "Set up automatic backups for game servers",
    how: "Configures backup schedules, retention, destinations, and verification for each server",
    diagram: {
      label: "Backup Configuration Flow",
      nodes: ["Admin", "Dashboard", "API", "Beacon", "Storage", "Backup"],
      edges: [
        "Admin->Dashboard: Configure backup settings",
        "Dashboard->API: Update backup configuration",
        "API->Database: Store backup settings",
        "API->Beacon: Schedule backup",
        "Beacon->Container: Create backup archive",
        "Container->Beacon: Backup data",
        "Beacon->Storage: Upload backup",
        "Storage->Beacon: Confirm upload",
        "Beacon->API: Report backup success",
        "API->Dashboard: Update backup list"
      ],
      boundary: "Forge Control Plane + Node",
      checks: ["Backup enabled", "Schedule valid", "Storage available", "Container accessible", "Upload successful"]
    },
    workflow: [
      "Navigate to Management > Servers",
      "Select server to configure",
      "Go to Backups tab",
      "Enable automatic backups",
      "Set backup interval",
      "Configure retention policy",
      "Select backup destination",
      "Set excluded files",
      "Save configuration",
      "Test backup creation"
    ],
    boundary: "Requires: Server exists, Node is healthy, Storage configured, Sufficient disk space",
    verify: [
      "Backup configuration saved",
      "First backup creates successfully",
      "Backup appears in list",
      "Backup can be downloaded",
      "Backup can be restored"
    ]
  },
  {
    slug: "deploy-application",
    group: "Applications",
    groupSummary: "Application deployment",
    name: "Deploy a Container Application",
    does: "Deploy an application from Git or Docker image",
    how: "Clones repository, builds image, and deploys container with configured resources and networking",
    diagram: {
      label: "Application Deployment Flow",
      nodes: ["Admin", "Dashboard", "API", "Beacon", "Git", "Builder", "Container"],
      edges: [
        "Admin->Dashboard: Configure application",
        "Dashboard->API: Create application",
        "API->Database: Store application config",
        "API->Beacon: Deploy application",
        "Beacon->Git: Clone repository",
        "Git->Beacon: Source code",
        "Beacon->Builder: Build image",
        "Builder->Beacon: Built image",
        "Beacon->Docker: Create container",
        "Docker->Container: Start application",
        "Container->Beacon: Health status",
        "Beacon->API: Report deployment success"
      ],
      boundary: "Forge Control Plane + Node",
      checks: ["Git accessible", "Build successful", "Image created", "Container healthy", "Deployment complete"]
    },
    workflow: [
      "Navigate to Management > Apps",
      "Click 'Create Application'",
      "Select deployment type (Git/Docker)",
      "Configure Git repository or Docker image",
      "Set build configuration",
      "Configure environment variables",
      "Set resource limits",
      "Configure networking",
      "Review and deploy",
      "Monitor deployment progress"
    ],
    boundary: "Requires: Valid repository/image, Node with capacity, Build tools available, Network configured",
    verify: [
      "Application appears in list",
      "Deployment starts",
      "Build completes successfully",
      "Container starts",
      "Health checks pass",
      "Application is accessible"
    ]
  },
  {
    slug: "setup-tls",
    group: "Security",
    groupSummary: "Security configuration",
    name: "Configure TLS for Custom Domain",
    does: "Set up HTTPS access with custom domain and TLS certificate",
    how: "Configures reverse proxy with Let's Encrypt certificates or custom certificates for secure access",
    diagram: {
      label: "TLS Configuration Flow",
      nodes: ["Admin", "Dashboard", "API", "Proxy", "DNS", "CA", "Browser"],
      edges: [
        "Admin->Dashboard: Configure domain",
        "Dashboard->API: Update domain settings",
        "API->Database: Store domain config",
        "Admin->DNS: Create DNS records",
        "DNS->CA: Verify domain ownership",
        "CA->Proxy: Issue certificate",
        "Proxy->API: Configure HTTPS",
        "Proxy->Browser: Serve HTTPS",
        "Browser->Proxy: HTTPS request",
        "Proxy->API: Proxy request"
      ],
      boundary: "Forge Control Plane + DNS + Certificate Authority",
      checks: ["Domain valid", "DNS propagated", "Port 80 open", "Port 443 open", "Certificate issued"]
    },
    workflow: [
      "Navigate to Administration > Domains",
      "Click 'Add Domain'",
      "Enter domain name",
      "Configure DNS records (A/AAAA)",
      "Wait for DNS propagation",
      "Select TLS provider (Let's Encrypt)",
      "Configure certificate settings",
      "Save domain configuration",
      "Verify TLS is working",
      "Test HTTPS access"
    ],
    boundary: "Requires: Valid domain, DNS control, Port 80/443 open, Proxy configured",
    verify: [
      "Domain appears in list",
      "Certificate issues successfully",
      "HTTPS access works",
      "No mixed content warnings",
      "Certificate is valid"
    ]
  }
];

export function manualFeatureBySlug(slug: string): ManualFeature | undefined {
  return manualFeatures.find((f) => f.slug === slug);
}
