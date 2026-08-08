import Link from "next/link";
import { DocsShell } from "@/components/docs/docs-shell";
import { featureGroups, manualFeatures } from "@/lib/features";
import Footer from "@/components/footer";

export default function UserManualPage() {
  const featuresByGroup = new Map<string, typeof manualFeatures>();
  for (const f of manualFeatures) {
    const list = featuresByGroup.get(f.group) ?? [];
    list.push(f);
    featuresByGroup.set(f.group, list);
  }
  return <DocsShell><main className="docs-main"><article className="manual-index">
    <div className="breadcrumbs"><Link href="/">Forge</Link><span>/</span><span>User manual</span></div>
    <p className="doc-kicker">Forge Control Plane</p><h1>The Forge user manual.</h1><p className="doc-summary">Open any panel capability for a focused explanation of what it controls, how it moves through Forge, its operational boundary, and what to verify before you rely on it.</p>
    <section className="manual-index-start"><div><small>START WITH THE PLATFORM</small><b>Understand the control plane before changing a workload.</b><p>Forge records intent and operational state; Beacon performs node-level work. Every manual page makes that boundary explicit.</p></div><Link className="docs-button primary" href="/docs/introduction">Get started <span>→</span></Link></section>
    {featureGroups.map((group) => <section className="manual-index-group" key={group.title}><header><p>{group.title}</p><h2>{group.summary}</h2></header><div>{(featuresByGroup.get(group.title) ?? []).map((feature) => <Link href={`/manual/${feature.slug}`} key={feature.slug}><span>{feature.status ?? "Available"}</span><b>{feature.name}</b><p>{feature.does}</p><i>Read manual →</i></Link>)}</div></section>)}
    <Footer />
  </article></main></DocsShell>;
}
