export default function ServerDetailLoading() {
  return (
    <div className="space-y-6" role="status" aria-live="polite" aria-label="Loading server detail">
      <div className="flex flex-col gap-3">
        <div className="h-7 w-56 animate-pulse rounded bg-white/[0.06]" />
        <div className="h-3 w-96 animate-pulse rounded bg-white/[0.04]" />
      </div>
      <div className="flex gap-2 border-b border-white/[0.06] pb-3">
        {[1, 2, 3, 4, 5].map((i) => (
          <div key={i} className="h-8 w-20 animate-pulse rounded-lg bg-white/[0.05]" />
        ))}
      </div>
      <div className="rounded-xl border border-white/[0.06] bg-[var(--surface)] p-6">
        <div className="h-64 animate-pulse rounded bg-white/[0.04]" />
      </div>
    </div>
  );
}
