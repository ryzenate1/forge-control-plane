"use client";

import { useParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Container, FileCode, Terminal } from "lucide-react";
import { fetchEgg, fetchNest } from "@/lib/api";
import { AdminEggVariables } from "@/components/admin/AdminEggVariables";
import { AdminPageLayout, Btn, Card, CardHeader, SectionHeader, AdminLoadingState, AdminErrorState, cn } from "@/components/admin/admin-ui";
import { useBreadcrumbLabel } from "@/lib/nav/breadcrumb-context";
import { adminPageGuides } from "@/components/admin/admin-page-guides";

function InfoRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-baseline gap-2">
      <span className="text-[10px] font-semibold uppercase tracking-widest text-text-muted">{label}</span>
      <span className={cn("text-sm text-text", mono && "font-mono text-xs")}>{value || "\u2014"}</span>
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

  const egg = eggQuery.data;

  // Two dynamic segments, both named on the shell's single trail: the nest
  // and the egg. This page used to draw its own second trail beneath the
  // shell's, and the two disagreed about depth.
  useBreadcrumbLabel(nestId, nestQuery.data?.name ?? null);
  useBreadcrumbLabel(eggId, egg?.name ?? null);

  if (eggQuery.isLoading) {
    return (
      <AdminPageLayout>
        <SectionHeader
          title="Egg Variables"
          sub="Service definition details and environment variable schema."
          info={adminPageGuides.eggs}
          backAction={() => router.push(`/admin/nests/${nestId}/eggs`)}
          backLabel="Eggs"
        />
        <Card>
          <CardHeader title="Egg" icon={FileCode} />
          <AdminLoadingState label="Loading egg\u2026" />
        </Card>
      </AdminPageLayout>
    );
  }

  if (eggQuery.isError || !egg) {
    return (
      <AdminPageLayout>
        <SectionHeader
          title="Egg Variables"
          sub="Service definition details and environment variable schema."
          info={adminPageGuides.eggs}
          backAction={() => router.push(`/admin/nests/${nestId}/eggs`)}
          backLabel="Eggs"
        />
        <div className="flex items-center gap-2">
          <Btn tone="ghost" onClick={() => router.push(`/admin/nests/${nestId}/eggs`)}>
            <ArrowLeft size={14} /> Back to Eggs
          </Btn>
        </div>
        <div className="p-4">
          <AdminErrorState message="Could not load egg. It may have been deleted." retry={() => void eggQuery.refetch()} />
        </div>
      </AdminPageLayout>
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
    <AdminPageLayout>
      <SectionHeader
        title={`Egg: ${egg.name}`}
        sub="Service definition details and environment variable schema."
        info={adminPageGuides.eggs}
        backAction={() => router.push(`/admin/nests/${nestId}/eggs`)}
        backLabel="Eggs"
      />
      {/* Egg summary card */}
      <Card className="p-5 sm:p-6">
        <CardHeader title="Egg Details" icon={FileCode} />
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
          <div className="space-y-3">
            <h2 className="text-lg font-semibold text-text">{egg.name}</h2>
            {egg.description && (
              <p className="text-sm leading-relaxed text-text-subtle">{egg.description}</p>
            )}
          </div>

          <div className="space-y-2.5">
            <h3 className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-widest text-text-muted">
              <Container size={12} /> Docker Images
            </h3>
            <div className="space-y-1">
              {dockerImages.length > 0 ? dockerImages.map((img, i) => (
                <code key={i} className="block truncate rounded bg-overlay px-2 py-1 font-mono text-[11px] text-text">
                  {img}
                </code>
              )) : <span className="text-xs text-text-muted">No images set</span>}
            </div>
          </div>

          <div className="space-y-2.5">
            <h3 className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-widest text-text-muted">
              <Terminal size={12} /> Startup
            </h3>
            <code className="block rounded bg-overlay px-2 py-1.5 font-mono text-[11px] leading-relaxed text-text">
              {egg.startup || "\u2014"}
            </code>
          </div>

          <div className="space-y-2.5">
            <h3 className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-widest text-text-muted">
              <FileCode size={12} /> Install
            </h3>
            <div className="space-y-1 text-xs text-text-subtle">
              <InfoRow label="Image" value={egg.installContainer || "alpine:3.21"} mono />
              <InfoRow label="Entrypoint" value={egg.installEntrypoint || "sh"} mono />
              <InfoRow label="Memory" value={egg.defaultMemoryMb ? `${egg.defaultMemoryMb} MB` : "Not set"} />
            </div>
          </div>
        </div>
      </Card>

      {/* Variables section */}
      <AdminEggVariables egg={egg} />
    </AdminPageLayout>
  );
}
