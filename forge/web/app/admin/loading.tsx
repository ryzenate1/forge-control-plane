export default function AdminLoading() {
  return (
    <div className="space-y-6">
      <div className="mb-6 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="h-8 w-48 animate-pulse rounded bg-white/[0.06]" />
          <div className="mt-2 h-4 w-72 animate-pulse rounded bg-white/[0.04]" />
        </div>
      </div>

      <div className="mb-6 grid grid-cols-2 gap-3 md:grid-cols-4">
        {[1, 2, 3, 4].map((i) => (
          <div key={i} className="rounded-xl border border-white/[0.06] bg-[#1e2536] p-4">
            <div className="mb-2 h-3 w-16 animate-pulse rounded bg-white/[0.06]" />
            <div className="h-7 w-20 animate-pulse rounded bg-white/[0.06]" />
            <div className="mt-1 h-3 w-32 animate-pulse rounded bg-white/[0.04]" />
          </div>
        ))}
      </div>

      <div className="overflow-x-auto rounded-xl border border-white/[0.06] bg-surface-card shadow-lg">
        <div className="flex h-11 items-center gap-2 border-b border-white/[0.06] bg-[#161b28] px-4">
          <div className="h-3 w-32 animate-pulse rounded bg-white/[0.06]" />
        </div>
        <div className="space-y-3 p-4">
          {[1, 2, 3, 4, 5].map((i) => (
            <div key={i} className="flex gap-4">
              <div className="h-3 w-1/3 animate-pulse rounded bg-white/[0.04]" />
              <div className="h-3 w-1/4 animate-pulse rounded bg-white/[0.04]" />
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
