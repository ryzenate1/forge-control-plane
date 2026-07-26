export type FeatureGroup = { title: string; summary: string; items: { name: string; does: string; how: string; status?: "Planning only" | "Registry only" }[] };
export type ManualDiagram = { label: string; nodes: string[]; edges: string[]; boundary: string; checks: string[] };
export type ManualFeature = FeatureGroup["items"][number] & { slug: string; group: string; groupSummary: string; diagram: ManualDiagram; workflow: string[]; boundary: string; verify: string[] };

export const featureGroups: FeatureGroup[] = [
  { title: "Operations", summary: "Observe the control plane, understand changes, and coordinate safe work.", items: [
    { name: "Overview", does: "Shows the live control-plane summary.", how: "Aggregates health, workload, node, and operation signals." },
    { name: "Monitoring", does: "Tracks platform and node health.", how: "Reads health observations and monitoring integrations." },
    { name: "Cron Jobs", does: "Schedules administrative shell tasks.", how: "Forge API loads enabled schedules from PostgreSQL and executes them in-process." },
    { name: "Host", does: "Provides host system information and management.", how: "Uses privileged node and host capabilities; restrict access." },
    { name: "Activity", does: "Shows human-readable audit history.", how: "Displays recorded control-plane activity." },
    { name: "Migrations & Recovery", does: "Plans movement and recovery work.", how: "The registered UI is planning metadata; no workload executor is available from this area.", status: "Planning only" },
  ]},
  { title: "Infrastructure", summary: "Describe placement, networking, storage, and the nodes that execute workloads.", items: [
    { name: "Regions and Locations", does: "Organize infrastructure geography.", how: "Associate nodes with regions and locations for placement context." },
    { name: "Nodes", does: "Registers Beacon hosts.", how: "Stores node identity, capacity, health, and runtime connection details." },
    { name: "Allocations", does: "Reserves workload ports.", how: "Maps TCP or UDP port assignments to eligible nodes." },
    { name: "Database Hosts", does: "Defines workload-database provisioning targets.", how: "Keeps hosted database credentials and placement separate from Forge PostgreSQL." },
    { name: "Mounts", does: "Shares approved host paths with workloads.", how: "Attaches controlled paths to selected node workloads." },
    { name: "Files and Terminal", does: "Administers host files and shells.", how: "Uses privileged node-side access; grant only to trusted operators." },
  ]},
  { title: "Management", summary: "Create the workloads and identities your organization manages.", items: [
    { name: "Servers", does: "Manages game-server instances.", how: "Combines a template, allocation, resource limits, storage, and Beacon runtime action." },
    { name: "Apps", does: "Manages container applications, Git sources, and Compose stacks.", how: "Creates deployments and revisions that Beacon builds or runs." },
    { name: "Users and Roles", does: "Controls accounts, limits, and additional roles.", how: "Applies administrative authorization and account configuration." },
    { name: "OAuth Clients", does: "Creates user-owned OAuth clients.", how: "Stores client credentials and application authorization settings." },
  ]},
  { title: "Services", summary: "Define repeatable templates and external integrations for workloads.", items: [
    { name: "Nests & Eggs", does: "Defines canonical game-server templates.", how: "Supplies startup variables, images, and service definitions." },
    { name: "App Templates", does: "Provides reusable application deployment templates.", how: "Prepares common workload configuration for app creation." },
    { name: "Compatibility Templates", does: "Maintains legacy template compatibility.", how: "Preserves older template shapes for migration or import." },
    { name: "Webhooks", does: "Delivers lifecycle events outward.", how: "Sends configured event notifications to external endpoints." },
    { name: "Plugins", does: "Registers plugin manifests.", how: "Stores metadata only; runtime plugin execution is not available.", status: "Registry only" },
    { name: "API Keys and Settings", does: "Configures application access and panel behavior.", how: "Stores credentials and operator-selected settings." },
  ]},
  { title: "Advanced", summary: "Coordinate deployments, traffic, scaling, security, and provider integrations.", items: [
    { name: "Docker", does: "Inspects containers, images, networks, and volumes.", how: "Uses the node runtime through Beacon and Docker access." },
    { name: "Scheduler and Auto-Scaler", does: "Scores placements and applies scaling policies.", how: "Uses capacity, constraints, affinity, and policy configuration." },
    { name: "Deployments and Preview Deployments", does: "Runs rolling, blue-green, and preview delivery flows.", how: "Creates deployment records, revisions, and health-aware rollout work." },
    { name: "Failover", does: "Defines failover policy and crash simulation.", how: "Requires a tested multi-node topology; it does not establish automatic stateful ownership failover by itself." },
    { name: "Load Balancer and Traffic", does: "Defines target groups, routes, and traffic policy.", how: "Selects and manages configured routing targets; validate proxy deployment for your topology." },
    { name: "Domains and Certificates", does: "Manages custom domains and TLS certificate records.", how: "Connects domains, DNS configuration, and gateway certificate workflows." },
    { name: "Cloud", does: "Connects provider accounts and instance provisioning.", how: "Creates provider-side infrastructure; validate Beacon bootstrap and hardening before production." },
    { name: "Compose", does: "Imports and manages Compose files.", how: "Parses stack definitions and coordinates lifecycle work through the platform." },
    { name: "Social Login and mTLS", does: "Configures identity providers and mutual TLS materials.", how: "Stores and applies dedicated identity and certificate settings." },
  ]},
];

