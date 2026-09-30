"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, ArrowLeftRight, Boxes, ExternalLink, HardDrive, History, KeyRound, Play, Plus, RefreshCw, RotateCcw, ShieldCheck, Trash2, Zap } from "lucide-react";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
  AdminPageLayout,
  AdminSelect,
  AdminTable,
  AdminTabs,
  AdminTd,
  AdminTBody,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  Pill,
  Textarea,
  type AdminTab,
} from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
import { sourceState, useNodesQuery, useServersQuery } from "@/lib/admin/telemetry";
import { deploymentStatusTone } from "@/lib/api/status";
import { formatBytes, formatDate } from "@/lib/utils";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
  addBackupRepository,
  backupEngineLabel,
  createBackupSnapshot,
  decodeBackupEnginePrunePolicy,
  defaultBackupEnginePrunePolicy,
  fetchBackupRepositories,
  fetchBackupRestoreJobs,
  fetchBackupSnapshots,
  initBackupRepository,
  pruneBackupRepository,
  removeBackupRepository,
  restoreBackupSnapshot,
  testBackupRepository,
  verifyBackupSnapshot,
  type AddBackupRepositoryRequest,
  type BackupEngine,
  type BackupEnginePrunePolicy,
  type BackupEngineRepository,
  type BackupEngineSnapshot,
} from "@/lib/api/backup-engine";

// Restic / Kopia backup engines. Repositories hold the encrypted backend
// (S3, local path, rest:, sftp:, gcrypt: …); snapshots are taken on the cron
// schedule from the repository's prune policy and can be verified, restored or
// pruned from here. Passwords are sealed by the API — the UI only ever sees a
// non-secret descriptor.
//
// These engine snapshots are NOT the artifacts listed under Backups; the two
// systems have separate endpoints and tables, and this page says so.

const ALL_KEY = ["admin", "backup-engines"];
const POLL_MS = 30_000;

/**
 * `restic snapshots --json` does not report a size or a file count, and the API
 * leaves both fields at their zero value for Restic repositories
 * (`backupengine/service.go:1254-1262`); only Kopia fills them
 * (`service.go:1371-1372`). Rendering that absence as `0 Bytes` tells the
 * operator every Restic backup is empty, which is a measurement the CLI never
 * made. Both cells therefore name the missing report instead.
 */
function engineSizeCell(engine: BackupEngine | undefined, bytes: number | undefined): string {
  if (engine !== "kopia") return "Not reported by restic";
  const value = Number(bytes);
  return Number.isFinite(value) && value >= 0 ? formatBytes(value) : "Not reported";
}

function engineFilesCell(engine: BackupEngine | undefined, files: number | undefined): string {
  if (engine !== "kopia") return "Not reported by restic";
  const value = Number(files);
  return Number.isFinite(value) && value >= 0 ? value.toLocaleString() : "Not reported";
}

const parsePathList = (raw: string): string[] => raw
  .split(/\r?\n/)
  .map((line) => line.trim())
  .filter((line) => line.length > 0);

interface OptionSource {
  id: string;
  name: string;
}

function idOptions(items: OptionSource[], currentValue?: string) {
  const options = items.map((item) => ({ value: item.id, label: `${item.name} · ${item.id.slice(0, 8)}` }));
  if (currentValue && !options.some((option) => option.value === currentValue)) {
    options.unshift({ value: currentValue, label: `${currentValue.slice(0, 8)} · not in the node list` });
  }
  return options;
}

