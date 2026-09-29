"use client";

import { cn } from "@/lib/utils";

type DotState =
  | "running"
  | "stopped"
  | "pending"
  | "installing"
  | "transferring"
  | "draining"
  | "failed"
  | "crashed"
  | "suspended"
  | "completed"
  | "cancelled"
  | string;

/**
 * Dot colour for one lane of the two-dot badge.
 *
 * Kept as an explicit state→class table (rather than routing through
 * `resolveTone` in `components/ui/forge/status.ts`) because the lanes need
 * hues the seven canonical tones deliberately collapse: in-flight transfer
 * states stay violet and queued states stay sky-blue so "moving data" and
 * "waiting" never render as the same chip. Anything unrecognised falls back
 * to the `unknown` treatment (grey) — never green, never zero.
 */
function stateToDotClass(state: DotState): string {
  const s = (state ?? "").toLowerCase();
  if (["running", "completed", "succeeded", "restored", "drained"].includes(s)) return "bg-[var(--success)] border-[var(--success)]";
  if (["installing", "starting", "provisioning", "preparing", "deploying"].includes(s)) return "bg-[var(--warning)] border-[var(--warning)]";
  if (["transferring", "transfer", "in_progress", "restoring", "draining"].includes(s)) return "bg-violet-500 border-violet-500";
  if (["failed", "error", "errored", "crashed", "fault", "suspended"].includes(s)) return "bg-[var(--danger)] border-[var(--danger)]";
  if (["pending", "planned", "queued", "waiting", "retrying"].includes(s)) return "bg-sky-500 border-sky-500";
  if (["stopped", "offline", "terminated", "cancelled", "unknown"].includes(s)) return "bg-[var(--text-subtle)] border-[var(--text-subtle)]";
  return "bg-[var(--text-subtle)] border-[var(--text-subtle)]";
}

export function GenerationFencedDots({
  desired,
  actual,
  generation,
  fenceGeneration,
  isFenced,
  size = 7,
  showLabel = false,
  className,
}: {
  desired?: string | null;
  actual?: string | null;
  generation?: number | null;
  fenceGeneration?: number | null;
  isFenced?: boolean;
  size?: number;
  showLabel?: boolean;
  className?: string;
}) {
  const d = desired ?? "pending";
  const a = actual ?? "pending";
  const fenced = isFenced ?? (typeof generation === "number" && typeof fenceGeneration === "number" && generation < fenceGeneration);
  const genLabel = typeof generation === "number" ? `g${generation}` : "";
  const fenceLabel = typeof fenceGeneration === "number" ? `f${fenceGeneration}` : "";
  const title = `desired:${d} actual:${a} ${genLabel}${fenceLabel ? ` / ${fenceLabel}` : ""}${fenced ? " — fenced" : ""}`;

  return (
    <div className={cn("flex flex-col items-center justify-center gap-[2px] shrink-0", className)} aria-label={title} title={title}>
      {/* stacked dots: top = desired, bottom = actual — signature StateLane */}
      <span
        aria-hidden
        className={cn("rounded-full border", stateToDotClass(d))}
        style={{ width: size, height: size }}
      />
      <span
        aria-hidden
        className={cn(
          "rounded-full border",
          stateToDotClass(a),
          fenced && "ring-2 ring-[color-mix(in_srgb,var(--danger)_70%,transparent)] ring-offset-1 ring-offset-[var(--canvas)] animate-[pulse_1.2s_ease-in-out_1]",
          a === "pending" && !fenced && "opacity-60",
        )}
        style={{ width: size, height: size }}
      />
      {showLabel && (typeof generation === "number" || typeof fenceGeneration === "number") ? (
        <span className="mt-0.5 font-mono text-[10px] leading-none text-[var(--text-subtle)]">
          {genLabel}
          {fenced ? "◉" : ""}
        </span>
      ) : null}
    </div>
  );
}

// Compact horizontal lane badge variant used in table cells / server headers.
// Signature element — use everywhere server/node state is shown, not just operations.
export function StateLanesBadge({
  desired,
  actual,
  generation,
  fenceGeneration,
  isFenced,
}: {
  desired?: string | null;
  actual?: string | null;
  generation?: number | null;
  fenceGeneration?: number | null;
  isFenced?: boolean;
}) {
  const fenced = isFenced ?? (typeof generation === "number" && typeof fenceGeneration === "number" && generation < fenceGeneration);
  return (
    <span className="inline-flex items-center gap-1.5 rounded-full border border-[var(--line)] bg-[var(--surface-raised)] px-2 py-1">
      <GenerationFencedDots
        desired={desired}
        actual={actual}
        generation={generation}
        fenceGeneration={fenceGeneration}
        isFenced={fenced}
        size={7}
      />
      <span className="font-mono text-[11px] font-semibold tracking-wider text-[var(--text-subtle)]">
        {desired ?? "—"} / {actual ?? "—"}
      </span>
      {typeof generation === "number" ? (
        <span className={cn("font-mono text-[10px]", fenced ? "text-[var(--danger)]" : "text-[var(--text-subtle)]")}>
          g{generation}
          {typeof fenceGeneration === "number" && fenceGeneration !== generation ? `→f${fenceGeneration}` : ""}
          {fenced ? " fenced" : ""}
        </span>
      ) : null}
    </span>
  );
}

// Server-card variant — denser, for server headers / lists. Ensures StateLane is signature
// on every server card, not only in operations timeline (per task: also server cards, node badges).
export function ServerStateLaneBadge(props: {
  desired?: string | null;
  actual?: string | null;
  generation?: number | null;
  fenceGeneration?: number | null;
  isFenced?: boolean;
}) {
  return <StateLanesBadge {...props} />;
}

// Node-badge variant — same signature for node health rows.
export function NodeStateLaneBadge(props: {
  desired?: string | null;
  actual?: string | null;
  generation?: number | null;
  fenceGeneration?: number | null;
  isFenced?: boolean;
}) {
  return <StateLanesBadge {...props} />;
}
