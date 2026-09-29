'use client';

import { useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import Link from 'next/link';
import { useToast } from '@/components/ui/toast';
import { Activity, AlertTriangle, BarChart3, ExternalLink, Plus, Play, Trash2, TrendingDown, Zap } from 'lucide-react';
import { fetchJSON, postJSON, putJSON, deleteJSON, unwrapList, unwrapData } from '@/lib/api';
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
  AdminPageLayout,
  AdminTable,
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
  StatsRow,
} from '@/components/admin/admin-ui';
import { FreshnessBadge } from '@/components/admin/telemetry-ui';
import { sourceState, worstSourceState } from '@/lib/admin/telemetry';
import { useConfirm } from '@/components/ui/confirm-dialog';

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

type PolicyDraft = typeof defaultForm;

/**
 * Structural validation the control plane does not perform.
 *
 * `Save` used to be gated only on a non-empty server ID, so an inverted range or a
 * threshold pair with no hysteresis could be persisted — after which the policy
 * simply never fires, with nothing in the UI saying so.
 */
function policyDefects(form: PolicyDraft): string[] {
  const defects: string[] = [];
  if (!form.serverId.trim()) defects.push('A server ID is required — the policy would apply to nothing.');
  if (!Number.isFinite(form.minMemoryMb) || !Number.isFinite(form.maxMemoryMb)) defects.push('Memory bounds must be numbers.');
  else if (form.minMemoryMb > form.maxMemoryMb) defects.push('Minimum memory must not exceed maximum memory.');
  else if (form.minMemoryMb === form.maxMemoryMb) defects.push('Memory min and max are equal, leaving no room to scale.');
  if (!Number.isFinite(form.minCpu) || !Number.isFinite(form.maxCpu)) defects.push('CPU bounds must be numbers.');
  else if (form.minCpu > form.maxCpu) defects.push('Minimum CPU must not exceed maximum CPU.');
  else if (form.minCpu === form.maxCpu) defects.push('CPU min and max are equal, leaving no room to scale.');
  if (!(form.scaleUpThreshold > form.scaleDownThreshold)) {
    defects.push('Scale-up threshold must be strictly above scale-down threshold, otherwise there is no hysteresis and the policy can oscillate.');
  }
  if (form.scaleUpThreshold <= 0 || form.scaleUpThreshold > 1) defects.push('Scale-up threshold must be between 0 and 1 (entered as a percentage).');
  if (form.scaleDownThreshold < 0 || form.scaleDownThreshold >= 1) defects.push('Scale-down threshold must be between 0 and just under 1.');
  if (!Number.isFinite(form.cooldownSeconds) || form.cooldownSeconds < 0) defects.push('Cooldown must be a non-negative number of seconds.');
  return defects;
}