function RepositoryFormModal({ nodes, onClose, pending, onSubmit, servers }: {
  nodes: OptionSource[];
  onClose: () => void;
  pending: boolean;
  onSubmit: (data: AddBackupRepositoryRequest) => void;
  servers: OptionSource[];
}) {
  const [name, setName] = useState("");
  const [engine, setEngine] = useState<BackupEngine>("restic");
  const [location, setLocation] = useState("");
  const [password, setPassword] = useState("");
  const [nodeId, setNodeId] = useState("");
  const [serverId, setServerId] = useState("");
  const [alreadyInitialized, setAlreadyInitialized] = useState(false);
  const [policy, setPolicy] = useState<BackupEnginePrunePolicy>(defaultBackupEnginePrunePolicy);
  const [pathsRaw, setPathsRaw] = useState("");

  const setCount = (key: "keepLast" | "keepHourly" | "keepDaily" | "keepWeekly" | "keepMonthly", value: string) => {
    const parsed = Number.parseInt(value, 10);
    setPolicy((prev) => ({ ...prev, [key]: Number.isFinite(parsed) && parsed >= 0 ? parsed : 0 }));
  };

  const submit = () => {
    onSubmit({
      name: name.trim(),
      engine,
      location: location.trim(),
      password: password.trim() || undefined,
      nodeId: nodeId || undefined,
      serverId: serverId || undefined,
      initialized: alreadyInitialized,
      prunePolicy: { ...policy, paths: parsePathList(pathsRaw) },
    });
  };

  return (
    <Modal
      description="Register a Restic or Kopia repository. The password is sealed at rest and never returned by the API."
      onClose={onClose}
      title="Add backup repository"
    >
      <div className="grid gap-4 md:grid-cols-2">
        <Input label="Name *" value={name} onChange={setName} placeholder="nightly-s3" />
        <AdminSelect
          label="Engine *"
          onChange={(v) => setEngine(v === "kopia" ? "kopia" : "restic")}
          options={[{ value: "restic", label: "Restic" }, { value: "kopia", label: "Kopia" }]}
          value={engine}
        />
        <div className="md:col-span-2">
          <Input
            label="Location *"
            mono
            value={location}
            onChange={setLocation}
            placeholder={engine === "kopia" ? "filesystem:/mnt/backups/kopia" : "s3:https://s3.example.com/bucket/prefix"}
          />
        </div>
        <div className="md:col-span-2">
          <Input
            label="Repository password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={setPassword}
            placeholder="Leave empty to keep using an env:VAR reference"
          />
          <p className="mt-1 text-xs leading-5 text-text-muted">
            Prefix with <span className="font-mono">env:</span> to resolve the secret from an environment
            variable on the target node instead of storing it.
          </p>
        </div>
        {/* A free-text UUID let an operator register a runner against a node that
            does not exist and only find out when the CLI failed. Both pickers now
            come from the inventory the rest of the admin reads. */}
        <AdminSelect
          label="Run the CLI on this node"
          mono
          onChange={setNodeId}
          options={idOptions(nodes, nodeId)}
          placeholder="Choose a node…"
          value={nodeId}
        />
        <AdminSelect
          label="Resolve the node from this server"
          mono
          onChange={setServerId}
          options={idOptions(servers, serverId)}
          placeholder="Choose a server…"
          value={serverId}
        />
        <Input label="Snapshot schedule (cron)" mono value={policy.schedule} onChange={(v) => setPolicy((prev) => ({ ...prev, schedule: v }))} placeholder="0 3 * * *" />
        <div className="flex items-end pb-1 text-xs leading-5 text-text-muted">
          Encryption is fixed by the engine: aes-256 for Restic, salsa2012-sha256 for Kopia.
        </div>
        <div className="grid grid-cols-2 gap-3 md:col-span-2 md:grid-cols-5">
          <Input label="Keep last" type="number" value={String(policy.keepLast)} onChange={(v) => setCount("keepLast", v)} />
          <Input label="Keep hourly" type="number" value={String(policy.keepHourly)} onChange={(v) => setCount("keepHourly", v)} />
          <Input label="Keep daily" type="number" value={String(policy.keepDaily)} onChange={(v) => setCount("keepDaily", v)} />
          <Input label="Keep weekly" type="number" value={String(policy.keepWeekly)} onChange={(v) => setCount("keepWeekly", v)} />
          <Input label="Keep monthly" type="number" value={String(policy.keepMonthly)} onChange={(v) => setCount("keepMonthly", v)} />
        </div>
        <div className="md:col-span-2">
          <Textarea
            label="Default snapshot paths"
            value={pathsRaw}
            onChange={setPathsRaw}
            rows={3}
            placeholder={"/var/lib/docker/volumes\n/etc/forge"}
          />
          {engine === "kopia" ? <p className="mt-1 text-xs text-text-muted">Kopia captures one root per snapshot; only the first path is used.</p> : null}
        </div>
        <label className="flex items-center gap-2 text-sm text-text-subtle md:col-span-2">
          <input
            type="checkbox"
            className="h-4 w-4 rounded border-line-strong bg-transparent"
            checked={alreadyInitialized}
            onChange={(e) => setAlreadyInitialized(e.target.checked)}
          />
          The repository already exists on the backend (skip init)
        </label>
        <label className="flex items-center gap-2 text-sm text-text-subtle md:col-span-2">
          <input
            type="checkbox"
            className="h-4 w-4 rounded border-line-strong bg-transparent"
            checked={policy.prune}
            onChange={(e) => setPolicy((prev) => ({ ...prev, prune: e.target.checked }))}
          />
          Garbage collect data after forgetting expired snapshots
        </label>
      </div>
      <ModalFooter onCancel={onClose} onConfirm={submit} confirmLabel="Add repository" disabled={pending || !name.trim() || !location.trim()} />
    </Modal>
  );
}

/**
 * The reason this page exists separately from Backups is verification, so the
 * restore dialog states it: an unverified snapshot can still be restored, but
 * the operator has to acknowledge that no integrity check has passed for it.
 */
