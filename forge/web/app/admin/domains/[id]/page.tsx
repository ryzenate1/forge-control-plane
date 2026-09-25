"use client";

import { useParams, useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, Globe, Shield, Save, Trash2, ExternalLink, Plus, ArrowUpRight } from "lucide-react";
import {
  fetchDomainSecurityHeaders,
  createDomainSecurityHeaders,
  updateDomainSecurityHeaders,
  deleteDomainSecurityHeaders,
  type SecurityHeaderConfig,
  type UpdateSecurityHeadersInput,
} from "@/lib/api/security";
import {
  fetchRedirects,
  createRedirect,
  updateRedirect,
  deleteRedirect,
  type RedirectRule,
  type CreateRedirectInput,
} from "@/lib/api/redirects";
import {
  fetchAdminProxyDomain,
  type ProxyDomain as ApiProxyDomain,
} from "@/lib/api/proxy-domains";
import {
  AdminPageLayout,
  AdminPageHeader,
  Card,
  CardHeader,
  Btn,
  Pill,
  Input,
  Modal,
  ModalFooter,
  AdminTabs,
  EmptyState,
} from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { OfflineBanner } from "@/components/shared/states-offline";

type ProxyDomain = {
  id: string;
  hostname: string;
  serviceId?: string;
  serviceType?: string;
  https: boolean;
  port: number;
  certType?: string;
  path?: string;
  createdAt?: string;
};

