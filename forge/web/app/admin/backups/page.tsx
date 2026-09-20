"use client";

import { useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, Database, Download, Folder, Lock, MoreVertical, Play, Plus, RefreshCw, RotateCcw, RotateCw, Server, ShieldCheck, Trash2, XCircle } from 'lucide-react';
import { ForgeDropdownMenu } from '@/components/ui/forge/overlay';
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
  AdminPageLayout,
  AdminSelect,
  AdminTable,
  AdminTabs,
  AdminTBody,
  AdminTd,
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
} from '@/components/admin/admin-ui';
import { FreshnessBadge, MetricTile } from '@/components/admin/telemetry-ui';
import { adminPageGuides } from '@/components/admin/admin-page-guides';
import { sourceState, worstSourceState } from '@/lib/admin/telemetry';
import { deploymentStatusTone } from '@/lib/api/status';
import { formatBytes, formatDate } from '@/lib/utils';
import { useToast } from '@/components/ui/toast';
import { useConfirm } from '@/components/ui/confirm-dialog';
import {
  artifactSizeBytes,
  artifactVerification,
  cancelBackupJob,
  createBackupConfig,
  createBackupJob,
  createRestore,
  deleteBackupArtifact,
  deleteBackupConfig,
  deleteBackupJob,
  deleteBackupRestore,
  downloadBackupArtifact,
  executeBackupConfig,
  fetchBackupArtifacts,
  fetchBackupConfigs,
  fetchBackupJobs,
  fetchBackupRestores,
  fetchBackupStorageProviders,
  fetchBackupSystemStatus,
  lockBackupArtifact,
  unlockBackupArtifact,
  updateBackupConfig,
  verifyBackupArtifact,
  type BackupArtifact,
  type BackupConfiguration,
  type BackupJob,
  type CreateBackupJobRequest,
  type CreateRestoreInput,
  type StorageProvider,
} from '@/lib/api/admin-backups';

// Everything below reads `/admin/backups/*`, which this page polls every 30
// seconds. Nothing here claims to be live: the header badge is derived from the
// queries' own `dataUpdatedAt`, so an ageing or failed read says so.
const ALL_KEY = ["admin", "backups"];
const POLL_MS = 30_000;

type BackupType = 'app' | 'volume' | 'database' | 'server';

const backupTypeOptions = [
  { value: 'app', label: 'App' },
  { value: 'volume', label: 'Volume' },
  { value: 'database', label: 'Database' },
  { value: 'server', label: 'Server' },
];

const typeWord: Record<string, string> = {
  app: 'app',
  volume: 'volume',
  database: 'database',
  server: 'server',
  manual: 'manual',
};

/** A size the control plane never measured renders as such, never as "0 B". */
function sizeCell(sizeBytes: number | null): string {
  return sizeBytes === null ? 'Not measured' : formatBytes(sizeBytes);
}

type ProgressRow = Pick<BackupJob, 'status' | 'progressPercentage' | 'bytesProcessed' | 'totalBytes' | 'currentPhase' | 'errorMessage'>;

/**
 * Progress for a job/restore row.
 *
 * A `pending` row has not reported any work yet, so it is labelled "Not started"
 * rather than given a measured `0%`. When a byte denominator exists it is shown,
 * because "40%" without a total is not a claim an operator can act on.
 */
function ProgressCell({ row }: { row: ProgressRow }) {
  if (row.status === 'completed') {
    return <span className="text-text-subtle">Finished</span>;
  }
  if (row.status === 'failed' || row.status === 'cancelled') {
    return <span className="text-text-subtle">{row.status === 'failed' ? 'Stopped by failure' : 'Cancelled'}</span>;
  }
  if (row.status !== 'running') {
    return <span className="text-text-muted">Not started</span>;
  }
  const pct = Math.max(0, Math.min(100, Math.round(Number.isFinite(row.progressPercentage) ? row.progressPercentage : 0)));
  const processed = row.bytesProcessed > 0 ? formatBytes(row.bytesProcessed) : null;
  const total = typeof row.totalBytes === 'number' && row.totalBytes > 0 ? formatBytes(row.totalBytes) : null;
  return (
    <span className="block space-y-0.5">
      <span className="block font-mono">{pct}%{row.currentPhase ? ` · ${row.currentPhase}` : ''}</span>
      <span className="block text-text-muted">
        {processed && total ? `${processed} of ${total}` : processed ?? 'No byte count reported'}
      </span>
    </span>
  );
}

/** The one target field the API requires for a given artifact type (`restore.go:218-223`). */
function targetFieldFor(type: string): 'targetServerId' | 'targetAppId' | 'targetDatabaseId' | 'targetVolumeId' {
  switch (type) {
    case 'app': return 'targetAppId';
    case 'database': return 'targetDatabaseId';
    case 'volume': return 'targetVolumeId';
    default: return 'targetServerId';
  }
}

/** The artifact's own source resource — the natural default restore target. */
function defaultTargetId(artifact: BackupArtifact): string {
  const field = targetFieldFor(artifact.artifactType);
  const sources = {
    targetAppId: artifact.sourceAppId,
    targetDatabaseId: artifact.sourceDatabaseId,
    targetVolumeId: artifact.sourceVolumeId,
    targetServerId: artifact.sourceServerId,
  } as const;
  return sources[field] ?? '';
}

