"use client";

import { AlertTriangle, RotateCcw } from "lucide-react";
import Link from "next/link";
import { useEffect } from "react";
import { Button } from "@/components/ui/primitives";

export default function Error({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => { console.error("[ApplicationError]", error); }, [error]);
  return <main className="grid min-h-screen place-items-center bg-[#090d14] p-4"><section className="ui-card w-full max-w-md p-6 text-center sm:p-8" role="alert"><span className="mx-auto grid h-14 w-14 place-items-center rounded-full bg-red-500/10 text-red-400"><AlertTriangle className="h-7 w-7" /></span><p className="mt-5 text-xs font-semibold uppercase tracking-[.18em] text-red-400">Unexpected error</p><h1 className="mt-2 text-2xl font-bold text-white">This page couldn’t be loaded</h1><p className="mt-3 text-sm leading-6 text-slate-400">Your data was not changed. Try rendering the page again, or return home if the problem continues.</p>{error.digest ? <p className="mt-3 font-mono text-xs text-slate-600">Reference: {error.digest}</p> : null}<div className="mt-6 flex flex-col gap-2 sm:flex-row sm:justify-center"><Button onClick={reset}><RotateCcw className="h-4 w-4" />Try again</Button><Link className="ui-button ui-button-secondary" href="/">Return home</Link></div></section></main>;
}
