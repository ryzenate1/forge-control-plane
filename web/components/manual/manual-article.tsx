import Link from "next/link";
import type { ManualFeature } from "@/lib/features";
import Footer from "@/components/footer";

export function ManualArticle({ feature }: { feature: ManualFeature }) {
  return <main className="docs-main"><article className="manual-article">
    <div className="breadcrumbs"><Link href="/">Forge</Link><span>/</span><Link href="/manual">User manual</Link><span>/</span><span>{feature.name}</span></div>
    <p className="doc-kicker">{feature.group} / user manual</p>
    <div className="manual-title"><div><h1>{feature.name}</h1><p className="doc-summary">{feature.does} {feature.how}</p></div>{feature.status && <span>{feature.status}</span>}</div>
    <div className="manual-intent"><b>Purpose</b><p>{feature.groupSummary}</p></div>

    <section className="manual-diagram" aria-label={`${feature.name} architecture diagram`}>
      <div className="manual-diagram-head"><div><small>ARCHITECTURE</small><h2>{feature.diagram.label}</h2></div><p>{feature.diagram.boundary}</p></div>
      <div className="manual-diagram-flow">{feature.diagram.nodes.map((node, index) => <div className="manual-diagram-node" key={node}><span>{String(index + 1).padStart(2, "0")}</span><b>{node}</b>{index < feature.diagram.nodes.length - 1 && <i>{feature.diagram.edges[index] ?? "observe"}</i>}</div>)}</div>
      <div className="manual-diagram-checks">{feature.diagram.checks.map((check) => <span key={check}>✓ {check}</span>)}</div>
    </section>

    <section><p className="doc-kicker">How to use it</p><h2>Operator workflow</h2><ol className="manual-workflow">{feature.workflow.map((step, index) => <li key={step}><span>{String(index + 1).padStart(2, "0")}</span><p>{step}</p></li>)}</ol></section>
    <section className="manual-two-col"><div><p className="doc-kicker">What Forge owns</p><h2>Boundary and safety</h2><p>{feature.boundary}</p><div className="manual-callout"><b>Use deliberate changes</b><p>Review the affected node, workload, route, or secret before saving. Capture state and health evidence before changes that affect data or public traffic.</p></div></div><div><p className="doc-kicker">Before you finish</p><h2>Verification checklist</h2><ul className="manual-checklist">{feature.verify.map((check) => <li key={check}>{check}</li>)}</ul></div></section>
    <section className="manual-next"><div><p className="doc-kicker">Continue the manual</p><h2>Use this feature in the wider Forge workflow.</h2><p>Move from platform understanding to the relevant operating guide when you need exact environment, network, deployment, or recovery steps.</p></div><div><Link href="/docs/introduction" className="docs-button primary">Get started in docs <span>→</span></Link><Link href="/manual" className="docs-button">Browse all features</Link></div></section>
    <Footer />
  </article></main>;
}