function ConfigFormModal({ onClose, storageProviders: sp, initial, pending, onSubmit }: {
  onClose: () => void;
  storageProviders: StorageProvider[];
  initial: BackupConfiguration | null;
  pending: boolean;
  onSubmit: (data: Partial<BackupConfiguration>) => void;
}) {
  const [targetId, setTargetId] = useState(initial ? (initial.appId || initial.databaseId || initial.volumeId || initial.serverId || '') : '');
  const [volumeServerId, setVolumeServerId] = useState(initial?.serverId || '');
  const [formData, setFormData] = useState<Partial<BackupConfiguration>>(initial ? {
    name: initial.name,
    description: initial.description,
    backupType: initial.backupType,
    isScheduled: initial.isScheduled,
    cronExpression: initial.cronExpression,
    storageProvider: initial.storageProvider,
    maxBackups: initial.maxBackups,
    retentionDays: initial.retentionDays,
    compressionEnabled: initial.compressionEnabled,
    encryptionEnabled: initial.encryptionEnabled,
    enabled: initial.enabled,
  } : {
    backupType: 'app',
    isScheduled: false,
    storageProvider: Array.isArray(sp) ? (sp.find(p => p.isDefault)?.name || sp[0]?.name || '') : '',
    maxBackups: 10,
    retentionDays: 30,
    compressionEnabled: true,
    encryptionEnabled: false,
    enabled: true,
  });

  const patch = (next: Partial<BackupConfiguration>) => setFormData(prev => ({ ...prev, ...next }));

  const handleSubmit = () => {
    const backupType = (formData.backupType || 'server') as BackupType;
    const targetField = { app: 'appId', volume: 'volumeId', database: 'databaseId', server: 'serverId' }[backupType];
    onSubmit({
      ...formData,
      [targetField]: targetId.trim(),
      ...(backupType === 'volume' ? { serverId: volumeServerId.trim() } : {}),
    });
  };

  return (
    <Modal description={initial ? 'Update how this policy runs.' : 'Create a policy for scheduled or on-demand backups.'} onClose={onClose} title={initial ? 'Edit backup policy' : 'Create backup policy'}>
      <div className="grid gap-4 md:grid-cols-2">
        <Input label="Name *" value={formData.name || ''} onChange={(v) => patch({ name: v })} />
        <AdminSelect
          label="Backup type *"
          value={formData.backupType || 'app'}
          onChange={(v) => patch({ backupType: v as BackupType })}
          options={backupTypeOptions}
        />
        <div className="md:col-span-2">
          <Input label="Description" value={formData.description || ''} onChange={(v) => patch({ description: v })} />
        </div>
        <Input label="Target ID *" mono value={targetId} onChange={setTargetId} />
        {formData.backupType === 'volume' && (
          <Input label="Server ID *" mono value={volumeServerId} onChange={setVolumeServerId} />
        )}
        <AdminSelect
          label="Storage provider *"
          value={formData.storageProvider || ''}
          onChange={(v) => patch({ storageProvider: v })}
          options={Array.isArray(sp) ? sp.map(p => ({ value: p.name, label: `${p.name} (${p.type})` })) : []}
          placeholder={Array.isArray(sp) && sp.length === 0 ? 'No providers registered' : 'Select a provider…'}
        />
        <Input label="Max backups" type="number" value={String(formData.maxBackups ?? 10)} onChange={(v) => patch({ maxBackups: Number.parseInt(v, 10) || 0 })} />
        <Input label="Retention days" type="number" value={String(formData.retentionDays ?? 30)} onChange={(v) => patch({ retentionDays: Number.parseInt(v, 10) || 0 })} />
        <div className="md:col-span-2">
          <Input label="Cron expression" mono value={formData.cronExpression || ''} onChange={(v) => patch({ cronExpression: v, isScheduled: v !== '' })} placeholder="0 0 * * *" />
          <p className="mt-1 text-xs text-text-muted">
            Leave empty for a manual-only policy. Five fields: minute hour day month weekday.
          </p>
        </div>
      </div>
      <div className="mt-4 flex flex-wrap items-center gap-4">
        <label className="flex items-center gap-2 text-sm text-text-subtle">
          <input checked={formData.compressionEnabled || false} className="rounded border-line-strong" onChange={(e) => patch({ compressionEnabled: e.target.checked })} type="checkbox" />
          Compression
        </label>
        <label className="flex items-center gap-2 text-sm text-text-subtle">
          <input checked={formData.encryptionEnabled || false} className="rounded border-line-strong" onChange={(e) => patch({ encryptionEnabled: e.target.checked })} type="checkbox" />
          Encryption
        </label>
        <label className="flex items-center gap-2 text-sm text-text-subtle">
          <input checked={formData.enabled || false} className="rounded border-line-strong" onChange={(e) => patch({ enabled: e.target.checked })} type="checkbox" />
          Enabled
        </label>
      </div>
      <ModalFooter
        onCancel={onClose}
        onConfirm={handleSubmit}
        disabled={pending || !formData.name?.trim() || !targetId.trim() || !formData.storageProvider?.trim() || (formData.backupType === 'volume' && !volumeServerId.trim())}
        confirmLabel={pending ? 'Saving…' : initial ? 'Save changes' : 'Create policy'}
      />
    </Modal>
  );
}

function CreateJobModal({ onClose, pending, onSubmit }: { onClose: () => void; pending: boolean; onSubmit: (data: CreateBackupJobRequest) => void }) {
  const [name, setName] = useState('');
  const [jobType, setJobType] = useState<CreateBackupJobRequest['jobType']>('server');
  const [targetId, setTargetId] = useState('');
  const [volumeServerId, setVolumeServerId] = useState('');
  const [description, setDescription] = useState('');

  const handleSubmit = () => {
    const targetField = { app: 'appId', volume: 'volumeId', database: 'databaseId', server: 'serverId' }[jobType];
    onSubmit({
      name: name.trim(),
      jobType,
      description: description.trim() || undefined,
      [targetField]: targetId.trim(),
      ...(jobType === 'volume' ? { serverId: volumeServerId.trim() } : {}),
    });
  };

  return (
    <Modal description="Run one on-demand backup for a single resource." onClose={onClose} title="Create backup job">
      <div className="grid gap-4 md:grid-cols-2">
        <Input label="Name *" value={name} onChange={setName} />
        <AdminSelect label="Backup type *" value={jobType} onChange={(v) => setJobType(v as CreateBackupJobRequest['jobType'])} options={backupTypeOptions} />
        <Input label="Target ID *" mono value={targetId} onChange={setTargetId} />
        {jobType === 'volume' && (
          <Input label="Server ID *" mono value={volumeServerId} onChange={setVolumeServerId} />
        )}
        <div className="md:col-span-2">
          <Input label="Description" value={description} onChange={setDescription} />
        </div>
      </div>
      <ModalFooter
        onCancel={onClose}
        onConfirm={handleSubmit}
        disabled={pending || !name.trim() || !targetId.trim() || (jobType === 'volume' && !volumeServerId.trim())}
        confirmLabel={pending ? 'Creating…' : 'Create job'}
      />
    </Modal>
  );
}

/**
 * Restore is the most destructive action in this slice: it writes stored backup
 * data over a live workload. The dialog therefore
 *   (a) states the artifact's real integrity verdict,
 *   (b) collects the one target the API requires — without it the POST is
 *       rejected with "exactly one target must be specified", which is what the
 *       previous version of this form always did,
 *   (c) maps its options onto the fields the server actually reads, and
 *   (d) is confirmed with its blast radius before anything is posted.
 * The menu never opens this dialog for an artifact whose last check failed.
 */
