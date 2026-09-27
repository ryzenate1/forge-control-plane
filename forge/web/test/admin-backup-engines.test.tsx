import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/admin/backups/engines",
  useSearchParams: () => new URLSearchParams(),
}));

import { renderWithQuery } from "@/test/render";
import { jsonResponse, mockFetchByUrl } from "@/test/fetch-mock";
import BackupEnginesPage from "@/app/admin/backups/engines/page";

/**
 * The invariant these tests protect: listing snapshots is not a database read.
 * `GET /admin/backup-engines/snapshots` shells out to the restic/kopia CLI on
 * the execution target (its handler uses `longRequestContext`, not the
 * 5-second request context), and a repository that was never initialised
 * answers 400 — which would surface in the UI as if the feature were broken.
 *
 * So the page must ask for snapshots only for a selected repository that is
 * actually initialised. The page auto-selects the first repository, which
 * makes the load-time behaviour worth pinning: with an uninitialised
 * repository at the head of the list, no CLI invocation may occur at all.
 *
 * Assertions go through the fetch mock rather than rendered copy so that
 * restyling the page cannot silently retire them.
 */

const prunePolicy = {
  schedule: "0 3 * * *",
  keepLast: 14,
  keepHourly: 0,
  keepDaily: 7,
  keepWeekly: 4,
  keepMonthly: 6,
  prune: true,
  paths: ["/var/lib/forge"],
};

const initializedRepo = {
  id: "r1",
  name: "nightly-offsite",
  engine: "restic",
  location: "s3:s3.amazonaws.com/forge-backups/nightly",
  encryption: "AES-256",
  prunePolicy,
  initialized: true,
  lastSnapshotAt: "2026-09-27T06:00:00Z",
  nextRunAt: "2026-09-28T03:00:00Z",
  createdAt: "2026-09-01T00:00:00Z",
};

const uninitializedRepo = {
  id: "r2",
  name: "staging-scratch",
  engine: "kopia",
  location: "/mnt/backups/staging",
  encryption: "",
  prunePolicy: { ...prunePolicy, schedule: "", paths: [] },
  initialized: false,
  createdAt: "2026-09-20T00:00:00Z",
};

const snapshots = [
  {
    id: "s1",
    repoId: "r1",
    snapshotId: "abc123def456789abc",
    paths: ["/var/lib/forge"],
    hostname: "edge-1",
    timestamp: "2026-09-27T06:00:00Z",
    dataFiles: 1204,
    totalSizeBytes: 1048576,
    verifiedAt: "2026-09-27T06:05:00Z",
  },
];

/** Every route the page may legitimately reach. Anything else throws. */
function route(repositories: unknown[]) {
  return mockFetchByUrl({
    "/admin/backup-engines/repositories": jsonResponse({ data: repositories }),
    "/admin/backup-engines/snapshots?repoId=r1": jsonResponse({ data: snapshots }),
    "/admin/backup-engines/restore?repoId=r1": jsonResponse({ data: [] }),
    "/admin/backup-engines/restore?repoId=r2": jsonResponse({ data: [] }),
    "/nodes?page=1&per_page=100": jsonResponse({ data: [], meta: { pagination: { total: 1 } } }),
  });
}

function snapshotCalls(calls: Array<{ url: string }>) {
  return calls.filter((call) => call.url.includes("/backup-engines/snapshots"));
}

describe("admin backup engines", () => {
  it("never invokes the engine CLI for an uninitialised repository", async () => {
    // Uninitialised repository at the head of the list, so the page's
    // auto-selection lands on it.
    const { calls } = route([uninitializedRepo, initializedRepo]);
    renderWithQuery(<BackupEnginesPage />);

    expect(await screen.findByText("staging-scratch")).toBeInTheDocument();
    expect(screen.getByText("nightly-offsite")).toBeInTheDocument();

    expect(snapshotCalls(calls)).toHaveLength(0);
  });

  it("reads snapshots once for the selected initialised repository", async () => {
    const { calls } = route([initializedRepo, uninitializedRepo]);
    renderWithQuery(<BackupEnginesPage />);

    expect(await screen.findByText("nightly-offsite")).toBeInTheDocument();
    // The snapshot listing resolves, so the row for it renders.
    expect(await screen.findByText(/abc123def456/)).toBeInTheDocument();

    const reads = snapshotCalls(calls);
    expect(reads).toHaveLength(1);
    expect(reads[0].url).toContain("repoId=r1");
  });
});