function RestoreModal({ onClose, onSubmit, pending, repo, snapshot }: {
  onClose: () => void;
  onSubmit: (targetPath: string) => void;
  pending: boolean;
  repo: BackupEngineRepository | null;
  snapshot: BackupEngineSnapshot | null;
}) {
  const [targetPath, setTargetPath] = useState("");
  const verified = Boolean(snapshot?.verifiedAt);
  return (
    <Modal
      description={`Restore snapshot ${snapshot?.snapshotId ?? ""} from ${repo ? `${backupEngineLabel(repo.engine)} “${repo.name}”` : "the selected repository"} into an absolute path on the target node.`}
      onClose={onClose}
      title="Restore snapshot"
    >
      <div className="space-y-4">
        <div className="rounded-lg border border-line bg-overlay-subtle p-3 text-xs leading-5">
          <dl className="grid grid-cols-1 gap-x-6 gap-y-1 sm:grid-cols-2">
            <div className="flex gap-2"><dt className="text-text-muted">Integrity</dt><dd>{verified ? `Verified ${snapshot?.verifiedAt ? formatDate(snapshot.verifiedAt) : "time not reported"}` : "Never verified"}</dd></div>
            <div className="flex gap-2"><dt className="text-text-muted">Size</dt><dd>{engineSizeCell(repo?.engine, snapshot?.totalSizeBytes)}</dd></div>
            <div className="flex gap-2"><dt className="text-text-muted">Engine</dt><dd>{repo ? backupEngineLabel(repo.engine) : "Not reported"}</dd></div>
            <div className="flex gap-2"><dt className="text-text-muted">Repository</dt><dd className="min-w-0 break-all">{repo?.location ?? "Not reported"}</dd></div>
          </dl>
          {verified ? null : (
            <p className="mt-2 text-warn">
              No integrity check has passed for this snapshot. Verify it first unless you are deliberately
              restoring unchecked data.
            </p>
          )}
        </div>
        <Input label="Target path *" mono value={targetPath} onChange={setTargetPath} placeholder="/restore/nightly-01" />
        <div className="rounded-lg border border-warn-line bg-warn-subtle p-3 text-xs leading-5 text-warn">
          Files are written under the target path on the machine running the CLI. Choose a directory that does
          not hold live data — a restore onto a populated path overwrites what is there.
        </div>
        {snapshot?.paths?.length ? (
          <div className="text-xs text-text-subtle">
            <span className="font-semibold text-text">Snapshot contents:</span>
            <ul className="mt-1 space-y-0.5 font-mono break-all">{snapshot.paths.map((p) => <li key={p}>{p}</li>)}</ul>
          </div>
        ) : (
          <p className="text-xs text-text-muted">This snapshot reports no paths.</p>
        )}
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => onSubmit(targetPath.trim())} confirmLabel="Start restore" destructive={!verified} disabled={pending || !targetPath.trim().startsWith("/")} />
    </Modal>
  );
}

