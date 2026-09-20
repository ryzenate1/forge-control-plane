"use client";

import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { Globe, Shield, Save, Trash2, ExternalLink, Plus, ArrowUpRight } from "lucide-react";
import {
  fetchDomainSecurityHeaders,
  createDomainSecurityHeaders,
  updateDomainSecurityHeaders,
  deleteDomainSecurityHeaders,
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
import { fetchServer } from "@/lib/api/servers";
import { ApiError } from "@/lib/api/http";
import { sourceState } from "@/lib/admin/telemetry";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { AdminPageLayout,
  AdminPageHeader,
  AdminLoadingState,
  AdminErrorState,
  Card,
  CardHeader,
  Btn,
  Pill,
  Input,
  Modal,
  ModalFooter,
  EmptyState,
  AdminTable,
  AdminTHead,
  AdminTh,
  AdminTBody,
  AdminTr,
  AdminTd,
} from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { cn } from "@/lib/utils";
import { OfflineBanner } from "@/components/shared/states-offline";

type ProxyDomain = {
  id: string;
  hostname: string;
  serviceId?: string;
  serviceType?: string;
  https: boolean;
  port: number;
  certType?: string;
  autoRenew?: boolean;
  path?: string;
  createdAt?: string;
};

/** Referrer-Policy values a browser accepts. Free text here used to write an
 *  ignored header into a live gateway route. */
const REFERRER_POLICIES = [
  "no-referrer",
  "no-referrer-when-downgrade",
  "origin",
  "origin-when-cross-origin",
  "same-origin",
  "strict-origin",
  "strict-origin-when-cross-origin",
  "unsafe-url",
];

/** HSTS max-age is a duration in seconds. `0` alongside `hstsEnabled: true`
 *  asks browsers to forget the policy, and values above one year are what the
 *  preload list requires — so both ends are bounded. */
const HSTS_MAX_AGE_MIN = 1;
const HSTS_MAX_AGE_MAX = 63072000;

export default function AdminDomainDetailPage() {
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { toast } = useToast();
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

  // Attribution. A proxy domain carries no serverId or organizationId of its
  // own; the owning workload is `serviceId` when `serviceType` is `server`, so
  // resolve that into a named server, its owner and its node rather than
  // printing an unresolved UUID or claiming nothing.
  const boundServerId = domain?.serviceType === "server" ? domain.serviceId ?? "" : "";
  const serverQuery = useQuery({
    queryKey: ["admin", "server", boundServerId],
    queryFn: () => fetchServer(boundServerId),
    enabled: !!boundServerId,
  });

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

  // Hydrate the form from the server copy once per override id. Re-running it
  // on every `existing` change meant a save invalidating its own query
  // overwrote anything typed while the response was in flight.
  const hydratedFrom = useRef<string | null>(null);
  useEffect(() => {
    if (!existing) return;
    if (hydratedFrom.current === existing.id) return;
    hydratedFrom.current = existing.id;
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
  }, [existing]);

  // These three controls write response headers into a live gateway route, so
  // the mistakes are blocked before submit rather than surfaced as a 200.
  const headerErrors: string[] = [];
  if (form.hstsEnabled && (form.hstsMaxAge ?? 0) < HSTS_MAX_AGE_MIN) headerErrors.push("HSTS Max-Age must be at least 1 second while HSTS is enabled.");
  if (form.hstsMaxAge != null && (form.hstsMaxAge > HSTS_MAX_AGE_MAX)) headerErrors.push(`HSTS Max-Age must be ${HSTS_MAX_AGE_MAX} seconds or less.`);
  if (form.cspEnabled && !(form.cspPolicy ?? "").trim()) headerErrors.push("CSP is enabled but the policy is empty — saving would send an empty Content-Security-Policy header.");
  if (form.referrerPolicy && !REFERRER_POLICIES.includes(form.referrerPolicy)) headerErrors.push("Referrer-Policy is not a value browsers recognise.");

  const [notice, setNotice] = useState<{ tone: "ok" | "removed"; text: string } | null>(null);

  const saveMutation = useMutation({
    mutationFn: async () => {
      setNotice(null);
      if (existing?.id) {
        return updateDomainSecurityHeaders(id, existing.id, form);
      }
      return createDomainSecurityHeaders(id, form);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["domain-security-headers-detail", id] });
      queryClient.invalidateQueries({ queryKey: ["domain-security-headers", id] });
      setNotice({ tone: "ok", text: "Override saved. The gateway applies it on its next configuration reload — this page cannot confirm the reload has happened." });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Could not save headers", message: e.message }),
  });

  const deleteMutation = useMutation({
    mutationFn: () => {
      setNotice(null);
      if (!existing?.id) return Promise.resolve();
      return deleteDomainSecurityHeaders(id, existing.id);
    },
    onSuccess: () => {
      hydratedFrom.current = null;
      queryClient.invalidateQueries({ queryKey: ["domain-security-headers-detail", id] });
      queryClient.invalidateQueries({ queryKey: ["domain-security-headers", id] });
      setNotice({ tone: "removed", text: "Override removed. This domain now falls back to the global security-header defaults." });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Could not remove override", message: e.message }),
  });

  const isLoading = domainQuery.isPending || (headersQuery.isPending && !headersQuery.isError);

  if (domainQuery.isError) {
    const err = domainQuery.error;
    const notFound = err instanceof ApiError && err.status === 404;
    return (
      <AdminPageLayout>
        <AdminPageHeader
          title={notFound ? "Domain not found" : "Domain could not be loaded"}
          description={notFound
            ? `No proxy domain is recorded under ${id}.`
            : `Loading ${id} failed. This is a request failure, not a missing domain.`}
          backAction={() => router.push("/admin/domains")}
          backLabel="Domains"
        />
        <div className="p-4">
          <AdminErrorState
            message={notFound ? "This domain is not in the gateway's proxy-domain list." : (err instanceof Error ? err.message : "Request failed")}
            retry={() => void domainQuery.refetch()}
          />
        </div>
      </AdminPageLayout>
    );
  }

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={() => window.location.reload()} />
      <AdminPageHeader
        title={domain?.hostname ?? id}
        description={`Gateway proxy domain · ${domain?.https ? "HTTPS" : "HTTP"} on port ${domain?.port ?? "unknown"}${domain?.path ? ` · path ${domain.path}` : ""}`}
        backAction={() => router.push("/admin/domains")}
        backLabel="Domains"
        status={<FreshnessBadge state={sourceState(domainQuery)} />}
        action={
          <div className="flex gap-2">
            <Btn tone="ghost" onClick={() => router.push("/admin/security")}>
              <Shield size={14} /> Security Overview
            </Btn>
            {domain?.hostname && (
              <a
                className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-overlay-subtle px-3 py-2 text-sm font-medium text-text hover:bg-overlay"
                href={`https://${domain.hostname}`}
                rel="noopener noreferrer"
                target="_blank"
              >
                <ExternalLink size={14} /> Open
              </a>
            )}
          </div>
        }
      />

      {isLoading && !domain ? <div className="p-4"><AdminLoadingState label="Loading proxy domain…" /></div> : null}

      <Card>
        <CardHeader title="Domain" icon={Globe} />
        <div className="grid gap-3 p-4 text-sm sm:grid-cols-2">
          <div>
            <p className="t-eyebrow">Domain ID</p>
            <p className="mt-1 font-mono text-xs text-text">{domain?.id ?? "—"}</p>
          </div>
          <div>
            <p className="t-eyebrow">Hostname</p>
            <p className="mt-1 font-mono text-xs text-text">{domain?.hostname ?? "—"}</p>
          </div>
          <div>
            <p className="t-eyebrow">Bound service</p>
            {serverQuery.data ? (
              <p className="mt-1 text-xs text-text">
                server /{" "}
                <Link className="underline hover:text-text" href={`/admin/servers/${boundServerId}`}>
                  {serverQuery.data.name}
                </Link>
              </p>
            ) : boundServerId ? (
              <p className="mt-1 text-xs text-text-subtle">
                server / <span className="font-mono">{boundServerId}</span>
                {serverQuery.isError ? " — name could not be resolved" : " — resolving…"}
              </p>
            ) : domain?.serviceId ? (
              <p className="mt-1 text-xs text-text-subtle">{domain.serviceType ?? "unknown"} / <span className="font-mono">{domain.serviceId}</span> — not a server binding, so no owner attribution is available</p>
            ) : (
              <p className="mt-1 text-xs text-text-subtle">Not bound to a service — this domain routes nowhere.</p>
            )}
          </div>
          <div>
            <p className="t-eyebrow">Owner</p>
            <p className="mt-1 text-xs text-text-subtle">
              {serverQuery.data?.owner || serverQuery.data?.ownerEmail
                ? serverQuery.data.owner ?? serverQuery.data.ownerEmail
                : boundServerId && serverQuery.isPending
                  ? "Resolving…"
                  : "Not attributable from this record"}
            </p>
          </div>
          <div>
            <p className="t-eyebrow">Node</p>
            <p className="mt-1 text-xs text-text-subtle">
              {serverQuery.data?.node ?? (boundServerId && serverQuery.isPending ? "Resolving…" : "Not reported")}
            </p>
          </div>
          <div>
            <p className="t-eyebrow">HTTPS</p>
            <Pill tone={domain?.https ? "green" : "neutral"}>{domain?.https ? "Enabled" : "Disabled"}</Pill>
          </div>
          <div>
            <p className="t-eyebrow">Certificate</p>
            <p className="mt-1 text-xs text-text-subtle">
              {domain?.certType ? `${domain.certType}${domain.autoRenew ? " · auto-renew on" : " · auto-renew off"}` : "No certificate bound"}
              {" · "}
              <Link className="underline hover:text-text" href="/admin/certificates">Certificates</Link>
            </p>
          </div>
        </div>
      </Card>

      <Card>
        <CardHeader title="Security Headers" icon={Shield} />
        <div className="p-4">
          <p className="mb-4 text-xs leading-5 text-text-subtle">
            These headers are injected on this domain&apos;s gateway responses. Without an override
            here, the global security-header defaults apply.
          </p>

          {headersQuery.isError ? (
            <div className="mb-3">
              <AdminErrorState
                message={`Security headers could not be loaded: ${(headersQuery.error as Error).message}. Saving now may create a second override.`}
                retry={() => void headersQuery.refetch()}
              />
            </div>
          ) : headersQuery.isPending ? (
            <div className="mb-3"><AdminLoadingState label="Loading the current override…" /></div>
          ) : !existing ? (
            <p className="mb-3 rounded-lg border border-line bg-overlay-subtle px-3 py-2 text-xs text-text-subtle">
              No per-domain override is set — the global header defaults are in force. Saving creates one.
            </p>
          ) : null}

          <div className="grid gap-4 sm:grid-cols-2">
            <label className="flex items-center gap-2 text-sm text-text">
              <input
                checked={!!form.hstsEnabled}
                className="accent-[var(--brand)]"
                onChange={(e) => setForm({ ...form, hstsEnabled: e.target.checked })}
                type="checkbox"
              />
              HSTS Enabled
            </label>
            <label className="block">
              <span className="ui-label">HSTS Max-Age (seconds, {HSTS_MAX_AGE_MIN}–{HSTS_MAX_AGE_MAX})</span>
              <input
                className="ui-input mt-1.5 w-full font-mono"
                max={HSTS_MAX_AGE_MAX}
                min={HSTS_MAX_AGE_MIN}
                onChange={(e) => setForm({ ...form, hstsMaxAge: Number(e.target.value) })}
                type="number"
                value={form.hstsMaxAge ?? 63072000}
              />
            </label>
            <label className="flex items-center gap-2 text-sm text-text">
              <input
                checked={!!form.hstsIncludeSubdomains}
                className="accent-[var(--brand)]"
                onChange={(e) => setForm({ ...form, hstsIncludeSubdomains: e.target.checked })}
                type="checkbox"
              />
              HSTS Include Subdomains
            </label>
            <label className="flex items-center gap-2 text-sm text-text">
              <input
                checked={!!form.hstsPreload}
                className="accent-[var(--brand)]"
                onChange={(e) => setForm({ ...form, hstsPreload: e.target.checked })}
                type="checkbox"
              />
              HSTS Preload
            </label>
            <label className="block">
              <span className="ui-label">X-Frame-Options</span>
              <select
                className="ui-input mt-1.5 w-full"
                onChange={(e) => setForm({ ...form, xFrameOptions: e.target.value })}
                value={form.xFrameOptions ?? "DENY"}
              >
                <option value="DENY">DENY</option>
                <option value="SAMEORIGIN">SAMEORIGIN</option>
                <option value="">(none)</option>
              </select>
            </label>
            <label className="block">
              <span className="ui-label">X-Content-Type-Options</span>
              <select
                className="ui-input mt-1.5 w-full"
                onChange={(e) => setForm({ ...form, xContentTypeOptions: e.target.value })}
                value={form.xContentTypeOptions ?? "nosniff"}
              >
                <option value="nosniff">nosniff</option>
                <option value="">(none)</option>
              </select>
            </label>
            <label className="block sm:col-span-2">
              <span className="ui-label">Referrer-Policy</span>
              <select
                className="ui-input mt-1.5 w-full font-mono"
                onChange={(e) => setForm({ ...form, referrerPolicy: e.target.value })}
                value={form.referrerPolicy ?? ""}
              >
                <option value="">(none)</option>
                {REFERRER_POLICIES.map((p) => <option key={p} value={p}>{p}</option>)}
              </select>
            </label>
            <label className="flex items-center gap-2 text-sm text-text">
              <input
                checked={!!form.cspEnabled}
                className="accent-[var(--brand)]"
                onChange={(e) => setForm({ ...form, cspEnabled: e.target.checked })}
                type="checkbox"
              />
              CSP Enabled
            </label>
            <label className="block sm:col-span-2">
              <span className="ui-label">CSP Policy{form.cspEnabled ? " (required while CSP is enabled)" : ""}</span>
              <textarea
                className="ui-input mt-1.5 w-full font-mono"
                onChange={(e) => setForm({ ...form, cspPolicy: e.target.value })}
                placeholder="default-src 'self'; ..."
                rows={3}
                value={form.cspPolicy ?? ""}
              />
            </label>
            <label className="block sm:col-span-2">
              <span className="ui-label">Permissions-Policy</span>
              <input
                className="ui-input mt-1.5 w-full font-mono"
                onChange={(e) => setForm({ ...form, permissionsPolicy: e.target.value })}
                placeholder="geolocation=(), microphone=()"
                value={form.permissionsPolicy ?? ""}
              />
            </label>
          </div>

          {headerErrors.length > 0 && (
            <ul aria-label="Fix before saving" className="ui-alert ui-alert-danger mt-4 list-disc space-y-1 pl-5 text-xs" role="alert">
              {headerErrors.map((e) => <li key={e}>{e}</li>)}
            </ul>
          )}

          <div className="mt-6 flex gap-2">
            <Btn
              disabled={headerErrors.length > 0 || saveMutation.isPending || domainQuery.isPending || headersQuery.isPending}
              onClick={() => saveMutation.mutate()}
              tone="primary"
            >
              <Save size={14} /> {saveMutation.isPending ? "Saving…" : existing ? "Update headers" : "Create headers"}
            </Btn>
            {existing && (
              <Btn tone="danger" disabled={deleteMutation.isPending} onClick={() => { void (async () => { if (await confirm({ title: "Delete security-header override?", description: `${domain?.hostname ?? id} will fall back to the global security-header defaults. This cannot be undone.`, danger: true, confirmLabel: "Delete override" })) deleteMutation.mutate(); })(); }}>
                <Trash2 size={14} /> Delete override
              </Btn>
            )}
          </div>
          {(saveMutation.isError || deleteMutation.isError) && (
            <p className="mt-3 text-xs text-danger">
              {(saveMutation.error as Error)?.message ?? (deleteMutation.error as Error)?.message}
            </p>
          )}
          {notice && (
            <p className={cn("mt-3 text-xs", notice.tone === "ok" ? "text-ok" : "text-text-subtle")}>{notice.text}</p>
          )}
        </div>
      </Card>

      <RedirectsSection domainId={id} hostname={domain?.hostname} />

      {renderConfirm()}
    </AdminPageLayout>
  );
}

