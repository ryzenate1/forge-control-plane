"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Globe, Plus, ShieldCheck, ShieldAlert, Trash2, CheckCircle2, Star, RefreshCw } from "lucide-react";
import { AdminPageLayout, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, AdminTabs, AdminTable, AdminTHead, AdminTh, AdminTBody, AdminTr, AdminTd, AdminSelect } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
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

  return (
    <AdminPageLayout>
      <SectionHeader
        title="DNS Providers"
        sub="DNS providers used for automatic DNS-01 challenges. Verify a provider, then set a default for issuance."
        action={<Btn onClick={() => setShowCreate(true)} className="bg-[var(--brand)] hover:bg-[var(--brand)]/90 text-white"><Plus size={14} /> Add Provider</Btn>}
      />

      <AdminTabs tabs={[{ id: "configured", label: "Configured" }, { id: "supported", label: "Supported Types" }]} active={tab} onChange={(v) => setTab(v as typeof tab)} />

      {tab === "configured" && (
        <Card>
          <CardHeader title="Configured Providers" icon={Globe} action={<Btn size="sm" tone="ghost" onClick={() => void configuredQuery.refetch()}><RefreshCw size={12} /> Refresh</Btn>} />
          {configuredQuery.isLoading ? <div className="p-8 text-center text-sm text-slate-400">Loading configured providers via fetchDnsProviders…</div>
            : configuredQuery.isError ? <div className="p-4"><div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">Failed: {(configuredQuery.error as Error).message} <Btn size="sm" tone="ghost" onClick={() => void configuredQuery.refetch()} className="ml-2">Retry</Btn></div></div>
            : providers.length === 0 ? <EmptyState icon={Globe} message="No DNS providers configured. Add a provider to enable automatic DNS-01 challenges." />
            : (
              <AdminTable label="Configured DNS providers">
                <AdminTHead><AdminTh>Name</AdminTh><AdminTh>Type</AdminTh><AdminTh>Default</AdminTh><AdminTh>Verified</AdminTh><AdminTh>Created</AdminTh><AdminTh></AdminTh></AdminTHead>
                <AdminTBody>
                  {providers.map((p: DNSProvider) => (
                    <AdminTr key={p.id}>
                      <AdminTd className="font-medium text-slate-200">{p.name}</AdminTd>
                      <AdminTd className="font-mono text-xs text-slate-400">{p.providerType ?? p.provider}</AdminTd>
                      <AdminTd>{p.isDefault ? <Pill tone="green"><Star size={10} className="mr-1" /> Default</Pill> : <Pill tone="neutral">—</Pill>}</AdminTd>
                      <AdminTd>{p.verified ? <Pill tone="green"><ShieldCheck size={10} className="mr-1" /> Verified</Pill> : <Pill tone="yellow"><ShieldAlert size={10} className="mr-1" /> Unverified</Pill>}</AdminTd>
                      <AdminTd className="text-xs text-slate-400">{p.createdAt ? new Date(p.createdAt).toLocaleString() : "—"}</AdminTd>
                      <AdminTd>
                        <div className="flex justify-end gap-1.5">
                          {!p.verified && <Btn size="sm" tone="ghost" disabled={verifyMut.isPending} onClick={() => verifyMut.mutate(p.id)} className="border border-[var(--brand)]/20"><CheckCircle2 size={12} /> Verify</Btn>}
                          {!p.isDefault && <Btn size="sm" tone="ghost" disabled={defaultMut.isPending} onClick={() => defaultMut.mutate(p.id)}><Star size={12} /> Set Default</Btn>}
                          <Btn size="sm" tone="danger" disabled={deleteMut.isPending} onClick={() => { void (async () => { if (await confirm({ title: `Delete DNS provider ${p.name}?`, description: "The provider and its encrypted credentials will be removed. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(p.id); })(); }}><Trash2 size={12} /></Btn>
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
          {supportedQuery.isLoading ? <div className="p-8 text-center text-sm text-slate-400">Loading supported providers…</div>
            : supportedQuery.isError ? <div className="p-4 text-sm text-red-300">Failed: {(supportedQuery.error as Error).message}</div>
            : (
              <div className="grid gap-3 p-4 sm:grid-cols-2">
                {supported.map((sp: DNSSupportedProvider) => (
                  <div key={sp.type} className="rounded-xl border border-white/[0.06] bg-[var(--surface)] p-4 hover:border-[var(--brand)]/30 transition">
                    <h4 className="text-sm font-semibold text-slate-100">{sp.name} <code className="font-mono text-xs text-slate-400">({sp.type})</code></h4>
                    <p className="text-xs text-slate-400 mt-1">{sp.description}</p>
                    {sp.credentialFields?.length > 0 && (
                      <div className="mt-2 flex flex-wrap gap-1.5">
                        {sp.credentialFields.map((f) => (
                          <span key={f.key} className="rounded-full border border-white/10 bg-white/[0.04] px-2 py-0.5 text-[11px] font-mono text-slate-300" title={f.description}>{f.key}{f.required ? "*" : ""}</span>
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
            <p className="text-xs text-slate-400">Credentials are stored encrypted and used only for DNS-01 challenges.</p>
            {selectedSupported ? (
              <div className="space-y-3 rounded-lg border border-white/[0.06] bg-[var(--surface)] p-4">
                <h5 className="text-xs font-semibold uppercase tracking-widest text-slate-400">Credentials for {selectedSupported.name}</h5>
                {selectedSupported.credentialFields.map((field) => (
                  <Input
                    key={field.key}
                    label={`${field.label}${field.required ? " *" : ""}`}
                    value={createForm.creds[field.key] ?? ""}
                    onChange={(v) => setCreateForm({ ...createForm, creds: { ...createForm.creds, [field.key]: v } })}
                    placeholder={field.key}
                    mono={field.type !== "password"}
                  />
                ))}
              </div>
            ) : (
              <p className="text-xs text-slate-500">Select a supported type to see credential fields, or type arbitrary keys.</p>
            )}
            {!selectedSupported && (
              <div className="space-y-2">
                <p className="text-xs text-slate-400">Custom credentials (key=value, one per line) for unknown provider types</p>
                <textarea
                  className="w-full rounded-lg border border-white/10 bg-[var(--surface)] p-3 font-mono text-xs text-slate-100"
                  rows={4}
                  placeholder="CF_DNS_API_TOKEN=xxxxx"
                  value={Object.entries(createForm.creds).map(([k,v]) => `${k}=${v}`).join("\n")}
                  onChange={(e) => {
                    const map: Record<string,string> = {};
                    e.target.value.split("\n").forEach((line) => {
                      const idx = line.indexOf("=");
                      if (idx > 0) map[line.slice(0, idx).trim()] = line.slice(idx+1).trim();
                    });
                    setCreateForm({ ...createForm, creds: map });
                  }}
                />
              </div>
            )}
            {createMut.error && <p className="text-sm text-red-300">{(createMut.error as Error).message}</p>}
          </div>
          <ModalFooter onCancel={() => setShowCreate(false)} onConfirm={() => createMut.mutate()} disabled={!createForm.name.trim() || !createForm.providerType || createMut.isPending} confirmLabel={createMut.isPending ? "Creating…" : "Create Provider"} />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
