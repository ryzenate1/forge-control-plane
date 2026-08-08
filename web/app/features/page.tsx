import Link from "next/link";
import { DocsShell } from "@/components/docs/docs-shell";
import { featureGroups, manualFeatures } from "@/lib/features";
import Footer from "@/components/footer";

export default function FeaturesPage() {
  const featureMap = new Map(manualFeatures.map(f => [f.name, f]));
  return <DocsShell><main className="docs-main"><article className="feature-page">
    <div className="breadcrumbs"><Link href="/">Forge</Link><span>/</span><span>Features</span></div>
    <p className="doc-kicker">Forge Control Plane</p>
    <h1>Everything the panel can manage.</h1>
    <p className="doc-summary">This is the practical map of Forge&apos;s administrator panel: what each area controls, how it fits into the platform, and where capability is intentionally limited today.</p>
    <div className="feature-hero-actions"><Link href="/docs/introduction" className="docs-button primary">Get started <span>→</span></Link><Link href="/docs/architecture" className="docs-button">Read the architecture</Link></div>
    <div className="feature-truth"><b>How to read this page</b><span>Available areas are backed by the current panel registry. Planning-only and registry-only areas are labelled so operators do not mistake navigation for a completed runtime workflow.</span></div>
    {featureGroups.map((group) => <section className="feature-group" key={group.title}><div className="feature-group-heading"><p>{group.title}</p><h2>{group.summary}</h2></div><div className="feature-list">{group.items.map((item) => { const feature = featureMap.get(item.name); return <Link className="feature-card-link" href={`/manual/${feature?.slug ?? ""}`} key={item.name}><div><h3>{item.name}</h3>{item.status && <small>{item.status}</small>}</div><p><b>What it does</b>{item.does}</p><p><b>How it works</b>{item.how}</p><em>Open user manual →</em></Link>; })}</div></section>)}
    <section className="feature-next"><p className="doc-kicker">Start with a workflow</p><h2>One calm path through Forge.</h2><ol><li>Create the organization, project, and environment that own the workload.</li><li>Register a Beacon node and verify its health.</li><li>Create allocations, templates, mounts, and database hosts as needed.</li><li>Deploy an application or game server, then watch operations and health.</li><li>Configure backup storage and practice a restore before relying on recovery.</li></ol><Link href="/docs/introduction" className="docs-button primary">Get started in the docs <span>→</span></Link></section>
    <Footer />
  </article></main></DocsShell>;
}
