import { fetchJSON, postJSON } from "./http";

export type CleanupInfo = {
  staleReservations: number;
  orphanedAllocations: number;
};

/**
 * The control plane serves a bare `{"staleReservations":n,"orphanedAllocations":n}`
 * (`cleanupsvc.CleanupInfo`, forge/api/internal/services/cleanup/service.go),
 * and both handlers send it un-enveloped. The `data` branch below only exists
 * for a future envelope; it is not a licence to accept anything else.
 */
export type CleanupInspectResponse = CleanupInfo & { data?: CleanupInfo };
export type CleanupRunResponse = CleanupInfo & { data?: CleanupInfo };

function countOf(obj: Record<string, unknown>, camel: string, snake: string): number | undefined {
  for (const key of [camel, snake]) {
    const raw = obj[key];
    if (typeof raw === "number" && Number.isFinite(raw)) return raw;
  }
  return undefined;
}

/**
 * Read the two counters, or **fail**.
 *
 * This used to coerce anything unrecognised into `{0, 0}` — which the page then
 * rendered as the verdict "clean". A contract change, a proxied error body or a
 * partial response all looked like a perfectly healthy platform. Unknown is not
 * zero: an unreadable response is an error the operator can see and retry, not
 * a number.
 */
function unwrapInfo(raw: unknown, what: string): CleanupInfo {
  if (!raw || typeof raw !== "object") {
    throw new Error(`${what} returned a response with no counters — the cleanup state is unknown.`);
  }
  const maybeData = (raw as { data?: unknown }).data;
  const obj = (maybeData && typeof maybeData === "object" ? maybeData : raw) as Record<string, unknown>;

  const stale = countOf(obj, "staleReservations", "stale_reservations");
  const orphaned = countOf(obj, "orphanedAllocations", "orphaned_allocations");

  if (stale === undefined || orphaned === undefined) {
    throw new Error(
      `${what} did not report both counters (stale reservations, orphaned allocations) — the cleanup state is unknown.`,
    );
  }
  return { staleReservations: stale, orphanedAllocations: orphaned };
}

export async function inspectCleanup(): Promise<CleanupInfo> {
  const raw = await fetchJSON<unknown>("/cleanup/inspect");
  return unwrapInfo(raw, "Cleanup inspect");
}

export async function runCleanup(): Promise<CleanupInfo> {
  const raw = await postJSON<unknown>("/cleanup/run", {});
  return unwrapInfo(raw, "Cleanup run");
}