export default function AdminDomainDetailPage() {
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const id = decodeURIComponent(params.id as string);

  const domainQuery = useQuery({
    queryKey: ["admin-proxy-domain", id],
    queryFn: () => fetchAdminProxyDomain(id) as Promise<ProxyDomain & ApiProxyDomain>,
  });

  const headersQuery = useQuery({
    queryKey: ["domain-security-headers-detail", id],
    queryFn: () => fetchDomainSecurityHeaders(id),
  });

  const domain = domainQuery.data;
  const existing = headersQuery.data;

  const [form, setForm] = useState<UpdateSecurityHeadersInput>({
    hstsEnabled: true,
    hstsMaxAge: 63072000,
    hstsIncludeSubdomains: true,
    hstsPreload: false,
    xFrameOptions: "DENY",
    xContentTypeOptions: "nosniff",
    referrerPolicy: "strict-origin-when-cross-origin",
    cspEnabled: false,
    cspPolicy: "",
    permissionsPolicy: "",
    customHeaders: {},
  });

  useEffect(() => {
    if (existing) {
      setForm({
        hstsEnabled: existing.hstsEnabled,
        hstsMaxAge: existing.hstsMaxAge,
        hstsIncludeSubdomains: existing.hstsIncludeSubdomains,
        hstsPreload: existing.hstsPreload,
        xFrameOptions: existing.xFrameOptions ?? "DENY",
        xContentTypeOptions: existing.xContentTypeOptions ?? "nosniff",
        referrerPolicy: existing.referrerPolicy ?? "strict-origin-when-cross-origin",
        cspEnabled: existing.cspEnabled,
        cspPolicy: existing.cspPolicy ?? "",
        permissionsPolicy: existing.permissionsPolicy ?? "",
        customHeaders: existing.customHeaders ?? {},
      });
    }
  }, [existing]);

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (existing?.id) {
        return updateDomainSecurityHeaders(id, existing.id, form);
      }
      return createDomainSecurityHeaders(id, form);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["domain-security-headers-detail", id] });
      queryClient.invalidateQueries({ queryKey: ["domain-security-headers", id] });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: () => {
      if (!existing?.id) return Promise.resolve();
      return deleteDomainSecurityHeaders(id, existing.id);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["domain-security-headers-detail", id] });
      queryClient.invalidateQueries({ queryKey: ["domain-security-headers", id] });
    },
  });

  const isLoading = domainQuery.isLoading || headersQuery.isLoading;

  if (isLoading) {
    return (
      <AdminPageLayout>
        <div className="p-8 text-center text-sm text-slate-300">Loading domain…</div>
      </AdminPageLayout>
    );
  }

  if (domainQuery.isError) {
    return (
      <AdminPageLayout>
        <AdminPageHeader
          title="Domain not found"
          description={domainQuery.error instanceof Error ? domainQuery.error.message : "Failed to load domain"}
          backAction={() => router.push("/admin/domains")}
          backLabel="Domains"
        />
        <Card className="p-6 text-center text-sm text-amber-300">
          Domain <code className="font-mono">{id}</code> could not be loaded. It may have been
          deleted or the API at <code className="font-mono">GET /api/v1/domains/:id</code> is
          unreachable.
        </Card>
      </AdminPageLayout>
    );
  }

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => window.location.reload()} />
      <AdminPageHeader
        title={domain?.hostname ?? id}
        description={`Proxy domain ${id} — per-domain security headers, certificate, and gateway routing`}
        backAction={() => router.push("/admin/domains")}
        backLabel="Domains"
        action={
          <div className="flex gap-2">
            <Btn tone="ghost" onClick={() => router.push("/admin/security")}>
              <Shield size={14} /> Security Overview
            </Btn>
            {domain?.hostname && (
              <a
                href={`https://${domain.hostname}`}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 rounded-lg bg-white/[0.06] px-3 py-2 text-sm font-medium text-slate-200 hover:bg-white/[0.1]"
              >
                <ExternalLink size={14} /> Open
              </a>
            )}
          </div>
        }
      />

      <Card>
        <CardHeader title="Domain" icon={Globe} />
        <div className="grid gap-3 p-4 text-sm sm:grid-cols-2">
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-400">ID</p>
            <p className="mt-1 font-mono text-xs text-slate-200">{domain?.id}</p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-400">Hostname</p>
            <p className="mt-1 font-mono text-xs text-slate-200">{domain?.hostname}</p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-400">Service</p>
            <p className="mt-1 text-xs text-slate-300">
              {domain?.serviceType ?? "—"} / {domain?.serviceId ?? "—"}
            </p>
          </div>
          <div>
            <p className="text-[10px] font-semibold uppercase tracking-widest text-slate-400">HTTPS</p>
            <Pill tone={domain?.https ? "green" : "neutral"}>{domain?.https ? "enabled" : "off"}</Pill>
          </div>
        </div>
      </Card>

      <Card>
        <CardHeader title="Security Headers" icon={Shield} />
        <div className="p-4">
          <p className="mb-4 text-xs leading-5 text-slate-400">
            Stored in <code className="rounded bg-white/[0.06] px-1 py-0.5 font-mono">security_headers</code>{" "}
            (see <code className="font-mono">store_security_headers.go:13</code>) and exposed via{" "}
            <code className="rounded bg-white/[0.06] px-1 py-0.5 font-mono">
              /api/v1/domains/:domainId/security-headers
            </code>{" "}
            and{" "}
            <code className="rounded bg-white/[0.06] px-1 py-0.5 font-mono">
              /api/v1/servers/:serverId/proxy-domains/:domainId/security-headers
            </code>
            . When set, the gateway (Caddy) injects them as response headers on the per-domain
            route — see <code className="font-mono">caddy_proxy.go:777 buildDomainRoutes</code>.
            Global defaults come from <code className="font-mono">middleware_security.go:31</code>.
          </p>

          {headersQuery.isError && (
            <p className="mb-3 text-xs text-amber-300">
              Failed to load headers: {(headersQuery.error as Error).message}
            </p>
          )}

          {!existing && (
            <p className="mb-3 rounded-lg border border-amber-500/20 bg-amber-500/10 px-3 py-2 text-xs text-amber-200">
              No per-domain override yet — global middleware headers apply. Saving creates a row in{" "}
              <code className="font-mono">security_headers</code>.
            </p>
          )}

          <div className="grid gap-4 sm:grid-cols-2">
            <label className="flex items-center gap-2 text-sm text-slate-200">
              <input
                type="checkbox"
                checked={!!form.hstsEnabled}
                onChange={(e) => setForm({ ...form, hstsEnabled: e.target.checked })}
                className="rounded border-white/10 bg-white/5"
              />
              HSTS Enabled
            </label>
            <div>
              <label className="block text-xs font-medium text-slate-300">HSTS Max-Age</label>
              <input
                type="number"
                value={form.hstsMaxAge ?? 63072000}
                onChange={(e) => setForm({ ...form, hstsMaxAge: Number(e.target.value) })}
                className="mt-1 w-full rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 py-2 text-sm text-slate-100 outline-none"
              />
            </div>
            <label className="flex items-center gap-2 text-sm text-slate-200">
              <input
                type="checkbox"
                checked={!!form.hstsIncludeSubdomains}
                onChange={(e) => setForm({ ...form, hstsIncludeSubdomains: e.target.checked })}
                className="rounded border-white/10 bg-white/5"
              />
              HSTS Include Subdomains
            </label>
            <label className="flex items-center gap-2 text-sm text-slate-200">
              <input
                type="checkbox"
                checked={!!form.hstsPreload}
                onChange={(e) => setForm({ ...form, hstsPreload: e.target.checked })}
                className="rounded border-white/10 bg-white/5"
              />
              HSTS Preload
            </label>
            <div>
              <label className="block text-xs font-medium text-slate-300">X-Frame-Options</label>
              <select
                value={form.xFrameOptions ?? "DENY"}
                onChange={(e) => setForm({ ...form, xFrameOptions: e.target.value })}
                className="mt-1 w-full rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 py-2 text-sm text-slate-100"
              >
                <option value="DENY">DENY</option>
                <option value="SAMEORIGIN">SAMEORIGIN</option>
                <option value="">(none)</option>
              </select>
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-300">X-Content-Type-Options</label>
              <select
                value={form.xContentTypeOptions ?? "nosniff"}
                onChange={(e) => setForm({ ...form, xContentTypeOptions: e.target.value })}
                className="mt-1 w-full rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 py-2 text-sm text-slate-100"
              >
                <option value="nosniff">nosniff</option>
                <option value="">(none)</option>
              </select>
            </div>
            <div className="sm:col-span-2">
              <label className="block text-xs font-medium text-slate-300">Referrer-Policy</label>
              <input
                value={form.referrerPolicy ?? ""}
                onChange={(e) => setForm({ ...form, referrerPolicy: e.target.value })}
                placeholder="strict-origin-when-cross-origin"
                className="mt-1 w-full rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 py-2 font-mono text-sm text-slate-100"
              />
            </div>
            <label className="flex items-center gap-2 text-sm text-slate-200">
              <input
                type="checkbox"
                checked={!!form.cspEnabled}
                onChange={(e) => setForm({ ...form, cspEnabled: e.target.checked })}
                className="rounded border-white/10 bg-white/5"
              />
              CSP Enabled
            </label>
            <div className="sm:col-span-2">
              <label className="block text-xs font-medium text-slate-300">CSP Policy</label>
              <textarea
                value={form.cspPolicy ?? ""}
                onChange={(e) => setForm({ ...form, cspPolicy: e.target.value })}
                placeholder="default-src 'self'; ..."
                rows={3}
                className="mt-1 w-full rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 py-2 font-mono text-xs text-slate-100"
              />
            </div>
            <div className="sm:col-span-2">
              <label className="block text-xs font-medium text-slate-300">Permissions-Policy</label>
              <input
                value={form.permissionsPolicy ?? ""}
                onChange={(e) => setForm({ ...form, permissionsPolicy: e.target.value })}
                placeholder="geolocation=(), microphone=() …"
                className="mt-1 w-full rounded-lg border border-white/10 bg-[var(--surface-input)] px-3 py-2 font-mono text-xs text-slate-100"
              />
            </div>
          </div>

          <div className="mt-6 flex gap-2">
            <Btn tone="primary" onClick={() => saveMutation.mutate()} disabled={saveMutation.isPending}>
              <Save size={14} /> {saveMutation.isPending ? "Saving…" : existing ? "Update headers" : "Create headers"}
            </Btn>
            {existing && (
              <Btn tone="danger" disabled={deleteMutation.isPending} onClick={() => { void (async () => { if (await confirm({ title: "Delete security-header override?", description: "This domain will fall back to the global security-header defaults. This cannot be undone.", danger: true, confirmLabel: "Delete override" })) deleteMutation.mutate(); })(); }}>
                <Trash2 size={14} /> Delete override
              </Btn>
            )}
            <Btn tone="ghost" onClick={() => router.push("/admin/security")}>
              <ArrowLeft size={14} /> Back
            </Btn>
          </div>
          {(saveMutation.isError || deleteMutation.isError) && (
            <p className="mt-3 text-xs text-red-300">
              {(saveMutation.error as Error)?.message ?? (deleteMutation.error as Error)?.message}
            </p>
          )}
          {(saveMutation.isSuccess || deleteMutation.isSuccess) && (
            <p className="mt-3 text-xs text-emerald-300">Saved — gateway route will pick it up on next sync.</p>
          )}
        </div>
      </Card>

      <RedirectsSection domainId={id} />

      {renderConfirm()}

      <Card className="p-4">
        <h4 className="mb-2 text-xs font-semibold uppercase tracking-widest text-slate-400">Wiring notes</h4>
        <ul className="list-disc space-y-1 pl-5 text-xs leading-5 text-slate-400">
          <li>
            API: <code className="font-mono">handlers_proxy_domains.go:312 registerSecurityHeadersRoutes</code> and{" "}
            <code className="font-mono">handlers_user_web.go:30 registerUserWebRoutes</code>.
          </li>
          <li>
            Store: <code className="font-mono">store_security_headers.go</code> + migration{" "}
            <code className="font-mono">117_domains_certificates.sql:49</code>.
          </li>
          <li>
            Client: <code className="font-mono">lib/api/security.ts:37</code> — canonical; server-scoped
            mirror at <code className="font-mono">/servers/:id/proxy-domains/:domainId/security-headers</code>{" "}
            (handlers_user_web.go:46).
          </li>
          <li>
            Gateway: per-domain headers are rendered as a Caddy{" "}
            <code className="font-mono">headers</code> handler in
            <code className="font-mono">caddy_proxy.go:777</code> (response.set).
          </li>
          <li>
            Redirects: <code className="font-mono">GET/POST /domains/:domainId/redirects</code>, <code className="font-mono">PUT/DELETE /domains/:domainId/redirects/:redirectId</code> via <code className="font-mono">lib/api/redirects.ts</code> (also re-exported from <code className="font-mono">lib/api/security.ts</code>) and proxy-domains mirror at <code className="font-mono">/servers/:serverId/proxy-domains/:domainId/redirects</code>.
          </li>
          <li>
            Proxy domains: <code className="font-mono">lib/api/proxy-domains.ts</code> — <code className="font-mono">GET /domains</code>, <code className="font-mono">POST /domains</code>, <code className="font-mono">GET/PUT/DELETE /domains/:id</code>, server scoped <code className="font-mono">/servers/:id/proxy-domains</code>.
          </li>
        </ul>
      </Card>
    </AdminPageLayout>
  );
}