const slugify = (value: string) => value.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "");

function manualProfile(group: string, item: FeatureGroup["items"][number]): Omit<ManualFeature, "name" | "does" | "how" | "status" | "slug" | "group" | "groupSummary"> {
  const base = {
    workflow: [
      `Open ${item.name} from the administrator panel and review the current state.`,
      `Set the specific configuration that expresses the desired ${item.name.toLowerCase()} outcome.`,
      "Save deliberately, then follow the operation, activity, health, or runtime observation that confirms the result.",
      "Keep the before-and-after evidence in Activity and verify the workload path before relying on the change.",
    ],
    boundary: "Configuration belongs in Forge; execution and observation happen on the relevant Forge node through Beacon.",
    verify: ["The operator has the required administrative role.", "The intended node, workload, or project is selected.", "The resulting operation or observation reaches a terminal state.", "A rollback path exists before changing production traffic or storage."],
  };
  if (group === "Operations") return { ...base, diagram: { label: "Control-plane observation flow", nodes: ["Administrator", "Forge Web", "Forge API", "PostgreSQL", "Beacon observations"], edges: ["review", "query", "durable state", "health and activity"], boundary: "Forge control plane", checks: ["API readiness", "node heartbeat", "recorded activity"] }, boundary: "Operations reads durable state and node observations. It does not give a node independent authority to schedule work." };
  if (group === "Infrastructure") return { ...base, diagram: { label: "Node placement and runtime flow", nodes: ["Administrator", "Forge API", "Placement record", "Beacon node", "Docker runtime"], edges: ["desired state", "reserve", "authenticated command", "container/files"], boundary: "Private node network", checks: ["node health", "capacity", "allocation or path availability"] }, boundary: "Infrastructure changes affect execution boundaries: nodes, allocations, mounts, database hosts, and privileged host access. Keep Beacon and private services off the public internet." };
  if (group === "Management") return { ...base, diagram: { label: "Workload management flow", nodes: ["Administrator", "Forge Web", "Workload record", "Operation", "Beacon"], edges: ["configure", "persist", "coordinate", "execute"], boundary: "Forge workload lifecycle", checks: ["template", "resources", "allocation", "node health"] }, boundary: "The panel records workload intent. Beacon performs the Docker and filesystem action against the selected node." };
  if (group === "Services") return { ...base, diagram: { label: "Definition and integration flow", nodes: ["Administrator", "Forge API", "Definition store", "Workload or event", "External endpoint"], edges: ["configure", "persist", "apply", "deliver"], boundary: "Forge control plane", checks: ["least privilege", "secret protection", "event destination"] }, boundary: "Templates and integrations define repeatable behavior. Review variables, credentials, and event destinations before they are assigned to live workloads." };
  return { ...base, diagram: { label: "Advanced control flow", nodes: ["Administrator", "Forge API", "Policy or route", "Beacon / gateway", "Observed result"], edges: ["intent", "validate", "apply", "verify"], boundary: "Control plane and execution plane", checks: ["capacity", "health check", "rollback plan"] }, boundary: "Advanced controls can affect placement, traffic, certificates, providers, and runtime behavior. Validate in a non-critical environment before applying them to production workloads." };
}

export const manualFeatures: ManualFeature[] = featureGroups.flatMap((group) => group.items.map((item) => ({
  ...item,
  slug: slugify(item.name),
  group: group.title,
  groupSummary: group.summary,
  ...manualProfile(group.title, item),
})));

export const manualFeatureBySlug = (slug: string) => manualFeatures.find((feature) => feature.slug === slug);
