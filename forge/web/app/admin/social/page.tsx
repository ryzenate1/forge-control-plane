'use client';

import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Globe, Eye, EyeOff, Save } from 'lucide-react';
import { fetchJSON, putJSON, type SocialProvider } from '@/lib/api';
import { AdminPageLayout, Btn, Card, CardHeader, EmptyState, Input, SectionHeader, AdminLoadingState, AdminErrorState } from '@/components/admin/admin-ui';
import { Alert } from '@/components/ui/primitives';
import { useToast } from '@/components/ui/toast';

type ProviderConfiguration = {
  enabled: boolean;
  clientId: string;
  clientSecret: string;
  issuerUrl: string;
};

type ProviderUpdate = {
  enabled: boolean;
  clientId?: string;
  clientSecret?: string;
  issuerUrl?: string;
};

export default function SocialProvidersPage() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const query = useQuery<SocialProvider[]>({
    queryKey: ['admin-social-providers'],
    queryFn: () => fetchJSON<SocialProvider[]>('/admin/social/providers'),
  });
  const providers = useMemo(() => query.data ?? [], [query.data]);
  const [local, setLocal] = useState<Record<string, ProviderConfiguration>>({});

  useEffect(() => {
    if (providers.length === 0) return;
    setLocal((current) => {
      const next = { ...current };
      for (const provider of providers) {
        next[provider.name] = {
          enabled: provider.enabled,
          clientId: provider.clientId,
          clientSecret: next[provider.name]?.clientSecret ?? '',
          issuerUrl: provider.issuerUrl ?? '',
        };
      }
      return next;
    });
  }, [providers]);

  const saveMut = useMutation({
    mutationFn: async (provider: SocialProvider) => {
      const cfg = local[provider.name];
      if (!cfg) throw new Error(`No editable configuration found for ${provider.name}.`);
      const body: ProviderUpdate = { enabled: cfg.enabled };
      if (cfg.clientId !== provider.clientId) body.clientId = cfg.clientId;
      if (cfg.issuerUrl !== (provider.issuerUrl ?? '')) body.issuerUrl = cfg.issuerUrl;
      if (cfg.clientSecret) body.clientSecret = cfg.clientSecret;
      await putJSON(`/admin/social/providers/${provider.id}`, body);
    },
    onSuccess: () => {
      toast({ tone: 'success', title: 'Provider saved', message: 'The provider settings were updated. Credentials are not verified until a user completes the provider sign-in flow.' });
      qc.invalidateQueries({ queryKey: ['admin-social-providers'] });
    },
    onError: (err) => toast({ tone: 'error', title: 'Failed to save provider', message: err instanceof Error ? err.message : 'An error occurred' }),
  });

  return (
    <AdminPageLayout>
      <SectionHeader title="Single Sign-On" sub="Social and enterprise SSO providers." />
      <Card>
        <CardHeader title={`${providers.length} providers`} icon={Globe} />
        {query.isLoading ? (
          <AdminLoadingState label="Loading providers…" />
        ) : query.isError ? (
          <div className="p-4"><AdminErrorState message={query.error instanceof Error ? query.error.message : "Failed to load social providers."} retry={() => void query.refetch()} /></div>
        ) : providers.length === 0 ? (
          <EmptyState icon={Globe} message="No social providers configured." />
        ) : (
          <div className="divide-y divide-line">
            {providers.map((provider) => {
              const cfg = local[provider.name] ?? { enabled: provider.enabled, clientId: provider.clientId, clientSecret: '', issuerUrl: provider.issuerUrl ?? '' };
              return (
                <ProviderRow
                  key={provider.id}
                  provider={provider}
                  {...cfg}
                  onChange={(change) => setLocal((previous) => ({ ...previous, [provider.name]: { ...cfg, ...change } }))}
                  onSave={() => saveMut.mutate(provider)}
                  saving={saveMut.isPending && saveMut.variables?.id === provider.id}
                />
              );
            })}
          </div>
        )}
        {saveMut.isError ? (
          <div className="border-t border-line p-4">
            <Alert tone="error" title="Could not save provider">{saveMut.error instanceof Error ? saveMut.error.message : 'Try again after reviewing the provider credentials.'}</Alert>
          </div>
        ) : null}
      </Card>
    </AdminPageLayout>
  );
}

function ProviderRow({
  provider, enabled, clientId, clientSecret, issuerUrl, onChange, onSave, saving,
}: {
  provider: SocialProvider;
  enabled: boolean;
  clientId: string;
  clientSecret: string;
  issuerUrl: string;
  onChange: (change: Partial<ProviderConfiguration>) => void;
  onSave: () => void;
  saving: boolean;
}) {
  const [showSecret, setShowSecret] = useState(false);
  const iconMap: Record<string, string> = { discord: '💬', steam: '🎮', authentik: '🔑' };
  const isAuthentik = provider.name === 'authentik';
  const isSteam = provider.name === 'steam';
  const secretLabel = isSteam ? 'Steam Web API Key' : 'Client Secret';

  return (
    <div className="p-4">
      <div className="mb-3 flex items-center gap-3">
        <span className="text-lg">{iconMap[provider.name] ?? '🔌'}</span>
        <div className="flex-1">
          <p className="font-semibold text-text">{provider.displayName}</p>
          <p className="text-xs text-text-muted">Provider key: {provider.name}</p>
        </div>
        <label className="flex cursor-pointer items-center gap-2 text-sm">
          <input type="checkbox" checked={enabled} onChange={(event) => onChange({ enabled: event.target.checked })} className="accent-[var(--brand)]" />
          <span className="text-text">Enabled</span>
        </label>
      </div>
      <p className="mb-3 text-xs text-text-muted">
        {isSteam
          ? 'Steam uses OpenID for sign-in and the Web API key only to retrieve the signed-in player profile.'
          : isAuthentik
            ? 'Enter the Authentik installation URL and the OAuth application credentials.'
            : 'Enter the OAuth application credentials registered with Discord.'}
      </p>
      <div className="grid gap-3 md:grid-cols-2">
        {isAuthentik ? <Input label="Authentik Issuer URL" value={issuerUrl} onChange={(value) => onChange({ issuerUrl: value })} placeholder="https://auth.example.com" /> : null}
        {!isSteam ? <Input label="Client ID" value={clientId} onChange={(value) => onChange({ clientId: value })} placeholder="OAuth client ID" /> : null}
        <div className="relative">
          <Input
            label={secretLabel}
            value={clientSecret}
            onChange={(value) => onChange({ clientSecret: value })}
            type={showSecret ? 'text' : 'password'}
            placeholder={provider.hasClientSecret ? 'Stored securely — enter a new value to replace it' : secretLabel}
            autoComplete="off"
          />
          <button type="button" aria-label={showSecret ? 'Hide secret' : 'Show secret'} className="absolute right-2 top-7 text-text-subtle hover:text-text" onClick={() => setShowSecret(!showSecret)}>
            {showSecret ? <EyeOff size={14} /> : <Eye size={14} />}
          </button>
        </div>
      </div>
      <div className="mt-3 flex justify-end">
        <Btn tone="primary" onClick={onSave} disabled={saving}>
          <Save size={14} /> {saving ? 'Saving...' : 'Save'}
        </Btn>
      </div>
    </div>
  );
}
