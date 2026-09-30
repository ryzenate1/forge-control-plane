export default function ServerLoading() {
  return (
    <div className="space-y-6" role="status" aria-live="polite" aria-label="Loading server">
      <div className="h-8 w-48 animate-pulse rounded bg-white/[0.06]" />
      <div className="h-4 w-72 animate-pulse rounded bg-white/[0.04]" />
      <div className="grid gap-3 sm:grid-cols-2">
        {[1, 2, 3, 4].map((i) => (
          <div key={i} className="rounded-xl border border-white/[0.06] bg-[var(--surface)] p-4">
            <div className="mb-3 h-4 w-24 animate-pulse rounded bg-white/[0.06]" />
            <div className="h-6 w-32 animate-pulse rounded bg-white/[0.06]" />
          </div>
        ))}
      </div>
    </div>
  );
}
