"use client";

import { AlertTriangle, RotateCcw } from "lucide-react";
import Link from "next/link";
import { useEffect } from "react";
import { Button } from "@/components/ui/primitives";
import { useT } from "@/components/TranslationProvider";

export default function Error({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  const t = useT();
  useEffect(() => { console.error("[ApplicationError]", error); }, [error]);
  return <main className="grid min-h-screen place-items-center bg-[var(--canvas)] p-4"><section className="ui-card w-full max-w-md p-6 text-center sm:p-8" role="alert"><span className="mx-auto grid h-14 w-14 place-items-center rounded-full bg-red-500/10 text-red-400"><AlertTriangle className="h-7 w-7" /></span><p className="mt-5 text-xs font-semibold uppercase tracking-[.18em] text-red-400">{t("pages.error.badge")}</p><h1 className="mt-2 text-2xl font-bold text-white">{t("pages.error.title")}</h1><p className="mt-3 text-sm leading-6 text-slate-400">{t("pages.error.description")}</p>{error.digest ? <p className="mt-3 font-mono text-xs text-slate-600">{t("pages.error.reference", { digest: error.digest })}</p> : null}<div className="mt-6 flex flex-col gap-2 sm:flex-row sm:justify-center"><Button onClick={reset}><RotateCcw className="h-4 w-4" />{t("pages.error.tryAgain")}</Button><Link className="ui-button ui-button-secondary" href="/">{t("pages.error.returnHome")}</Link></div></section></main>;
}