function RedirectsSection({ domainId, hostname }: { domainId: string; hostname?: string }) {
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
      <CardHeader title="Redirects" icon={ArrowUpRight} action={<Btn size="sm" tone="primary" onClick={() => setShowCreate(true)} className="bg-[var(--brand)] hover:bg-[color-mix(in_srgb,var(--brand)_90%,transparent)] text-white"><Plus size={12} /> Add Redirect</Btn>} />
      {redirectsQuery.isPending ? <div className="p-4"><AdminLoadingState label="Loading redirects…" /></div>
        : redirectsQuery.isError ? <div className="p-4"><AdminErrorState message={`Redirects could not be loaded: ${(redirectsQuery.error as Error).message}`} retry={() => void redirectsQuery.refetch()} /></div>
        : redirects.length === 0 ? <div className="p-4"><EmptyState icon={ArrowUpRight} title="No redirects" message={`No redirect rules are configured for ${hostname ?? "this domain"}.`} /></div>
        : (
          <AdminTable label={`Redirect rules for ${hostname ?? domainId}`}>
            <AdminTHead><AdminTh>Source</AdminTh><AdminTh>Target</AdminTh><AdminTh>Status code</AdminTh><AdminTh>Enabled</AdminTh><AdminTh></AdminTh></AdminTHead>
            <AdminTBody>
              {redirects.map((r) => (
                <AdminTr key={r.id}>
                  <AdminTd className="font-mono text-xs text-text">{r.sourcePath}</AdminTd>
                  <AdminTd className="max-w-xs truncate font-mono text-xs text-text-subtle" title={r.targetUrl}>{r.targetUrl}</AdminTd>
                  <AdminTd><Pill tone={r.statusCode === 301 || r.statusCode === 308 ? "green" : "blue"}>{r.statusCode} {r.statusCode === 301 || r.statusCode === 308 ? "permanent" : "temporary"}</Pill></AdminTd>
                  <AdminTd><Pill tone={r.enabled ? "green" : "neutral"}>{r.enabled ? "Enabled" : "Disabled"}</Pill></AdminTd>
                  <AdminTd className="text-right">
                    <div className="flex justify-end gap-1.5">
                      <Btn size="sm" tone="ghost" onClick={() => setEditing(r)}>Edit</Btn>
                      <Btn ariaLabel={`Delete redirect ${r.sourcePath}`} size="sm" tone="danger" disabled={deleteMut.isPending && deleteMut.variables === r.id} loading={deleteMut.isPending && deleteMut.variables === r.id} onClick={() => { void (async () => { if (await confirm({ title: "Delete redirect?", description: `${hostname ?? domainId}: ${r.sourcePath} → ${r.targetUrl}. Visitors requesting the source path will get the domain's normal response. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deleteMut.mutate(r.id); })(); }}><Trash2 size={12} /></Btn>
                    </div>
                  </AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
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

  // A redirect writes a gateway rule for a live hostname, so an unparseable
  // target or a source that is not a path is blocked before submit.
  const sourcePathError = !sourcePath.trim().startsWith("/") ? "Source path must start with /." : "";
  const targetUrlError = /^https?:\/\/\S+\.\S+/.test(targetUrl.trim()) ? "" : "Target must be an absolute http(s) URL.";
  const formError = sourcePathError || targetUrlError;

  return (
    <Modal description={existing ? "Change where visitors requesting this path are sent." : "Send visitors who request this path somewhere else."} onClose={onClose} title={existing ? "Edit Redirect" : "Add Redirect"}>
      <div className="space-y-4">
        <Input label="Source Path" mono onChange={setSourcePath} placeholder="/old/*" value={sourcePath} />
        {sourcePath && sourcePathError ? <p className="text-xs text-danger">{sourcePathError}</p> : null}
        <Input label="Target URL" mono onChange={setTargetUrl} placeholder="https://example.com/new" value={targetUrl} />
        {targetUrl && targetUrlError ? <p className="text-xs text-danger">{targetUrlError}</p> : null}
        <div className="grid grid-cols-2 gap-3">
          <label className="block">
            <span className="ui-label">Status Code</span>
            <select className="ui-input mt-1.5 w-full" onChange={(e) => setStatusCode(e.target.value)} value={statusCode}>
              <option value="301">301 Permanent</option>
              <option value="302">302 Found</option>
              <option value="307">307 Temporary</option>
              <option value="308">308 Permanent</option>
            </select>
          </label>
          <label className="flex items-center gap-2 self-end text-sm text-text">
            <input checked={enabled} className="accent-[var(--brand)]" onChange={(e) => setEnabled(e.target.checked)} type="checkbox" /> Enabled
          </label>
        </div>
        <div className="flex flex-wrap gap-4">
          <label className="flex items-center gap-2 text-sm text-text">
            <input checked={regex} className="accent-[var(--brand)]" onChange={(e) => setRegex(e.target.checked)} type="checkbox" /> Treat source as a regular expression
          </label>
          <label className="flex items-center gap-2 text-sm text-text">
            <input checked={preservePath} className="accent-[var(--brand)]" onChange={(e) => setPreservePath(e.target.checked)} type="checkbox" /> Append the matched path to the target
          </label>
        </div>
        {error && <p className="text-sm text-danger">{error.message}</p>}
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => existing ? updateMut.mutate() : createMut.mutate()} disabled={!!formError || !sourcePath.trim() || !targetUrl.trim() || isPending} confirmLabel={isPending ? "Saving…" : existing ? "Update" : "Create"} />
    </Modal>
  );
}
