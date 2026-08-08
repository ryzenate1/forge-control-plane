"use client";

import { useT } from "@/components/TranslationProvider";

export default function Loading() {
  const t = useT();
  return (
    <main aria-label={t("pages.loading.label")} className="grid min-h-screen place-items-center bg-[#090d14] p-6 text-slate-300" role="status">
      <div className="text-center">
        <div aria-hidden="true" className="mx-auto h-9 w-9 animate-spin rounded-full border-2 border-slate-700 border-t-red-500" />
        <p className="mt-3 text-sm">{t("pages.loading.status")}</p>
      </div>
    </main>
  );
}
