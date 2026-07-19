"use client";

import { AlertTriangle, RotateCcw } from "lucide-react";
import Link from "next/link";
import { useEffect } from "react";

export default function AdminError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => { console.error("[AdminError]", error); }, [error]);

  return (
    <div className="flex min-h-[60vh] items-center justify-center p-4">
      <section className="w-full max-w-md p-6 text-center sm:p-8" role="alert">
        <span className="mx-auto grid h-14 w-14 place-items-center rounded-full bg-red-500/10 text-red-400">
          <AlertTriangle className="h-7 w-7" />
        </span>
        <p className="mt-5 text-xs font-semibold uppercase tracking-[.18em] text-red-400">Admin error</p>
        <h1 className="mt-2 text-2xl font-bold text-white">This admin page couldn&rsquo;t be loaded</h1>
        <p className="mt-3 text-sm leading-6 text-slate-400">
          Your data was not changed. Try rendering the page again, or return to the admin dashboard.
        </p>
        {error.digest ? (
          <p className="mt-3 font-mono text-xs text-slate-600">Reference: {error.digest}</p>
        ) : null}
        <div className="mt-6 flex flex-col gap-2 sm:flex-row sm:justify-center">
          <button
            className="inline-flex items-center gap-2 rounded-lg bg-[#dc2626] px-4 py-2 text-sm font-bold text-white hover:bg-[#b91c1c] disabled:opacity-60"
            onClick={reset}
            type="button"
          >
            <RotateCcw className="h-4 w-4" />
            Try again
          </button>
          <Link
            className="inline-flex items-center gap-2 rounded-lg border border-white/[0.12] bg-white/[0.04] px-4 py-2 text-sm font-bold text-slate-300 hover:bg-white/[0.08]"
            href="/admin/overview"
          >
            Return to dashboard
          </Link>
        </div>
      </section>
    </div>
  );
}
