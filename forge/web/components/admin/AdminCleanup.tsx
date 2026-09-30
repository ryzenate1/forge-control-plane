"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, RefreshCw, Trash2, Database, Layers, Clock } from "lucide-react";
import { inspectCleanup, runCleanup } from "@/lib/api/cleanup";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { OfflineBanner } from "@/components/shared/states-offline";
import { FreshnessBadge } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
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

/** A count that has not been reported is not `0`. */
function readout(value: number | undefined): string | number {
  return typeof value === "number" ? value : "—";
}

export function AdminCleanup() {
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
  // `inspectCleanup` now rejects a payload that does not report both counters, so
  // an unreadable response lands in `isError` and renders an error state — these
  // stay `undefined` (rendered as `—`) rather than being coerced to a clean zero.
  const stale = info?.staleReservations;
  const orphaned = info?.orphanedAllocations;
  const measured = typeof stale === "number" && typeof orphaned === "number";
  const totalStale = measured ? stale + orphaned : undefined;

  const countPhrase = (value: number | undefined, noun: string) =>
    typeof value === "number" ? `${value} ${noun}` : `an unknown number of ${noun}`;
  // The inspect endpoint reports counts only — it carries no timestamp, so the
  // "last" moment this page can honestly name is its own last successful read,
  // not a server-side run time. dataUpdatedAt is 0 until the first success.
  const lastReadLabel = inspectQ.dataUpdatedAt
    ? new Date(inspectQ.dataUpdatedAt).toLocaleTimeString()
    : "—";

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => void inspectQ.refetch()} />
      <SectionHeader
        sub="Garbage collection for stale platform resources — expired placement reservations and allocations left without a server. Not per-node disk pruning, which lives under Image & Cache Cleanup."
        status={<FreshnessBadge state={sourceState(inspectQ, 30_000)} />}
        action={
          <div className="flex gap-2">
            <Btn tone="ghost" size="sm" onClick={() => void inspectQ.refetch()} disabled={inspectQ.isFetching}>
              <RefreshCw size={14} className={inspectQ.isFetching ? "animate-spin" : ""} /> Refresh
            </Btn>
            <Btn
              tone="primary"
              loading={runMut.isPending}
              disabled={inspectQ.isLoading || inspectQ.isError}
              onClick={() => {
                void (async () => {
                  const ok = await confirm({
                    title: "Run cleanup?",
                    description: measured
                      ? `This will expire ${countPhrase(stale, "stale placement reservations")} and clean ${countPhrase(orphaned, "orphaned allocations")}. This action cannot be undone.`
                      : "Inspect has not reported the current counts, so the blast radius of this run is unknown. It will still expire whatever it finds as stale. This action cannot be undone.",
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
        <Card className="p-6">
          <AdminLoadingState label="Inspecting…" />
        </Card>
      ) : inspectQ.isError ? (
        <Card className="p-4">
          <AdminErrorState
            message={`Inspect failed, so the cleanup state is unknown: ${inspectQ.error instanceof Error ? inspectQ.error.message : "request failed"}. This is not a clean platform — it is an unreadable one.`}
            retry={() => void inspectQ.refetch()}
          />
        </Card>
      ) : (
        <>
          <StatsRow
            items={[
              { label: "Stale reservations", value: readout(stale), icon: AlertTriangle, tone: stale === undefined ? "unknown" : stale > 0 ? "yellow" : "neutral" },
              { label: "Orphaned allocations", value: readout(orphaned), icon: Layers, tone: orphaned === undefined ? "unknown" : orphaned > 0 ? "red" : "neutral" },
              { label: "Total stale", value: readout(totalStale), icon: Database, tone: totalStale === undefined ? "unknown" : totalStale > 0 ? "yellow" : "green" },
              { label: "Last read", value: lastReadLabel, icon: Clock, tone: "neutral" },
            ]}
          />

          <div className="grid gap-6 md:grid-cols-2">
            <Card>
              <CardHeader title="Inspect" icon={Trash2} />
              <div className="space-y-3">
                <div className="grid grid-cols-2 gap-3">
                  <div className="rounded-xl border border-warn-line bg-warn-subtle p-4">
                    <div className="t-eyebrow text-warn">Stale reservations</div>
                    <div className="t-readout mt-1 font-mono text-warn">{readout(stale)}</div>
                    <div className="text-meta text-warn">expired placement reservations</div>
                  </div>
                  <div className="rounded-xl border border-danger-line bg-danger-subtle p-4">
                    <div className="t-eyebrow text-danger">Orphaned allocations</div>
                    <div className="t-readout mt-1 font-mono text-danger">{readout(orphaned)}</div>
                    <div className="text-meta text-danger">allocations without a server</div>
                  </div>
                </div>
                <dl className="divide-y divide-line rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] text-xs">
                  <div className="flex items-center justify-between px-3 py-2">
                    <dt className="text-[var(--text-subtle)]">Stale reservations</dt>
                    <dd className="font-mono text-[var(--text)]">{readout(stale)}</dd>
                  </div>
                  <div className="flex items-center justify-between px-3 py-2">
                    <dt className="text-[var(--text-subtle)]">Orphaned allocations</dt>
                    <dd className="font-mono text-[var(--text)]">{readout(orphaned)}</dd>
                  </div>
                  <div className="flex items-center justify-between px-3 py-2">
                    <dt className="text-[var(--text-subtle)]">Verdict</dt>
                    <dd>
                      {!measured ? (
                        <Pill tone="unknown">not measured</Pill>
                      ) : totalStale && totalStale > 0 ? (
                        <Pill tone="yellow">{totalStale} to clean</Pill>
                      ) : (
                        <Pill tone="green">nothing stale</Pill>
                      )}
                    </dd>
                  </div>
                </dl>
                <p className="text-xs leading-5 text-[var(--text-subtle)]">
                  Inspect is a dry run: it counts expired placement reservations and orphaned allocations without mutating
                  beyond the expiry marker.
                </p>
              </div>
            </Card>

            <Card>
              <CardHeader
                title="Run"
                icon={Trash2}
                action={
                  !measured ? (
                    <Pill tone="unknown">not measured</Pill>
                  ) : totalStale && totalStale > 0 ? (
                    <Pill tone="yellow">{totalStale} to clean</Pill>
                  ) : (
                    <Pill tone="green">nothing stale</Pill>
                  )
                }
              />
              <div className="space-y-3">
                <p className="text-sm leading-6 text-[var(--text-subtle)]">
                  Runs the periodic reaper on demand: expires stale reservations, deletes allocations that are not part of
                  any active placement, and records a <code className="font-mono text-xs">reservation expired</code> event.
                  Writes are rate-limited and require an admin session.
                </p>

                {runMut.isError && (
                  <div className="ui-alert ui-alert-danger">{(runMut.error as Error).message}</div>
                )}
                {runMut.isSuccess && (
                  <div className="ui-alert ui-alert-success">
                    Cleanup completed — {runMut.data.staleReservations} reservations, {runMut.data.orphanedAllocations} allocations.
                  </div>
                )}

                <div className="flex gap-2">
                  <Btn
                    tone={totalStale && totalStale > 0 ? "primary" : "ghost"}
                    loading={runMut.isPending}
                    onClick={() => {
                      void (async () => {
                        const ok = await confirm({
                          title: "Run cleanup now?",
                          description: measured
                            ? `Will clean ${countPhrase(stale, "stale reservations")} and ${countPhrase(orphaned, "orphaned allocations")}.`
                            : "Counts are currently unreported, so this run has an unknown blast radius.",
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
                  The same pass also runs on a <span className="font-mono text-[var(--text)]">5 minute</span> schedule in the
                  background, so an idle counter here is expected between runs rather than a fault.
                </div>
              </div>
            </Card>
          </div>

          <Card className="p-4">
            <p className="text-sm leading-6 text-[var(--text-subtle)]">
              Registered under <span className="font-medium text-[var(--text)]">Operations &amp; Lifecycle</span>. Failed
              remote deletions (force-delete leftovers awaiting manual cleanup) are a separate queue at{" "}
              <a href="/admin/orphans" className="font-medium text-[var(--brand)] hover:underline">
                /admin/orphans
              </a>
              {" — "}cleanup here is the <em>automatic</em> reaper for placements and allocations, not manual orphan
              resolution, and it is not the per-node disk pruning under Image &amp; Cache Cleanup.
            </p>
          </Card>
        </>
      )}

      {renderConfirm()}
    </AdminPageLayout>
  );
}
