import Link from "next/link";
import { DocsShell } from "@/components/docs/docs-shell";
import Footer from "@/components/footer";

const panelMap = [
  { name: "Operations", summary: "Overview, monitoring, schedules, host controls, activity, and recovery planning." },
  { name: "Infrastructure", summary: "Regions, locations, Beacon nodes, allocations, database hosts, mounts, files, and terminal access." },
  { name: "Management", summary: "Game servers, applications, users, roles, and OAuth clients." },
  { name: "Services", summary: "Game and application templates, webhooks, API keys, plugins, and settings." },
  { name: "Advanced", summary: "Docker, scheduling, deployments, traffic, domains, certificates, cloud, Compose, and security." },
] as const;

const beaconLinks = [
  { name: "Health Dashboard", description: "Live health status of the Beacon node and subsystems.", href: "/health" },
  { name: "System Information", description: "Version, resources, Docker status, and runtime details.", href: "/system" },
  { name: "Backup Management", description: "Create, restore, and delete server backups.", href: "/backups" },
] as const;

export default function Home() {
  return <DocsShell><main className="docs-main"><article className="forge-start">
    <section className="forge-hero"><p className="doc-kicker">Forge Control Plane</p><h1>One place to direct the infrastructure you run.</h1><p className="doc-summary">Forge gives operators a shared control plane for applications, game servers, databases, network allocations, backups, and the nodes that execute them.</p><div className="home-actions"><Link className="docs-button primary" href="/docs/introduction">Get started <span>→</span></Link><Link className="docs-button" href="/features">Explore all features</Link></div></section>

    <section className="start-explainer beacon-links"><div><p className="doc-kicker">Beacon Management</p><h2>Monitor and manage your nodes.</h2><p>Real-time health checks, system resource monitoring, and server backup management for Beacon nodes.</p></div><div className="start-card"><b>Quick access</b><div className="mt-3 flex flex-col gap-2">{beaconLinks.map((link) => <Link key={link.name} href={link.href} className="block rounded-lg border border-line bg-paper px-4 py-3 font-bold text-ink hover:border-red-300 hover:text-red-dark">{link.name}<span className="mt-0.5 block text-xs font-normal text-muted">{link.description}</span></Link>)}</div></div></section>

    <section className="forge-diagram" aria-label="Forge architecture diagram"><div className="forge-diagram-public"><small>PUBLIC ACCESS</small><b>Browser</b><span>Forge Web and Forge API</span></div><i>HTTPS</i><div className="forge-diagram-control"><small>FORGE CONTROL PLANE</small><b>Desired state · operations · health</b><span>PostgreSQL is the durable record. Redis supports coordination.</span></div><i>authenticated commands and observations</i><div className="forge-diagram-nodes"><div><small>BEACON NODE A</small><b>Docker · files · SFTP</b></div><div><small>BEACON NODE B</small><b>capacity for recovery</b></div></div></section>

    <section className="start-explainer"><div><p className="doc-kicker">What Forge is</p><h2>A control plane, not just a panel.</h2><p>Forge Web gives people a clear interface. Forge API records what should exist, schedules durable work, and coordinates status. PostgreSQL keeps the durable control-plane state. Nodes do the physical execution work through Beacon.</p></div><div className="start-card"><b>What happens when you deploy?</b><ol><li>An operator describes the workload.</li><li>Forge records and coordinates the operation.</li><li>Beacon runs the node action through Docker.</li><li>Health and observations return to Forge.</li></ol></div></section>

    <section className="concept-grid"><article><small>01 / BEACON</small><h2>The node agent.</h2><p>Beacon runs where workloads run. It has the Docker socket, the workload data directory, console, SFTP, backup adapters, and health reporting. It is not another web panel and it does not schedule independently.</p><Link href="/docs/beacon">Understand Beacon →</Link></article><article><small>02 / HEALTH</small><h2>Observe before you act.</h2><p>Forge combines platform health, node observations, container state, operations, logs, and monitoring. Use this evidence before moving work, changing traffic, or making recovery decisions.</p><Link href="/docs/architecture">See the architecture →</Link></article><article><small>03 / RECOVERY</small><h2>Restore from shared evidence.</h2><p>Recovery is built around a verified backup, eligible destination capacity, and a Beacon that can access shared storage. It is restore-oriented, not automatic ownership failover.</p><Link href="/docs/backup-recovery">Read recovery guidance →</Link></article><article><small>04 / EVACUATION</small><h2>Move deliberately.</h2><p>Evacuation needs more than a button: drain the source, verify a second healthy Beacon, reserve destination capacity, restore or redeploy, then validate before retiring the source.</p><Link href="/docs/backup-recovery">Plan an evacuation →</Link></article></section>

    <section className="panel-map"><div><p className="doc-kicker">The administrator panel</p><h2>Every section has a job.</h2><p>The panel groups control-plane decisions by the part of Forge they affect. Some advanced areas are intentionally marked as planning or registry-only where their runtime path is not complete.</p><Link href="/features" className="docs-button primary">See all panel features <span>→</span></Link></div><div>{panelMap.map((section) => <Link href="/features" key={section.name}><b>{section.name}</b><span>{section.summary}</span><i>→</i></Link>)}</div></section>

    <section className="startup-guide"><p className="doc-kicker">Startup guide</p><h2>Build a safe first workload.</h2><div>{[["01", "Establish ownership", "Create the organization, project, and environment that will own the workload."], ["02", "Add execution", "Enroll a Beacon node, confirm its health, and understand its capacity and storage."], ["03", "Prepare access", "Create a template, allocation, optional mount, and workload database host."], ["04", "Deploy and observe", "Create the app or game server, then watch the operation, logs, and health signals."], ["05", "Prove recovery", "Configure backups and perform a restore test before the workload is important."]].map(([number, title, body]) => <article key={number}><small>{number}</small><h3>{title}</h3><p>{body}</p></article>)}</div><Link href="/docs/introduction" className="docs-button primary">Get started in Forge Docs <span>→</span></Link></section>
    <Footer />
  </article></main></DocsShell>;
}
