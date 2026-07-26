"use client";

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { API_BASE_URL, fetchJSON, postJSON, patchJSON, deleteJSON } from '@/lib/api/http';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Progress } from '@/components/ui/progress';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Plus, Search, MoreVertical, RefreshCw, Trash2, Lock, Unlock, Play, XCircle, RotateCcw, Download, Database, Server, AppWindow, Folder } from 'lucide-react';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { toast, Toaster } from '@/components/ui/sonner';

// Types
interface BackupConfiguration {
  id: string;
  name: string;
  description: string;
  backupType: 'app' | 'volume' | 'database' | 'server';
  serverId?: string;
  appId?: string;
  databaseId?: string;
  volumeId?: string;
  isScheduled: boolean;
  cronExpression: string;
  storageProvider: string;
  maxBackups: number;
  retentionDays: number;
  compressionEnabled: boolean;
  encryptionEnabled: boolean;
  enabled: boolean;
  lastStatus: string;
  lastRunAt: string | null;
  nextRunAt: string | null;
  createdAt: string;
  updatedAt: string;
}

interface BackupJob {
  id: string;
  name: string;
  jobType: 'app' | 'volume' | 'database' | 'server' | 'manual';
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  progress: number;
  bytesProcessed: number;
  totalBytes: number;
  startedAt: string | null;
  completedAt: string | null;
  triggeredBy: string;
  createdAt: string;
}

interface CreateBackupJobRequest {
  name: string;
  jobType: 'app' | 'volume' | 'database' | 'server';
  serverId?: string;
  appId?: string;
  databaseId?: string;
  volumeId?: string;
  description?: string;
}

interface BackupArtifact {
  id: string;
  name: string;
  displayName: string;
  artifactType: 'app' | 'volume' | 'database' | 'server';
  storageProvider: string;
  fileSize: number;
  fileHash: string;
  status: string;
  isVerified: boolean;
  isLocked: boolean;
  createdAt: string;
  uploadedAt: string | null;
}

interface BackupRestore {
  id: string;
  name: string;
  description?: string;
  overwrite?: boolean;
  restoreType: 'app' | 'volume' | 'database' | 'server';
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  progress: number;
  startedAt: string | null;
  completedAt: string | null;
  triggeredBy: string;
  createdAt: string;
}

interface StorageProvider {
  id: string;
  name: string;
  type: string;
  enabled: boolean;
  isDefault: boolean;
}

interface BackupSystemStatus {
  backupConfigurations: {
    total: number;
    scheduled: number;
    enabled: number;
  };
  backupJobs: {
    total: number;
    running: number;
    pending: number;
    failed: number;
  };
  backupArtifacts: {
    total: number;
    verified: number;
    locked: number;
    expired: number;
  };
  backupRestores: {
    total: number;
    completed: number;
    failed: number;
  };
  storageProviders: number;
}

// API Functions
const api = {
  getBackupConfigs: (): Promise<BackupConfiguration[]> =>
    fetchJSON<BackupConfiguration[]>('/admin/backups/configs'),

  createBackupConfig: (data: Partial<BackupConfiguration>): Promise<BackupConfiguration> =>
    postJSON<BackupConfiguration>('/admin/backups/configs', data),

  updateBackupConfig: (id: string, data: Partial<BackupConfiguration>): Promise<BackupConfiguration> =>
    patchJSON<BackupConfiguration>(`/admin/backups/configs/${encodeURIComponent(id)}`, data),

  deleteBackupConfig: (id: string): Promise<void> =>
    deleteJSON(`/admin/backups/configs/${encodeURIComponent(id)}`),

  executeBackupConfig: (id: string): Promise<BackupJob> =>
    postJSON<BackupJob>(`/admin/backups/configs/${encodeURIComponent(id)}/execute`),

  getBackupJobs: (): Promise<BackupJob[]> =>
    fetchJSON<BackupJob[]>('/admin/backups/jobs'),

  createBackupJob: (data: CreateBackupJobRequest): Promise<BackupJob> =>
    postJSON<BackupJob>('/admin/backups/jobs', data),

  cancelBackupJob: async (id: string): Promise<void> => {
    await postJSON(`/admin/backups/jobs/${encodeURIComponent(id)}/cancel`);
  },

  deleteBackupJob: (id: string): Promise<void> =>
    deleteJSON(`/admin/backups/jobs/${encodeURIComponent(id)}`),

  getBackupArtifacts: (): Promise<BackupArtifact[]> =>
    fetchJSON<BackupArtifact[]>('/admin/backups/artifacts'),

  deleteBackupArtifact: (id: string): Promise<void> =>
    deleteJSON(`/admin/backups/artifacts/${encodeURIComponent(id)}`),

  lockBackupArtifact: async (id: string, reason: string): Promise<void> => {
    await postJSON(`/admin/backups/artifacts/${encodeURIComponent(id)}/lock`, { reason });
  },

  unlockBackupArtifact: async (id: string): Promise<void> => {
    await postJSON(`/admin/backups/artifacts/${encodeURIComponent(id)}/unlock`);
  },

  downloadBackupArtifact: async (id: string): Promise<Blob> => {
    const response = await fetch(`${API_BASE_URL}/admin/backups/artifacts/${encodeURIComponent(id)}/download`, {
      credentials: 'include',
    });
    if (!response.ok) throw new Error('Failed to download backup artifact');
    return response.blob();
  },

  getBackupRestores: (): Promise<BackupRestore[]> =>
    fetchJSON<BackupRestore[]>('/admin/backups/restores'),

  createRestore: (data: Partial<BackupRestore> & { artifactId: string }): Promise<BackupRestore> =>
    postJSON<BackupRestore>('/admin/backups/restore', data),

  deleteBackupRestore: (id: string): Promise<void> =>
    deleteJSON(`/admin/backups/restores/${encodeURIComponent(id)}`),

  getStorageProviders: (): Promise<StorageProvider[]> =>
    fetchJSON<StorageProvider[]>('/admin/backups/storage-providers'),

  getBackupSystemStatus: (): Promise<BackupSystemStatus> =>
    fetchJSON<BackupSystemStatus>('/admin/backups/status'),
};

