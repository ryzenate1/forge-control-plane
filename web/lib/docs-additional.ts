// ============================================================================
// FORGE PLANE ADDITIONAL DOCUMENTATION PAGES
// This file contains the new pages requested by the user
// ============================================================================

import type { DocCallout, DocSection, DocPage } from './docs';

// Export the same types for consistency
export type { DocCallout, DocSection, DocPage };

// ============================================================================
// 1. DEPLOYMENT MODELS AND ARCHITECTURE
// ============================================================================

export const deploymentModelsPage: DocPage = {
  slug: "deployment-models",
  group: "Installation",
  title: "Deployment Models and Architecture",
  summary: "Understand the different ways to deploy Forge Plane, from all-in-one development to multi-node production.",
  sections: [
    {
      heading: "Deployment Model Overview",
      body: [
        "Forge Plane supports multiple deployment models to accommodate different use cases, from simple development setups to large-scale production environments. The key distinction is between the control plane (Forge Web + Forge API + services) and the compute nodes (Beacon + Docker)."
      ]
    },
    {
      heading: "Deployment Model Comparison",
      table: {
        headers: ["Model", "Control Plane", "Beacon Nodes", "Use Case", "Complexity"],
        rows: [
          ["All-in-One (Development)", "1 host", "Same host", "Development, testing, learning", "⭐ Low"],
          ["Control Plane + 1 Node", "1 host", "Same or separate host", "Small production, first deployment", "⭐⭐ Medium"],
          ["Control Plane + Multiple Nodes", "1 host", "Multiple separate hosts", "Production workloads, scaling", "⭐⭐⭐ Medium"],
          ["High Availability Control Plane", "Multiple hosts", "Multiple hosts", "Enterprise, high availability", "⭐⭐⭐⭐ High"]
        ]
      }
    },
    {
      heading: "All-in-One Deployment (Same Machine)",
      body: [
        "In this model, the entire Forge Plane stack runs on a single machine. This is the simplest deployment and is ideal for development, testing, and small-scale deployments."
      ],
      table: {
        headers: ["Component", "Runs On", "Port", "Access"],
        rows: [
          ["Forge Web", "Single host", "3000", "Loopback (via proxy)"],
          ["Forge API", "Single host", "8080", "Loopback (via proxy)"],
          ["PostgreSQL", "Single host", "5432", "Loopback"],
          ["Redis", "Single host", "6379", "Loopback"],
          ["Beacon", "Single host", "9090", "Loopback"],
          ["Reverse Proxy", "Single host", "80, 443", "Public"],
          ["Game Containers", "Single host", "Various", "Public (via allocations)"]
        ]
      }
    },
    {
      heading: "All-in-One Architecture Diagram",
      code: {
        label: "All-in-One Deployment",
        value: `+\`+\`+\`mermaid
flowchart TB
    subgraph "Single Host"
        direction TB
        
        subgraph "Control Plane Services"
            Proxy[Reverse Proxy\\nPorts: 80, 443]
            Web[Forge Web\\nPort: 3000]
            API[Forge API\\nPort: 8080]
            PostgreSQL[PostgreSQL\\nPort: 5432]
            Redis[Redis\\nPort: 6379]
        end
        
        subgraph "Node Services"
            Beacon[Beacon\\nPort: 9090]
            Docker[Docker Engine]
            Game1[Game Server 1]
            Game2[Game Server 2]
            SFTP[SFTP\\nPort: 2022]
        end
        
        DataDir[(Data Directory\\n/srv/forge/servers)]
    end
    
    Internet((Internet\\nUsers/Players)) -->|HTTPS| Proxy
    Proxy -->|HTTP| Web
    Proxy -->|/api| API
    Web -->|API Calls| API
    API -->|SQL| PostgreSQL
    API -->|Cache| Redis
    API -->|Commands| Beacon
    Beacon -->|Docker Socket| Docker
    Docker -->|Containers| Game1
    Docker -->|Containers| Game2
    Beacon -->|SFTP| SFTP
    SFTP -->|File Access| DataDir
    Game1 -->|Data| DataDir
    Game2 -->|Data| DataDir
    
    style Internet fill:#f9f,stroke:#333
    style Proxy fill:#0af,stroke:#333
    style Web fill:#0af,stroke:#333
    style API fill:#0af,stroke:#333
    style PostgreSQL fill:#0af,stroke:#333
    style Redis fill:#0af,stroke:#333
    style Beacon fill:#0f0,stroke:#333
    style Docker fill:#0f0,stroke:#333
    style Game1 fill:#0f0,stroke:#333
    style Game2 fill:#0f0,stroke:#333
    style SFTP fill:#0f0,stroke:#333
    style DataDir fill:#ff0,stroke:#333
+\`+\`+\``
      }
    },
    {
      heading: "When to Use All-in-One",
      body: [
        "The all-in-one deployment is ideal for:"
      ],
      bullets: [
        "Development and testing environments",
        "Learning and evaluating Forge Plane",
        "Small deployments with a few game servers",
        "First production deployment to validate the platform",
        "Single VPS deployments where simplicity is preferred over scalability"
      ],
      callout: {
        tone: "note",
        title: "All-in-One Limitations",
        body: "The all-in-one model has limited scalability. All workloads share the same host resources, and if the host fails, both the control plane and all workloads become unavailable. For production, consider separating the control plane from compute nodes."
      }
    }
  ]
};

// ============================================================================
// 2. BEACON INSTALLATION DOCUMENTATION
// ============================================================================