export default function BackupEnginesPage() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [activeTab, setActiveTab] = useState("repositories");
  const [isAddOpen, setIsAddOpen] = useState(false);
  const [selectedRepoId, setSelectedRepoId] = useState("");
  const [snapshotPaths, setSnapshotPaths] = useState("");
  const [snapshotSearch, setSnapshotSearch] = useState("");
  const [restoreTarget, setRestoreTarget] = useState<{ repo: BackupEngineRepository; snapshot: BackupEngineSnapshot } | null>(null);

  const repositoriesQuery = useQuery({
    queryKey: [...ALL_KEY, "repositories"],
    queryFn: fetchBackupRepositories,
    refetchInterval: POLL_MS,
  });

  const repositories = useMemo(() => repositoriesQuery.data ?? [], [repositoriesQuery.data]);
  const selectedRepo = useMemo(
    () => repositories.find((repo) => repo.id === selectedRepoId) ?? repositories[0] ?? null,
    [repositories, selectedRepoId],
  );

  const nodesQuery = useNodesQuery();
  const serversQuery = useServersQuery();

  const snapshotsQuery = useQuery({
    queryKey: [...ALL_KEY, "snapshots", selectedRepo?.id ?? ""],
    queryFn: () => fetchBackupSnapshots(selectedRepo?.id ?? ""),
    // Snapshots shell out to the restic/kopia CLI on the execution target —
    // never invoke it for a repository that was never initialised (400) or
    // when the snapshots tab is not visible.
    enabled: Boolean(selectedRepo?.id) && selectedRepo?.initialized === true && activeTab === "snapshots",
  });

  const restoreJobsQuery = useQuery({
    queryKey: [...ALL_KEY, "restores"],
    queryFn: () => fetchBackupRestoreJobs(),
    refetchInterval: POLL_MS,
    enabled: activeTab === "restores",
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ALL_KEY });
  };

  const addRepoMut = useMutation({
    mutationFn: (data: AddBackupRepositoryRequest) => addBackupRepository(data),
    onSuccess: (repo) => {
      invalidate();
      setIsAddOpen(false);
      toast({ tone: "success", title: `${backupEngineLabel(repo.engine)} repository “${repo.name}” registered` });
    },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to register repository" }),
  });

  const removeRepoMut = useMutation({
    mutationFn: (id: string) => removeBackupRepository(id),
    onSuccess: () => {
      invalidate();
      toast({ tone: "success", title: "Repository removed", message: "The repository on the backend is untouched." });
    },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to remove repository" }),
  });

  const testRepoMut = useMutation({
    mutationFn: (id: string) => testBackupRepository(id),
    onSuccess: () => {
      invalidate();
      toast({ tone: "success", title: "Repository reachable and password accepted" });
    },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Connection test failed" }),
  });

  const initRepoMut = useMutation({
    mutationFn: (id: string) => initBackupRepository(id),
    onSuccess: () => {
      invalidate();
      toast({ tone: "success", title: "Repository initialised" });
    },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to initialise repository" }),
  });

  const pruneRepoMut = useMutation({
    mutationFn: (id: string) => pruneBackupRepository(id),
    onSuccess: () => {
      invalidate();
      toast({ tone: "success", title: "Expired snapshots forgotten and data garbage collected" });
    },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Prune failed" }),
  });

  const createSnapshotMut = useMutation({
    mutationFn: (vars: { repoId: string; paths: string[] }) => createBackupSnapshot(vars.repoId, vars.paths),
    onSuccess: (snapshot) => {
      invalidate();
      setSnapshotPaths("");
      void snapshotsQuery.refetch();
      // Restic reports no size for a snapshot, so the toast must not interpolate
      // one — "created (0 Bytes)" was a measurement the CLI never made.
      const size = selectedRepo?.engine === "kopia" && snapshot.totalSizeBytes > 0 ? ` (${formatBytes(snapshot.totalSizeBytes)})` : "";
      toast({ tone: "success", title: `Snapshot ${snapshot.snapshotId} created${size}`, message: size ? undefined : "Size not reported by this engine." });
    },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to create snapshot" }),
  });

  const verifySnapshotMut = useMutation({
    mutationFn: (vars: { repoId: string; snapshotId: string }) => verifyBackupSnapshot(vars.repoId, vars.snapshotId),
    onSuccess: (_data, vars) => {
      invalidate();
      void snapshotsQuery.refetch();
      toast({ tone: "success", title: `Snapshot ${vars.snapshotId} verified` });
    },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Verification failed" }),
  });

  const restoreMut = useMutation({
    mutationFn: (vars: { repoId: string; snapshotId: string; targetPath: string }) =>
      restoreBackupSnapshot(vars.repoId, vars.snapshotId, vars.targetPath),
    onSuccess: (job) => {
      invalidate();
      setRestoreTarget(null);
      setActiveTab("restores");
      // The POST returns a *started* job. Anything other than an immediate
      // failure is progress, not a failure — the previous copy toasted
      // "Restore failed" for work that had merely not finished yet.
      if (job.status === "failed") {
        toast({ tone: "error", title: "Restore failed", message: job.error || "See the restore job for the reason." });
      } else if (job.status === "completed") {
        toast({ tone: "success", title: `Restored to ${job.targetPath}` });
      } else {
        toast({ tone: "success", title: `Restore started (${job.status})`, message: `Following progress for ${job.targetPath} in the Restores tab.` });
      }
    },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to start restore" }),
  });

  const confirmPrune = async (repo: BackupEngineRepository) => {
    if (await confirm({
      title: `Prune “${repo.name}”?`,
      description: "Snapshots outside the retention policy are forgotten and their data is garbage collected. This cannot be undone.",
      danger: true,
      confirmLabel: "Prune",
    })) pruneRepoMut.mutate(repo.id);
  };

  const confirmRemove = async (repo: BackupEngineRepository) => {
    if (await confirm({
      title: `Remove “${repo.name}”?`,
      description: "Only the registration and its tracked snapshot rows are removed; the repository on the backend is untouched.",
      danger: true,
      confirmLabel: "Remove",
    })) removeRepoMut.mutate(repo.id);
  };

  const startRestore = async (repo: BackupEngineRepository, snapshot: BackupEngineSnapshot, targetPath: string) => {
    if (!snapshot.verifiedAt) {
      const ok = await confirm({
        title: "Restore an unverified snapshot?",
        description: `Snapshot ${snapshot.snapshotId} has never passed an integrity check. Restoring it writes data of unknown integrity to ${targetPath}.`,
        danger: true,
        confirmLabel: "Restore anyway",
      });
      if (!ok) return;
    }
    restoreMut.mutate({ repoId: repo.id, snapshotId: snapshot.snapshotId, targetPath });
  };

  const renderRepositories = () => (
    <div className="space-y-4">
      <Card>
        <CardHeader
          title="Repositories"
          icon={Boxes}
          action={<Btn size="sm" tone="ghost" onClick={() => invalidate()} disabled={repositoriesQuery.isFetching}><RefreshCw size={14} /> Refresh</Btn>}
        />
        {repositoriesQuery.isLoading ? (
          <AdminLoadingState label="Loading repositories…" />
        ) : repositoriesQuery.isError ? (
          <div className="p-4">
            <AdminErrorState message={`Repositories could not be read: ${repositoriesQuery.error.message}`} retry={() => void repositoriesQuery.refetch()} />
          </div>
        ) : repositories.length === 0 ? (
          <EmptyState icon={Boxes} title="No backup repositories" message="Register a Restic or Kopia repository to start snapshotting." sub="S3, local filesystem, rest:, sftp: and gcrypt: backends are all supported." />
        ) : (
          <AdminTable label="Backup repositories">
            <AdminTHead>
              <AdminTh>Name</AdminTh>
              <AdminTh>Engine</AdminTh>
              <AdminTh>Location</AdminTh>
              <AdminTh>Schedule</AdminTh>
              <AdminTh>Last snapshot</AdminTh>
              <AdminTh>State</AdminTh>
              <AdminTh><span /></AdminTh>
            </AdminTHead>
            <AdminTBody>
              {repositories.map((repo) => {
                const policy = decodeBackupEnginePrunePolicy(repo.prunePolicy);
                return (
                  <AdminTr key={repo.id}>
                    <AdminTd className="font-medium text-text">
                      {repo.name}
                      {/* repo.artifactId is the only field that ties an engine
                          repository to a classic backup artifact. It was fetched
                          and dropped; showing it answers "where did this come
                          from?" without guessing. */}
                      {repo.artifactId ? (
                        <span className="mt-0.5 block text-xs font-normal">
                          <Link className="inline-flex items-center gap-1 text-text-subtle underline underline-offset-2" href="/admin/backups">
                            <ExternalLink size={11} /> From backup artifact <span className="font-mono">{repo.artifactId.slice(0, 8)}</span>
                          </Link>
                        </span>
                      ) : null}
                    </AdminTd>
                    <AdminTd><Pill tone="neutral">{backupEngineLabel(repo.engine)}</Pill></AdminTd>
                    <AdminTd className="max-w-[20rem] break-all font-mono text-xs text-text-subtle">{repo.location}</AdminTd>
                    <AdminTd className="text-xs text-text-subtle">
                      {policy.schedule ? (
                        <span className="block">
                          <span className="font-mono">{policy.schedule}</span>
                          <span className="block text-text-muted">
                            {repo.nextRunAt ? `Next: ${formatDate(repo.nextRunAt)}` : "No next run scheduled"}
                          </span>
                        </span>
                      ) : <span>Manual only</span>}
                    </AdminTd>
                    <AdminTd className="text-xs text-text-subtle">{formatDate(repo.lastSnapshotAt, "None yet")}</AdminTd>
                    <AdminTd>
                      <div className="space-y-1">
                        <Pill tone={repo.initialized ? "ok" : "warn"}>{repo.initialized ? "Initialised" : "Needs init"}</Pill>
                        {repo.lastError ? <p className="max-w-[18rem] break-words text-xs text-warn">{repo.lastError}</p> : null}
                      </div>
                    </AdminTd>
                    <AdminTd>
                      <div className="flex flex-wrap items-center justify-end gap-2">
                        <Btn ariaLabel={`Test the connection to ${repo.name}`} size="sm" tone="ghost" onClick={() => testRepoMut.mutate(repo.id)} loading={testRepoMut.isPending && testRepoMut.variables === repo.id}>
                          <Zap size={14} /> Test connection
                        </Btn>
                        {!repo.initialized ? (
                          <Btn ariaLabel={`Initialise ${repo.name}`} size="sm" tone="ghost" onClick={() => initRepoMut.mutate(repo.id)} loading={initRepoMut.isPending && initRepoMut.variables === repo.id}>
                            <KeyRound size={14} /> Init
                          </Btn>
                        ) : null}
                        <Btn size="sm" tone="ghost" onClick={() => void confirmPrune(repo)} disabled={pruneRepoMut.isPending}>
                          <Archive size={14} /> Prune
                        </Btn>
                        <Btn size="sm" tone="ghost" onClick={() => { setSelectedRepoId(repo.id); setActiveTab("snapshots"); }}>
                          <History size={14} /> Snapshots
                        </Btn>
                        <Btn ariaLabel={`Remove ${repo.name} from this panel`} size="sm" tone="danger" onClick={() => void confirmRemove(repo)} disabled={removeRepoMut.isPending}>
                          <Trash2 size={14} />
                        </Btn>
                      </div>
                    </AdminTd>
                  </AdminTr>
                );
              })}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>
      <Card>
        <CardHeader icon={Plus} title="Add a repository" />
        <div className="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
          <p className="max-w-prose text-sm leading-5 text-text-subtle">
            Repositories are snapshotted on the panel cron schedule and pruned with the retention rules you set
            here. These snapshots live only in this page — the artifacts under{" "}
            <Link className="underline underline-offset-2" href="/admin/backups">Backups</Link> come from the
            separate policy/job pipeline.
          </p>
          <Btn onClick={() => setIsAddOpen(true)}><Plus size={14} /> Add repository</Btn>
        </div>
      </Card>
    </div>
  );

  const renderSnapshots = () => {
    const snapshots = snapshotsQuery.data ?? [];
    const needle = snapshotSearch.trim().toLowerCase();
    const visibleSnapshots = needle
      ? snapshots.filter((snapshot) => `${snapshot.snapshotId} ${snapshot.hostname} ${snapshot.paths.join(" ")}`.toLowerCase().includes(needle))
      : snapshots;
    const paths = parsePathList(snapshotPaths);
    const effectivePaths = paths.length > 0 ? paths : decodeBackupEnginePrunePolicy(selectedRepo?.prunePolicy).paths;
    const needsInit = Boolean(selectedRepo) && selectedRepo?.initialized !== true;
    const snapshotBlockReason = !selectedRepo
      ? "Select a repository first."
      : needsInit
        ? "Repository is not initialised — the CLI would reject a snapshot here."
        : effectivePaths.length === 0
          ? "No paths to snapshot: enter them above or set defaults on the repository."
          : null;

    return (
      <div className="space-y-4">
        <Card>
          <CardHeader icon={HardDrive} title="Repository" />
          <div className="grid gap-4 px-5 py-4 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto] md:items-end">
            <AdminSelect
              label="Repository"
              value={selectedRepo?.id ?? ""}
              onChange={setSelectedRepoId}
              options={repositories.map((repo) => ({ value: repo.id, label: `${repo.name} · ${backupEngineLabel(repo.engine)}` }))}
              placeholder="Select a repository"
            />
            <Textarea
              label="Paths to snapshot (one per line)"
              value={snapshotPaths}
              onChange={setSnapshotPaths}
              rows={2}
              placeholder={effectivePaths.length > 0 ? effectivePaths.join("\n") : "/var/lib/docker/volumes"}
            />
            <div className="flex flex-wrap gap-2">
              <Btn
                onClick={() => { if (selectedRepo) createSnapshotMut.mutate({ repoId: selectedRepo.id, paths: effectivePaths }); }}
                disabled={snapshotBlockReason !== null}
                loading={createSnapshotMut.isPending}
              >
                <Play size={14} /> Snapshot now
              </Btn>
              <Btn tone="ghost" onClick={() => void snapshotsQuery.refetch()} disabled={!selectedRepo?.initialized || snapshotsQuery.isFetching}>
                <RefreshCw size={14} /> Refresh
              </Btn>
              <Btn tone="warning" onClick={() => { if (selectedRepo) void confirmPrune(selectedRepo); }} disabled={!selectedRepo || pruneRepoMut.isPending}>
                <Archive size={14} /> Prune
              </Btn>
            </div>
          </div>
          {/* Capability gating: the reason is on screen, not in a tooltip, and the
              control is disabled rather than enabled-and-lying. */}
          {snapshotBlockReason ? (
            <p className="mx-5 mb-4 text-xs leading-5 text-warn">{snapshotBlockReason}</p>
          ) : null}
          {needsInit && selectedRepo ? (
            <div className="mx-5 mb-5 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-warn-line bg-warn-subtle p-3 text-xs leading-5 text-warn">
              <span>
                “{selectedRepo.name}” is registered but not initialised, so its snapshot list cannot be read.
              </span>
              <Btn
                size="sm"
                tone="ghost"
                loading={initRepoMut.isPending && initRepoMut.variables === selectedRepo.id}
                onClick={() => initRepoMut.mutate(selectedRepo.id)}
              >
                <KeyRound size={14} /> Initialise repository
              </Btn>
            </div>
          ) : null}
          {selectedRepo?.lastError ? (
            <div className="mx-5 mb-5 rounded-lg border border-danger-line bg-danger-subtle p-3 text-xs leading-5 text-danger">{selectedRepo.lastError}</div>
          ) : null}
        </Card>

        <Card>
          <CardHeader icon={History} title="Snapshots" />
          <div className="space-y-3 px-4 pb-3">
            <div className="flex flex-wrap items-center gap-3">
              <div className="min-w-[240px] flex-1">
                <Input label="Filter snapshots" value={snapshotSearch} onChange={setSnapshotSearch} placeholder="Snapshot id, host or path…" />
              </div>
              <FreshnessBadge state={sourceState(snapshotsQuery)} />
            </div>
          </div>
          {!selectedRepo ? (
            <EmptyState icon={HardDrive} message="Select a repository above to browse its snapshots." title="No repository selected" />
          ) : needsInit ? (
            <EmptyState icon={KeyRound} title="Repository needs initialisation" message="Snapshots are only listed once the repository is initialised, because listing them runs the engine CLI against it." sub="Use “Initialise repository” above, or “Init” on the Repositories tab." />
          ) : snapshotsQuery.isLoading ? (
            <AdminLoadingState label="Listing snapshots…" />
          ) : snapshotsQuery.isError ? (
            <div className="p-4">
              <AdminErrorState message={`Snapshots could not be read: ${snapshotsQuery.error.message}`} retry={() => void snapshotsQuery.refetch()} />
            </div>
          ) : snapshots.length === 0 ? (
            <EmptyState icon={History} title="No snapshots yet" message="Run “Snapshot now” or wait for the scheduled snapshot." />
          ) : visibleSnapshots.length === 0 ? (
            <EmptyState icon={History} title="No matching snapshot" message={`No snapshot matches “${snapshotSearch}”.`} />
          ) : (
            <AdminTable label="Snapshots">
              <AdminTHead>
                <AdminTh>Snapshot</AdminTh>
                <AdminTh>Paths</AdminTh>
                <AdminTh>Host</AdminTh>
                <AdminTh>Size</AdminTh>
                <AdminTh>Files</AdminTh>
                <AdminTh>Taken</AdminTh>
                <AdminTh>Verified</AdminTh>
                <AdminTh><span /></AdminTh>
              </AdminTHead>
              <AdminTBody>
                {visibleSnapshots.map((snapshot) => (
                  <AdminTr key={`${snapshot.repoId}:${snapshot.snapshotId}`}>
                    <AdminTd className="font-mono text-xs text-text">{snapshot.snapshotId}</AdminTd>
                    <AdminTd className="max-w-[16rem] break-all text-xs text-text-subtle">
                      {snapshot.paths.length > 0 ? snapshot.paths.join(", ") : <span className="text-text-muted">No paths reported</span>}
                    </AdminTd>
                    <AdminTd className="text-xs text-text-subtle">{snapshot.hostname || <span className="text-text-muted">Not reported</span>}</AdminTd>
                    <AdminTd className="text-xs text-text-subtle">{engineSizeCell(selectedRepo.engine, snapshot.totalSizeBytes)}</AdminTd>
                    <AdminTd className="text-xs text-text-subtle">{engineFilesCell(selectedRepo.engine, snapshot.dataFiles)}</AdminTd>
                    <AdminTd className="text-xs text-text-subtle">{formatDate(snapshot.timestamp, "Not reported")}</AdminTd>
                    <AdminTd>
                      {snapshot.verifiedAt
                        ? <Pill tone="ok">{formatDate(snapshot.verifiedAt)}</Pill>
                        : <Pill tone="unknown">Not verified</Pill>}
                    </AdminTd>
                    <AdminTd>
                      <div className="flex flex-wrap items-center justify-end gap-2">
                        <Btn
                          size="sm"
                          tone="ghost"
                          disabled={verifySnapshotMut.isPending}
                          loading={verifySnapshotMut.isPending && verifySnapshotMut.variables?.snapshotId === snapshot.snapshotId}
                          onClick={() => verifySnapshotMut.mutate({ repoId: snapshot.repoId, snapshotId: snapshot.snapshotId })}
                        >
                          <ShieldCheck size={14} /> Verify
                        </Btn>
                        <Btn size="sm" onClick={() => setRestoreTarget({ repo: selectedRepo, snapshot })} disabled={!selectedRepo.initialized}>
                          <RotateCcw size={14} /> Restore
                        </Btn>
                      </div>
                    </AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          )}
        </Card>
      </div>
    );
  };

  const renderRestores = () => {
    const jobs = restoreJobsQuery.data ?? [];
    return (
      <Card>
        <CardHeader
          title="Restore jobs"
          icon={RotateCcw}
          action={<Btn size="sm" tone="ghost" onClick={() => void restoreJobsQuery.refetch()} disabled={restoreJobsQuery.isFetching}><RefreshCw size={14} /> Refresh</Btn>}
        />
        {restoreJobsQuery.isLoading ? (
          <AdminLoadingState label="Loading restore jobs…" />
        ) : restoreJobsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState message={`Restore jobs could not be read: ${restoreJobsQuery.error.message}`} retry={() => void restoreJobsQuery.refetch()} />
          </div>
        ) : jobs.length === 0 ? (
          <EmptyState icon={RotateCcw} message="No restores have been run from the backup engines yet." title="No engine restores" />
        ) : (
          <AdminTable label="Restore jobs">
            <AdminTHead>
              <AdminTh>Repository</AdminTh>
              <AdminTh>Snapshot</AdminTh>
              <AdminTh>Target path</AdminTh>
              <AdminTh>Status</AdminTh>
              <AdminTh>Progress</AdminTh>
              <AdminTh>Started</AdminTh>
              <AdminTh>Finished</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {jobs.map((job) => {
                const repo = repositories.find((candidate) => candidate.id === job.repoId);
                const pct = Math.max(0, Math.min(100, Math.round(job.progressPct)));
                return (
                  <AdminTr key={job.id}>
                    <AdminTd className="text-sm text-text">
                      {repo ? repo.name : <span className="text-text-muted">Repository not in this list</span>}
                    </AdminTd>
                    <AdminTd className="font-mono text-xs text-text-subtle">{job.snapshotId}</AdminTd>
                    <AdminTd className="max-w-[16rem] break-all font-mono text-xs text-text-subtle">{job.targetPath}</AdminTd>
                    <AdminTd><Pill tone={deploymentStatusTone(job.status)}>{job.status}</Pill></AdminTd>
                    <AdminTd>
                      {job.status === "running" ? (
                        <div className="flex items-center gap-2">
                          <div aria-hidden="true" className="h-1.5 w-24 overflow-hidden rounded-full bg-overlay-strong">
                            <div className="h-full rounded-full bg-info" style={{ width: `${pct}%` }} />
                          </div>
                          <span className="font-mono text-xs text-text-subtle">{pct}%</span>
                        </div>
                      ) : (
                        <span className="text-xs text-text-muted">
                          {job.status === "completed" ? "Finished" : job.status === "pending" ? "Not started" : `${job.status}`}
                        </span>
                      )}
                    </AdminTd>
                    <AdminTd className="text-xs text-text-subtle">{formatDate(job.startedAt, "Not started")}</AdminTd>
                    <AdminTd className="text-xs text-text-subtle">
                      {formatDate(job.completedAt, job.status === "running" ? "Running" : "Not finished")}
                      {job.error ? <span className="mt-0.5 block max-w-[18rem] break-words text-danger">{job.error}</span> : null}
                    </AdminTd>
                  </AdminTr>
                );
              })}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>
    );
  };

  const tabs: AdminTab[] = [
    { id: "repositories", label: "Repositories" },
    { id: "snapshots", label: "Engine snapshots" },
    { id: "restores", label: "Engine restores" },
  ];

  const activeQuery = activeTab === "snapshots" ? snapshotsQuery : activeTab === "restores" ? restoreJobsQuery : repositoriesQuery;
  const tabFreshness = sourceState(activeQuery, POLL_MS);

  return (
    <AdminPageLayout>
      <AdminPageHeader
        action={
          <div className="flex items-center gap-2">
            <Btn tone="ghost" onClick={() => invalidate()} disabled={repositoriesQuery.isFetching}>
              <RefreshCw size={14} className={repositoriesQuery.isFetching ? "animate-spin" : undefined} /> Refresh
            </Btn>
            <Btn onClick={() => router.push('/admin/backups')} tone="ghost">
              <ArrowLeftRight size={14} /> Backups pipeline
            </Btn>
            <Btn onClick={() => setIsAddOpen(true)}><Plus size={14} /> Add repository</Btn>
          </div>
        }
        info={adminPageGuides.backupEngines}
        status={<FreshnessBadge state={tabFreshness} />}
      />

      <AdminTabs active={activeTab} onChange={setActiveTab} tabs={tabs} />

      {activeTab === "repositories" && renderRepositories()}
      {activeTab === "snapshots" && renderSnapshots()}
      {activeTab === "restores" && renderRestores()}

      {isAddOpen ? (
        <RepositoryFormModal
          nodes={(nodesQuery.data ?? []).map((node) => ({ id: node.id, name: node.name }))}
          onClose={() => setIsAddOpen(false)}
          onSubmit={(data) => addRepoMut.mutate(data)}
          pending={addRepoMut.isPending}
          servers={(serversQuery.data ?? []).map((server) => ({ id: server.id, name: server.name }))}
        />
      ) : null}

      {restoreTarget ? (
        <RestoreModal
          repo={restoreTarget.repo}
          snapshot={restoreTarget.snapshot}
          onClose={() => setRestoreTarget(null)}
          pending={restoreMut.isPending}
          onSubmit={(targetPath) => void startRestore(restoreTarget.repo, restoreTarget.snapshot, targetPath)}
        />
      ) : null}

      {renderConfirm()}
    </AdminPageLayout>
  );
}
