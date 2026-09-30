export default function ConsoleLoading() {
  return (
    <div className="space-y-6" role="status" aria-live="polite" aria-label="Loading console">
      <div className="flex flex-col gap-3">
        <div className="h-6 w-40 animate-pulse rounded bg-white/[0.06]" />
        <div className="h-4 w-64 animate-pulse rounded bg-white/[0.04]" />
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {[1, 2, 3, 4, 5, 6].map((i) => (
          <div key={i} className="rounded-xl border border-white/[0.06] bg-[var(--surface)] p-4">
            <div className="mb-3 h-4 w-24 animate-pulse rounded bg-white/[0.06]" />
            <div className="h-6 w-32 animate-pulse rounded bg-white/[0.06]" />
            <div className="mt-2 h-3 w-full animate-pulse rounded bg-white/[0.04]" />
          </div>
        ))}
      </div>
      <div className="rounded-xl border border-white/[0.06] bg-[var(--surface)] p-4">
        <div className="mb-3 h-4 w-32 animate-pulse rounded bg-white/[0.06]" />
        <div className="space-y-3">
          {[1, 2, 3].map((i) => (
            <div key={i} className="flex gap-4">
              <div className="h-3 w-1/3 animate-pulse rounded bg-white/[0.04]" />
              <div className="h-3 w-1/4 animate-pulse rounded bg-white/[0.04]" />
              <div className="h-3 w-1/5 animate-pulse rounded bg-white/[0.04]" />
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
