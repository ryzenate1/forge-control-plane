export default function Loading() {
  return (
    <main aria-label="Loading page" className="grid min-h-screen place-items-center bg-[#090d14] p-6 text-slate-300" role="status">
      <div className="text-center">
        <div aria-hidden="true" className="mx-auto h-9 w-9 animate-spin rounded-full border-2 border-slate-700 border-t-red-500" />
        <p className="mt-3 text-sm">Loading…</p>
      </div>
    </main>
  );
}
