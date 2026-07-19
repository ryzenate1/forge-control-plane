import Link from "next/link";
import { ArrowLeft, FileQuestion } from "lucide-react";

export default function NotFound() {
  return <main className="grid min-h-screen place-items-center bg-[#090d14] p-4"><section className="ui-card w-full max-w-md p-6 text-center sm:p-8"><span className="mx-auto grid h-14 w-14 place-items-center rounded-full bg-white/[0.05] text-slate-400"><FileQuestion className="h-7 w-7" /></span><p className="mt-5 text-xs font-semibold uppercase tracking-[.18em] text-red-400">404 · Not found</p><h1 className="mt-2 text-2xl font-bold text-white">That page isn’t here</h1><p className="mt-3 text-sm leading-6 text-slate-400">The address may be incorrect, or the page may have moved.</p><Link className="ui-button ui-button-primary mt-6" href="/"><ArrowLeft className="h-4 w-4" />Return home</Link></section></main>;
}
