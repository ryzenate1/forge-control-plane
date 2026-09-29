import type { PageInfoDisclosureProps } from "@/components/ui/page-info-disclosure";

export const adminPageGuides = {
  applications: { title: "Applications", triggerLabel: "About Applications", description: "Define workloads, then deploy them to your infrastructure.", sections: [{ title: "Build and deploy", content: "An application defines its source, environment, ports, and resources. Use Deployments to release it to a host." }] },
  appCreation: { title: "Create Application", triggerLabel: "About application setup", description: "Set up an application source, configuration, and deployment preferences.", sections: [{ title: "Target selection", content: "You may select a Beacon and region, or leave either automatic. Forge uses the current placement policy and reported node state when it chooses a target." }] },
  nests: {
    title: "Service Definitions",
    triggerLabel: "About Service Definitions",
    eyebrow: "Architecture & Semantics",
    description: "Reusable game-server blueprints (nests and eggs)",
    sections: [
      { title: "Nests and eggs", content: "A nest groups related games. Each egg is one runnable definition inside it: container images, startup command, installation script, and configurable variables." },
      { title: "Variables and install", content: "Eggs expose variables when a server is created. The install script and container prepare the server files; the stop command controls graceful shutdown." },
      { title: "Import and export", content: "Eggs can be exported as JSON and re-imported, or imported from Compatibility Templates. Deleting a nest removes its eggs but leaves existing servers running." },
    ],
  },
  appTemplates: {
    title: "App Templates",
    triggerLabel: "About App Templates",
    eyebrow: "Architecture & Semantics",
    description: "Application deployment blueprints",
    sections: [
      { title: "Template storage", content: "Built-in templates ship with Forge. Custom templates are saved in this browser; they are not a shared server-side catalog." },
      { title: "Template types", content: "A template is a Docker image, a Git repository, or a Compose document, plus default ports, environment variables, and CPU, memory and disk defaults." },
      { title: "Using templates", content: "Templates seed the Create Application wizard. Editing a template never changes applications already created from it." },
    ],
  },
  compatibility: {
    title: "Compatibility Templates",
    triggerLabel: "About Compatibility Templates",
    eyebrow: "Architecture & Semantics",
    description: "Legacy compatibility templates kept for imported installs",
    sections: [
      { title: "Choosing a template", content: "Use Service Definitions for new game-server blueprints and App Templates for application presets. Compatibility Templates support existing imported configurations." },
      { title: "Game Template Catalog", content: "The catalog below holds preconfigured game-server definitions. Importing one creates an egg inside the nest you choose." },
      { title: "Lifecycle", content: "Templates are edited and deleted here. Removing a template does not stop servers that were already created from it." },
    ],
  },
  registries: {
    title: "Image Registries",
    triggerLabel: "About Image Registries",
    eyebrow: "Architecture & Semantics",
    description: "Private Docker registry credentials for image pull and push",
    sections: [
      { title: "Registry access", content: "Add a registry address and credentials, then verify the connection. Verification performs a login against the registry." },
      { title: "Scope", content: "Global registries are available to all users. Non-global entries are scoped to the teams that own them." },
      { title: "Pull and push", content: "Beacon nodes use these credentials to pull private images and push built images. A missing or expired credential surfaces as an image-pull failure, not a build error." },
    ],
  },
  forgefile: {
    title: "Forgefile",
    triggerLabel: "About Forgefile",
    eyebrow: "Architecture & Semantics",
    description: "Environment-as-code: validate and apply a Forgefile",
    sections: [
      { title: "Validate and apply", content: "Validate your forge.yaml before applying it. Applying updates the project identified by its slug and records a new manifest version." },
      { title: "Saved manifests", content: "Applied manifests are stored server-side and listed by slug. Selecting one shows its version, timestamp, and stored document." },
      { title: "Schema", content: "Top level: project (name, slug), deploy entries (app, compose or database sources), database definitions, and environment names." },
    ],
  },
  tags: {
    title: "Tags",
    triggerLabel: "About Tags",
    eyebrow: "Architecture & Semantics",
    description: "Colour-coded labels for organising resources",
    sections: [
      { title: "Consistent naming", content: "Use short labels for a team, purpose, or environment. A shared vocabulary makes resources easier to find." },
      { title: "Colour handling", content: "Colours are free-form hex values validated on save. The palette below offers one-click seeds; the preview shows exactly how the pill renders." },
      { title: "Scope", content: "Tags are shared across the whole panel and can be assigned to applications, servers, and environments. Deleting a tag removes it from every resource using it." },
    ],
  },
  appStore: {
    title: "App Store",
    triggerLabel: "About App Store",
    eyebrow: "Architecture & Semantics",
    description: "Browse and install pre-built applications",
    sections: [
      { title: "Browse vs Installed", content: "Browse lists every available application. Installed lists the installations created from them, with their live status." },
      { title: "Installation", content: "Choose an app, review its configuration and resource requirements, and explicitly select the target node. Nothing is placed automatically." },
      { title: "Bundled templates", content: "Sync bundled imports the packaged Coolify template catalog. Installed apps keep running when the catalog refreshes; use Upgrade to redeploy one." },
    ],
  },
  databases: { title: "Databases", triggerLabel: "About Databases", description: "One place to provision, connect, and operate database workloads.", sections: [{ title: "Database surfaces", content: "Overview summarizes the inventory. Database Hosts stores external MySQL and PostgreSQL connections; the other tabs manage containers, managed databases, and database services." }] },
  catalog: {
    title: "Service Catalog",
    triggerLabel: "About Service Catalog",
    eyebrow: "Architecture & Semantics",
    description: "Provision managed services (databases, caches, queues) from the catalog",
    sections: [
      { title: "Where data comes from", content: "Catalog entries describe the available services, versions, providers, and resource requirements. Versions and providers reflect the current catalog, not live workload state." },
      { title: "Provisioning", content: "Pick a version, an optional environment for connection variables, a node and region, then compute and storage sizes. The wizard reviews the placement before deploying." },
      { title: "Retention", content: "Retention policies prune old provisioned instances. Running retention is an explicit action; it reports how many instances were pruned." },
    ],
  },
  eggs: { title: "Eggs", triggerLabel: "About Eggs", description: "Runnable game-server definitions within a service family.", sections: [{ title: "Definition contents", content: "An egg specifies container images, startup behavior, installation steps, and the variables exposed when a server is created." }] },
  appMounts: {
    title: "App Storage",
    triggerLabel: "About App Storage",
    eyebrow: "Architecture & Semantics",
    description: "Per-application persistent storage: volumes, binds, tmpfs and seed files",
    sections: [
      { title: "Mount types", content: "Volume mounts use a named Docker volume, bind mounts map an allowed host path, tmpfs is ephemeral in-memory storage, and seed files materialise database-stored content at a target path on every deploy." },
      { title: "Validation and deploys", content: "Definitions are validated by the server before they are stored, so reserved targets and disallowed bind prefixes are rejected here rather than at deploy time. The deploy pipeline injects each mount into the app compose document." },
      { title: "App Storage vs Storage Mounts", content: "This page declares storage a single app carries through redeploys. Storage Mounts is the node-wide allowlist of host paths a bind mount may use." },
    ],
  },
  operations: {
    title: "Operations",
    triggerLabel: "About Operations",
    eyebrow: "Architecture & Semantics",
    description: "Control-plane operation history and manual controls",
    sections: [
      { title: "Plans before execution", content: "Evacuation and recovery plans are saved first, then started explicitly. Migration records track server moves between Beacon nodes." },
      { title: "Refresh", content: "Migration, recovery and installer workflow lists poll every 10 seconds." },
    ],
  },
  backups: {
    title: "Backups",
    triggerLabel: "About Backups",
    eyebrow: "Architecture & Semantics",
    description: "Backup policies, jobs, artifacts and restores",
    sections: [
      { title: "Pipeline", content: "Policies define schedules, jobs execute them, artifacts are the stored results, and restores bring data back." },
      { title: "Refresh", content: "Every backup list polls every 30 seconds." },
    ],
  },
  backupEngines: {
    title: "Backup Engines",
    triggerLabel: "About Backup Engines",
    eyebrow: "Architecture & Semantics",
    description: "Restic and Kopia repositories, snapshots, verification and restores",
    sections: [
      { title: "Repositories and snapshots", content: "Repositories hold the encrypted backend. Snapshots can be verified, restored or pruned. Passwords are sealed by the API and never returned." },
      { title: "Refresh", content: "Repositories and restore jobs poll every 30 seconds; snapshots load on demand for the selected repository." },
    ],
  },
  migrations: {
    title: "Migrations",
    triggerLabel: "About Migrations",
    eyebrow: "Architecture & Semantics",
    description: "Live workload migration jobs and recovery plans",
    sections: [
      { title: "Migration vs recovery", content: "Migrations move a server between Beacons. Recovery plans restore workloads from a failed Beacon. Neither runs until it is started explicitly." },
      { title: "Refresh", content: "Migration and recovery lists poll every 10 seconds." },
    ],
  },
  cronJobs: {
    title: "Cron Jobs",
    triggerLabel: "About Cron Jobs",
    eyebrow: "Architecture & Semantics",
    description: "Scheduled and recurring automation",
    sections: [
      { title: "Schedules", content: "Jobs run on standard 5-field cron expressions. Select a job to inspect its execution history." },
      { title: "Refresh", content: "The job list polls every 30 seconds; execution history polls every 10 seconds." },
    ],
  },
  dockerEvents: {
    title: "Docker Events",
    triggerLabel: "About Docker Events",
    eyebrow: "Architecture & Semantics",
    description: "Live container lifecycle events streamed from every Beacon node",
    sections: [
      { title: "Live timeline", content: "Newest first and capped at MAX_DOCKER_EVENTS by design: this is a what-just-happened view, and the activity log is the place for history. Pause the feed to inspect a moment without losing your place." },
      { title: "Refresh", content: "The feed polls every 5 seconds; relative timestamps tick every 15 seconds." },
    ],
  },
  reconciliation: {
    title: "Reconciliation",
    triggerLabel: "About Reconciliation",
    eyebrow: "Architecture & Semantics",
    description: "Drift detection and desired-state reconciliation",
    sections: [
      { title: "Plans before execution", content: "Trigger a run, review diffs and drifts, then confirm. Destructive plans are flagged before confirmation." },
      { title: "Refresh", content: "Summary and plans poll every 15 seconds; events load on demand." },
    ],
  },
  orphans: {
    title: "Orphaned Resources",
    triggerLabel: "About Orphaned Resources",
    eyebrow: "Architecture & Semantics",
    description: "Servers and databases with no owning record",
    sections: [
      { title: "Resolving", content: "Force-deleted resources that could not be removed remotely are tracked here. Resolve only after manually confirming remote cleanup is complete. Resolving records the resolution; it does not retry deletion." },
    ],
  },
} satisfies Record<string, PageInfoDisclosureProps>;
