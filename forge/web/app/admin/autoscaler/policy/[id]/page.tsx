'use client';

import { useMemo, useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useToast } from '@/components/ui/toast';
import { useParams, useRouter } from 'next/navigation';
import { AlertTriangle, BarChart3, Play, Trash2, Zap } from 'lucide-react';
import { fetchJSON, putJSON, postJSON, deleteJSON } from '@/lib/api';
import {
  AdminErrorState,
  AdminLoadingState,
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
import { FreshnessBadge } from '@/components/admin/telemetry-ui';
import { sourceState } from '@/lib/admin/telemetry';
import { useConfirm } from '@/components/ui/confirm-dialog';
import { formatDate } from '@/lib/utils';

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

/**
 * `GET /admin/autoscaler/metrics` (handlers_autoscaler.go:80) answers with
 * `autoscaler.Service.Metrics()` — process-wide counters for the whole fleet, with
 * no server or policy scoping available on that route. They are rendered here
 * because this is the only metrics surface, but labelled as fleet-global so they
 * are never read as "this workload's scaling history".
 */
type AutoscalerMetrics = {
  scaleUpEventsTotal: number;
  scaleDownEventsTotal: number;
  scalingErrorsTotal: number;
  activePolicies: number;
};

/** A reading that never arrived is drawn as `—`, never as `0`. */
function readout(value: number | undefined, known: boolean): string | number {
  return known && typeof value === 'number' ? value : '—';
}

/**
 * Structural checks the API does not make. `Save` used to be gated on nothing, so
 * an inverted or degenerate policy could be persisted and would then simply never
 * fire — silently. These are warnings rather than blocks on the detail page so an
 * operator can still see what is stored, but the edit form refuses to save.
 */
function policyDefects(policy: ScalingPolicy): string[] {
  const defects: string[] = [];
  if (policy.minMemoryMb > policy.maxMemoryMb) defects.push('Minimum memory is above maximum memory.');
  if (policy.minCpu > policy.maxCpu) defects.push('Minimum CPU is above maximum CPU.');
  if (policy.minMemoryMb === policy.maxMemoryMb) defects.push('Memory min and max are equal, so there is no room to scale memory.');
  if (policy.minCpu === policy.maxCpu) defects.push('CPU min and max are equal, so there is no room to scale CPU.');
  if (policy.scaleUpThreshold <= policy.scaleDownThreshold) {
    defects.push('Scale-up threshold is not above the scale-down threshold, so the policy has no hysteresis and can oscillate or never settle.');
  }
  if (policy.scaleUpThreshold <= 0 || policy.scaleUpThreshold > 1) defects.push('Scale-up threshold is outside the 0–1 fraction the service compares against.');
  return defects;
}

export default function AdminPolicyDetailPage() {
  const [confirm, renderConfirm] = useConfirm();
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const id = params.id as string;

  const [editing, setEditing] = useState(false);

  const policyQuery = useQuery({
    queryKey: ['admin', 'autoscaler', 'policy', id],
    queryFn: async () =>
      (await fetchJSON<ApiResponse<ScalingPolicy>>(`/admin/autoscaler/policies/${encodeURIComponent(id)}`)).data,
    retry: false,
  });

  const metricsQuery = useQuery({
    queryKey: ['admin', 'autoscaler', 'metrics'],
    queryFn: async () =>
      (await fetchJSON<ApiResponse<AutoscalerMetrics>>('/admin/autoscaler/metrics')).data,
    retry: false,
  });

  const [form, setForm] = useState<ScalingPolicy | null>(null);

  const policy = policyQuery.data;
  const metrics = metricsQuery.data;
  const metricsKnown = !metricsQuery.isPending && !metricsQuery.isError;
  const defects = useMemo(() => (policy ? policyDefects(policy) : []), [policy]);
  const formDefects = useMemo(() => (form ? policyDefects(form) : []), [form]);

  const updateMutation = useMutation({
    mutationFn: (data: Partial<ScalingPolicy>) => putJSON(`/admin/autoscaler/policies/${encodeURIComponent(id)}`, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler', 'policy', id] });
      setEditing(false);
      toast({ tone: 'success', title: 'Policy updated' });
    },
    onError: (err) => toast({ tone: 'error', title: 'Failed to update policy', message: err instanceof Error ? err.message : 'An error occurred' }),
  });

  const deleteMutation = useMutation({
    mutationFn: () => deleteJSON(`/admin/autoscaler/policies/${encodeURIComponent(id)}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler'] });
      router.push('/admin/autoscaler');
    },
    onError: (err) => toast({ tone: 'error', title: 'Failed to delete policy', message: err instanceof Error ? err.message : 'An error occurred' }),
  });

  const evaluateMutation = useMutation({
    mutationFn: (serverId: string) => postJSON(`/admin/autoscaler/evaluate/${encodeURIComponent(serverId)}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler', 'policy', id] });
      queryClient.invalidateQueries({ queryKey: ['admin', 'autoscaler', 'metrics'] });
      toast({ tone: 'success', title: 'Evaluation run', message: 'The autoscaler evaluated this workload against its thresholds.' });
    },
    onError: (err) => toast({ tone: 'error', title: 'Failed to evaluate autoscaler', message: err instanceof Error ? err.message : 'An error occurred' }),
  });

  const confirmEvaluate = (serverId: string) => {
    void (async () => {
      const ok = await confirm({
        title: 'Evaluate this workload now?',
        description:
          'This runs the scaling decision immediately rather than waiting for the next cycle. If the thresholds are met the autoscaler will change this workload\'s resource allocation, subject to the policy cooldown.',
        confirmLabel: 'Evaluate',
        danger: true,
      });
      if (ok) evaluateMutation.mutate(serverId);
    })();
  };

  const confirmDelete = () => {
    void (async () => {
      if (!policy) return;
      const ok = await confirm({
        title: 'Delete this autoscale policy?',
        description: `Automatic scaling for workload ${policy.serverId} stops: memory and CPU stay wherever they are now and nothing adjusts them under load. This cannot be undone.`,
        danger: true,
        confirmLabel: 'Delete',
      });
      if (ok) deleteMutation.mutate();
    })();
  };

  if (policyQuery.isPending) {
    return <AdminLoadingState label="Loading scaling policy…" />;
  }

  if (policyQuery.isError) {
    return (
      <AdminErrorState
        message={`This policy could not be loaded: ${policyQuery.error instanceof Error ? policyQuery.error.message : 'request failed'}. Its settings are unknown — this is not a policy with empty values.`}
        retry={() => void policyQuery.refetch()}
      />
    );
  }

  if (!policy) {
    return (
      <EmptyState
        icon={BarChart3}
        title="Policy not found"
        message="The request succeeded and reported no policy with this ID. It may have been deleted."
      />
    );
  }

  return (
    <div className="space-y-5">
      <SectionHeader
        title={`Policy: ${policy.serverId}`}
        sub={`Created ${formatDate(policy.createdAt, 'a date that was not reported')} · updated ${formatDate(policy.updatedAt, 'not reported')}`}
        backAction={() => router.push('/admin/autoscaler')}
        backLabel="Workload Autoscaling"
        status={<FreshnessBadge state={sourceState(policyQuery)} />}
        action={
          <div className="flex gap-2">
            <Btn
              tone="warning"
              onClick={() => confirmEvaluate(policy.serverId)}
              disabled={evaluateMutation.isPending}
            >
              <Play size={14} /> Evaluate Now
            </Btn>
            <Btn tone="danger" onClick={confirmDelete}>
              <Trash2 size={14} /> Delete
            </Btn>
          </div>
        }
      />

      {defects.length > 0 ? (
        <div role="status" className="ui-alert ui-alert-warning items-start gap-2">
          <AlertTriangle aria-hidden="true" className="mt-0.5 shrink-0" size={14} />
          <div>
            <p className="font-semibold">This policy cannot scale as written:</p>
            <ul className="mt-1 list-disc space-y-0.5 pl-4">
              {defects.map((d) => <li key={d}>{d}</li>)}
            </ul>
          </div>
        </div>
      ) : null}

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
          <div className="grid grid-cols-2 gap-4">
            <Field label="Server ID" value={policy.serverId} mono />
            <div>
              <p className="t-eyebrow">Status</p>
              <p className="mt-1">
                <Pill tone={policy.enabled ? 'green' : 'neutral'}>{policy.enabled ? 'Enabled' : 'Disabled'}</Pill>
              </p>
            </div>
            <Field label="Min memory" value={`${policy.minMemoryMb} MB`} />
            <Field label="Max memory" value={`${policy.maxMemoryMb} MB`} />
            <Field label="Min CPU" value={`${policy.minCpu}%`} />
            <Field label="Max CPU" value={`${policy.maxCpu}%`} />
            <Field label="Scale-up threshold" value={`${(policy.scaleUpThreshold * 100).toFixed(0)}%`} />
            <Field label="Scale-down threshold" value={`${(policy.scaleDownThreshold * 100).toFixed(0)}%`} />
            <Field label="Cooldown" value={`${policy.cooldownSeconds}s`} />
          </div>
          <p className="mt-3 text-meta leading-5 text-text-subtle">
            Current allocation and replica count are not reported by the autoscaler API, so this page can only show what the
            policy is configured to do — not what the workload is doing now. That comparison lives on the server&rsquo;s own page.
          </p>
        </Card>

        <Card>
          <CardHeader
            title="Autoscaler counters (fleet-wide)"
            icon={Zap}
            action={!metricsKnown ? <Pill tone="unknown">not loaded</Pill> : null}
          />
          <div className="grid grid-cols-3 gap-4">
            <Tile label="Scale ups" value={readout(metrics?.scaleUpEventsTotal, metricsKnown)} tone="text-ok" />
            <Tile label="Scale downs" value={readout(metrics?.scaleDownEventsTotal, metricsKnown)} tone="text-info" />
            <Tile label="Errors" value={readout(metrics?.scalingErrorsTotal, metricsKnown)} tone="text-danger" />
          </div>
          {metricsQuery.isError ? (
            <div className="mt-3">
              <AdminErrorState
                message={`Counters unavailable: ${metricsQuery.error instanceof Error ? metricsQuery.error.message : 'request failed'}. The dashes above mean no reading, not zero events.`}
                retry={() => void metricsQuery.refetch()}
              />
            </div>
          ) : null}
          <p className="mt-3 text-meta leading-5 text-text-subtle">
            These are process-wide totals for every policy this control plane has evaluated since it started, and they reset on
            restart. They are not this server&rsquo;s scaling history, and there is no per-policy metric endpoint to scope them to.
          </p>
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
            <label className="flex items-center gap-2 text-sm font-medium text-text">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
              />
              Enabled
            </label>
            <Input label="Min memory (MB)" type="number" value={String(form.minMemoryMb)} onChange={(v) => setForm({ ...form, minMemoryMb: Number(v) })} />
            <Input label="Max memory (MB)" type="number" value={String(form.maxMemoryMb)} onChange={(v) => setForm({ ...form, maxMemoryMb: Number(v) })} />
            <Input label="Min CPU (%)" type="number" value={String(form.minCpu)} onChange={(v) => setForm({ ...form, minCpu: Number(v) })} />
            <Input label="Max CPU (%)" type="number" value={String(form.maxCpu)} onChange={(v) => setForm({ ...form, maxCpu: Number(v) })} />
            <Input label="Scale-up threshold (%)" type="number" value={String(form.scaleUpThreshold * 100)} onChange={(v) => setForm({ ...form, scaleUpThreshold: Number(v) / 100 })} />
            <Input label="Scale-down threshold (%)" type="number" value={String(form.scaleDownThreshold * 100)} onChange={(v) => setForm({ ...form, scaleDownThreshold: Number(v) / 100 })} />
            <Input label="Cooldown (seconds)" type="number" value={String(form.cooldownSeconds)} onChange={(v) => setForm({ ...form, cooldownSeconds: Number(v) })} />
          </div>
          {formDefects.length > 0 ? (
            <div className="ui-alert ui-alert-warning mt-4">
              <ul className="list-disc space-y-0.5 pl-4">
                {formDefects.map((d) => <li key={d}>{d}</li>)}
              </ul>
            </div>
          ) : null}
          <ModalFooter
            onCancel={() => setEditing(false)}
            onConfirm={() => updateMutation.mutate(form)}
            confirmLabel={updateMutation.isPending ? 'Saving…' : 'Save'}
            disabled={updateMutation.isPending || !form.serverId.trim() || formDefects.length > 0}
          />
        </Modal>
      )}
      {renderConfirm()}
    </div>
  );
}

function Field({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <p className="t-eyebrow">{label}</p>
      <p className={`mt-1 text-sm text-text${mono ? ' font-mono' : ''}`}>{value}</p>
    </div>
  );
}

function Tile({ label, value, tone }: { label: string; value: string | number; tone: string }) {
  return (
    <div className="rounded-lg border border-line bg-[var(--surface-raised)] p-3 text-center">
      <p className="t-eyebrow">{label}</p>
      <p className={`t-readout mt-1 ${value === '—' ? 'text-unknown' : tone}`}>{value}</p>
    </div>
  );
}