// Helper Functions
const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 Bytes';
  const k = 1024;
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

const formatDate = (dateString: string | null): string => {
  if (!dateString) return 'N/A';
  return new Date(dateString).toLocaleString();
};

const getStatusColor = (status: string): string => {
  switch (status) {
    case 'completed':
    case 'verified':
    case 'restored':
      return 'bg-green-500';
    case 'running':
    case 'restoring':
      return 'bg-blue-500';
    case 'pending':
      return 'bg-yellow-500';
    case 'failed':
    case 'cancelled':
    case 'restore_failed':
      return 'bg-red-500';
    default:
      return 'bg-gray-500';
  }
};

const getBackupTypeIcon = (type: string) => {
  switch (type) {
    case 'app':
      return <AppWindow className="h-4 w-4" />;
    case 'volume':
      return <Folder className="h-4 w-4" />;
    case 'database':
      return <Database className="h-4 w-4" />;
    case 'server':
      return <Server className="h-4 w-4" />;
    default:
      return null;
  }
};

// Main Component
export default function BackupManagementPage() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState('overview');
  const [searchQuery, setSearchQuery] = useState('');

  const [isCreateConfigOpen, setIsCreateConfigOpen] = useState(false);
  const [isCreateJobOpen, setIsCreateJobOpen] = useState(false);
  const [isRestoreOpen, setIsRestoreOpen] = useState(false);
  const [selectedArtifact, setSelectedArtifact] = useState<BackupArtifact | null>(null);

  const ALL_KEY = ["admin", "backups"];

  const backupConfigsQuery = useQuery({
    queryKey: [...ALL_KEY, "configs"],
    queryFn: api.getBackupConfigs,
    refetchInterval: 30_000,
  });

  const backupJobsQuery = useQuery({
    queryKey: [...ALL_KEY, "jobs"],
    queryFn: api.getBackupJobs,
    refetchInterval: 30_000,
  });

  const backupArtifactsQuery = useQuery({
    queryKey: [...ALL_KEY, "artifacts"],
    queryFn: api.getBackupArtifacts,
    refetchInterval: 30_000,
  });

  const backupRestoresQuery = useQuery({
    queryKey: [...ALL_KEY, "restores"],
    queryFn: api.getBackupRestores,
    refetchInterval: 30_000,
  });

  const storageProvidersQuery = useQuery({
    queryKey: [...ALL_KEY, "storage-providers"],
    queryFn: api.getStorageProviders,
    refetchInterval: 30_000,
  });

  const systemStatusQuery = useQuery({
    queryKey: [...ALL_KEY, "status"],
    queryFn: api.getBackupSystemStatus,
    refetchInterval: 30_000,
  });

  const backupConfigs = backupConfigsQuery.data ?? [];
  const backupJobs = backupJobsQuery.data ?? [];
  const backupArtifacts = backupArtifactsQuery.data ?? [];
  const backupRestores = backupRestoresQuery.data ?? [];
  const storageProviders = storageProvidersQuery.data ?? [];
  const systemStatus = systemStatusQuery.data ?? null;
  const loading = backupConfigsQuery.isLoading || backupJobsQuery.isLoading || backupArtifactsQuery.isLoading || backupRestoresQuery.isLoading || storageProvidersQuery.isLoading || systemStatusQuery.isLoading;
  const error = backupConfigsQuery.error || backupJobsQuery.error || backupArtifactsQuery.error || backupRestoresQuery.error || storageProvidersQuery.error || systemStatusQuery.error;

  const invalidateAll = () => queryClient.invalidateQueries({ queryKey: ALL_KEY });

  const createConfigMut = useMutation({
    mutationFn: (data: Partial<BackupConfiguration>) => api.createBackupConfig(data),
    onSuccess: () => { invalidateAll(); toast.success('Backup configuration created successfully'); setIsCreateConfigOpen(false); },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to create backup configuration'),
  });

  const executeConfigMut = useMutation({
    mutationFn: (id: string) => api.executeBackupConfig(id),
    onSuccess: () => { invalidateAll(); toast.success('Backup configuration executed successfully'); },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to execute backup configuration'),
  });

  const createJobMut = useMutation({
    mutationFn: (data: CreateBackupJobRequest) => api.createBackupJob(data),
    onSuccess: () => {
      invalidateAll();
      toast.success('Backup job created successfully');
      setIsCreateJobOpen(false);
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to create backup job'),
  });

  const deleteConfigMut = useMutation({
    mutationFn: (id: string) => api.deleteBackupConfig(id),
    onSuccess: () => { invalidateAll(); toast.success('Backup configuration deleted successfully'); },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to delete backup configuration'),
  });

  const deleteJobMut = useMutation({
    mutationFn: (id: string) => api.deleteBackupJob(id),
    onSuccess: () => { invalidateAll(); toast.success('Backup job deleted successfully'); },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to delete backup job'),
  });

  const cancelJobMut = useMutation({
    mutationFn: (id: string) => api.cancelBackupJob(id),
    onSuccess: () => { invalidateAll(); toast.success('Backup job cancelled successfully'); },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to cancel backup job'),
  });

  const deleteArtifactMut = useMutation({
    mutationFn: (id: string) => api.deleteBackupArtifact(id),
    onSuccess: () => { invalidateAll(); toast.success('Backup artifact deleted successfully'); },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to delete backup artifact'),
  });

  const lockArtifactMut = useMutation({
    mutationFn: (id: string) => api.lockBackupArtifact(id, 'Manual lock'),
    onSuccess: () => { invalidateAll(); toast.success('Backup artifact locked successfully'); },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to lock backup artifact'),
  });

  const unlockArtifactMut = useMutation({
    mutationFn: (id: string) => api.unlockBackupArtifact(id),
    onSuccess: () => { invalidateAll(); toast.success('Backup artifact unlocked successfully'); },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to unlock backup artifact'),
  });

  const downloadArtifactMut = useMutation({
    mutationFn: async ({ id, name }: { id: string; name: string }) => {
      const blob = await api.downloadBackupArtifact(id);
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = name;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to download backup artifact'),
  });

  const createRestoreMut = useMutation({
    mutationFn: (data: Partial<BackupRestore> & { artifactId: string }) => api.createRestore(data),
    onSuccess: () => { invalidateAll(); toast.success('Restore operation created successfully'); setIsRestoreOpen(false); setSelectedArtifact(null); },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to create restore operation'),
  });

  const deleteRestoreMut = useMutation({
    mutationFn: (id: string) => api.deleteBackupRestore(id),
    onSuccess: () => { invalidateAll(); toast.success('Backup restore deleted successfully'); },
    onError: (err) => toast.error(err instanceof Error ? err.message : 'Failed to delete backup restore'),
  });

  const handleSearch = (query: string) => setSearchQuery(query);

  const filteredConfigs = backupConfigs.filter(config =>
    config.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    config.description.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const filteredJobs = backupJobs.filter(job =>
    job.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    job.status.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const filteredArtifacts = backupArtifacts.filter(artifact =>
    artifact.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    artifact.displayName.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const filteredRestores = backupRestores.filter(restore =>
    restore.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    restore.status.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const renderStatusBadge = (status: string) => (
    <Badge className={getStatusColor(status)}>
      {status}
    </Badge>
  );

  const renderBackupTypeBadge = (type: string) => (
    <Badge variant="outline" className="flex items-center gap-1">
      {getBackupTypeIcon(type)}
      {type}
    </Badge>
  );

  const renderProgressBar = (progress: number) => (
    <div className="w-full max-w-xs">
      <Progress value={progress} className="h-2" />
      <span className="text-xs text-slate-500">{Math.round(progress)}%</span>
    </div>
  );

  const CreateConfigDialog = () => {
    const [targetId, setTargetId] = useState('');
    const [volumeServerId, setVolumeServerId] = useState('');
    const [formData, setFormData] = useState<Partial<BackupConfiguration>>({
      backupType: 'app',
      isScheduled: false,
      storageProvider: storageProviders.find(p => p.isDefault)?.name || storageProviders[0]?.name || '',
      maxBackups: 10,
      retentionDays: 30,
      compressionEnabled: true,
      encryptionEnabled: false,
      enabled: true,
    });

    const handleSubmit = (e: React.FormEvent) => {
      e.preventDefault();
      const backupType = formData.backupType || 'server';
      const targetField = {
        app: 'appId',
        volume: 'volumeId',
        database: 'databaseId',
        server: 'serverId',
      }[backupType];
      createConfigMut.mutate({
        ...formData,
        [targetField]: targetId.trim(),
        ...(backupType === 'volume' ? { serverId: volumeServerId.trim() } : {}),
      });
    };

    return (
      <Dialog open={isCreateConfigOpen} onOpenChange={setIsCreateConfigOpen}>
        <DialogTrigger asChild>
          <Button onClick={() => setIsCreateConfigOpen(true)}>
            <Plus className="mr-2 h-4 w-4" /> Create Configuration
          </Button>
        </DialogTrigger>
        <DialogContent className="sm:max-w-[600px]">
          <DialogHeader>
            <DialogTitle>Create Backup Configuration</DialogTitle>
            <DialogDescription>
              Create a new backup configuration for scheduled or on-demand backups.
            </DialogDescription>
          </DialogHeader>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="name">Name *</Label>
                <Input
                  id="name"
                  value={formData.name || ''}
                  onChange={(e) => setFormData({...formData, name: e.target.value})}
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="backupType">Backup Type *</Label>
                <Select
                  value={formData.backupType}
                  onValueChange={(value) => setFormData({...formData, backupType: value as 'app' | 'volume' | 'database' | 'server'})}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select backup type" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="app">App</SelectItem>
                    <SelectItem value="volume">Volume</SelectItem>
                    <SelectItem value="database">Database</SelectItem>
                    <SelectItem value="server">Server</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Input
                id="description"
                value={formData.description || ''}
                onChange={(e) => setFormData({...formData, description: e.target.value})}
              />
            </div>

            <div className={formData.backupType === 'volume' ? "grid grid-cols-2 gap-4" : "space-y-2"}>
              <div className="space-y-2">
                <Label htmlFor="config-target">Target ID *</Label>
                <Input id="config-target" value={targetId} onChange={(e) => setTargetId(e.target.value)} required />
              </div>
              {formData.backupType === 'volume' && (
                <div className="space-y-2">
                  <Label htmlFor="config-volume-server">Server ID *</Label>
                  <Input id="config-volume-server" value={volumeServerId} onChange={(e) => setVolumeServerId(e.target.value)} required />
                </div>
              )}
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="storageProvider">Storage Provider *</Label>
                <Select
                  value={formData.storageProvider}
                  onValueChange={(value) => setFormData({...formData, storageProvider: value})}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select storage provider" />
                  </SelectTrigger>
                  <SelectContent>
                    {storageProviders.map(provider => (
                      <SelectItem key={provider.id} value={provider.name}>
                        {provider.name} ({provider.type})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label htmlFor="maxBackups">Max Backups</Label>
                <Input
                  id="maxBackups"
                  type="number"
                  value={formData.maxBackups || 0}
                  onChange={(e) => setFormData({...formData, maxBackups: parseInt(e.target.value) || 0})}
                  min="1"
                />
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="retentionDays">Retention Days</Label>
                <Input
                  id="retentionDays"
                  type="number"
                  value={formData.retentionDays || 0}
                  onChange={(e) => setFormData({...formData, retentionDays: parseInt(e.target.value) || 0})}
                  min="1"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="cronExpression">Cron Expression</Label>
                <Input
                  id="cronExpression"
                  value={formData.cronExpression || ''}
                  onChange={(e) => setFormData({...formData, cronExpression: e.target.value, isScheduled: e.target.value !== ''})}
                  placeholder="0 0 * * *"
                />
              </div>
            </div>

            <div className="flex items-center space-x-4">
              <Label className="flex items-center gap-2 cursor-pointer">
                <Input
                  type="checkbox"
                  checked={formData.compressionEnabled || false}
                  onChange={(e) => setFormData({...formData, compressionEnabled: e.target.checked})}
                />
                Enable Compression
              </Label>

              <Label className="flex items-center gap-2 cursor-pointer">
                <Input
                  type="checkbox"
                  checked={formData.encryptionEnabled || false}
                  onChange={(e) => setFormData({...formData, encryptionEnabled: e.target.checked})}
                />
                Enable Encryption
              </Label>

              <Label className="flex items-center gap-2 cursor-pointer">
                <Input
                  type="checkbox"
                  checked={formData.enabled || false}
                  onChange={(e) => setFormData({...formData, enabled: e.target.checked})}
                />
                Enabled
              </Label>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setIsCreateConfigOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={createConfigMut.isPending || !targetId.trim() || (formData.backupType === 'volume' && !volumeServerId.trim())}>Create Configuration</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    );
  };

  const CreateJobDialog = () => {
    const [name, setName] = useState('');
    const [jobType, setJobType] = useState<CreateBackupJobRequest['jobType']>('server');
    const [targetId, setTargetId] = useState('');
    const [volumeServerId, setVolumeServerId] = useState('');
    const [description, setDescription] = useState('');

    const handleSubmit = (e: React.FormEvent) => {
      e.preventDefault();
      const targetField = {
        app: 'appId',
        volume: 'volumeId',
        database: 'databaseId',
        server: 'serverId',
      }[jobType];
      createJobMut.mutate({
        name: name.trim(),
        jobType,
        description: description.trim() || undefined,
        [targetField]: targetId.trim(),
        ...(jobType === 'volume' ? { serverId: volumeServerId.trim() } : {}),
      });
    };

    return (
      <Dialog open={isCreateJobOpen} onOpenChange={setIsCreateJobOpen}>
        <DialogContent className="sm:max-w-[520px]">
          <DialogHeader>
            <DialogTitle>Create Backup Job</DialogTitle>
            <DialogDescription>Create an on-demand backup for one application resource.</DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="job-name">Name *</Label>
              <Input id="job-name" value={name} onChange={(e) => setName(e.target.value)} required />
            </div>
            {jobType === 'volume' && (
              <div className="space-y-2">
                <Label htmlFor="job-volume-server">Server ID *</Label>
                <Input id="job-volume-server" value={volumeServerId} onChange={(e) => setVolumeServerId(e.target.value)} required />
              </div>
            )}
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="job-type">Backup Type *</Label>
                <Select value={jobType} onValueChange={(value) => setJobType(value as CreateBackupJobRequest['jobType'])}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="app">App</SelectItem>
                    <SelectItem value="volume">Volume</SelectItem>
                    <SelectItem value="database">Database</SelectItem>
                    <SelectItem value="server">Server</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="job-target">Target ID *</Label>
                <Input id="job-target" value={targetId} onChange={(e) => setTargetId(e.target.value)} required />
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="job-description">Description</Label>
              <Input id="job-description" value={description} onChange={(e) => setDescription(e.target.value)} />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setIsCreateJobOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={createJobMut.isPending || !name.trim() || !targetId.trim() || (jobType === 'volume' && !volumeServerId.trim())}>
                {createJobMut.isPending ? 'Creating...' : 'Create Job'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    );
  };

  const RestoreDialog = () => {
    const [formData, setFormData] = useState<Partial<BackupRestore> & { artifactId: string }>({
      artifactId: selectedArtifact?.id || '',
      restoreType: selectedArtifact?.artifactType || 'app',
      triggeredBy: 'manual',
    });

    const handleSubmit = (e: React.FormEvent) => {
      e.preventDefault();
      createRestoreMut.mutate(formData);
    };

    if (!selectedArtifact) return null;

    return (
      <Dialog open={isRestoreOpen} onOpenChange={setIsRestoreOpen}>
        <DialogContent className="sm:max-w-[500px]">
          <DialogHeader>
            <DialogTitle>Restore Backup</DialogTitle>
            <DialogDescription>
              Restore from backup artifact: {selectedArtifact.displayName || selectedArtifact.name}
            </DialogDescription>
          </DialogHeader>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="name">Restore Name</Label>
              <Input
                id="name"
                value={formData.name || ''}
                onChange={(e) => setFormData({...formData, name: e.target.value})}
                placeholder={`restore-${selectedArtifact.name}-${new Date().toISOString().slice(0, 10)}`}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="restoreType">Restore Type</Label>
              <Select
                value={formData.restoreType}
                onValueChange={(value) => setFormData({...formData, restoreType: value as 'app' | 'volume' | 'database' | 'server'})}
                disabled
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select restore type" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="app">App</SelectItem>
                  <SelectItem value="volume">Volume</SelectItem>
                  <SelectItem value="database">Database</SelectItem>
                  <SelectItem value="server">Server</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Input
                id="description"
                value={formData.description || ''}
                onChange={(e) => setFormData({...formData, description: e.target.value})}
                placeholder="Optional description for this restore operation"
              />
            </div>

            <div className="flex items-center space-x-4">
              <Label className="flex items-center gap-2 cursor-pointer">
                <Input
                  type="checkbox"
                  checked={formData.overwrite || false}
                  onChange={(e) => setFormData({...formData, overwrite: e.target.checked})}
                />
                Overwrite Existing
              </Label>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setIsRestoreOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={createRestoreMut.isPending}>Start Restore</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    );
  };

  const OverviewTab = () => (
    <div className="space-y-6">
      {error && (
        <Alert variant="destructive">
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error instanceof Error ? error.message : 'Failed to fetch data'}</AlertDescription>
        </Alert>
      )}

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Backup Configurations</CardTitle>
            <Server className="h-4 w-4 text-slate-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{systemStatus?.backupConfigurations.total || 0}</div>
            <p className="text-xs text-slate-500">
              {systemStatus?.backupConfigurations.scheduled || 0} scheduled, {systemStatus?.backupConfigurations.enabled || 0} enabled
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Backup Jobs</CardTitle>
            <Database className="h-4 w-4 text-slate-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{systemStatus?.backupJobs.total || 0}</div>
            <p className="text-xs text-slate-500">
              {systemStatus?.backupJobs.running || 0} running, {systemStatus?.backupJobs.pending || 0} pending
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Backup Artifacts</CardTitle>
            <Folder className="h-4 w-4 text-slate-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{systemStatus?.backupArtifacts.total || 0}</div>
            <p className="text-xs text-slate-500">
              {systemStatus?.backupArtifacts.verified || 0} verified, {systemStatus?.backupArtifacts.locked || 0} locked
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Restore Operations</CardTitle>
            <RotateCcw className="h-4 w-4 text-slate-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{systemStatus?.backupRestores.total || 0}</div>
            <p className="text-xs text-slate-500">
              {systemStatus?.backupRestores.completed || 0} completed
            </p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Storage Providers</CardTitle>
          <CardDescription>Configured storage backends for backups</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-2">
            {storageProviders.map(provider => (
              <div key={provider.id} className="flex items-center justify-between p-2 border rounded-lg">
                <div className="flex items-center gap-2">
                  <span className="font-medium">{provider.name}</span>
                  <Badge variant={provider.isDefault ? 'default' : 'outline'}>
                    {provider.type}
                  </Badge>
                  {provider.isDefault && <Badge variant="default">Default</Badge>}
                </div>
                <Badge variant={provider.enabled ? 'outline' : 'secondary'}>
                  {provider.enabled ? 'Enabled' : 'Disabled'}
                </Badge>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Quick Actions</CardTitle>
          <CardDescription>Common backup operations</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-2">
          <CreateConfigDialog />
          <Button variant="outline" onClick={() => setIsCreateJobOpen(true)}>
            <Plus className="mr-2 h-4 w-4" /> Create Backup Job
          </Button>
          <Button variant="outline" onClick={() => queryClient.invalidateQueries({ queryKey: ALL_KEY })} disabled={loading}>
            <RefreshCw className="mr-2 h-4 w-4" /> Refresh Data
          </Button>
        </CardContent>
      </Card>
    </div>
  );

  const ConfigurationsTab = () => (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="relative">
          <Search className="absolute left-2 top-2.5 h-4 w-4 text-slate-500" />
          <Input
            placeholder="Search configurations..."
            value={searchQuery}
            onChange={(e) => handleSearch(e.target.value)}
            className="pl-8 w-[300px]"
          />
        </div>
        <CreateConfigDialog />
      </div>

      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Storage</TableHead>
                <TableHead>Schedule</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredConfigs.length > 0 ? (
                filteredConfigs.map(config => (
                  <TableRow key={config.id}>
                    <TableCell className="font-medium">{config.name}</TableCell>
                    <TableCell>{renderBackupTypeBadge(config.backupType)}</TableCell>
                    <TableCell>{config.storageProvider}</TableCell>
                    <TableCell>
                      {config.isScheduled ? (
                        <span className="text-sm">
                          {config.cronExpression}
                          {config.nextRunAt && (
                            <span className="block text-xs text-slate-500">
                              Next: {formatDate(config.nextRunAt)}
                            </span>
                          )}
                        </span>
                      ) : (
                        <span className="text-sm text-slate-500">Manual</span>
                      )}
                    </TableCell>
                    <TableCell>{renderStatusBadge(config.enabled ? 'enabled' : 'disabled')}</TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" className="h-8 w-8 p-0">
                            <MoreVertical className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuLabel>Actions</DropdownMenuLabel>
                          <DropdownMenuItem onClick={() => executeConfigMut.mutate(config.id)}>
                            <Play className="mr-2 h-4 w-4" /> Execute Now
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem onClick={() => router.push(`/admin/backups/configs/${config.id}`)}>
                            <Server className="mr-2 h-4 w-4" /> View Details
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => router.push(`/admin/backups/configs/${config.id}/edit`)}>
                            <Server className="mr-2 h-4 w-4" /> Edit
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem onClick={() => deleteConfigMut.mutate(config.id)} className="text-red-500">
                            <Trash2 className="mr-2 h-4 w-4" /> Delete
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                ))
              ) : (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-slate-500">
                    No backup configurations found
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );

  const JobsTab = () => (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="relative">
          <Search className="absolute left-2 top-2.5 h-4 w-4 text-slate-500" />
          <Input
            placeholder="Search jobs..."
            value={searchQuery}
            onChange={(e) => handleSearch(e.target.value)}
            className="pl-8 w-[300px]"
          />
        </div>
        <Button onClick={() => setIsCreateJobOpen(true)}>
          <Plus className="mr-2 h-4 w-4" /> Create Job
        </Button>
      </div>

      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Progress</TableHead>
                <TableHead>Triggered By</TableHead>
                <TableHead>Created At</TableHead>
                <TableHead>Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredJobs.length > 0 ? (
                filteredJobs.map(job => (
                  <TableRow key={job.id}>
                    <TableCell className="font-medium">{job.name}</TableCell>
                    <TableCell>{renderBackupTypeBadge(job.jobType)}</TableCell>
                    <TableCell>{renderStatusBadge(job.status)}</TableCell>
                    <TableCell>
                      {job.status === 'running' && renderProgressBar(job.progress)}
                      {job.status !== 'running' && <span className="text-sm">{job.progress}%</span>}
                    </TableCell>
                    <TableCell>{job.triggeredBy}</TableCell>
                    <TableCell>{formatDate(job.createdAt)}</TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" className="h-8 w-8 p-0">
                            <MoreVertical className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuLabel>Actions</DropdownMenuLabel>
                          {job.status === 'running' && (
                            <DropdownMenuItem onClick={() => cancelJobMut.mutate(job.id)}>
                              <XCircle className="mr-2 h-4 w-4" /> Cancel
                            </DropdownMenuItem>
                          )}
                          <DropdownMenuItem onClick={() => router.push(`/admin/backups/jobs/${job.id}`)}>
                            <Server className="mr-2 h-4 w-4" /> View Details
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem onClick={() => deleteJobMut.mutate(job.id)} className="text-red-500">
                            <Trash2 className="mr-2 h-4 w-4" /> Delete
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                ))
              ) : (
                <TableRow>
                  <TableCell colSpan={7} className="text-center text-slate-500">
                    No backup jobs found
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );

  const ArtifactsTab = () => (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="relative">
          <Search className="absolute left-2 top-2.5 h-4 w-4 text-slate-500" />
          <Input
            placeholder="Search artifacts..."
            value={searchQuery}
            onChange={(e) => handleSearch(e.target.value)}
            className="pl-8 w-[300px]"
          />
        </div>
      </div>

      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Storage</TableHead>
                <TableHead>Size</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Verified</TableHead>
                <TableHead>Locked</TableHead>
                <TableHead>Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredArtifacts.length > 0 ? (
                filteredArtifacts.map(artifact => (
                  <TableRow key={artifact.id}>
                    <TableCell className="font-medium">{artifact.displayName || artifact.name}</TableCell>
                    <TableCell>{renderBackupTypeBadge(artifact.artifactType)}</TableCell>
                    <TableCell>{artifact.storageProvider}</TableCell>
                    <TableCell>{formatBytes(artifact.fileSize)}</TableCell>
                    <TableCell>{renderStatusBadge(artifact.status)}</TableCell>
                    <TableCell>
                      {artifact.isVerified ? (
                        <Badge variant="outline" className="bg-green-500">Yes</Badge>
                      ) : (
                        <Badge variant="outline">No</Badge>
                      )}
                    </TableCell>
                    <TableCell>
                      {artifact.isLocked ? (
                        <Badge variant="outline" className="bg-yellow-500">Yes</Badge>
                      ) : (
                        <Badge variant="outline">No</Badge>
                      )}
                    </TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" className="h-8 w-8 p-0">
                            <MoreVertical className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuLabel>Actions</DropdownMenuLabel>
                          <DropdownMenuItem onClick={() => downloadArtifactMut.mutate({ id: artifact.id, name: artifact.name })}>
                            <Download className="mr-2 h-4 w-4" /> Download
                          </DropdownMenuItem>
                          {artifact.isLocked ? (
                            <DropdownMenuItem onClick={() => unlockArtifactMut.mutate(artifact.id)}>
                              <Unlock className="mr-2 h-4 w-4" /> Unlock
                            </DropdownMenuItem>
                          ) : (
                            <DropdownMenuItem onClick={() => lockArtifactMut.mutate(artifact.id)}>
                              <Lock className="mr-2 h-4 w-4" /> Lock
                            </DropdownMenuItem>
                          )}
                          <DropdownMenuItem onClick={() => {
                            setSelectedArtifact(artifact);
                            setIsRestoreOpen(true);
                          }}>
                            <RotateCcw className="mr-2 h-4 w-4" /> Restore
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem onClick={() => deleteArtifactMut.mutate(artifact.id)} className="text-red-500">
                            <Trash2 className="mr-2 h-4 w-4" /> Delete
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                ))
              ) : (
                <TableRow>
                  <TableCell colSpan={8} className="text-center text-slate-500">
                    No backup artifacts found
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );

  const RestoresTab = () => (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="relative">
          <Search className="absolute left-2 top-2.5 h-4 w-4 text-slate-500" />
          <Input
            placeholder="Search restores..."
            value={searchQuery}
            onChange={(e) => handleSearch(e.target.value)}
            className="pl-8 w-[300px]"
          />
        </div>
      </div>

      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Progress</TableHead>
                <TableHead>Triggered By</TableHead>
                <TableHead>Created At</TableHead>
                <TableHead>Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredRestores.length > 0 ? (
                filteredRestores.map(restore => (
                  <TableRow key={restore.id}>
                    <TableCell className="font-medium">{restore.name}</TableCell>
                    <TableCell>{renderBackupTypeBadge(restore.restoreType)}</TableCell>
                    <TableCell>{renderStatusBadge(restore.status)}</TableCell>
                    <TableCell>
                      {restore.status === 'running' && renderProgressBar(restore.progress)}
                      {restore.status !== 'running' && <span className="text-sm">{restore.progress}%</span>}
                    </TableCell>
                    <TableCell>{restore.triggeredBy}</TableCell>
                    <TableCell>{formatDate(restore.createdAt)}</TableCell>
                    <TableCell>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" className="h-8 w-8 p-0">
                            <MoreVertical className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuLabel>Actions</DropdownMenuLabel>
                          <DropdownMenuItem onClick={() => router.push(`/admin/backups/restores/${restore.id}`)}>
                            <Server className="mr-2 h-4 w-4" /> View Details
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem onClick={() => deleteRestoreMut.mutate(restore.id)} className="text-red-500">
                            <Trash2 className="mr-2 h-4 w-4" /> Delete
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                ))
              ) : (
                <TableRow>
                  <TableCell colSpan={7} className="text-center text-slate-500">
                    No restore operations found
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );

  return (
    <div className="container mx-auto py-6">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-3xl font-bold">Backup & Recovery</h1>
          <p className="text-slate-500">Manage backup configurations, jobs, artifacts, and restore operations</p>
        </div>
        <Button variant="outline" onClick={() => queryClient.invalidateQueries({ queryKey: ALL_KEY })} disabled={loading}>
          <RefreshCw className="mr-2 h-4 w-4" /> Refresh
        </Button>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-4">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="configurations">Configurations</TabsTrigger>
          <TabsTrigger value="jobs">Jobs</TabsTrigger>
          <TabsTrigger value="artifacts">Artifacts</TabsTrigger>
          <TabsTrigger value="restores">Restores</TabsTrigger>
        </TabsList>

        <TabsContent value="overview">
          <OverviewTab />
        </TabsContent>

        <TabsContent value="configurations">
          <ConfigurationsTab />
        </TabsContent>

        <TabsContent value="jobs">
          <JobsTab />
        </TabsContent>

        <TabsContent value="artifacts">
          <ArtifactsTab />
        </TabsContent>

        <TabsContent value="restores">
          <RestoresTab />
        </TabsContent>
      </Tabs>

      <RestoreDialog />
      <CreateJobDialog />
      <Toaster />
    </div>
  );
}
