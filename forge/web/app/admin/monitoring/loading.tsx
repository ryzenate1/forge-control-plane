export default function MonitoringLoading() {
  return (
    <div className="mx-auto w-full max-w-[1280px] space-y-6" role="status" aria-live="polite" aria-label="Loading monitoring">
      <div className="border-b border-[var(--line)] pb-6">
        <div className="h-3 w-40 animate-pulse rounded bg-white/[0.06]" />
        <div className="mt-3 h-8 w-56 animate-pulse rounded bg-white/[0.06]" />
        <div className="mt-2 h-4 w-96 animate-pulse rounded bg-white/[0.04]" />
      </div>
      <div className="flex gap-3 border-y border-[var(--line)] py-3">
        <div className="h-8 w-40 animate-pulse rounded-lg bg-white/[0.05]" />
        <div className="h-8 w-48 animate-pulse rounded-full bg-white/[0.05]" />
        <div className="ml-auto h-8 w-40 animate-pulse rounded-lg bg-white/[0.05]" />
      </div>
      <div className="h-[380px] animate-pulse rounded-xl border border-[var(--line)] bg-[var(--surface)]" />
      <div className="grid grid-cols-12 gap-6">
        <div className="col-span-12 lg:col-span-8 h-64 animate-pulse rounded-xl border border-[var(--line)] bg-[var(--surface)]" />
        <div className="col-span-12 lg:col-span-4 h-64 animate-pulse rounded-xl border border-[var(--line)] bg-[var(--surface)]" />
      </div>
    </div>
  );
}