export const beaconInstallationPage: DocPage = {
  slug: "beacon-complete",
  group: "Beacon Node Guide",
  title: "Beacon Node Agent - Complete Installation Guide",
  summary: "Comprehensive Beacon installation documentation for production node agents.",
  sections: [
    {
      heading: "Beacon's Purpose",
      body: [
        "Beacon is Forge Plane's per-node execution agent. It serves as the bridge between the central control plane and the actual workload execution on each node.",
        "Beacon's primary responsibilities include:"
      ],
      bullets: [
        "Receiving authenticated commands from the Forge API",
        "Executing Docker operations (container create, start, stop, delete, etc.)",
        "Managing workload files and directories",
        "Providing console access to running containers",
        "Running an SFTP server for secure file transfer",
        "Creating and restoring backups (local and S3-compatible)",
        "Reporting health status and resource usage",
        "Monitoring container and node health",
        "Executing scheduled tasks (cron jobs)",
        "Managing its own lifecycle (upgrades, restarts)"
      ],
      callout: {
        tone: "note",
        title: "Important Distinction",
        body: "Beacon is NOT another web panel. It does NOT perform scheduling or placement decisions. It ONLY executes work assigned by the control plane and reports back status. All intelligence and coordination happens in the Forge API."
      }
    },
    {
      heading: "Supported Operating Systems",
      body: [
        "Beacon is officially supported on the following operating systems, as verified by the installer scripts and actual code:"
      ],
      table: {
        headers: ["OS", "Version", "Architecture", "Status", "Evidence"],
        rows: [
          ["Ubuntu", "22.04 LTS (Jammy)", "amd64, arm64", "✅ Fully Supported", "Listed in scripts/install.sh, tested in CI"],
          ["Ubuntu", "24.04 LTS (Noble)", "amd64, arm64", "✅ Fully Supported", "Listed in scripts/install.sh, tested in CI"],
          ["Debian", "12 (Bookworm)", "amd64, arm64", "✅ Fully Supported", "Listed in scripts/install.sh"],
          ["macOS", "14-15", "amd64, arm64", "🟡 Development Only", "Recognized by installer; Docker Desktop required"]
        ]
      },
      callout: {
        tone: "warning",
        title: "Production Recommendation",
        body: "For production Beacon nodes, use Ubuntu 22.04 LTS or 24.04 LTS on amd64 or arm64 architecture. These are the most tested and supported configurations."
      }
    },
    {
      heading: "Supported Architectures",
      body: [
        "Beacon supports the following CPU architectures:"
      ],
      bullets: [
        "amd64 (x86_64) - Fully supported on all verified operating systems",
        "arm64 (AArch64) - Fully supported on Ubuntu 22.04/24.04 and Debian 12"
      ],
      callout: {
        tone: "note",
        title: "Architecture Considerations",
        body: "ARM64 support depends on Docker image availability. Most official images (PostgreSQL, Redis, etc.) support ARM64, but some game server images may not. Always verify that your required images have ARM64 variants before deploying on ARM64 hardware."
      }
    },
    {
      heading: "Runtime Requirements",
      body: [
        "Beacon has the following runtime requirements:"
      ],
      table: {
        headers: ["Requirement", "Minimum Version", "Purpose"],
        rows: [
          ["Docker Engine", "24.0+", "Container runtime for workload execution"],
          ["Docker Compose v2", "2.24.4+", "Multi-container orchestration (for standalone deployment)"],
          ["Docker Socket", "/var/run/docker.sock", "Communication with Docker Engine"],
          ["Git", "2.x", "For Git-based application deployments (optional)"],
          ["curl", "Any", "For health checks and external requests"],
          ["OpenSSL", "Any", "For TLS and certificate operations"]
        ]
      }
    },
    {
      heading: "Docker Socket Detection",
      body: [
        "Beacon automatically detects and uses the Docker socket:"
      ],
      bullets: [
        "By default, Beacon uses /var/run/docker.sock",
        "The Docker socket can be configured via the DOCKER_SOCKET_PATH environment variable",
        "Beacon mounts the Docker socket as a volume in the container",
        "The socket must be accessible by the user running Beacon",
        "For security, the socket should have restricted permissions (660 or 666)"
      ],
      code: {
        label: "Docker socket configuration",
        value: `# Default Docker socket path
DOCKER_SOCKET_PATH=/var/run/docker.sock

# In compose.beacon.yml, the socket is mounted as:
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro

# Check socket permissions
ls -la /var/run/docker.sock

# Fix permissions if needed (temporary)
sudo chmod 666 /var/run/docker.sock`
      }
    },
    {
      heading: "Directory Layout",
      body: [
        "Beacon uses the following directory structure on each node:"
      ],
      table: {
        headers: ["Directory", "Purpose", "Default Path", "Persistent"],
        rows: [
          ["Data Directory", "Stores all workload files (game servers, applications)", "/srv/forge/servers", "✅ Yes"],
          ["Backup Directory", "Stores local backups", "/backups", "✅ Yes"],
          ["Configuration", "Beacon configuration files", "/etc/forge/beacon", "❌ No"],
          ["Logs", "Beacon and workload logs", "/var/log/forge/beacon", "❌ No"],
          ["Temporary Files", "Temporary build and operation files", "/tmp/forge", "❌ No"]
        ]
      },
      code: {
        label: "Create directory structure",
        value: `# Create data directory
sudo mkdir -p /srv/forge/servers
sudo chown -R 1000:1000 /srv/forge/servers

# Create backup directory
sudo mkdir -p /backups
sudo chown -R 1000:1000 /backups

# Create log directory
sudo mkdir -p /var/log/forge/beacon
sudo chown -R 1000:1000 /var/log/forge/beacon

# Create temporary directory
sudo mkdir -p /tmp/forge
sudo chown -R 1000:1000 /tmp/forge`
      }
    }
  ]
};

// ============================================================================
// Export all pages for merging with main docs
// ============================================================================

export const additionalPages: DocPage[] = [
  deploymentModelsPage,
  beaconInstallationPage
];
