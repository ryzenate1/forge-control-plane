"use client";

import { useParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Container, FileCode, Terminal } from "lucide-react";
import { fetchEgg, fetchNest } from "@/lib/api";
import { AdminEggVariables } from "@/components/admin/AdminEggVariables";
import { AdminErrorState, AdminLoadingState, AdminPageLayout, Btn, Card, SectionHeader, cn } from "@/components/admin/admin-ui";
import { useBreadcrumbLabel } from "@/lib/nav/breadcrumb-context";

function InfoRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-baseline gap-2">
      <span className="t-meta">{label}</span>
      <span className={cn("text-sm text-text", mono && "font-mono text-xs")}>{value || "—"}</span>
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

  const backToEggs = (
    <Btn tone="ghost" ariaLabel="Back to egg definitions" onClick={() => router.push(`/admin/nests/${nestId}/eggs`)}>
      <ArrowLeft size={14} /> Eggs
    </Btn>
  );

  // Every branch renders the same frame, so the deepest level of the drill-down
  // does not change geometry depending on query state.
  if (eggQuery.isLoading) {
    return (
      <AdminPageLayout>
        <SectionHeader title="Egg" backAction={() => router.push(`/admin/nests/${nestId}/eggs`)} backLabel="Eggs" />
        <AdminLoadingState label="Loading the egg definition…" />
      </AdminPageLayout>
    );
  }

  if (eggQuery.isError || !egg) {
    return (
      <AdminPageLayout>
        <SectionHeader title="Egg" backAction={() => router.push(`/admin/nests/${nestId}/eggs`)} backLabel="Eggs" />
        <AdminErrorState
          message={`This egg could not be loaded: ${eggQuery.error instanceof Error ? eggQuery.error.message : "Unknown error"}. It may have been deleted.`}
          retry={() => void eggQuery.refetch()}
        />
        {backToEggs}
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
        backAction={() => router.push(`/admin/nests/${nestId}/eggs`)}
        backLabel="Eggs"
      />
      {/* Egg summary card */}
      <Card className="p-5 sm:p-6">
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
          <div className="space-y-3">
            <h2 className="text-lg font-semibold text-text">Definition summary</h2>
            {egg.description && (
              <p className="text-sm leading-relaxed text-text-subtle">{egg.description}</p>
            )}
          </div>

          <div className="space-y-2.5">
            <h3 className="flex items-center gap-1.5 t-meta">
              <Container size={12} /> Docker images
            </h3>
            <div className="space-y-1">
              {dockerImages.length > 0 ? dockerImages.map((img, i) => (
                <code key={i} className="block break-all rounded bg-overlay-subtle px-2 py-1 font-mono text-xs text-text">
                  {img}
                </code>
              )) : <span className="text-xs text-text-muted">No images set</span>}
            </div>
          </div>

          <div className="space-y-2.5">
            <h3 className="flex items-center gap-1.5 t-meta">
              <Terminal size={12} /> Startup
            </h3>
            <code className="block break-all rounded bg-overlay-subtle px-2 py-1.5 font-mono text-xs leading-relaxed text-text">
              {egg.startup || "—"}
            </code>
          </div>

          <div className="space-y-2.5">
            <h3 className="flex items-center gap-1.5 t-meta">
              <FileCode size={12} /> Install
            </h3>
            <div className="space-y-1 text-xs text-text-subtle">
              {/* No fabricated defaults: the old `|| "alpine:3.21"` and `|| "sh"`
                  presented values the definition does not have. */}
              <InfoRow label="Container" value={egg.installContainer || "Not set"} mono />
              <InfoRow label="Entrypoint" value={egg.installEntrypoint || "Not set"} mono />
              <InfoRow label="Memory" value={egg.defaultMemoryMb ? `${egg.defaultMemoryMb} MiB` : "Not set"} />
            </div>
          </div>
        </div>
      </Card>

      {/* Variables section */}
      <AdminEggVariables egg={egg} />
    </AdminPageLayout>
  );
}