export default function AdminAutoscalerPage() {
  const [confirm, renderConfirm] = useConfirm();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [search, setSearch] = useState('');
  const [showCreate, setShowCreate] = useState(false);
  const [editingPolicy, setEditingPolicy] = useState<ScalingPolicy | null>(null);
  const [form, setForm] = useState<PolicyDraft>(defaultForm);

  const policiesQuery = useQuery({
    queryKey: ['admin', 'autoscaler', 'policies'],
    queryFn: async () => unwrapList(await fetchJSON<ScalingPolicy[]>('/admin/autoscaler/policies')),
    retry: false,
  });
  const metricsQuery = useQuery({
    queryKey: ['admin', 'autoscaler', 'metrics'],
    queryFn: async () => unwrapData(await fetchJSON<AutoscalerMetrics>('/admin/autoscaler/metrics')),
    retry: false,
  });

  const policies = useMemo(() => policiesQuery.data ?? [], [policiesQuery.data]);
  const metrics = metricsQuery.data;
  const policiesKnown = !policiesQuery.isPending && !policiesQuery.isError;
  const metricsKnown = !metricsQuery.isPending && !metricsQuery.isError;

  const createMutation = useMutation({
    mutationFn: (data: PolicyDraft) => postJSON('/admin/autoscaler/policies', data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler', 'policies'] });
      setShowCreate(false);
      setForm(defaultForm);
      toast({ tone: 'success', title: 'Scaling policy created' });
    },
    onError: (err) => toast({ tone: 'error', title: 'Failed to create policy', message: err instanceof Error ? err.message : 'An error occurred' }),
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<ScalingPolicy> }) =>
      putJSON(`/admin/autoscaler/policies/${encodeURIComponent(id)}`, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler', 'policies'] });
      setEditingPolicy(null);
      toast({ tone: 'success', title: 'Scaling policy updated' });
    },
    onError: (err) => toast({ tone: 'error', title: 'Failed to update policy', message: err instanceof Error ? err.message : 'An error occurred' }),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/autoscaler/policies/${encodeURIComponent(id)}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler', 'policies'] });
      toast({ tone: 'success', title: 'Scaling policy deleted' });
    },
    onError: (err) => toast({ tone: 'error', title: 'Failed to delete policy', message: err instanceof Error ? err.message : 'An error occurred' }),
  });

  // `POST /admin/autoscaler/evaluate/:serverId` makes the scaling decision now
  // instead of on the next cycle, and can change live allocations. It is confirmed,
  // and its result invalidates both the policies and the counters.
  const evaluateMutation = useMutation({
    mutationFn: (serverId: string) => postJSON(`/admin/autoscaler/evaluate/${encodeURIComponent(serverId)}`),
    onSuccess: (_data, serverId) => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler'] });
      toast({ tone: 'success', title: 'Evaluation run', message: `The autoscaler evaluated ${serverId}. Any change it made is reflected in the policy list.` });
    },
    onError: (err) => toast({ tone: 'error', title: 'Failed to evaluate autoscaler', message: err instanceof Error ? err.message : 'An error occurred' }),
  });

  const confirmEvaluate = (serverId: string) => {
    void (async () => {
      const ok = await confirm({
        title: `Evaluate ${serverId} now?`,
        description: 'This runs the scaling decision immediately rather than waiting for the next cycle. If the thresholds are met, the workload\'s allocation changes, subject to the policy cooldown.',
        confirmLabel: 'Evaluate',
        danger: true,
      });
      if (ok) evaluateMutation.mutate(serverId);
    })();
  };

  const filtered = policies.filter(
    (p) => !search || p.serverId.toLowerCase().includes(search.toLowerCase()),
  );

  const formDefects = policyDefects(form);

  return (
    <AdminPageLayout>
      <AdminPageHeader
        status={<FreshnessBadge state={worstSourceState([sourceState(policiesQuery, 30_000), sourceState(metricsQuery, 30_000)])} />}
        action={
          <Btn tone="primary" onClick={() => setShowCreate(true)}>
            <Plus size={14} /> Create Policy
          </Btn>
        }
      />

      <StatsRow
        items={[
          { label: 'Total policies', value: policiesKnown ? policies.length : '—', icon: BarChart3 },
          { label: 'Active policies', value: metricsKnown ? (metrics?.activePolicies ?? '—') : '—', icon: Activity, tone: 'green' },
          { label: 'Scale-up events', value: metricsKnown ? (metrics?.scaleUpEventsTotal ?? '—') : '—', icon: Zap, tone: 'blue' },
          { label: 'Scale-down events', value: metricsKnown ? (metrics?.scaleDownEventsTotal ?? '—') : '—', icon: TrendingDown, tone: 'neutral' },
          { label: 'Errors', value: metricsKnown ? (metrics?.scalingErrorsTotal ?? '—') : '—', icon: AlertTriangle, tone: 'red' },
        ]}
      />

      <Card>
        <CardHeader title="Scaling Policies" icon={Activity} />
        <div className="mb-4">
          <Input label="Search by server ID" value={search} onChange={setSearch} placeholder="server ID or fragment" />
        </div>
        {policiesQuery.isPending ? (
          <AdminLoadingState label="Loading policies…" />
        ) : policiesQuery.isError ? (
          <AdminErrorState
            message={`Scaling policies could not be loaded: ${policiesQuery.error instanceof Error ? policiesQuery.error.message : 'request failed'}. Nothing shown here is evidence that no policy exists.`}
            retry={() => void policiesQuery.refetch()}
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={BarChart3}
            title={search ? 'No matching policies' : 'No scaling policies'}
            message={
              search
                ? `No policy's server ID contains “${search.trim()}”. The list loaded successfully and none matched.`
                : 'The policy list loaded successfully and is empty, so nothing is scaling automatically right now.'
            }
          />
        ) : (
          <AdminTable label="Workload scaling policies">
            <AdminTHead>
              <AdminTh>Server ID</AdminTh>
              <AdminTh>Memory range</AdminTh>
              <AdminTh>CPU range</AdminTh>
              <AdminTh>Thresholds</AdminTh>
              <AdminTh>Cooldown</AdminTh>
              <AdminTh>Status</AdminTh>
              <AdminTh>Actions</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {filtered.map((policy) => {
                const degenerate = policy.minMemoryMb >= policy.maxMemoryMb || policy.minCpu >= policy.maxCpu || policy.scaleUpThreshold <= policy.scaleDownThreshold;
                return (
                  <AdminTr key={policy.id}>
                    <AdminTd className="font-mono text-xs text-text">
                      <Link className="inline-flex items-center gap-1 rounded hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" href={`/admin/autoscaler/policy/${encodeURIComponent(policy.id)}`}>
                        {policy.serverId}
                        <ExternalLink aria-hidden="true" size={11} />
                      </Link>
                    </AdminTd>
                    <AdminTd className="text-meta text-text-subtle">
                      {policy.minMemoryMb} MB – {policy.maxMemoryMb} MB
                      {policy.minMemoryMb >= policy.maxMemoryMb ? <Pill className="ml-2" tone="unknown">no headroom</Pill> : null}
                    </AdminTd>
                    <AdminTd className="text-meta text-text-subtle">
                      {policy.minCpu}% – {policy.maxCpu}%
                      {policy.minCpu >= policy.maxCpu ? <Pill className="ml-2" tone="unknown">no headroom</Pill> : null}
                    </AdminTd>
                    <AdminTd className="text-meta text-text-subtle">
                      ↑ {(policy.scaleUpThreshold * 100).toFixed(0)}% / ↓ {(policy.scaleDownThreshold * 100).toFixed(0)}%
                      {policy.scaleUpThreshold <= policy.scaleDownThreshold ? <Pill className="ml-2" tone="unknown">no hysteresis</Pill> : null}
                    </AdminTd>
                    <AdminTd className="text-meta text-text-subtle">{policy.cooldownSeconds}s</AdminTd>
                    <AdminTd>
                      <Pill tone={policy.enabled ? 'green' : 'neutral'}>{policy.enabled ? 'Enabled' : 'Disabled'}</Pill>
                    </AdminTd>
                    <AdminTd>
                      <div className="flex gap-1">
                        <Btn
                          size="sm"
                          tone="ghost"
                          onClick={() => {
                            setEditingPolicy(policy);
                            setForm({ ...defaultForm, ...policy });
                          }}
                        >
                          Edit
                        </Btn>
                        <Btn
                          size="sm"
                          tone="ghost"
                          onClick={() => confirmEvaluate(policy.serverId)}
                          disabled={evaluateMutation.isPending}
                        >
                          <Play size={12} /> Evaluate
                        </Btn>
                        <Btn
                          size="sm"
                          tone="danger"
                          ariaLabel={`Delete the scaling policy for ${policy.serverId}`}
                          title={degenerate ? 'This policy cannot currently fire as written' : undefined}
                          onClick={() => {
                            void (async () => {
                              const ok = await confirm({
                                title: 'Delete this autoscale policy?',
                                description: `Automatic scaling for workload ${policy.serverId} stops; memory and CPU stay where they are and nothing adjusts them under load. This cannot be undone.`,
                                danger: true,
                                confirmLabel: 'Delete',
                              });
                              if (ok) deleteMutation.mutate(policy.id);
                            })();
                          }}
                        >
                          <Trash2 size={12} />
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

      {showCreate && (
        <PolicyFormModal
          title="Create Scaling Policy"
          form={form}
          defects={formDefects}
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
          title={`Edit Scaling Policy — ${editingPolicy.serverId}`}
          form={form}
          defects={formDefects}
          onChange={setForm}
          onSave={() => updateMutation.mutate({ id: editingPolicy.id, data: form })}
          onClose={() => setEditingPolicy(null)}
          saving={updateMutation.isPending}
        />
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}

function PolicyFormModal({
  title,
  form,
  defects,
  onChange,
  onSave,
  onClose,
  saving,
}: {
  title: string;
  form: PolicyDraft;
  defects: string[];
  onChange: (f: PolicyDraft) => void;
  onSave: () => void;
  onClose: () => void;
  saving: boolean;
}) {
  const set = (k: keyof PolicyDraft, v: string) =>
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
        <label className="flex items-center gap-2 text-sm font-medium text-text">
          <input
            type="checkbox"
            checked={form.enabled}
            onChange={(e) => onChange({ ...form, enabled: e.target.checked })}
          />
          Enabled
        </label>
        <Input label="Min memory (MB)" type="number" value={String(form.minMemoryMb)} onChange={(v) => set('minMemoryMb', v)} />
        <Input label="Max memory (MB)" type="number" value={String(form.maxMemoryMb)} onChange={(v) => set('maxMemoryMb', v)} />
        <Input label="Min CPU (%)" type="number" value={String(form.minCpu)} onChange={(v) => set('minCpu', v)} />
        <Input label="Max CPU (%)" type="number" value={String(form.maxCpu)} onChange={(v) => set('maxCpu', v)} />
        <Input label="Scale-up threshold (%)" type="number" value={String(form.scaleUpThreshold * 100)} onChange={(v) => onChange({ ...form, scaleUpThreshold: Number(v) / 100 })} />
        <Input label="Scale-down threshold (%)" type="number" value={String(form.scaleDownThreshold * 100)} onChange={(v) => onChange({ ...form, scaleDownThreshold: Number(v) / 100 })} />
        <Input label="Cooldown (seconds)" type="number" value={String(form.cooldownSeconds)} onChange={(v) => set('cooldownSeconds', v)} />
      </div>
      {defects.length > 0 ? (
        <div className={`ui-alert ${form.serverId.trim() ? 'ui-alert-warning' : 'ui-alert-danger'} mt-4`}>
          <ul className="list-disc space-y-0.5 pl-4">
            {defects.map((d) => <li key={d}>{d}</li>)}
          </ul>
        </div>
      ) : null}
      <ModalFooter
        onCancel={onClose}
        onConfirm={onSave}
        confirmLabel={saving ? 'Saving…' : 'Save'}
        disabled={saving || defects.length > 0}
      />
    </Modal>
  );
}
