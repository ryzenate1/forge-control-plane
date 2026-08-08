import Link from "next/link";
import { HealthDashboardWrapper } from "@/components/dynamic-wrappers";
import { DocsShell } from "@/components/docs/docs-shell";
import Footer from "@/components/footer";

export default function HealthPage() {
  return (
    <DocsShell>
      <main className="docs-main">
        <article>
          <div className="breadcrumbs">
            <Link href="/">Forge</Link><span>/</span>
            <Link href="/features">Features</Link><span>/</span>
            <span>Health</span>
          </div>
          <p className="doc-kicker">Beacon Management</p>
          <h1 className="font-serif text-4xl font-semibold tracking-tight text-ink">Health Dashboard</h1>
          <p className="doc-summary">Live health status of the Beacon node and subsystems.</p>
          <HealthDashboardWrapper />
        </article>
        <Footer />
      </main>
    </DocsShell>
  );
}