function RestoreModal({ artifact, onClose, pending, onSubmit }: {
  artifact: BackupArtifact;
  onClose: () => void;
  pending: boolean;
  onSubmit: (data: CreateRestoreInput) => void;
}) {
  const verdict = artifactVerification(artifact);
  const targetField = targetFieldFor(artifact.artifactType);
  const [targetId, setTargetId] = useState(defaultTargetId(artifact));
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [overwrite, setOverwrite] = useState(false);
  const [snapshotFirst, setSnapshotFirst] = useState(true);

  const size = artifactSizeBytes(artifact);
  const canSubmit = targetId.trim().length > 0 && !pending;

  const handleSubmit = () => {
    if (!canSubmit) return;
    onSubmit({
      artifactId: artifact.id,
      restoreType: artifact.artifactType,
      [targetField]: targetId.trim(),
      name: name.trim() || undefined,
      description: description.trim() || undefined,
      triggeredBy: 'manual',
      restoreOptions: {
        overwriteExisting: overwrite,
        createBackupBeforeRestore: snapshotFirst,
      },
    });
  };

  return (
    <Modal description={`Restore “${artifact.displayName || artifact.name}” onto a live resource.`} onClose={onClose} title="Restore backup">
      <div className="space-y-4">
        <div className="rounded-lg border border-line bg-overlay-subtle p-3 text-xs leading-5">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="font-semibold text-text">{artifact.displayName || artifact.name}</span>
            <Pill tone="neutral">{typeWord[artifact.artifactType] ?? artifact.artifactType}</Pill>
            <span className="text-text-muted">{artifact.storageProvider ? `Stored on ${artifact.storageProvider}` : 'Storage provider not reported'}</span>
          </div>
          <dl className="mt-2 grid grid-cols-1 gap-x-6 gap-y-1 sm:grid-cols-2">
            <div className="flex gap-2"><dt className="text-text-muted">Size</dt><dd className="font-mono text-text">{sizeCell(size)}</dd></div>
            <div className="flex gap-2">
              <dt className="text-text-muted">Integrity</dt>
              <dd className="text-text">
                {verdict.state === 'verified'
                  ? `Verified${verdict.verifiedAt ? ` ${formatDate(verdict.verifiedAt)}` : ' (time not reported)'}`
                  : verdict.state === 'failed'
                    ? `Last check failed (${verdict.attempts} attempt${verdict.attempts === 1 ? '' : 's'})`
                    : 'Never checked'}
              </dd>
            </div>
          </dl>
          {/* What the control plane actually does — restore.go:378-388. */}
          <p className="mt-2 text-text-subtle">
            Before writing, the control plane re-downloads and re-hashes this artifact and refuses the
            restore if that check fails.
          </p>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <Input label={`${typeWord[artifact.artifactType] ?? 'target'} ID *`} mono onChange={setTargetId} value={targetId} />
          <Input label="Restore name (optional)" onChange={setName} placeholder={`restore-${artifact.name}`} value={name} />
          <div className="md:col-span-2">
            <Input label="Description" onChange={setDescription} placeholder="Why this restore is running" value={description} />
          </div>
        </div>

        <div className="space-y-2">
          <label className="flex items-start gap-2 text-sm text-text-subtle">
            <input checked={overwrite} className="mt-1 rounded border-line-strong" onChange={(e) => setOverwrite(e.target.checked)} type="checkbox" />
            <span>
              Overwrite existing files
              <span className="mt-0.5 block text-xs text-text-muted">
                Off: the restore stops if the target already holds data at the same paths.
                On: matching files at the target are replaced.
              </span>
            </span>
          </label>
          <label className="flex items-start gap-2 text-sm text-text-subtle">
            <input checked={snapshotFirst} className="mt-1 rounded border-line-strong" onChange={(e) => setSnapshotFirst(e.target.checked)} type="checkbox" />
            <span>
              Snapshot the target first
              <span className="mt-0.5 block text-xs text-text-muted">
                Captures the target&apos;s current data as a rollback artifact before writing. Recommended
                while the target is live.
              </span>
            </span>
          </label>
        </div>
      </div>
      <ModalFooter
        confirmLabel={pending ? 'Starting…' : 'Restore now'}
        destructive
        disabled={!canSubmit}
        onCancel={onClose}
        onConfirm={handleSubmit}
      />
    </Modal>
  );
}

