'use client';

import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Fingerprint, Eye, EyeOff, Save, MessageCircle, Gamepad2, KeyRound, Plug } from 'lucide-react';
import { fetchJSON, putJSON, type SocialProvider } from '@/lib/api';
import { AdminErrorState, AdminLoadingState, AdminPageHeader, AdminPageLayout, Btn, Card, CardHeader, EmptyState, Input } from '@/components/admin/admin-ui';
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
      {/* Title, subtitle and the Fingerprint glyph come from admin-registry.ts:213.
          The page used to restate the registry sentence and append its own
          provider list + disclaimer; the disclaimer is now stated where it
          matters — next to the action it qualifies. */}
      <AdminPageHeader />
      <Card>
        <CardHeader title={query.isSuccess ? `${providers.length} providers` : "Providers"} icon={Fingerprint} />
        <p className="ui-hint border-b border-line px-4 pb-3">
          Saving writes the credentials you enter. It does not contact the provider, so nothing here claims a working sign-in until a user completes that flow.
        </p>
        {query.isLoading ? (
          <div className="p-4"><AdminLoadingState label="Loading providers…" /></div>
        ) : query.isError ? (
          <div className="p-4"><AdminErrorState message={query.error instanceof Error ? query.error.message : 'Social providers could not be loaded.'} retry={() => void query.refetch()} /></div>
        ) : providers.length === 0 ? (
          <EmptyState icon={Fingerprint} title="No providers" message="No social providers are configured. Enable one below to offer SSO sign-in." />
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
  const IconMap: Record<string, typeof Fingerprint> = { discord: MessageCircle, steam: Gamepad2, authentik: KeyRound };
  const isAuthentik = provider.name === 'authentik';
  const isSteam = provider.name === 'steam';
  const secretLabel = isSteam ? 'Steam Web API Key' : 'Client Secret';

  return (
    <div className="p-4">
      <div className="mb-3 flex items-center gap-3">
        {(() => { const ProviderIcon = IconMap[provider.name] ?? Plug; return <ProviderIcon size={18} className="text-text-subtle" />; })()}
        <div className="flex-1">
          <p className="font-semibold text-text">{provider.displayName}</p>
          <p className="t-meta">Provider key: {provider.name}</p>
        </div>
        <label className="flex cursor-pointer items-center gap-2 text-sm">
          <input type="checkbox" checked={enabled} onChange={(event) => onChange({ enabled: event.target.checked })} className="accent-[var(--brand)]" />
          <span className="text-text">Enabled</span>
        </label>
      </div>
      <p className="ui-hint mb-3">
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
