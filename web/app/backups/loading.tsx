// TODO: Replace with page-specific skeleton matching actual content layout
import { DocsShell } from '@/components/docs/docs-shell';

export default function Loading() {
  return (
    <DocsShell>
      <main className="docs-main" id="main-content" role="status" aria-live="polite">
        <p className="sr-only">Loading backups…</p>
        <div className="animate-pulse space-y-4">
          <div className="h-8 bg-line/30 rounded w-1/3" />
          <div className="h-4 bg-line/30 rounded w-2/3" />
          <div className="h-32 bg-line/30 rounded" />
        </div>
      </main>
    </DocsShell>
  );
}