function RedirectsSection({ domainId }: { domainId: string }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [showCreate, setShowCreate] = useState(false);
  const [editing, setEditing] = useState<RedirectRule | null>(null);

  const redirectsQuery = useQuery({
    queryKey: ["domain-redirects", domainId],
    queryFn: () => fetchRedirects(domainId),
  });
  const redirects = useMemo(() => redirectsQuery.data ?? [], [redirectsQuery.data]);

  const deleteMut = useMutation({
    mutationFn: (redirectId: string) => deleteRedirect(domainId, redirectId),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["domain-redirects", domainId] }); toast({ tone: "success", title: "Redirect deleted" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  return (
    <Card>
      <CardHeader title="Redirects — /domains/:domainId/redirects" icon={ArrowUpRight} action={<Btn size="sm" tone="primary" onClick={() => setShowCreate(true)} className="bg-[var(--brand)] hover:bg-[var(--brand)]/90 text-white"><Plus size={12} /> Add Redirect</Btn>} />
      <div className="p-3 text-xs text-slate-400">Wired via <code className="font-mono">lib/api/redirects.ts</code> (also <code className="font-mono">lib/api/security.ts</code> re-export) — CRUD maps to admin handlers at <code className="font-mono">handlers_proxy_domains.go</code>. Server mirror at <code className="font-mono">/servers/:id/proxy-domains/:domainId/redirects</code>.</div>
      {redirectsQuery.isLoading ? <div className="p-6 text-center text-sm text-slate-400">Loading redirects via fetchRedirects…</div>
        : redirectsQuery.isError ? <div className="p-4"><div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">Could not load redirects: {(redirectsQuery.error as Error).message} <Btn size="sm" tone="ghost" onClick={() => void redirectsQuery.refetch()} className="ml-2">Retry</Btn></div></div>
        : redirects.length === 0 ? <div className="p-6"><EmptyState icon={ArrowUpRight} message="No redirect rules. Use Add Redirect to POST /domains/:id/redirects with {sourcePath, targetUrl, statusCode}." /></div>
        : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead><tr className="border-b border-white/[0.06] bg-[var(--surface-input)] text-left text-[10px] uppercase tracking-widest text-slate-500"><th className="px-4 py-3">Source</th><th className="px-4 py-3">Target</th><th className="px-4 py-3">Code</th><th className="px-4 py-3">Enabled</th><th className="px-4 py-3"></th></tr></thead>
              <tbody className="divide-y divide-white/[0.04]">
                {redirects.map((r) => (
                  <tr key={r.id} className="hover:bg-white/[0.02]">
                    <td className="px-4 py-3 font-mono text-xs text-slate-200">{r.sourcePath}</td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-400 truncate max-w-[260px]">{r.targetUrl}</td>
                    <td className="px-4 py-3"><Pill tone={r.statusCode >= 300 && r.statusCode < 400 ? "green" : "neutral"}>{r.statusCode}</Pill></td>
                    <td className="px-4 py-3"><Pill tone={r.enabled ? "green" : "yellow"}>{r.enabled ? "enabled" : "disabled"}</Pill></td>
                    <td className="px-4 py-3 text-right space-x-1">
                      <Btn size="sm" tone="ghost" onClick={() => setEditing(r)}>Edit</Btn>
                      <Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: "Delete redirect?", description: `${r.sourcePath} → ${r.targetUrl}. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deleteMut.mutate(r.id); })(); }}><Trash2 size={12} /></Btn>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      {showCreate && <RedirectFormModal domainId={domainId} onClose={() => setShowCreate(false)} />}
      {editing && <RedirectFormModal domainId={domainId} existing={editing} onClose={() => setEditing(null)} />}
      {renderConfirm()}
    </Card>
  );
}

function RedirectFormModal({ domainId, existing, onClose }: { domainId: string; existing?: RedirectRule; onClose: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [sourcePath, setSourcePath] = useState(existing?.sourcePath ?? "/old");
  const [targetUrl, setTargetUrl] = useState(existing?.targetUrl ?? "https://example.com/new");
  const [statusCode, setStatusCode] = useState(String(existing?.statusCode ?? 301));
  const [enabled, setEnabled] = useState(existing?.enabled ?? true);
  const [regex, setRegex] = useState(existing?.regex ?? false);
  const [preservePath, setPreservePath] = useState(existing?.preservePath ?? false);

  const createMut = useMutation({
    mutationFn: () => createRedirect(domainId, { sourcePath: sourcePath.trim(), targetUrl: targetUrl.trim(), statusCode: Number(statusCode), enabled, regex, preservePath } as CreateRedirectInput),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["domain-redirects", domainId] }); toast({ tone: "success", title: "Redirect created" }); onClose(); },
    onError: (e: Error) => toast({ tone: "error", title: "Create failed", message: e.message }),
  });
  const updateMut = useMutation({
    mutationFn: () => updateRedirect(domainId, existing!.id, { sourcePath: sourcePath.trim(), targetUrl: targetUrl.trim(), statusCode: Number(statusCode), enabled, regex, preservePath }),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["domain-redirects", domainId] }); toast({ tone: "success", title: "Redirect updated" }); onClose(); },
    onError: (e: Error) => toast({ tone: "error", title: "Update failed", message: e.message }),
  });

  const isPending = createMut.isPending || updateMut.isPending;
  const error = (createMut.error ?? updateMut.error) as Error | null;

  return (
    <Modal title={existing ? "Edit Redirect" : "Add Redirect"} description={existing ? `PUT /domains/${domainId}/redirects/${existing.id}` : `POST /domains/${domainId}/redirects`} onClose={onClose}>
      <div className="space-y-4">
        <Input label="Source Path" value={sourcePath} onChange={setSourcePath} placeholder="/old/*" mono />
        <Input label="Target URL" value={targetUrl} onChange={setTargetUrl} placeholder="https://example.com/new" mono />
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Status Code</label>
            <select value={statusCode} onChange={(e) => setStatusCode(e.target.value)} className="h-9 w-full rounded-lg border border-white/10 bg-[var(--surface)] px-3 text-sm text-slate-100">
              <option value="301">301 Permanent</option>
              <option value="302">302 Found</option>
              <option value="307">307 Temporary</option>
              <option value="308">308 Permanent</option>
            </select>
          </div>
          <label className="flex items-center gap-2 pt-6 text-sm text-slate-300"><input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="accent-[var(--brand)]" /> Enabled</label>
        </div>
        <div className="flex gap-4">
          <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={regex} onChange={(e) => setRegex(e.target.checked)} className="accent-[var(--brand)]" /> Regex</label>
          <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={preservePath} onChange={(e) => setPreservePath(e.target.checked)} className="accent-[var(--brand)]" /> Preserve path</label>
        </div>
        <p className="text-xs text-slate-400">Wires <code className="font-mono">createRedirect / updateRedirect</code> → admin proxy-domain handlers; server mirror <code className="font-mono">/servers/:id/proxy-domains/:domainId/redirects</code>.</p>
        {error && <p className="text-sm text-red-300">{error.message}</p>}
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => existing ? updateMut.mutate() : createMut.mutate()} disabled={!sourcePath.trim() || !targetUrl.trim() || isPending} confirmLabel={isPending ? "Saving…" : existing ? "Update" : "Create"} />
    </Modal>
  );
}
