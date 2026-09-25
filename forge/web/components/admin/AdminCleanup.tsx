"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, RefreshCw, Trash2, Clock, Database, Layers } from "lucide-react";
import { inspectCleanup, runCleanup } from "@/lib/api/cleanup";
import { useT } from "@/components/TranslationProvider";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { OfflineBanner } from "@/components/shared/states-offline";
import {
  AdminPageLayout,
  SectionHeader,
  Card,
  CardHeader,
  Btn,
  Pill,
  AdminLoadingState,
  AdminErrorState,
  StatsRow,
} from "./admin-ui";

export function AdminCleanup() {
  const t = useT();
  const { toast } = useToast();
  const qc = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();

  const inspectQ = useQuery({
    queryKey: ["admin-cleanup-inspect"],
    queryFn: inspectCleanup,
    retry: false,
    refetchInterval: 30_000,
  });

  const runMut = useMutation({
    mutationFn: runCleanup,
    onSuccess: (data) => {
      void qc.invalidateQueries({ queryKey: ["admin-cleanup-inspect"] });
      toast({
        tone: "success",
        title: `Cleanup finished — ${data.staleReservations} stale reservations, ${data.orphanedAllocations} orphaned allocations`,
      });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Cleanup failed", message: e.message }),
  });

  const info = inspectQ.data;
  const stale = info?.staleReservations ?? 0;
  const orphaned = info?.orphanedAllocations ?? 0;
  const totalStale = stale + orphaned;

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => void inspectQ.refetch()} />
      <SectionHeader
        title="Cleanup"
        sub="Garbage collection — Inspect & Run cleanup for stale resources (handlers_cleanup.go:8). Stale placement reservations and orphaned allocations without a server."
        action={
          <div className="flex gap-2">
            <Btn tone="ghost" size="sm" onClick={() => void inspectQ.refetch()} disabled={inspectQ.isFetching}>
              <RefreshCw size={14} className={inspectQ.isFetching ? "animate-spin" : ""} /> Refresh
            </Btn>
            <Btn
              tone="primary"
              loading={runMut.isPending}
              disabled={inspectQ.isLoading}
              onClick={() => {
                void (async () => {
                  const ok = await confirm({
                    title: "Run cleanup?",
                    description: `This will expire ${stale} stale placement reservations and clean ${orphaned} orphaned allocations. This action cannot be undone.`,
                    confirmLabel: "Run cleanup",
                    danger: true,
                  });
                  if (ok) runMut.mutate();
                })();
              }}
            >
              <Trash2 size={14} /> Run cleanup
            </Btn>
          </div>
        }
      />

      {inspectQ.isLoading ? (
        <Card className="border border-[var(--line)] bg-[var(--surface)] p-6">
          <AdminLoadingState label="Inspecting…" />
        </Card>
      ) : inspectQ.isError ? (
        <Card className="border border-[var(--line)] bg-[var(--surface)] p-4">
          <AdminErrorState message={(inspectQ.error as Error).message} retry={() => void inspectQ.refetch()} />
        </Card>
      ) : (
        <>
          <StatsRow
            items={[
              { label: "Stale reservations", value: stale, icon: Clock, tone: stale > 0 ? "yellow" : "neutral" },
              { label: "Orphaned allocations", value: orphaned, icon: Layers, tone: orphaned > 0 ? "red" : "neutral" },
              { label: "Total stale", value: totalStale, icon: AlertTriangle, tone: totalStale > 0 ? "yellow" : "green" },
              { label: "Last inspect", value: new Date().toLocaleTimeString(), icon: Database, tone: "neutral" },
            ]}
          />

          <div className="grid gap-6 md:grid-cols-2">
            <Card className="border border-[var(--line)] bg-[var(--surface)]">
              <CardHeader title="Inspect — GET /cleanup/inspect" icon={Trash2} />
              <div className="space-y-3 p-4">
                <div className="grid grid-cols-2 gap-3">
                  <div className="rounded-xl border border-amber-500/20 bg-amber-500/10 p-4">
                    <div className="text-[11px] uppercase tracking-widest text-amber-300">Stale reservations</div>
                    <div className="mt-1 font-mono text-2xl font-bold text-amber-200">{stale}</div>
                    <div className="text-xs text-amber-200/70">expired placement reservations</div>
                  </div>
                  <div className="rounded-xl border border-red-500/20 bg-red-500/10 p-4">
                    <div className="text-[11px] uppercase tracking-widest text-red-300">Orphaned allocations</div>
                    <div className="mt-1 font-mono text-2xl font-bold text-red-200">{orphaned}</div>
                    <div className="text-xs text-red-200/70">allocations without server</div>
                  </div>
                </div>
                <pre className="overflow-auto rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3 text-xs leading-5 text-[var(--text-subtle)]">
                  {JSON.stringify(inspectQ.data, null, 2)}
                </pre>
                <p className="text-xs leading-5 text-[var(--text-subtle)]">
                  Inspect is a dry-run: it calls <code className="font-mono text-[11px]">ExpirePlacementReservations</code> to count expired rows and counts orphaned allocations ( <code className="font-mono">alloc.server == nil</code> ) without mutating beyond the expiry marker.
                </p>
              </div>
            </Card>

            <Card className="border border-[var(--line)] bg-[var(--surface)]">
              <CardHeader title="Run — POST /cleanup/run" icon={Trash2} action={totalStale > 0 ? <Pill tone="yellow">{totalStale} to clean</Pill> : <Pill tone="green">clean</Pill>} />
              <div className="space-y-3 p-4">
                <p className="text-sm leading-6 text-[var(--text-subtle)]">
                  Runs the 5-minute ticker job on demand: expires stale reservations, deletes orphaned allocations that are not part of any active placement, and emits{" "}
                  <code className="font-mono text-xs">EventReservationExpired</code>. Writes are rate-limited and require <code className="font-mono text-xs">admin</code>.
                </p>

                {runMut.isError && (
                  <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200">{(runMut.error as Error).message}</div>
                )}
                {runMut.isSuccess && (
                  <div className="rounded-lg border border-emerald-500/20 bg-emerald-500/10 p-3 text-sm text-emerald-200">
                    Cleanup completed — {runMut.data.staleReservations} reservations, {runMut.data.orphanedAllocations} allocations.
                    <pre className="mt-2 overflow-auto rounded bg-black/20 p-2 font-mono text-xs">{JSON.stringify(runMut.data, null, 2)}</pre>
                  </div>
                )}

                <div className="flex gap-2">
                  <Btn
                    tone={totalStale > 0 ? "primary" : "ghost"}
                    loading={runMut.isPending}
                    onClick={() => {
                      void (async () => {
                        const ok = await confirm({
                          title: "Run cleanup now?",
                          description: `Will clean ${stale} stale reservations and ${orphaned} orphaned allocations.`,
                          danger: true,
                          confirmLabel: "Run now",
                        });
                        if (ok) runMut.mutate();
                      })();
                    }}
                  >
                    <Trash2 size={14} /> {runMut.isPending ? "Cleaning…" : "Run now"}
                  </Btn>
                  <Btn tone="ghost" size="sm" onClick={() => void inspectQ.refetch()}>
                    Re-inspect
                  </Btn>
                </div>

                <div className="rounded-lg border border-[var(--line)] bg-[var(--canvas)] p-3 text-xs leading-5 text-[var(--text-subtle)]">
                  Background: the service also runs every <span className="font-mono text-[var(--text)]">5 min</span> via <code className="font-mono">Service.Start(ctx)</code>.
                </div>
              </div>
            </Card>
          </div>

          <Card className="border border-[var(--line)] bg-[var(--surface)] p-4">
            <p className="text-sm leading-6 text-[var(--text-subtle)]">
              Registered under <span className="font-medium text-[var(--text)]">Operations & Lifecycle</span>. Orphan remediation (force-delete leftovers) lives separately at{" "}
              <a href="/admin/orphans" className="font-medium text-[var(--brand)] hover:underline">
                /admin/orphans
              </a>{" "}
              — cleanup here is the <em>automatic</em> reaper for placements/allocations, not manual orphan resolution.
            </p>
          </Card>
        </>
      )}

      {renderConfirm()}
    </AdminPageLayout>
  );
}
