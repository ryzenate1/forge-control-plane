'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useParams, useRouter } from 'next/navigation';
import { ArrowLeft, BarChart3, Play, Trash2, Zap } from 'lucide-react';
import { fetchJSON, putJSON, postJSON, deleteJSON } from '@/modules/infrastructure/api';
import {
  Btn,
  Card,
  CardHeader,
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

type ApiResponse<T> = {
  data: T;
};

type AutoscalerMetrics = {
  scaleUpEventsTotal: number;
  scaleDownEventsTotal: number;
  scalingErrorsTotal: number;
  activePolicies: number;
};

export default function AdminPolicyDetailPage() {
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const id = params.id as string;

  const [editing, setEditing] = useState(false);

  const policyQuery = useQuery({
    queryKey: ['admin', 'autoscaler', 'policy', id],
    queryFn: async () =>
      (await fetchJSON<ApiResponse<ScalingPolicy>>(`/admin/autoscaler/policies/${id}`)).data,
  });

  const metricsQuery = useQuery({
    queryKey: ['admin', 'autoscaler', 'metrics'],
    queryFn: async () =>
      (await fetchJSON<ApiResponse<AutoscalerMetrics>>('/admin/autoscaler/metrics')).data,
  });

  const [form, setForm] = useState<ScalingPolicy | null>(null);

  const policy = policyQuery.data;
  const metrics = metricsQuery.data;

  const updateMutation = useMutation({
    mutationFn: (data: Partial<ScalingPolicy>) => putJSON(`/admin/autoscaler/policies/${id}`, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler', 'policy', id] });
      setEditing(false);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteJSON(`/admin/autoscaler/policies/${id}`),
    onSuccess: () => router.push('/admin/autoscaler'),
  });

  const evaluateMutation = useMutation({
    mutationFn: (serverId: string) => postJSON(`/admin/autoscaler/evaluate/${serverId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler', 'policy', id] });
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler', 'metrics'] });
    },
  });

  if (policyQuery.isLoading) {
    return <div className="p-8 text-center text-sm text-slate-500">Loading policy...</div>;
  }

  if (!policy) {
    return <div className="p-8 text-center text-sm text-red-300">Policy not found.</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-4">
        <Btn tone="ghost" onClick={() => router.push('/admin/autoscaler')}>
          <ArrowLeft size={14} /> Back
        </Btn>
        <div className="flex-1">
          <SectionHeader
            title={`Policy: ${policy.serverId}`}
            sub={`Created ${new Date(policy.createdAt).toLocaleDateString()}`}
            action={
              <div className="flex gap-2">
                <Btn
                  tone="warning"
                  onClick={() => evaluateMutation.mutate(policy.serverId)}
                  disabled={evaluateMutation.isPending}
                >
                  <Play size={14} /> Evaluate Now
                </Btn>
                <Btn
                  tone="danger"
                  onClick={() => {
                    if (confirm('Delete this policy?')) deleteMutation.mutate();
                  }}
                >
                  <Trash2 size={14} /> Delete
                </Btn>
              </div>
            }
          />
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader
            title="Policy Settings"
            icon={BarChart3}
            action={
              <Btn
                size="sm"
                tone="ghost"
                onClick={() => {
                  setForm(policy);
                  setEditing(true);
                }}
              >
                Edit
              </Btn>
            }
          />
          <div className="grid grid-cols-2 gap-4 p-4">
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">
                Server ID
              </p>
              <p className="mt-1 font-mono text-sm text-slate-200">{policy.serverId}</p>
            </div>
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">
                Status
              </p>
              <p className="mt-1">
                <Pill tone={policy.enabled ? 'green' : 'neutral'}>
                  {policy.enabled ? 'Enabled' : 'Disabled'}
                </Pill>
              </p>
            </div>
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">
                Min Memory
              </p>
              <p className="mt-1 text-sm text-slate-200">{policy.minMemoryMb} MB</p>
            </div>
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">
                Max Memory
              </p>
              <p className="mt-1 text-sm text-slate-200">{policy.maxMemoryMb} MB</p>
            </div>
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">
                Min CPU
              </p>
              <p className="mt-1 text-sm text-slate-200">{policy.minCpu}%</p>
            </div>
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">
                Max CPU
              </p>
              <p className="mt-1 text-sm text-slate-200">{policy.maxCpu}%</p>
            </div>
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">
                Scale Up Threshold
              </p>
              <p className="mt-1 text-sm text-slate-200">{policy.scaleUpThreshold * 100}%</p>
            </div>
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">
                Scale Down Threshold
              </p>
              <p className="mt-1 text-sm text-slate-200">{policy.scaleDownThreshold * 100}%</p>
            </div>
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">
                Cooldown
              </p>
              <p className="mt-1 text-sm text-slate-200">{policy.cooldownSeconds}s</p>
            </div>
          </div>
        </Card>

        <Card>
          <CardHeader title="Autoscaler Metric Summary" icon={Zap} />
          <div className="grid grid-cols-3 gap-4 p-4">
            <div className="rounded-lg border border-white/[0.06] bg-[#151b27] p-3 text-center">
              <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">
                Scale Ups
              </p>
              <p className="mt-1 text-2xl font-bold text-emerald-400">
                {metrics?.scaleUpEventsTotal ?? 0}
              </p>
            </div>
            <div className="rounded-lg border border-white/[0.06] bg-[#151b27] p-3 text-center">
              <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">
                Scale Downs
              </p>
              <p className="mt-1 text-2xl font-bold text-blue-400">
                {metrics?.scaleDownEventsTotal ?? 0}
              </p>
            </div>
            <div className="rounded-lg border border-white/[0.06] bg-[#151b27] p-3 text-center">
              <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-500">
                Errors
              </p>
              <p className="mt-1 text-2xl font-bold text-red-400">
                {metrics?.scalingErrorsTotal ?? 0}
              </p>
            </div>
          </div>
        </Card>
      </div>

      {editing && form && (
        <Modal title="Edit Policy" onClose={() => setEditing(false)}>
          <div className="grid gap-4 sm:grid-cols-2">
            <Input
              label="Server ID"
              value={form.serverId}
              onChange={(v) => setForm({ ...form, serverId: v })}
            />
            <label className="flex items-center gap-2 text-sm font-medium text-slate-300 pt-6">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                className="rounded border-white/10 bg-[#161b28]"
              />
              Enabled
            </label>
            <Input
              label="Min Memory (MB)"
              type="number"
              value={String(form.minMemoryMb)}
              onChange={(v) => setForm({ ...form, minMemoryMb: Number(v) })}
            />
            <Input
              label="Max Memory (MB)"
              type="number"
              value={String(form.maxMemoryMb)}
              onChange={(v) => setForm({ ...form, maxMemoryMb: Number(v) })}
            />
            <Input
              label="Min CPU (%)"
              type="number"
              value={String(form.minCpu)}
              onChange={(v) => setForm({ ...form, minCpu: Number(v) })}
            />
            <Input
              label="Max CPU (%)"
              type="number"
              value={String(form.maxCpu)}
              onChange={(v) => setForm({ ...form, maxCpu: Number(v) })}
            />
            <Input
              label="Scale Up Threshold (%)"
              type="number"
              value={String(form.scaleUpThreshold * 100)}
              onChange={(v) => setForm({ ...form, scaleUpThreshold: Number(v) / 100 })}
            />
            <Input
              label="Scale Down Threshold (%)"
              type="number"
              value={String(form.scaleDownThreshold * 100)}
              onChange={(v) => setForm({ ...form, scaleDownThreshold: Number(v) / 100 })}
            />
            <Input
              label="Cooldown (seconds)"
              type="number"
              value={String(form.cooldownSeconds)}
              onChange={(v) => setForm({ ...form, cooldownSeconds: Number(v) })}
            />
          </div>
          <ModalFooter
            onCancel={() => setEditing(false)}
            onConfirm={() => updateMutation.mutate(form)}
            confirmLabel={updateMutation.isPending ? 'Saving...' : 'Save'}
            disabled={updateMutation.isPending}
          />
        </Modal>
      )}
    </div>
  );
}
