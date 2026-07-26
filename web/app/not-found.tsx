import Link from "next/link";
import { DocsShell } from "@/components/docs/docs-shell";

export default function NotFound() {
  return (
    <DocsShell>
      <main className="docs-main flex flex-col items-center justify-center p-8 text-center">
        <h1 className="font-serif text-6xl font-semibold text-ink mb-4">404</h1>
        <p className="text-lg text-muted mb-8">The page you&apos;re looking for doesn&apos;t exist.</p>
        <Link href="/" className="docs-button primary">Back to Home</Link>
      </main>
    </DocsShell>
  );
}
