"use client";

import { AlertTriangle, RotateCcw } from "lucide-react";
import Link from "next/link";
import { useEffect } from "react";

export default function GlobalError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    console.error("[GlobalError]", error);
  }, [error]);

  return (
    <html lang="en">
      <body className="bg-[var(--canvas)] antialiased">
        <main className="grid min-h-screen place-items-center p-4">
          <section className="w-full max-w-md rounded-2xl border border-white/[0.06] bg-[var(--surface)] p-6 text-center sm:p-8" role="alert">
            <span className="mx-auto grid h-14 w-14 place-items-center rounded-full bg-red-500/10 text-red-400">
              <AlertTriangle className="h-7 w-7" />
            </span>
            <p className="mt-5 text-xs font-semibold uppercase tracking-[.18em] text-red-400">Application error</p>
            <h1 className="mt-2 text-2xl font-bold text-white">Something went wrong</h1>
            <p className="mt-3 text-sm leading-6 text-slate-300">
              An unexpected error stopped this page from loading. Your data was not changed. Try again, or return home.
            </p>
            {error.digest ? (
              <p className="mt-3 font-mono text-xs text-slate-600">Reference: {error.digest}</p>
            ) : null}
            <div className="mt-6 flex flex-col gap-2 sm:flex-row sm:justify-center">
              <button
                className="inline-flex items-center gap-2 rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-bold text-white hover:bg-[var(--brand-dark)]"
                onClick={() => reset()}
                type="button"
              >
                <RotateCcw className="h-4 w-4" />
                Try again
              </button>
              <Link
                className="inline-flex items-center gap-2 rounded-lg border border-white/[0.12] bg-white/[0.04] px-4 py-2 text-sm font-bold text-slate-300 hover:bg-white/[0.08]"
                href="/"
              >
                Return home
              </Link>
            </div>
          </section>
        </main>
      </body>
    </html>
  );
}
