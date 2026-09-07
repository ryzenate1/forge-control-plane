import type { PageInfoDisclosureProps } from "@/components/ui/page-info-disclosure";

export const adminPageGuides = {
  applications: { title: "Applications", triggerLabel: "About Applications", description: "Define workloads, then deploy them to your infrastructure.", sections: [{ title: "Build and deploy", content: "An application defines its source, environment, ports, and resources. Use Deployments to release it to a host." }] },
  appCreation: { title: "Create Application", triggerLabel: "About application setup", description: "Set up an application source, configuration, and deployment preferences.", sections: [{ title: "Target selection", content: "You may select a Beacon and region, or leave either automatic. Forge uses the current placement policy and reported node state when it chooses a target." }] },
  nests: { title: "Service Definitions", triggerLabel: "About Service Definitions", description: "Reusable blueprints for game servers.", sections: [{ title: "Nests and eggs", content: "Nests group related games. Each egg defines a container image, startup command, installation script, and configurable variables." }] },
  appTemplates: { title: "App Templates", triggerLabel: "About App Templates", description: "Starting points for the Create Application wizard.", sections: [{ title: "Template storage", content: "Built-in templates are included with Forge. Custom templates are saved in this browser; they are not a shared server-side catalog." }] },
  compatibility: { title: "Legacy Templates", triggerLabel: "About Legacy Templates", description: "Maintain templates used by imported installations.", sections: [{ title: "Choosing a template", content: "Use Service Definitions for new game-server blueprints and App Templates for application presets. Legacy Templates support existing imported configurations." }] },
  registries: { title: "Image Registries", triggerLabel: "About Image Registries", description: "Manage credentials for private container images.", sections: [{ title: "Registry access", content: "Add a registry address and credentials, then verify the connection. Global registries are available to all users." }] },
  forgefile: { title: "Forgefile", triggerLabel: "About Forgefile", description: "Define projects and environments in a versioned manifest.", sections: [{ title: "Validate and apply", content: "Validate your forge.yaml before applying it. Applying updates the project identified by its slug and records a new manifest version." }] },
  tags: { title: "Tags", triggerLabel: "About Tags", description: "Organize resources with reusable, color-coded labels.", sections: [{ title: "Consistent naming", content: "Use short labels for a team, purpose, or environment. A shared vocabulary makes resources easier to find." }] },
  appStore: { title: "App Store", triggerLabel: "About App Store", description: "Browse ready-to-deploy applications and manage their installations.", sections: [{ title: "Installation", content: "Choose an app, review its configuration and resource requirements, and explicitly select the target node. Installed apps appear in the Installed view." }] },
  databases: { title: "Databases", triggerLabel: "About Databases", description: "One place to provision, connect, and operate database workloads.", sections: [{ title: "Database surfaces", content: "Overview summarizes the inventory. Database Hosts stores external MySQL and PostgreSQL connections; the other tabs manage containers, managed databases, and database services." }] },
  catalog: { title: "Service Catalog", triggerLabel: "About Service Catalog", description: "Provision supported databases, caches, queues, and storage services.", sections: [{ title: "Provisioning", content: "Catalog entries describe the available versions, providers, resource requirements, and configuration. Review an entry before choosing its target and provisioning it." }] },
  eggs: { title: "Eggs", triggerLabel: "About Eggs", description: "Runnable game-server definitions within a service family.", sections: [{ title: "Definition contents", content: "An egg specifies container images, startup behavior, installation steps, and the variables exposed when a server is created." }] },
  appMounts: {
    title: "App Storage",
    triggerLabel: "About App Storage",
    description: "Per-application persistent storage: volumes, binds, tmpfs and seed files",
    sections: [
      { title: "Mount types", content: "Volume mounts use a named Docker volume, bind mounts map an allowed host path, tmpfs is ephemeral in-memory storage, and seed files materialise database-stored content at a target path on every deploy." },
      { title: "Validation and deploys", content: "Definitions are validated by the server before they are stored, so reserved targets and disallowed bind prefixes are rejected here rather than at deploy time. The deploy pipeline injects each mount into the app compose document." },
      { title: "App Storage vs Storage Mounts", content: "This page declares storage a single app carries through redeploys. Storage Mounts is the node-wide allowlist of host paths a bind mount may use." },
    ],
  },
  backupEngines: {
    title: "Backup Engines",
    triggerLabel: "About Backup Engines",
    description: "Restic and Kopia repositories, snapshots, verification and restores",
    sections: [
      { title: "Repositories and snapshots", content: "Repositories hold the encrypted backend. Snapshots can be verified, restored or pruned. Passwords are sealed by the API and never returned." },
      { title: "Refresh", content: "Repositories and restore jobs poll every 30 seconds; snapshots load on demand for the selected repository." },
    ],
  },
  backups: {
    title: "Backups",
    triggerLabel: "About Backups",
    description: "Backup policies, jobs, artifacts and restores",
    sections: [
      { title: "Pipeline", content: "Policies define schedules, jobs execute them, artifacts are the stored results, and restores bring data back." },
      { title: "Refresh", content: "Every backup list polls every 30 seconds." },
    ],
  },
  cronJobs: {
    title: "Cron Jobs",
    triggerLabel: "About Cron Jobs",
    description: "Scheduled and recurring automation",
    sections: [
      { title: "Schedules", content: "Jobs run on standard 5-field cron expressions. Select a job to inspect its execution history." },
      { title: "Refresh", content: "The job list polls every 30 seconds; execution history polls every 10 seconds." },
    ],
  },
  dockerEvents: {
    title: "Docker Events",
    triggerLabel: "About Docker Events",
    description: "Live container lifecycle events streamed from every Beacon node",
    sections: [
      { title: "Live timeline", content: "Newest first and capped by design: this is a what-just-happened view, and the activity log is the place for history. Pause the feed to inspect a moment without losing your place." },
      { title: "Refresh", content: "The feed polls every 5 seconds; relative timestamps tick every 15 seconds." },
    ],
  },
  migrations: {
    title: "Migrations",
    triggerLabel: "About Migrations",
    description: "Live workload migration jobs and recovery plans",
    sections: [
      { title: "Migration vs recovery", content: "Migrations move a server between Beacons. Recovery plans restore workloads from a failed Beacon. Neither runs until it is started explicitly." },
      { title: "Refresh", content: "Migration and recovery lists poll every 10 seconds." },
    ],
  },
  operations: {
    title: "Operations",
    triggerLabel: "About Operations",
    description: "Control-plane operation history and manual controls",
    sections: [
      { title: "Plans before execution", content: "Evacuation and recovery plans are saved first, then started explicitly. Migration records track server moves between Beacon nodes." },
      { title: "Refresh", content: "Migration, recovery and installer workflow lists poll every 10 seconds." },
    ],
  },
  orphans: {
    title: "Orphaned Resources",
    triggerLabel: "About Orphaned Resources",
    description: "Servers and databases with no owning record",
    sections: [
      { title: "Resolving", content: "Force-deleted resources that could not be removed remotely are tracked here. Resolve only after manually confirming remote cleanup is complete. Resolving records the resolution; it does not retry deletion." },
    ],
  },
  reconciliation: {
    title: "Reconciliation",
    triggerLabel: "About Reconciliation",
    description: "Drift detection and desired-state reconciliation",
    sections: [
      { title: "Plans before execution", content: "Trigger a run, review diffs and drifts, then confirm. Destructive plans are flagged before confirmation." },
      { title: "Refresh", content: "Summary and plans poll every 15 seconds; events load on demand." },
    ],
  },
} satisfies Record<string, PageInfoDisclosureProps>;
