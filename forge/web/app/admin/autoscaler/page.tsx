'use client';

import { useState, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Activity, AlertTriangle, BarChart3, Plus, Play, Trash2, Zap } from 'lucide-react';
import { fetchJSON, postJSON, putJSON, deleteJSON } from '@/modules/infrastructure/api';
import {
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  Pill,
  SectionHeader,
} from '@/components/admin/admin-ui';

type ScalingPolicy = {
  id: string;
  serverId: string;
  minMemoryMb: number;
  maxMemoryMb: number;
  minCpu: number;
  maxCpu: number;
  scaleUpThreshold: number;
  scaleDownThreshold: number;
  cooldownSeconds: number;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

type AutoscalerMetrics = {
  scaleUpEventsTotal: number;
  scaleDownEventsTotal: number;
  scalingErrorsTotal: number;
  activePolicies: number;
};

const defaultForm = {
  serverId: '',
  minMemoryMb: 512,
  maxMemoryMb: 4096,
  minCpu: 10,
  maxCpu: 100,
  scaleUpThreshold: 0.8,
  scaleDownThreshold: 0.3,
  cooldownSeconds: 300,
  enabled: true,
};

export default function AdminAutoscalerPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const [showCreate, setShowCreate] = useState(false);
  const [editingPolicy, setEditingPolicy] = useState<ScalingPolicy | null>(null);
  const [form, setForm] = useState(defaultForm);

  const policiesQuery = useQuery({
    queryKey: ['admin', 'autoscaler', 'policies'],
    queryFn: () =>
      fetchJSON<ScalingPolicy[]>('/admin/autoscaler/policies'),
  });
  const metricsQuery = useQuery({
    queryKey: ['admin', 'autoscaler', 'metrics'],
    queryFn: () =>
      fetchJSON<AutoscalerMetrics>('/admin/autoscaler/metrics'),
  });

  const policies = useMemo(() => policiesQuery.data ?? [], [policiesQuery.data]);
  const metrics = metricsQuery.data;

  const createMutation = useMutation({
    mutationFn: (data: typeof defaultForm) => postJSON('/admin/autoscaler/policies', data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler', 'policies'] });
      setShowCreate(false);
      setForm(defaultForm);
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<ScalingPolicy> }) =>
      putJSON(`/admin/autoscaler/policies/${id}`, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler', 'policies'] });
      setEditingPolicy(null);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/autoscaler/policies/${id}`),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler', 'policies'] }),
  });

  const evaluateMutation = useMutation({
    mutationFn: (serverId: string) => postJSON(`/admin/autoscaler/evaluate/${serverId}`),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler', 'metrics'] }),
  });

  const filtered = policies.filter(
    (p) => !search || p.serverId.toLowerCase().includes(search.toLowerCase()),
  );

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Auto-Scaler"
        sub="Scaling policies that automatically adjust resources based on load thresholds."
        action={
          <Btn tone="primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> Create Policy
          </Btn>
        }
      />

      <div className="grid gap-4 md:grid-cols-3">
        <Card className="p-4">
          <div className="flex items-center gap-2 text-xs uppercase tracking-wider text-slate-500 mb-1">
            <BarChart3 size={12} /> Total Policies
          </div>
          <div className="text-2xl font-bold text-slate-100">{policies.length}</div>
        </Card>
        <Card className="p-4">
          <div className="flex items-center gap-2 text-xs uppercase tracking-wider text-slate-500 mb-1">
            <Zap size={12} /> Scale Up Events
          </div>
          <div className="text-2xl font-bold text-emerald-400">
            {metrics?.scaleUpEventsTotal ?? 0}
          </div>
        </Card>
        <Card className="p-4">
          <div className="flex items-center gap-2 text-xs uppercase tracking-wider text-slate-500 mb-1">
            <AlertTriangle size={12} /> Errors
          </div>
          <div className="text-2xl font-bold text-red-400">{metrics?.scalingErrorsTotal ?? 0}</div>
        </Card>
      </div>

      <Card>
        <CardHeader title="Scaling Policies" icon={Activity} />
        <div className="flex items-center gap-3 p-4">
          <Input placeholder="Search by Server ID" value={search} onChange={setSearch} />
        </div>
        {policiesQuery.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading policies...</div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={BarChart3} message="No scaling policies configured." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Server ID</th>
                  <th className="px-4 py-3">Memory Range</th>
                  <th className="px-4 py-3">CPU Range</th>
                  <th className="px-4 py-3">Thresholds</th>
                  <th className="px-4 py-3">Cooldown</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filtered.map((policy) => (
                  <tr key={policy.id} className="hover:bg-white/[0.02]">
                    <td className="px-4 py-3 font-mono text-xs font-medium text-slate-200">
                      {policy.serverId}
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
                      {policy.minMemoryMb} MB – {policy.maxMemoryMb} MB
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
                      {policy.minCpu}% – {policy.maxCpu}%
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
                      ↑ {policy.scaleUpThreshold * 100}% / ↓ {policy.scaleDownThreshold * 100}%
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">{policy.cooldownSeconds}s</td>
                    <td className="px-4 py-3">
                      <Pill tone={policy.enabled ? 'green' : 'neutral'}>
                        {policy.enabled ? 'Enabled' : 'Disabled'}
                      </Pill>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex gap-1">
                        <Btn
                          size="sm"
                          tone="ghost"
                          onClick={() => {
                            setEditingPolicy(policy);
                            setForm({ ...policy });
                          }}
                        >
                          Edit
                        </Btn>
                        <Btn
                          size="sm"
                          tone="ghost"
                          onClick={() => evaluateMutation.mutate(policy.serverId)}
                          disabled={evaluateMutation.isPending}
                        >
                          <Play size={12} /> Evaluate
                        </Btn>
                        <Btn
                          size="sm"
                          tone="danger"
                          onClick={() => {
                            if (confirm('Delete this policy?')) deleteMutation.mutate(policy.id);
                          }}
                        >
                          <Trash2 size={12} />
                        </Btn>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {showCreate && (
        <PolicyFormModal
          title="Create Scaling Policy"
          form={form}
          onChange={setForm}
          onSave={() => createMutation.mutate(form)}
          onClose={() => {
            setShowCreate(false);
            setForm(defaultForm);
          }}
          saving={createMutation.isPending}
        />
      )}

      {editingPolicy && (
        <PolicyFormModal
          title="Edit Scaling Policy"
          form={form}
          onChange={setForm}
          onSave={() => updateMutation.mutate({ id: editingPolicy.id, data: form })}
          onClose={() => setEditingPolicy(null)}
          saving={updateMutation.isPending}
        />
      )}
    </div>
  );
}

function PolicyFormModal({
  title,
  form,
  onChange,
  onSave,
  onClose,
  saving,
}: {
  title: string;
  form: typeof defaultForm;
  onChange: (f: typeof defaultForm) => void;
  onSave: () => void;
  onClose: () => void;
  saving: boolean;
}) {
  const set = (k: keyof typeof defaultForm, v: string) =>
    onChange({ ...form, [k]: k === 'serverId' ? v : Number(v) });

  return (
    <Modal title={title} onClose={onClose}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Input
          label="Server ID"
          value={form.serverId}
          onChange={(v) => set('serverId', v)}
          placeholder="e.g. srv_abc123"
        />
        <label className="flex items-center gap-2 text-sm font-medium text-slate-300 pt-6">
          <input
            type="checkbox"
            checked={form.enabled}
            onChange={(e) => onChange({ ...form, enabled: e.target.checked })}
            className="rounded border-white/10 bg-[#161b28]"
          />
          Enabled
        </label>
        <Input
          label="Min Memory (MB)"
          type="number"
          value={String(form.minMemoryMb)}
          onChange={(v) => set('minMemoryMb', v)}
        />
        <Input
          label="Max Memory (MB)"
          type="number"
          value={String(form.maxMemoryMb)}
          onChange={(v) => set('maxMemoryMb', v)}
        />
        <Input
          label="Min CPU (%)"
          type="number"
          value={String(form.minCpu)}
          onChange={(v) => set('minCpu', v)}
        />
        <Input
          label="Max CPU (%)"
          type="number"
          value={String(form.maxCpu)}
          onChange={(v) => set('maxCpu', v)}
        />
        <Input
          label="Scale Up Threshold (%)"
          type="number"
          value={String(form.scaleUpThreshold * 100)}
          onChange={(v) => onChange({ ...form, scaleUpThreshold: Number(v) / 100 })}
        />
        <Input
          label="Scale Down Threshold (%)"
          type="number"
          value={String(form.scaleDownThreshold * 100)}
          onChange={(v) => onChange({ ...form, scaleDownThreshold: Number(v) / 100 })}
        />
        <Input
          label="Cooldown (seconds)"
          type="number"
          value={String(form.cooldownSeconds)}
          onChange={(v) => set('cooldownSeconds', v)}
        />
      </div>
      <ModalFooter
        onCancel={onClose}
        onConfirm={onSave}
        confirmLabel={saving ? 'Saving...' : 'Save'}
        disabled={saving || !form.serverId}
      />
    </Modal>
  );
}
