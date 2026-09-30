"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { Shield, Plus, Trash2, RotateCw, AlertTriangle, CheckCircle, XCircle } from "lucide-react";
import { fetchJSON, postJSON, deleteJSON } from "@/lib/api";
import { importDomainCertificate, attachCertificateToDomain } from "@/lib/api/acme";
import { AdminPageLayout, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, AdminLoadingState, AdminErrorState } from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";

type Certificate = {
  id: string;
  domains: string[];
  issuer: string;
  certificate: string;
  expiresAt: string;
  autoRenew: boolean;
  provider: string;
  challengeType: string;
  wildcard: boolean;
  createdAt: string;
  updatedAt: string;
};

export default function AdminCertificatesPage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [search, setSearch] = useState("");
  const [showUploadModal, setShowUploadModal] = useState(false);
  const [uploadForm, setUploadForm] = useState({ domainId: "", certificate: "", privateKey: "", issuer: "custom", autoRenew: false });
  const [showAttachModal, setShowAttachModal] = useState(false);
  const [attachForm, setAttachForm] = useState({ certId: "", domainId: "", mode: "letsencrypt" as "letsencrypt" | "custom" });
  const [showIssueModal, setShowIssueModal] = useState(false);
  const [issueForm, setIssueForm] = useState({ domains: "", email: "", challengeType: "http-01" as "http-01" | "dns-01", dnsProvider: "" });

  const certsQuery = useQuery({
    queryKey: ["admin", "certificates"],
    queryFn: () => fetchJSON<{ data: Certificate[] }>("/certificates"),
  });

  const certificates = useMemo(() => certsQuery.data?.data ?? [], [certsQuery.data]);

  const filtered = certificates.filter((c) =>
    !search || c.domains.some((d) => d.toLowerCase().includes(search.toLowerCase())) || c.provider.toLowerCase().includes(search.toLowerCase())
  );

  const uploadMutation = useMutation({
    mutationFn: () => {
      // POST /certificates/upload stores an inventory certificate; POST
      // /certificates (domainId required) binds an operator certificate to a
      // proxy domain. The Domain ID field selects between the two so it is
      // never silently dropped.
      if (uploadForm.domainId.trim()) {
        return importDomainCertificate({
          domainId: uploadForm.domainId.trim(),
          certificate: uploadForm.certificate,
          privateKey: uploadForm.privateKey,
          issuer: uploadForm.issuer || undefined,
          autoRenew: false,
        });
      }
      return postJSON("/certificates/upload", { certificate: uploadForm.certificate, privateKey: uploadForm.privateKey });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "certificates"] });
      setShowUploadModal(false);
      setUploadForm({ domainId: "", certificate: "", privateKey: "", issuer: "custom", autoRenew: false });
      toast({ tone: "success", title: "Certificate uploaded" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to upload certificate", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/certificates/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "certificates"] }),
    onError: (err) => toast({ tone: "error", title: "Failed to delete certificate", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const renewMutation = useMutation({
    mutationFn: (id: string) => postJSON(`/certificates/${id}/renew`, {}),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "certificates"] }),
    onError: (err) => toast({ tone: "error", title: "Failed to renew certificate", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const issueMutation = useMutation({
    mutationFn: () => {
      const domains = issueForm.domains.split(/[,\n]/).map((d) => d.trim()).filter(Boolean);
      return postJSON("/certificates/issue", {
        domains,
        email: issueForm.email,
        challengeType: issueForm.challengeType,
        ...(issueForm.challengeType === "dns-01" && issueForm.dnsProvider ? { dnsProvider: issueForm.dnsProvider } : {}),
        autoRenew: true,
      });
    },    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "certificates"] });
      setShowIssueModal(false);
      setIssueForm({ domains: "", email: "", challengeType: "http-01", dnsProvider: "" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to issue certificate", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const attachMutation = useMutation({
    mutationFn: () => {
      // PUT /domains/:id { certType: "letsencrypt" } provisions via the Caddy
      // TLS manager; custom PEM binding is done at upload time through POST
      // /certificates with a domainId (see upload modal).
      return attachCertificateToDomain(attachForm.domainId.trim(), { certType: "letsencrypt", autoRenew: true, https: true });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "certificates"] });
      setShowAttachModal(false);
      setAttachForm({ certId: "", domainId: "", mode: "letsencrypt" });
      toast({ tone: "success", title: "Let's Encrypt enabled on domain" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to attach certificate", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const isExpiring = (expiresAt: string) => {
    const days = (new Date(expiresAt).getTime() - Date.now()) / (1000 * 86400);
    return days < 30;
  };

  const isExpired = (expiresAt: string) => new Date(expiresAt) < new Date();

  return (
    <AdminPageLayout>
      <SectionHeader
        title="Certificates"
        sub="Public TLS certificates and automated issuance."
        action={
          <div className="flex gap-2">
            <Btn size="sm" tone="primary" onClick={() => setShowIssueModal(true)}>
              <Plus size={12} /> Request Certificate
            </Btn>
            <Btn size="sm" tone="ghost" onClick={() => setShowUploadModal(true)}>
              <Plus size={12} /> Upload Certificate
            </Btn>
            <Btn size="sm" tone="ghost" onClick={() => setShowAttachModal(true)}>
              <Shield size={12} /> Attach to Domain
            </Btn>
          </div>
        }
      />

      <Card>
        <CardHeader title="Certificates" icon={Shield} />
        <div className="flex items-center gap-3 p-4">
          <Input placeholder="Search certificates..." value={search} onChange={setSearch} />
        </div>

        {certsQuery.isLoading ? (
          <AdminLoadingState label="Loading certificates…" />
        ) : certsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={certsQuery.error instanceof Error ? certsQuery.error.message : "Failed to load certificates"}
              retry={() => void certsQuery.refetch()}
            />
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Shield} message="No certificates configured. Upload a certificate or use the ACME service to provision one." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-line text-left text-[10px] uppercase tracking-widest text-text-muted">
                  <th className="px-4 py-3">Domains</th>
                  <th className="px-4 py-3">Provider</th>
                  <th className="px-4 py-3">Expiry</th>
                  <th className="px-4 py-3">Auto-Renew</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {Array.isArray(filtered) && filtered.map((cert) => (
                  <tr key={cert.id} className="hover:bg-overlay-subtle">
                    <td className="px-4 py-3">
                      <div className="flex flex-col gap-0.5">
                        {Array.isArray(cert.domains) && cert.domains.map((d, i) => (
                          <span key={i} className="font-mono text-xs font-medium text-text">{d}</span>
                        ))}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <Pill tone={cert.provider === "letsencrypt" ? "blue" : "neutral"}>
                        {cert.provider}
                      </Pill>
                    </td>
                    <td className="px-4 py-3 text-xs text-text-subtle">
                      {cert.expiresAt ? new Date(cert.expiresAt).toLocaleDateString() : "—"}
                    </td>
                    <td className="px-4 py-3">
                      <Pill tone={cert.autoRenew ? "green" : "neutral"}>
                        {cert.autoRenew ? "Enabled" : "Disabled"}
                      </Pill>
                    </td>
                    <td className="px-4 py-3">
                      {isExpired(cert.expiresAt) ? (
                        <div className="flex items-center gap-1.5"><XCircle size={14} className="text-danger" /><Pill tone="red">Expired</Pill></div>
                      ) : isExpiring(cert.expiresAt) ? (
                        <div className="flex items-center gap-1.5"><AlertTriangle size={14} className="text-warn" /><Pill tone="yellow">Expiring</Pill></div>
                      ) : (
                        <div className="flex items-center gap-1.5"><CheckCircle size={14} className="text-ok" /><Pill tone="green">Valid</Pill></div>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex gap-1">
                        {cert.autoRenew && (
                          <Btn size="sm" tone="ghost" onClick={() => renewMutation.mutate(cert.id)} disabled={renewMutation.isPending}>
                            <RotateCw size={12} /> Renew
                          </Btn>
                        )}
                        <Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Delete certificate for ${cert.domains[0] ?? cert.id.slice(0, 8)}?`, description: "HTTPS traffic will stop being served with this certificate. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMutation.mutate(cert.id); })(); }}>
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

      {showIssueModal && (
        <Modal title="Request Certificate (Let's Encrypt)" onClose={() => setShowIssueModal(false)}>
          <div className="space-y-4">
            <Input label="Domains" value={issueForm.domains} onChange={(v) => setIssueForm({ ...issueForm, domains: v })} placeholder="example.com, *.example.com" />
            <Input label="Contact Email" value={issueForm.email} onChange={(v) => setIssueForm({ ...issueForm, email: v })} placeholder="admin@example.com" />
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Challenge Type</label>
              <select
                className="w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 py-2 text-sm text-text outline-none"
                value={issueForm.challengeType}
                onChange={(e) => setIssueForm({ ...issueForm, challengeType: e.target.value as "http-01" | "dns-01" })}
              >
                <option value="http-01">HTTP-01 (single domains)</option>
                <option value="dns-01">DNS-01 (supports wildcards)</option>
              </select>
            </div>
            {issueForm.challengeType === "dns-01" && (
              <Input label="DNS Provider" value={issueForm.dnsProvider} onChange={(v) => setIssueForm({ ...issueForm, dnsProvider: v })} placeholder="cloudflare / route53 / gandi" />
            )}
            <p className="text-xs text-text-subtle">Issued via ACME. HTTP-01 requires the domain to already resolve to this panel; use DNS-01 for wildcards.</p>
          </div>
          <ModalFooter
            onCancel={() => setShowIssueModal(false)}
            onConfirm={() => issueMutation.mutate()}
            confirmLabel={issueMutation.isPending ? "Requesting..." : "Request"}
            disabled={issueMutation.isPending || !issueForm.domains.trim()}
          />
        </Modal>
      )}

      {showUploadModal && (
        <Modal title="Upload Custom Certificate" onClose={() => setShowUploadModal(false)}>
          <div className="space-y-4">
            <Input label="Domain ID (optional — binds to a proxy domain)" value={uploadForm.domainId} onChange={(v) => setUploadForm({ ...uploadForm, domainId: v })} placeholder="Domain UUID (empty = inventory only)" />
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Certificate (PEM)</label>
              <textarea
                className="h-24 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 py-2 text-xs font-mono text-text outline-none focus:border-[color-mix(in_srgb,var(--brand)_60%,transparent)] focus:ring-1 focus:ring-[color-mix(in_srgb,var(--brand)_30%,transparent)]"
                value={uploadForm.certificate}
                onChange={(e) => setUploadForm({ ...uploadForm, certificate: e.target.value })}
                placeholder="-----BEGIN CERTIFICATE-----"
              />
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Private Key (PEM)</label>
              <textarea
                className="h-24 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 py-2 text-xs font-mono text-text outline-none focus:border-[color-mix(in_srgb,var(--brand)_60%,transparent)] focus:ring-1 focus:ring-[color-mix(in_srgb,var(--brand)_30%,transparent)]"
                value={uploadForm.privateKey}
                onChange={(e) => setUploadForm({ ...uploadForm, privateKey: e.target.value })}
                placeholder="-----BEGIN PRIVATE KEY-----"
              />
            </div>
            <Input label="Issuer" value={uploadForm.issuer} onChange={(v) => setUploadForm({ ...uploadForm, issuer: v })} placeholder="custom" />
            <label className="flex items-center gap-2 text-sm font-medium text-text">
              <input type="checkbox" checked={uploadForm.autoRenew} onChange={(e) => setUploadForm({ ...uploadForm, autoRenew: e.target.checked })} className="rounded border-line bg-[var(--surface-input)]" />
              Auto-renew (Caddy managed)
            </label>
            <p className="text-xs text-text-subtle">With a Domain ID the PEM is imported and bound to that proxy domain (POST /certificates). Without one it is stored as inventory (POST /certificates/upload). Imported certificates are never auto-renewed via ACME.</p>
          </div>
          <ModalFooter
            onCancel={() => setShowUploadModal(false)}
            onConfirm={() => uploadMutation.mutate()}
            confirmLabel={uploadMutation.isPending ? "Uploading..." : "Upload"}
            disabled={uploadMutation.isPending || !uploadForm.certificate || !uploadForm.privateKey}
          />
        </Modal>
      )}

      {showAttachModal && (
        <Modal title="Attach Let's Encrypt to Domain" onClose={() => setShowAttachModal(false)}>
          <div className="space-y-4">
            <Input label="Proxy Domain ID" value={attachForm.domainId} onChange={(v) => setAttachForm({ ...attachForm, domainId: v })} placeholder="Domain UUID from /admin/domains" />
            <p className="text-xs text-slate-400">Sets PUT /domains/:id {"{ certType: \"letsencrypt\", autoRenew: true }"} so the Caddy gateway provisions and renews the certificate for that hostname. The hostname must already resolve to this panel and ports 80/443 must be reachable.</p>
          </div>
          <ModalFooter
            onCancel={() => setShowAttachModal(false)}
            onConfirm={() => attachMutation.mutate()}
            confirmLabel={attachMutation.isPending ? "Attaching..." : "Attach"}
            disabled={attachMutation.isPending || !attachForm.domainId.trim()}
          />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
