"use client";

import { useParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, ChevronRight, Container, Cpu, FileCode, Terminal } from "lucide-react";
import { fetchEgg, fetchNest } from "@/lib/api";
import { AdminEggVariables } from "@/components/admin/AdminEggVariables";
import { Btn, Card, cn } from "@/components/admin/admin-ui";

function InfoRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-baseline gap-2">
      <span className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">{label}</span>
      <span className={cn("text-sm text-slate-200", mono && "font-mono text-xs")}>{value || "\u2014"}</span>
    </div>
  );
}

export default function EggVariablesPage() {
  const params = useParams();
  const router = useRouter();
  const eggId = params.eggId as string;
  const nestId = params.nestId as string;

  const nestQuery = useQuery({
    queryKey: ["nest", nestId],
    queryFn: () => fetchNest(nestId),
    enabled: Boolean(nestId),
  });
  const eggQuery = useQuery({
    queryKey: ["egg", eggId],
    queryFn: () => fetchEgg(eggId),
    enabled: Boolean(eggId),
  });

  const nest = nestQuery.data;
  const egg = eggQuery.data;

  if (eggQuery.isLoading) {
    return (
      <div className="flex items-center justify-center py-20 text-sm text-slate-500">
        Loading egg\u2026
      </div>
    );
  }

  if (eggQuery.isError || !egg) {
    return (
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <Btn tone="ghost" onClick={() => router.push(`/admin/nests/${nestId}/eggs`)}>
            <ArrowLeft size={14} /> Back to Eggs
          </Btn>
        </div>
        <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-4 text-sm text-red-200">
          Could not load egg. It may have been deleted.
        </div>
      </div>
    );
  }

  const dockerImages = (() => {
    const val = egg.dockerImages;
    if (!val) return egg.dockerImage ? [egg.dockerImage] : [];
    if (Array.isArray(val)) return val;
    if (typeof val === "object" && val !== null) return Object.values(val);
    return [];
  })();

  return (
    <div className="space-y-6">
      {/* Navigation — breadcrumb with readable names */}
      <nav className="flex items-center gap-1.5 text-xs text-slate-500">
        <button onClick={() => router.push("/admin/nests")} className="transition hover:text-slate-300" type="button">Nests</button>
        <ChevronRight size={12} className="text-slate-600" />
        <button onClick={() => router.push(`/admin/nests/${nestId}/eggs`)} className="transition hover:text-slate-300" type="button">
          {nest?.name ?? "Nest"}
        </button>
        <ChevronRight size={12} className="text-slate-600" />
        <span className="text-slate-300">{egg.name}</span>
        <ChevronRight size={12} className="text-slate-600" />
        <span className="text-slate-400">Variables</span>
      </nav>

      {/* Egg summary card */}
      <Card className="p-5 sm:p-6">
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
          <div className="space-y-3">
            <h2 className="text-lg font-semibold text-slate-100">{egg.name}</h2>
            {egg.description && (
              <p className="text-sm leading-relaxed text-slate-400">{egg.description}</p>
            )}
          </div>

          <div className="space-y-2.5">
            <h3 className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-widest text-slate-500">
              <Container size={12} /> Docker Images
            </h3>
            <div className="space-y-1">
              {dockerImages.length > 0 ? dockerImages.map((img, i) => (
                <code key={i} className="block truncate rounded bg-white/[0.04] px-2 py-1 font-mono text-[11px] text-slate-300">
                  {img}
                </code>
              )) : <span className="text-xs text-slate-600">No images set</span>}
            </div>
          </div>

          <div className="space-y-2.5">
            <h3 className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-widest text-slate-500">
              <Terminal size={12} /> Startup
            </h3>
            <code className="block rounded bg-white/[0.04] px-2 py-1.5 font-mono text-[11px] leading-relaxed text-slate-300">
              {egg.startup || "\u2014"}
            </code>
          </div>

          <div className="space-y-2.5">
            <h3 className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-widest text-slate-500">
              <FileCode size={12} /> Install
            </h3>
            <div className="space-y-1 text-xs text-slate-400">
              <InfoRow label="Image" value={egg.installContainer || "alpine:3.21"} mono />
              <InfoRow label="Entrypoint" value={egg.installEntrypoint || "sh"} mono />
              <InfoRow label="Memory" value={egg.defaultMemoryMb ? `${egg.defaultMemoryMb} MB` : "Not set"} />
            </div>
          </div>
        </div>
      </Card>

      {/* Variables section */}
      <AdminEggVariables egg={egg} />
    </div>
  );
}