export default function BackupManagementPage() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [activeTab, setActiveTab] = useState('overview');
  const [searchQuery, setSearchQuery] = useState('');

  const [isCreateConfigOpen, setIsCreateConfigOpen] = useState(false);
  const [editingConfig, setEditingConfig] = useState<BackupConfiguration | null>(null);
  const [isCreateJobOpen, setIsCreateJobOpen] = useState(false);
  const [isRestoreOpen, setIsRestoreOpen] = useState(false);
  const [selectedArtifact, setSelectedArtifact] = useState<BackupArtifact | null>(null);

  const backupConfigsQuery = useQuery({ queryKey: [...ALL_KEY, "configs"], queryFn: fetchBackupConfigs, refetchInterval: POLL_MS });
  const backupJobsQuery = useQuery({ queryKey: [...ALL_KEY, "jobs"], queryFn: fetchBackupJobs, refetchInterval: POLL_MS });
  const backupArtifactsQuery = useQuery({ queryKey: [...ALL_KEY, "artifacts"], queryFn: fetchBackupArtifacts, refetchInterval: POLL_MS });
  const backupRestoresQuery = useQuery({ queryKey: [...ALL_KEY, "restores"], queryFn: fetchBackupRestores, refetchInterval: POLL_MS });
  const storageProvidersQuery = useQuery({ queryKey: [...ALL_KEY, "storage-providers"], queryFn: fetchBackupStorageProviders, refetchInterval: POLL_MS });
  const systemStatusQuery = useQuery({ queryKey: [...ALL_KEY, "status"], queryFn: fetchBackupSystemStatus, refetchInterval: POLL_MS });

  const backupConfigs = useMemo(() => Array.isArray(backupConfigsQuery.data) ? backupConfigsQuery.data : [], [backupConfigsQuery.data]);
  const backupJobs = useMemo(() => Array.isArray(backupJobsQuery.data) ? backupJobsQuery.data : [], [backupJobsQuery.data]);
  const backupArtifacts = useMemo(() => Array.isArray(backupArtifactsQuery.data) ? backupArtifactsQuery.data : [], [backupArtifactsQuery.data]);
  const backupRestores = useMemo(() => Array.isArray(backupRestoresQuery.data) ? backupRestoresQuery.data : [], [backupRestoresQuery.data]);
  const storageProviders = useMemo(() => Array.isArray(storageProvidersQuery.data) ? storageProvidersQuery.data : [], [storageProvidersQuery.data]);
  const systemStatus = systemStatusQuery.data;

  const freshness = worstSourceState([
    sourceState(backupConfigsQuery, POLL_MS),
    sourceState(backupJobsQuery, POLL_MS),
    sourceState(backupArtifactsQuery, POLL_MS),
    sourceState(backupRestoresQuery, POLL_MS),
    sourceState(storageProvidersQuery, POLL_MS),
    sourceState(systemStatusQuery, POLL_MS),
  ]);

  const loading = backupConfigsQuery.isLoading || backupJobsQuery.isLoading || backupArtifactsQuery.isLoading || backupRestoresQuery.isLoading || storageProvidersQuery.isLoading || systemStatusQuery.isLoading;
  const aggregateError = backupConfigsQuery.error || backupJobsQuery.error || backupArtifactsQuery.error || backupRestoresQuery.error || storageProvidersQuery.error || systemStatusQuery.error;

  const invalidateAll = () => queryClient.invalidateQueries({ queryKey: ALL_KEY });

  const createConfigMut = useMutation({
    mutationFn: (data: Partial<BackupConfiguration>) => createBackupConfig(data),
    onSuccess: () => { invalidateAll(); toast({ tone: "success", title: "Backup policy created" }); setIsCreateConfigOpen(false); setEditingConfig(null); },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to create backup policy" }),
  });

  const updateConfigMut = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<BackupConfiguration> }) => updateBackupConfig(id, data),
    onSuccess: () => { invalidateAll(); toast({ tone: "success", title: "Backup policy updated" }); setIsCreateConfigOpen(false); setEditingConfig(null); },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to update backup policy" }),
  });

  const executeConfigMut = useMutation({
    mutationFn: (id: string) => executeBackupConfig(id),
    onSuccess: () => { invalidateAll(); toast({ tone: "success", title: "Backup job started for this policy" }); },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to start backup job" }),
  });

  const createJobMut = useMutation({
    mutationFn: (data: CreateBackupJobRequest) => createBackupJob(data),
    onSuccess: () => { invalidateAll(); toast({ tone: "success", title: "Backup job created" }); setIsCreateJobOpen(false); },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to create backup job" }),
  });

  const deleteConfigMut = useMutation({
    mutationFn: (id: string) => deleteBackupConfig(id),
    onSuccess: () => { invalidateAll(); toast({ tone: "success", title: "Backup policy deleted" }); },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to delete backup policy" }),
  });

  const deleteJobMut = useMutation({
    mutationFn: (id: string) => deleteBackupJob(id),
    onSuccess: () => { invalidateAll(); toast({ tone: "success", title: "Backup job deleted" }); },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to delete backup job" }),
  });

  const cancelJobMut = useMutation({
    mutationFn: (id: string) => cancelBackupJob(id),
    onSuccess: () => { invalidateAll(); toast({ tone: "success", title: "Backup job cancelled" }); },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to cancel backup job" }),
  });

  const deleteArtifactMut = useMutation({
    mutationFn: (id: string) => deleteBackupArtifact(id),
    onSuccess: () => { invalidateAll(); toast({ tone: "success", title: "Backup artifact deleted" }); },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to delete backup artifact" }),
  });

  const lockArtifactMut = useMutation({
    mutationFn: (id: string) => lockBackupArtifact(id, 'Manual lock'),
    onSuccess: () => { invalidateAll(); toast({ tone: "success", title: "Artifact locked", message: "Retention pruning will skip it." }); },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to lock artifact" }),
  });

  const unlockArtifactMut = useMutation({
    mutationFn: (id: string) => unlockBackupArtifact(id),
    onSuccess: () => { invalidateAll(); toast({ tone: "success", title: "Artifact unlocked", message: "Retention pruning may expire it again." }); },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to unlock artifact" }),
  });

  const verifyArtifactMut = useMutation({
    mutationFn: (id: string) => verifyBackupArtifact(id),
    onSuccess: () => {
      invalidateAll();
      toast({ tone: "success", title: "Integrity check passed", message: "Stored file re-hashed against its recorded digest." });
    },
    onError: (err) => toast({
      tone: "error",
      title: "Integrity check failed",
      message: err instanceof Error ? err.message : "The artifact could not be re-hashed.",
    }),
  });

  const downloadArtifactMut = useMutation({
    mutationFn: async ({ id, name }: { id: string; name: string }) => {
      const blob = await downloadBackupArtifact(id);
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = name;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
      return blob.size;
    },
    onSuccess: (bytes) => toast({ tone: "success", title: "Artifact downloaded", message: `${formatBytes(bytes)} received.` }),
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to download artifact" }),
  });

  const createRestoreMut = useMutation({
    mutationFn: (data: CreateRestoreInput) => createRestore(data),
    onSuccess: (restore) => {
      invalidateAll();
      setIsRestoreOpen(false);
      setSelectedArtifact(null);
      if (restore.status === 'failed') {
        toast({ tone: 'error', title: 'Restore failed', message: restore.errorMessage || 'See the restore record for the reason.' });
      } else {
        toast({ tone: 'success', title: 'Restore started', message: 'Follow progress in the Restores tab.' });
        setActiveTab('restores');
      }
    },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to start restore" }),
  });

  const deleteRestoreMut = useMutation({
    mutationFn: (id: string) => deleteBackupRestore(id),
    onSuccess: () => { invalidateAll(); toast({ tone: "success", title: "Restore record deleted" }); },
    onError: (err) => toast({ tone: "error", title: err instanceof Error ? err.message : "Failed to delete restore record" }),
  });

  const needle = searchQuery.trim().toLowerCase();

  const filteredConfigs = useMemo(() => backupConfigs.filter(config =>
    config.name.toLowerCase().includes(needle)
    || (config.description ?? '').toLowerCase().includes(needle)
    || (config.storageProvider ?? '').toLowerCase().includes(needle),
  ), [backupConfigs, needle]);

  const filteredJobs = useMemo(() => backupJobs.filter(job =>
    job.name.toLowerCase().includes(needle) || job.status.toLowerCase().includes(needle),
  ), [backupJobs, needle]);

  const filteredArtifacts = useMemo(() => backupArtifacts.filter(artifact =>
    artifact.name.toLowerCase().includes(needle)
    || (artifact.displayName ?? '').toLowerCase().includes(needle)
    || artifact.status.toLowerCase().includes(needle),
  ), [backupArtifacts, needle]);

  const filteredRestores = useMemo(() => backupRestores.filter(restore =>
    restore.name.toLowerCase().includes(needle)
    || restore.status.toLowerCase().includes(needle)
    || (restore.artifactId ?? '').toLowerCase().includes(needle),
  ), [backupRestores, needle]);

  const artifactById = useMemo(() => {
    const map = new Map<string, BackupArtifact>();
    for (const artifact of backupArtifacts) map.set(artifact.id, artifact);
    return map;
  }, [backupArtifacts]);

  const renderStatusPill = (status: string) => (
    <Pill tone={deploymentStatusTone(status)}>{status}</Pill>
  );

  const renderVerification = (artifact: BackupArtifact) => {
    const verdict = artifactVerification(artifact);
    if (verdict.state === 'verified') {
      return (
        <div className="space-y-0.5">
          <Pill tone="ok">Verified</Pill>
          <p className="text-text-muted">{verdict.verifiedAt ? formatDate(verdict.verifiedAt) : 'Time not reported'}</p>
        </div>
      );
    }
    if (verdict.state === 'failed') {
      return (
        <div className="space-y-0.5">
          <Pill tone="danger">Check failed</Pill>
          <p className="text-text-muted">{verdict.attempts} attempt{verdict.attempts === 1 ? '' : 's'}, none passed</p>
        </div>
      );
    }
    return (
      <div className="space-y-0.5">
        <Pill tone="unknown">Not checked</Pill>
        <p className="text-text-muted">Run Verify to re-hash it</p>
      </div>
    );
  };

  const renderOverview = () => (
    <div className="space-y-6">
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <MetricTile
          context={systemStatus ? `${systemStatus.backupConfigurations.scheduled} scheduled, ${systemStatus.backupConfigurations.enabled} enabled` : undefined}
          kind="configured"
          label="Backup policies"
          missingReason={systemStatusQuery.isError ? 'The status read failed' : 'Waiting for the status read'}
          state={sourceState(systemStatusQuery, POLL_MS)}
          value={systemStatus?.backupConfigurations.total}
        />
        <MetricTile
          context={systemStatus ? `${systemStatus.backupJobs.running} running, ${systemStatus.backupJobs.pending} pending` : undefined}
          kind="derived"
          label="Backup jobs"
          missingReason={systemStatusQuery.isError ? 'The status read failed' : 'Waiting for the status read'}
          state={sourceState(systemStatusQuery, POLL_MS)}
          tone={systemStatus && systemStatus.backupJobs.failed > 0 ? 'warn' : undefined}
          value={systemStatus?.backupJobs.total}
        />
        <MetricTile
          context={systemStatus ? `${systemStatus.backupArtifacts.verified} verified, ${systemStatus.backupArtifacts.locked} locked` : undefined}
          kind="derived"
          label="Backup artifacts"
          missingReason={systemStatusQuery.isError ? 'The status read failed' : 'Waiting for the status read'}
          state={sourceState(systemStatusQuery, POLL_MS)}
          value={systemStatus?.backupArtifacts.total}
        />
        <MetricTile
          context={systemStatus ? `${systemStatus.backupRestores.completed} completed, ${systemStatus.backupRestores.failed} failed` : undefined}
          kind="derived"
          label="Restore operations"
          missingReason={systemStatusQuery.isError ? 'The status read failed' : 'Waiting for the status read'}
          state={sourceState(systemStatusQuery, POLL_MS)}
          tone={systemStatus && systemStatus.backupRestores.failed > 0 ? 'warn' : undefined}
          value={systemStatus?.backupRestores.total}
        />
      </div>

      <Card>
        <CardHeader icon={Server} title="Storage providers" />
        {storageProvidersQuery.isLoading ? (
          <AdminLoadingState label="Loading storage providers…" />
        ) : storageProvidersQuery.isError ? (
          <div className="p-4">
            <AdminErrorState message={`Storage providers could not be read: ${storageProvidersQuery.error.message}`} retry={() => void storageProvidersQuery.refetch()} />
          </div>
        ) : storageProviders.length === 0 ? (
          <EmptyState icon={Server} message="No storage providers are registered, so a policy has nowhere to write." title="No storage providers" />
        ) : (
          <div className="divide-y divide-line">
            {storageProviders.map(provider => (
              <div className="flex flex-wrap items-center justify-between gap-2 px-5 py-3" key={provider.id}>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-text">{provider.name}</span>
                  <Pill tone="neutral">{provider.type}</Pill>
                  {provider.isDefault ? <Pill tone="ok">Default</Pill> : null}
                </div>
                <Pill tone={provider.enabled ? "ok" : "neutral"}>{provider.enabled ? 'Enabled' : 'Disabled'}</Pill>
              </div>
            ))}
          </div>
        )}
      </Card>

      <Card>
        <CardHeader title="Create" />
        <div className="flex flex-wrap gap-3 p-5">
          <Btn onClick={() => setIsCreateConfigOpen(true)}><Plus size={14} /> New backup policy</Btn>
          <Btn onClick={() => setIsCreateJobOpen(true)} tone="ghost"><Plus size={14} /> New backup job</Btn>
        </div>
      </Card>
    </div>
  );

  const renderConfigurations = () => (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <Input label="Search policies" onChange={setSearchQuery} placeholder="Name, description or provider…" value={searchQuery} />
        <Btn onClick={() => setIsCreateConfigOpen(true)}><Plus size={14} /> New policy</Btn>
      </div>

      <Card>
        <CardHeader title="Backup policies" />
        {backupConfigsQuery.isLoading ? (
          <AdminLoadingState label="Loading policies…" />
        ) : backupConfigsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState message={`Policies could not be read: ${backupConfigsQuery.error.message}`} retry={() => void backupConfigsQuery.refetch()} />
          </div>
        ) : filteredConfigs.length === 0 ? (
          <EmptyState
            icon={Server}
            message={needle ? `No policy matches “${searchQuery}”.` : "No backup policies yet. A policy schedules backups for one resource."}
            title={needle ? "No match" : "No backup policies"}
          />
        ) : (
          <AdminTable label="Backup policies">
            <AdminTHead>
              <AdminTh>Name</AdminTh>
              <AdminTh>Type</AdminTh>
              <AdminTh>Storage</AdminTh>
              <AdminTh>Schedule</AdminTh>
              <AdminTh>Last run</AdminTh>
              <AdminTh>State</AdminTh>
              <AdminTh><span /></AdminTh>
            </AdminTHead>
            <AdminTBody>
              {filteredConfigs.map(config => (
                <AdminTr key={config.id}>
                  <AdminTd className="font-medium">{config.name}</AdminTd>
                  <AdminTd><Pill tone="neutral">{typeWord[config.backupType] ?? config.backupType}</Pill></AdminTd>
                  <AdminTd className="text-text-subtle">{config.storageProvider || '—'}</AdminTd>
                  <AdminTd className="text-xs text-text-subtle">
                    {config.isScheduled ? (
                      config.cronExpression ? (
                        <span className="block">
                          <span className="font-mono">{config.cronExpression}</span>
                          <span className="block text-text-muted">
                            {config.nextRunAt ? `Next: ${formatDate(config.nextRunAt)}` : 'No next run scheduled'}
                          </span>
                        </span>
                      ) : <span className="text-warn">Scheduled with no cron expression</span>
                    ) : <span>Manual</span>}
                  </AdminTd>
                  <AdminTd className="text-xs text-text-subtle">
                    {config.lastRunAt ? (
                      <span className="block space-y-0.5">
                        <span className="block">{formatDate(config.lastRunAt)}</span>
                        {config.lastStatus ? <Pill tone={deploymentStatusTone(config.lastStatus)}>{config.lastStatus}</Pill> : null}
                        {config.lastError ? <span className="mt-0.5 block max-w-[16rem] break-words text-warn">{config.lastError}</span> : null}
                      </span>
                    ) : (
                      <span className="text-text-muted">Never run</span>
                    )}
                  </AdminTd>
                  <AdminTd>
                    <Pill tone={config.enabled ? 'ok' : 'neutral'}>{config.enabled ? 'Enabled' : 'Disabled'}</Pill>
                  </AdminTd>
                  <AdminTd>
                    <ForgeDropdownMenu
                      items={[
                        { id: "execute", icon: <Play className="h-4 w-4" />, label: executeConfigMut.isPending && executeConfigMut.variables === config.id ? "Starting…" : "Run now", onSelect: () => executeConfigMut.mutate(config.id) },
                        { id: "edit", icon: <Server className="h-4 w-4" />, label: "Edit", onSelect: () => { setEditingConfig(config); setIsCreateConfigOpen(true); } },
                        {
                          id: "delete",
                          icon: <Trash2 className="h-4 w-4" />,
                          label: "Delete",
                          tone: "danger",
                          onSelect: () => { void (async () => { if (await confirm({ title: `Delete backup policy "${config.name}"?`, description: "Scheduled backups from this policy will stop. Artifacts it already produced are kept. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteConfigMut.mutate(config.id); })(); },
                        },
                      ]}
                      label={`Actions for ${config.name}`}
                      trigger={<MoreVertical size={14} />}
                    />
                  </AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>
    </div>
  );

  const renderJobs = () => (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <Input label="Search jobs" onChange={setSearchQuery} placeholder="Name or status…" value={searchQuery} />
        <Btn onClick={() => setIsCreateJobOpen(true)}><Plus size={14} /> New job</Btn>
      </div>

      <Card>
        <CardHeader title="Backup jobs" />
        {backupJobsQuery.isLoading ? (
          <AdminLoadingState label="Loading jobs…" />
        ) : backupJobsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState message={`Jobs could not be read: ${backupJobsQuery.error.message}`} retry={() => void backupJobsQuery.refetch()} />
          </div>
        ) : filteredJobs.length === 0 ? (
          <EmptyState
            icon={Database}
            message={needle ? `No job matches “${searchQuery}”.` : "No backup jobs yet. A job is one run of a policy, or one on-demand backup."}
            title={needle ? "No match" : "No backup jobs"}
          />
        ) : (
          <AdminTable label="Backup jobs">
            <AdminTHead>
              <AdminTh>Name</AdminTh>
              <AdminTh>Type</AdminTh>
              <AdminTh>Status</AdminTh>
              <AdminTh>Progress</AdminTh>
              <AdminTh>Triggered by</AdminTh>
              <AdminTh>Created</AdminTh>
              <AdminTh><span /></AdminTh>
            </AdminTHead>
            <AdminTBody>
              {filteredJobs.map(job => (
                <AdminTr key={job.id}>
                  <AdminTd className="font-medium">{job.name}</AdminTd>
                  <AdminTd><Pill tone="neutral">{typeWord[job.jobType] ?? job.jobType}</Pill></AdminTd>
                  <AdminTd>
                    {renderStatusPill(job.status)}
                    {job.errorMessage ? <span className="mt-0.5 block max-w-[18rem] break-words text-xs text-warn">{job.errorMessage}</span> : null}
                  </AdminTd>
                  <AdminTd className="text-xs text-text-subtle"><ProgressCell row={job} /></AdminTd>
                  <AdminTd className="text-xs text-text-subtle">{job.triggeredBy || '—'}</AdminTd>
                  <AdminTd className="text-xs text-text-subtle">{formatDate(job.createdAt, 'Not reported')}</AdminTd>
                  <AdminTd>
                    <ForgeDropdownMenu
                      items={[
                        ...(job.status === 'running' ? [{ id: "cancel", icon: <XCircle className="h-4 w-4" />, label: "Cancel", onSelect: () => cancelJobMut.mutate(job.id) }] : []),
                        {
                          id: "delete",
                          icon: <Trash2 className="h-4 w-4" />,
                          label: "Delete",
                          tone: "danger",
                          onSelect: () => { void (async () => { if (await confirm({ title: `Delete backup job "${job.name}"?`, description: "The job record is removed. Artifacts it already produced are kept.", danger: true, confirmLabel: "Delete" })) deleteJobMut.mutate(job.id); })(); },
                        },
                      ]}
                      label={`Actions for ${job.name}`}
                      trigger={<MoreVertical size={14} />}
                    />
                  </AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>
    </div>
  );

  const renderArtifacts = () => (
    <div className="space-y-4">
      <Input label="Search artifacts" onChange={setSearchQuery} placeholder="Name or status…" value={searchQuery} />

      <p className="rounded-lg border border-line bg-overlay-subtle px-3 py-2 text-xs leading-5 text-text-subtle">
        These are artifacts of the classic backup pipeline (policies → jobs → artifacts). Restic and Kopia
        engine snapshots are a separate system with their own repositories, and they are not listed here.
        {' '}
        <button className="text-text underline underline-offset-2" onClick={() => router.push('/admin/backups/engines')} type="button">
          Open Backup Engines
        </button>
        .
      </p>

      <Card>
        <CardHeader title="Backup artifacts" />
        {backupArtifactsQuery.isLoading ? (
          <AdminLoadingState label="Loading artifacts…" />
        ) : backupArtifactsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState message={`Artifacts could not be read: ${backupArtifactsQuery.error.message}`} retry={() => void backupArtifactsQuery.refetch()} />
          </div>
        ) : filteredArtifacts.length === 0 ? (
          <EmptyState
            icon={Folder}
            message={needle ? `No artifact matches “${searchQuery}”.` : "No backup artifacts yet. A finished backup job produces one."}
            title={needle ? "No match" : "No backup artifacts"}
          />
        ) : (
          <AdminTable label="Backup artifacts">
            <AdminTHead>
              <AdminTh>Name</AdminTh>
              <AdminTh>Type</AdminTh>
              <AdminTh>Storage</AdminTh>
              <AdminTh>Size</AdminTh>
              <AdminTh>Status</AdminTh>
              <AdminTh>Integrity</AdminTh>
              <AdminTh>Retention lock</AdminTh>
              <AdminTh><span /></AdminTh>
            </AdminTHead>
            <AdminTBody>
              {filteredArtifacts.map(artifact => {
                const verdict = artifactVerification(artifact);
                const size = artifactSizeBytes(artifact);
                const restoreBlockReason = verdict.state === 'failed'
                  ? 'Restore blocked: the last integrity check failed. Verify the artifact first.'
                  : null;
                return (
                  <AdminTr key={artifact.id}>
                    <AdminTd className="font-medium">{artifact.displayName || artifact.name}</AdminTd>
                    <AdminTd><Pill tone="neutral">{typeWord[artifact.artifactType] ?? artifact.artifactType}</Pill></AdminTd>
                    <AdminTd className="text-xs text-text-subtle">{artifact.storageProvider || '—'}</AdminTd>
                    <AdminTd className="text-xs text-text-subtle">{sizeCell(size)}</AdminTd>
                    <AdminTd>{renderStatusPill(artifact.status)}</AdminTd>
                    <AdminTd className="text-xs">{renderVerification(artifact)}</AdminTd>
                    <AdminTd className="text-xs">
                      {artifact.isLocked ? <Pill tone="warn">Locked</Pill> : <span className="text-text-muted">Not locked</span>}
                      {artifact.isLocked && artifact.lockReason ? <span className="mt-0.5 block max-w-[14rem] break-words text-text-muted">{artifact.lockReason}</span> : null}
                    </AdminTd>
                    <AdminTd>
                      <ForgeDropdownMenu
                        items={[
                          { id: "verify", icon: <ShieldCheck className="h-4 w-4" />, label: verifyArtifactMut.isPending && verifyArtifactMut.variables === artifact.id ? "Verifying…" : "Verify now", disabled: verifyArtifactMut.isPending, onSelect: () => verifyArtifactMut.mutate(artifact.id) },
                          { id: "download", icon: <Download className="h-4 w-4" />, label: downloadArtifactMut.isPending && downloadArtifactMut.variables?.id === artifact.id ? "Downloading…" : "Download", disabled: downloadArtifactMut.isPending, onSelect: () => downloadArtifactMut.mutate({ id: artifact.id, name: artifact.name }) },
                          ...(artifact.isLocked
                            ? [{ id: "unlock", icon: <RotateCw className="h-4 w-4" />, label: "Unlock", onSelect: () => unlockArtifactMut.mutate(artifact.id) }]
                            : [{ id: "lock", icon: <Lock className="h-4 w-4" />, label: "Lock", onSelect: () => lockArtifactMut.mutate(artifact.id) }]),
                          { id: "restore", icon: <RotateCcw className="h-4 w-4" />, label: 'Restore…', disabled: Boolean(restoreBlockReason), onSelect: () => { setSelectedArtifact(artifact); setIsRestoreOpen(true); } },
                          {
                            id: "delete",
                            icon: <Trash2 className="h-4 w-4" />,
                            label: "Delete",
                            tone: "danger",
                            onSelect: () => { void (async () => { if (await confirm({ title: `Delete backup artifact "${artifact.displayName || artifact.name}"?`, description: "The stored backup file is deleted from its storage provider. Other artifacts are untouched. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteArtifactMut.mutate(artifact.id); })(); },
                          },
                        ]}
                        label={`Actions for ${artifact.displayName || artifact.name}`}
                        trigger={<MoreVertical size={14} />}
                      />
                      {restoreBlockReason ? (
                        <p className="mt-1 max-w-[16rem] text-[11px] leading-4 text-warn">{restoreBlockReason}</p>
                      ) : null}
                    </AdminTd>
                  </AdminTr>
                );
              })}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>
    </div>
  );

  const renderRestores = () => (
    <div className="space-y-4">
      <Input label="Search restores" onChange={setSearchQuery} placeholder="Name, status or artifact ID…" value={searchQuery} />

      <Card>
        <CardHeader title="Restore operations" />
        {backupRestoresQuery.isLoading ? (
          <AdminLoadingState label="Loading restores…" />
        ) : backupRestoresQuery.isError ? (
          <div className="p-4">
            <AdminErrorState message={`Restores could not be read: ${backupRestoresQuery.error.message}`} retry={() => void backupRestoresQuery.refetch()} />
          </div>
        ) : filteredRestores.length === 0 ? (
          <EmptyState
            icon={RotateCw}
            message={needle ? `No restore matches “${searchQuery}”.` : "No restores have been run from this pipeline."}
            title={needle ? "No match" : "No restore operations"}
          />
        ) : (
          <AdminTable label="Restore operations">
            <AdminTHead>
              <AdminTh>Name</AdminTh>
              <AdminTh>From artifact</AdminTh>
              <AdminTh>Type</AdminTh>
              <AdminTh>Status</AdminTh>
              <AdminTh>Progress</AdminTh>
              <AdminTh>Triggered by</AdminTh>
              <AdminTh>Created</AdminTh>
              <AdminTh><span /></AdminTh>
            </AdminTHead>
            <AdminTBody>
              {filteredRestores.map(restore => {
                const source = restore.artifactId ? artifactById.get(restore.artifactId) : undefined;
                return (
                  <AdminTr key={restore.id}>
                    <AdminTd className="font-medium">{restore.name}</AdminTd>
                    <AdminTd className="text-xs text-text-subtle">
                      {restore.artifactId ? (
                        <span className="block max-w-[16rem] truncate font-mono" title={source ? `${source.displayName || source.name} · ${restore.artifactId}` : `Artifact not in this list · ${restore.artifactId}`}>
                          {source ? (source.displayName || source.name) : restore.artifactId}
                        </span>
                      ) : (
                        <span className="text-text-muted">Artifact not reported</span>
                      )}
                    </AdminTd>
                    <AdminTd><Pill tone="neutral">{typeWord[restore.restoreType] ?? restore.restoreType}</Pill></AdminTd>
                    <AdminTd>
                      {renderStatusPill(restore.status)}
                      {restore.errorMessage ? <span className="mt-0.5 block max-w-[18rem] break-words text-xs text-warn">{restore.errorMessage}</span> : null}
                    </AdminTd>
                    <AdminTd className="text-xs text-text-subtle"><ProgressCell row={restore} /></AdminTd>
                    <AdminTd className="text-xs text-text-subtle">{restore.triggeredBy || '—'}</AdminTd>
                    <AdminTd className="text-xs text-text-subtle">{formatDate(restore.createdAt, 'Not reported')}</AdminTd>
                    <AdminTd>
                      <ForgeDropdownMenu
                        items={[
                          {
                            id: "delete",
                            icon: <Trash2 className="h-4 w-4" />,
                            label: "Delete record",
                            tone: "danger",
                            onSelect: () => { void (async () => { if (await confirm({ title: `Delete restore record "${restore.name}"?`, description: "Only the record is removed; the artifact and the restored resource are untouched.", danger: true, confirmLabel: "Delete" })) deleteRestoreMut.mutate(restore.id); })(); },
                          },
                        ]}
                        label={`Actions for ${restore.name}`}
                        trigger={<MoreVertical size={14} />}
                      />
                    </AdminTd>
                  </AdminTr>
                );
              })}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>
    </div>
  );

  const tabs: Array<{ id: string; label: string }> = [
    { id: "overview", label: "Overview" },
    { id: "policies", label: "Policies" },
    { id: "jobs", label: "Jobs" },
    { id: "artifacts", label: "Artifacts" },
    { id: "restores", label: "Restores" },
  ];

  // Search is scoped to one tab: a single shared string silently filtered all
  // four lists, so typing in Policies changed the Jobs rows too.
  const changeTab = (id: string) => { setActiveTab(id); setSearchQuery(''); };

  const confirmRestore = (data: CreateRestoreInput) => {
    const field = targetFieldFor(data.restoreType);
    const targetId = (data[field] as string | undefined) ?? '';
    void (async () => {
      const ok = await confirm({
        title: 'Restore this backup?',
        description: `This writes the stored backup over the ${typeWord[data.restoreType] ?? data.restoreType} ${targetId}. `
          + `${data.restoreOptions?.overwriteExisting ? 'Existing files at the target are replaced.' : 'The restore stops if the target already holds data at the same paths. '} `
          + `${data.restoreOptions?.createBackupBeforeRestore ? 'A snapshot of the target is taken first, so this can be rolled back.' : 'No pre-restore snapshot was requested, so this cannot be rolled back.'}`,
        danger: true,
        confirmLabel: 'Start restore',
      });
      if (ok) createRestoreMut.mutate(data);
    })();
  };

  return (
    <AdminPageLayout>
      <AdminPageHeader
        action={
          <div className="flex items-center gap-2">
            <Btn disabled={loading} onClick={() => void invalidateAll()} tone="ghost">
              <RefreshCw className={freshness.refreshing ? 'animate-spin' : undefined} size={14} /> Refresh
            </Btn>
            <Btn onClick={() => router.push('/admin/backups/engines')} tone="ghost">
              <Archive size={14} /> Backup Engines
            </Btn>
          </div>
        }
        info={adminPageGuides.backups}
        status={<FreshnessBadge state={freshness} />}
      />

      {aggregateError ? (
        <AdminErrorState
          message={`One or more backup reads failed: ${aggregateError instanceof Error ? aggregateError.message : 'unknown error'}`}
          retry={() => void invalidateAll()}
        />
      ) : null}

      <AdminTabs active={activeTab} onChange={changeTab} tabs={tabs} />

      {activeTab === "overview" && renderOverview()}
      {activeTab === "policies" && renderConfigurations()}
      {activeTab === "jobs" && renderJobs()}
      {activeTab === "artifacts" && renderArtifacts()}
      {activeTab === "restores" && renderRestores()}

      {isCreateConfigOpen ? (
        <ConfigFormModal
          initial={editingConfig}
          onClose={() => { setIsCreateConfigOpen(false); setEditingConfig(null); }}
          onSubmit={(data) => { if (editingConfig) updateConfigMut.mutate({ id: editingConfig.id, data }); else createConfigMut.mutate(data); }}
          pending={createConfigMut.isPending || updateConfigMut.isPending}
          storageProviders={storageProviders}
        />
      ) : null}

      {isCreateJobOpen ? (
        <CreateJobModal
          onClose={() => setIsCreateJobOpen(false)}
          onSubmit={(data) => createJobMut.mutate(data)}
          pending={createJobMut.isPending}
        />
      ) : null}

      {isRestoreOpen && selectedArtifact ? (
        <RestoreModal
          artifact={selectedArtifact}
          onClose={() => { setIsRestoreOpen(false); setSelectedArtifact(null); }}
          onSubmit={confirmRestore}
          pending={createRestoreMut.isPending}
        />
      ) : null}

      {renderConfirm()}
    </AdminPageLayout>
  );
}
