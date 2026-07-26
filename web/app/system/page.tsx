import Link from "next/link";
import dynamic from "next/dynamic";
const SystemInfoDisplay = dynamic(() => import("@/components/system-info").then(m => ({ default: m.SystemInfoDisplay })), { ssr: false });
import { DocsShell } from "@/components/docs/docs-shell";
import Footer from "@/components/footer";

export default function SystemPage() {
  return (
    <DocsShell>
      <main className="docs-main">
        <article>
          <div className="breadcrumbs">
            <Link href="/">Forge</Link><span>/</span>
            <Link href="/features">Features</Link><span>/</span>
            <span>System</span>
          </div>
          <p className="doc-kicker">Beacon Management</p>
          <h1 className="font-serif text-4xl font-semibold tracking-tight text-ink">System Information</h1>
          <p className="doc-summary">Version, resources, Docker status, and runtime details.</p>
          <SystemInfoDisplay />
        </article>
        <Footer />
      </main>
    </DocsShell>
  );
}
