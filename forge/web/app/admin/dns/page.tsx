"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, ShieldCheck, ShieldAlert, Trash2, CheckCircle2, Star, RefreshCw } from "lucide-react";
import { GlobeGridIcon } from "@/components/ui/forge-icons";
import { AdminPageLayout, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, AdminTabs, AdminTable, AdminTHead, AdminTh, AdminTBody, AdminTr, AdminTd, AdminSelect, AdminLoadingState, AdminErrorState } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import {
  fetchDnsProviders,
  fetchSupportedDNSProviders,
  createDNSProvider,
  verifyDNSProvider,
  setDefaultDNSProvider,
  deleteDNSProvider,
  type DNSProvider,
  type DNSSupportedProvider,
} from "@/lib/api/dns";

export default function AdminDNSPage() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [tab, setTab] = useState<"configured" | "supported">("configured");
  const [showCreate, setShowCreate] = useState(false);
  const [createForm, setCreateForm] = useState<{ name: string; providerType: string; creds: Record<string, string> }>({ name: "", providerType: "cloudflare", creds: {} });
  // Raw text of the custom-credential escape hatch. Re-deriving the textarea's value from the
  // parsed map on every keystroke discarded any line without `=`, so the operator typed a whole
  // credential block, saw nothing change and submitted an empty `creds`.
  const [customCredsText, setCustomCredsText] = useState("");

  const configuredQuery = useQuery({ queryKey: ["dns-configured"], queryFn: fetchDnsProviders });
  const supportedQuery = useQuery({ queryKey: ["dns-supported"], queryFn: fetchSupportedDNSProviders, enabled: tab === "supported" || showCreate });

  const providers = useMemo(() => configuredQuery.data ?? [], [configuredQuery.data]);
  const supported = useMemo(() => supportedQuery.data ?? [], [supportedQuery.data]);

  const createMut = useMutation({
    mutationFn: () => createDNSProvider({ name: createForm.name.trim(), providerType: createForm.providerType, credentials: createForm.creds }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["dns-configured"] });
      setShowCreate(false);
      setCreateForm({ name: "", providerType: "cloudflare", creds: {} });
      setCustomCredsText("");
      toast({ tone: "success", title: "DNS provider created" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Create failed", message: e.message }),
  });

  const verifyMut = useMutation({
    mutationFn: async (id: string) => {
      const result = await verifyDNSProvider(id);
      if (!result.ok) throw new Error("The server reported the provider verification did not complete.");
      return result;
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["dns-configured"] }); toast({ tone: "success", title: "Provider verified" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Verify failed", message: e.message }),
  });

  const defaultMut = useMutation({
    mutationFn: async (id: string) => {
      const result = await setDefaultDNSProvider(id);
      if (!result.ok) throw new Error("The server reported the default provider was not set.");
      return result;
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["dns-configured"] }); toast({ tone: "success", title: "Default provider set" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Set default failed", message: e.message }),
  });

  const deleteMut = useMutation({
    mutationFn: async (id: string) => {
      const result = await deleteDNSProvider(id);
      if (!result.ok) throw new Error("The server reported the DNS provider was not deleted.");
      return result;
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["dns-configured"] }); toast({ tone: "success", title: "Provider deleted" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  const selectedSupported = supported.find((s) => s.type === createForm.providerType);

  // The custom-credential textarea keeps its own raw text so a half-typed line is not discarded,
  // and reports what it could not parse instead of silently dropping it.
  const customCredsLines = customCredsText.split("\n").map((l) => l.trim()).filter(Boolean);
  const customCredsRecognised = customCredsLines.filter((l) => l.indexOf("=") > 0).length;
  const customCredsIgnored = customCredsLines.length - customCredsRecognised;

  const applyCustomCreds = (text: string) => {
    setCustomCredsText(text);
    const map: Record<string, string> = {};
    text.split("\n").forEach((line) => {
      const idx = line.indexOf("=");
      if (idx > 0) map[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
    });
    setCreateForm((prev) => ({ ...prev, creds: map }));
  };

  const missingRequiredCreds = selectedSupported
    ? selectedSupported.credentialFields.filter((f) => f.required && !(createForm.creds[f.key] ?? "").trim())
    : [];

  const rowBusy = (id: string, mutation: { isPending: boolean; variables: unknown }) =>
    mutation.isPending && mutation.variables === id;

  return (
    <AdminPageLayout>
      <SectionHeader
        status={<FreshnessBadge state={sourceState(configuredQuery)} />}
        action={<Btn onClick={() => setShowCreate(true)} className="bg-[var(--brand)] hover:bg-[color-mix(in_srgb,var(--brand)_90%,transparent)] text-white"><Plus size={14} /> Add Provider</Btn>}
      />

      <AdminTabs tabs={[{ id: "configured", label: "Configured" }, { id: "supported", label: "Supported Types" }]} active={tab} onChange={(v) => setTab(v as typeof tab)} />

      {tab === "configured" && (
        <Card>
          <CardHeader title="Configured Providers" icon={GlobeGridIcon} action={<Btn size="sm" tone="ghost" onClick={() => void configuredQuery.refetch()}><RefreshCw size={12} /> Refresh</Btn>} />
          {configuredQuery.isLoading ? <div className="p-4"><AdminLoadingState label="Loading configured providers…" /></div>
            : configuredQuery.isError ? <div className="p-4"><AdminErrorState message={configuredQuery.error instanceof Error ? configuredQuery.error.message : "Could not load DNS providers"} retry={() => void configuredQuery.refetch()} /></div>
            : providers.length === 0 ? <EmptyState icon={GlobeGridIcon} title="No DNS providers" message="No DNS providers configured. Add a provider to enable automatic DNS-01 challenges." />
            : (
              <AdminTable label="Configured DNS providers">
                <AdminTHead><AdminTh>Name</AdminTh><AdminTh>Type</AdminTh><AdminTh>Default</AdminTh><AdminTh>Verified</AdminTh><AdminTh>Created</AdminTh><AdminTh></AdminTh></AdminTHead>
                <AdminTBody>
                  {providers.map((p: DNSProvider) => (
                    <AdminTr key={p.id}>
                      <AdminTd className="font-medium text-text">{p.name}</AdminTd>
                      <AdminTd className="font-mono text-xs text-text-subtle">{p.providerType ?? p.provider ?? "—"}</AdminTd>
                      <AdminTd>{p.isDefault ? <Pill tone="green"><Star size={10} className="mr-1" /> Default</Pill> : <Pill tone="neutral">Not default</Pill>}</AdminTd>
                      <AdminTd>{p.verified ? <Pill tone="green"><ShieldCheck size={10} className="mr-1" /> Verified</Pill> : <Pill tone="yellow"><ShieldAlert size={10} className="mr-1" /> Not verified</Pill>}</AdminTd>
                      <AdminTd className="text-xs text-text-subtle">{p.createdAt ? new Date(p.createdAt).toLocaleString() : "Not reported"}</AdminTd>
                      <AdminTd>
                        <div className="flex justify-end gap-1.5">
                          {!p.verified && <Btn size="sm" tone="ghost" disabled={rowBusy(p.id, verifyMut)} loading={rowBusy(p.id, verifyMut)} onClick={() => verifyMut.mutate(p.id)} className="border border-[color-mix(in_srgb,var(--brand)_20%,transparent)]"><CheckCircle2 size={12} /> Verify</Btn>}
                          {!p.isDefault && <Btn size="sm" tone="ghost" disabled={rowBusy(p.id, defaultMut)} loading={rowBusy(p.id, defaultMut)} onClick={() => defaultMut.mutate(p.id)}><Star size={12} /> Set Default</Btn>}
                          <Btn ariaLabel={`Delete DNS provider ${p.name}`} size="sm" tone="danger" disabled={rowBusy(p.id, deleteMut)} loading={rowBusy(p.id, deleteMut)} onClick={() => { void (async () => { if (await confirm({ title: `Delete DNS provider ${p.name}?`, description: "The provider and its encrypted credentials will be removed. Certificates that still depend on it cannot renew via DNS-01. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(p.id); })(); }}><Trash2 size={12} /></Btn>
                        </div>
                      </AdminTd>
                    </AdminTr>
                  ))}
                </AdminTBody>
              </AdminTable>
            )}
        </Card>
      )}

      {tab === "supported" && (
        <Card>
          <CardHeader title="Supported Provider Types" icon={ShieldCheck} />
          {supportedQuery.isLoading ? <div className="p-4"><AdminLoadingState label="Loading supported providers…" /></div>
            : supportedQuery.isError ? <div className="p-4"><AdminErrorState message={(supportedQuery.error as Error).message} retry={() => void supportedQuery.refetch()} /></div>
            : (
              <div className="grid gap-3 p-4 sm:grid-cols-2">
                {supported.map((sp: DNSSupportedProvider) => (
                  <div key={sp.type} className="rounded-xl border border-line bg-overlay-subtle p-4 transition hover:border-[color-mix(in_srgb,var(--brand)_30%,transparent)]">
                    <h2 className="text-sm font-semibold text-text">{sp.name} <code className="font-mono text-xs text-text-subtle">({sp.type})</code></h2>
                    <p className="mt-1 text-xs text-text-subtle">{sp.description}</p>
                    {sp.credentialFields?.length > 0 && (
                      <div className="mt-2 flex flex-wrap gap-1.5">
                        {sp.credentialFields.map((f) => (
                          <span key={f.key} className="rounded-full border border-line bg-overlay-subtle px-2 py-0.5 font-mono text-xs text-text-muted">
                            {f.key}{f.required ? " (required)" : ""}
                            {f.description ? <span className="ml-1 text-text-subtle">— {f.description}</span> : null}
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            )}
        </Card>
      )}

      {showCreate && (
        <Modal title="Add DNS Provider" onClose={() => setShowCreate(false)} wide>
          <div className="space-y-4">
            <Input label="Name" value={createForm.name} onChange={(v) => setCreateForm({ ...createForm, name: v })} placeholder="My Cloudflare" />
            <AdminSelect label="Provider Type" value={createForm.providerType} onChange={(v) => setCreateForm({ ...createForm, providerType: v, creds: {} })} options={(supported ?? []).map((s) => ({ value: s.type, label: `${s.name} (${s.type})` }))} />
            <p className="text-xs text-text-subtle">Credentials are stored encrypted and used only for DNS-01 challenges. Secret fields are masked as you type; once saved, a credential is never shown again.</p>
            {selectedSupported ? (
              <div className="space-y-3 rounded-lg border border-line bg-overlay-subtle p-4">
                <h2 className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Credentials for {selectedSupported.name}</h2>
                {selectedSupported.credentialFields.map((field) => {
                  // The backend classifies each field (`forge/api/internal/services/dns/service.go`
                  // CredentialField.Type: password | text | number). Forwarding it is what masks
                  // API tokens, secret keys and account keys; using it only to pick a font left
                  // every secret readable on screen while the copy above promised safety.
                  const secret = field.type === "password";
                  return (
                    <Input
                      key={field.key}
                      autoComplete={secret ? "new-password" : "off"}
                      label={`${field.label}${field.required ? " *" : ""}`}
                      mono={!secret}
                      onChange={(v) => setCreateForm({ ...createForm, creds: { ...createForm.creds, [field.key]: v } })}
                      placeholder={secret ? "••••••••" : field.key}
                      required={field.required}
                      type={field.type || "text"}
                      value={createForm.creds[field.key] ?? ""}
                    />
                  );
                })}
              </div>
            ) : (
              <p className="text-xs text-text-subtle">Select a supported type to see credential fields, or enter arbitrary keys below.</p>
            )}
            {!selectedSupported && (
              <div className="space-y-2">
                <label className="ui-label" htmlFor="dns-custom-creds">
                  Custom credentials (key=value, one per line) for unknown provider types
                </label>
                <textarea
                  aria-describedby="dns-custom-creds-hint"
                  autoComplete="off"
                  className="w-full rounded-lg border border-line bg-overlay-subtle p-3 font-mono text-xs text-text"
                  id="dns-custom-creds"
                  onChange={(e) => applyCustomCreds(e.target.value)}
                  placeholder="CF_DNS_API_TOKEN=xxxxx"
                  rows={4}
                  spellCheck={false}
                  value={customCredsText}
                />
                <p className="text-xs text-text-subtle" id="dns-custom-creds-hint">
                  {customCredsLines.length === 0
                    ? "No keys entered."
                    : customCredsIgnored > 0
                      ? `${customCredsRecognised} key(s) recognised · ${customCredsIgnored} line(s) ignored — each line must be key=value`
                      : `${customCredsRecognised} key(s) recognised`}
                </p>
              </div>
            )}
            {createMut.error && <p className="text-sm text-danger">{(createMut.error as Error).message}</p>}
          </div>
          <ModalFooter onCancel={() => setShowCreate(false)} onConfirm={() => createMut.mutate()} disabled={!createForm.name.trim() || !createForm.providerType || createMut.isPending || missingRequiredCreds.length > 0} confirmLabel={missingRequiredCreds.length > 0 ? `Missing ${missingRequiredCreds[0].label}` : createMut.isPending ? "Creating…" : "Create Provider"} />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
