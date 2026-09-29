"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { Award, Plus, Trash2, RotateCw, Upload } from "lucide-react";
import { fetchJSON, postJSON, deleteJSON } from "@/lib/api";
import { AdminPageLayout, AdminSelect, AdminTable, AdminTBody, AdminTd, AdminTh, AdminTHead, AdminTr, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, AdminLoadingState, AdminErrorState } from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { formatDate } from "@/lib/utils";
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

/** A certificate imported through `POST /certificates` before the PEM was parsed
 *  was stored with a zero `expires_at`, which the backend reads as "expired long
 *  ago". A zero time is not a date, so it renders as unknown instead. */
function expiryDate(value?: string): Date | null {
  if (!value) return null;
  const d = new Date(value);
  if (Number.isNaN(d.valueOf()) || d.getUTCFullYear() < 2) return null;
  return d;
}

function remainingDays(expires: Date | null): number | null {
  if (!expires) return null;
  return (expires.getTime() - Date.now()) / (1000 * 86400);
}

function ExpiryPill({ expires }: { expires: Date | null }) {
  const days = remainingDays(expires);
  if (days === null) return <Pill tone="unknown">Expiry not reported</Pill>;
  if (days < 0) return <Pill tone="red">Expired {Math.abs(Math.round(days))}d ago</Pill>;
  if (days < 30) return <Pill tone="yellow">Expires in {Math.round(days)}d</Pill>;
  return <Pill tone="green">Valid · {Math.round(days)}d left</Pill>;
}

export default function AdminCertificatesPage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [search, setSearch] = useState("");
  const [showUploadModal, setShowUploadModal] = useState(false);
  const [uploadForm, setUploadForm] = useState({ domainId: "", certificate: "", privateKey: "", issuer: "custom" });
  const [showIssueModal, setShowIssueModal] = useState(false);
  const [issueForm, setIssueForm] = useState({ domains: "", email: "", challengeType: "http-01" as "http-01" | "dns-01", dnsProvider: "" });

  const certsQuery = useQuery({
    queryKey: ["admin", "certificates"],
    queryFn: () => fetchJSON<{ data: Certificate[] }>("/certificates"),
  });

  const certificates = useMemo(() => {
    const list = certsQuery.data?.data;
    return Array.isArray(list) ? list : [];
  }, [certsQuery.data]);

  const filtered = certificates.filter((c) =>
    !search || (c.domains ?? []).some((d) => d.toLowerCase().includes(search.toLowerCase())) || (c.provider ?? "").toLowerCase().includes(search.toLowerCase())
  );

  // `POST /certificates` (handlers_proxy_domains.go:309) takes domainId,
  // certificate, privateKey and issuer, so all four are posted — the five-field
  // form used to post only the two PEMs, silently discarding the domain binding
  // and the issuer. `autoRenew` is deliberately NOT offered: the handler
  // (handlers_proxy_domains.go:366) ignores it because an imported certificate
  // has no ACME order behind it, and answers with a `warning` instead.
  const uploadMutation = useMutation({
    mutationFn: () => postJSON<{ warning?: string }>("/certificates", {
      domainId: uploadForm.domainId.trim(),
      certificate: uploadForm.certificate.trim(),
      privateKey: uploadForm.privateKey.trim(),
      issuer: uploadForm.issuer.trim(),
    }),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: ["admin", "certificates"] });
      setShowUploadModal(false);
      setUploadForm({ domainId: "", certificate: "", privateKey: "", issuer: "custom" });
      toast({
        tone: res?.warning ? "warning" : "success",
        title: res?.warning ? "Certificate imported, not auto-renewed" : "Certificate imported",
        message: res?.warning || "Bound to the domain you selected. The gateway picks it up on its next reload.",
      });
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
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "certificates"] });
      toast({ tone: "success", title: "Renewal requested" });
    },
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
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "certificates"] });
      setShowIssueModal(false);
      setIssueForm({ domains: "", email: "", challengeType: "http-01", dnsProvider: "" });
      toast({ tone: "success", title: "Certificate issued" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to issue certificate", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const uploadError =
    !uploadForm.domainId.trim() ? "A domain to bind the certificate to is required."
      : !uploadForm.certificate.trim() || !uploadForm.privateKey.trim() ? "Both the certificate and the private key PEMs are required."
        : "";

  const issueDomains = issueForm.domains.split(/[,\n]/).map((d) => d.trim()).filter(Boolean);

  return (
    <AdminPageLayout>
      <SectionHeader
        status={<FreshnessBadge state={sourceState(certsQuery)} />}
        action={
          <div className="flex gap-2">
            <Btn size="sm" tone="primary" onClick={() => setShowIssueModal(true)}>
              <Plus size={12} /> Request Certificate
            </Btn>
            <Btn size="sm" tone="ghost" onClick={() => setShowUploadModal(true)}>
              <Upload size={12} /> Upload Certificate
            </Btn>
          </div>
        }
      />

      <Card>
        <CardHeader title="Certificates" icon={Award} />
        <div className="flex items-center gap-3 p-4">
          <Input label="Search" onChange={setSearch} placeholder="Search by domain or provider…" value={search} />
        </div>

        {certsQuery.isPending ? (
          <div className="p-4"><AdminLoadingState label="Loading certificates…" /></div>
        ) : certsQuery.isError ? (
          <div className="p-4"><AdminErrorState message={certsQuery.error instanceof Error ? certsQuery.error.message : "Failed to load certificates"} retry={() => void certsQuery.refetch()} /></div>
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={Award}
            message={search
              ? `No certificate covers a domain or provider containing “${search}”.`
              : certificates.length === 0
                ? "No certificates are stored. Request one from the CA, or import one you already hold."
                : `${certificates.length} certificate(s) were returned but none could be listed — the response was unusable.`}
            title={search ? "No certificates match the search" : "No certificates"}
          />
        ) : (
          <AdminTable label="Certificates">
            <AdminTHead><AdminTh>Domains</AdminTh><AdminTh>Provider</AdminTh><AdminTh>Issuer</AdminTh><AdminTh>Expiry</AdminTh><AdminTh>Auto-Renew</AdminTh><AdminTh></AdminTh></AdminTHead>
            <AdminTBody>
              {filtered.map((cert) => {
                const expires = expiryDate(cert.expiresAt);
                const sanList = Array.isArray(cert.domains) ? cert.domains : [];
                const renewing = renewMutation.isPending && renewMutation.variables === cert.id;
                const removing = deleteMutation.isPending && deleteMutation.variables === cert.id;
                return (
                  <AdminTr key={cert.id}>
                    <AdminTd className="max-w-xs">
                      <span className="break-all font-mono text-xs font-medium text-text" title={sanList.join(", ")}>
                        {sanList.length > 0 ? sanList.join(", ") : "No domains recorded"}
                      </span>
                    </AdminTd>
                    <AdminTd><Pill tone={cert.provider === "letsencrypt" ? "blue" : "neutral"}>{cert.provider || "Unknown provider"}</Pill></AdminTd>
                    <AdminTd className="text-xs text-text-subtle">{cert.issuer || "Not reported"}</AdminTd>
                    <AdminTd>
                      <div className="flex flex-col gap-1">
                        <ExpiryPill expires={expires} />
                        <span className="text-xs text-text-subtle">{expires ? formatDate(cert.expiresAt, "Date unknown") : "Expiry not reported"}</span>
                      </div>
                    </AdminTd>
                    <AdminTd>
                      <div className="flex items-center gap-1.5">
                        <Pill tone={cert.autoRenew ? "green" : "neutral"}>{cert.autoRenew ? "Automatic" : "Manual only"}</Pill>
                        <Btn
                          ariaLabel={`Renew certificate for ${sanList[0] ?? cert.id}`}
                          disabled={renewing || (renewMutation.isPending && !renewing)}
                          loading={renewing}
                          onClick={() => renewMutation.mutate(cert.id)}
                          size="sm"
                          tone="ghost"
                        >
                          <RotateCw size={12} /> Renew
                        </Btn>
                      </div>
                    </AdminTd>
                    <AdminTd>
                      <Btn
                        ariaLabel={`Delete certificate for ${sanList[0] ?? cert.id}`}
                        disabled={removing || (deleteMutation.isPending && !removing)}
                        loading={removing}
                        onClick={() => { void (async () => { if (await confirm({ title: `Revoke and delete the certificate for ${sanList[0] ?? cert.id}?`, description: "The pair is revoked at the CA and removed. HTTPS for " + (sanList.length > 1 ? `${sanList.length} hostnames` : (sanList[0] ?? "this hostname")) + " stops being served until another certificate covers them. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMutation.mutate(cert.id); })(); }}
                        size="sm"
                        tone="danger"
                      >
                        <Trash2 size={12} />
                      </Btn>
                    </AdminTd>
                  </AdminTr>
                );
              })}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>

      {showIssueModal && (
        <Modal onClose={() => setShowIssueModal(false)} title="Request a certificate">
          <div className="space-y-4">
            <Input label="Domains" mono onChange={(v) => setIssueForm({ ...issueForm, domains: v })} placeholder="example.com, *.example.com" value={issueForm.domains} />
            <Input label="Contact Email" onChange={(v) => setIssueForm({ ...issueForm, email: v })} placeholder="admin@example.com" type="email" value={issueForm.email} />
            <AdminSelect
              label="Challenge Type"
              onChange={(v) => setIssueForm({ ...issueForm, challengeType: v as "http-01" | "dns-01" })}
              options={[{ value: "http-01", label: "HTTP-01 (single domains)" }, { value: "dns-01", label: "DNS-01 (supports wildcards)" }]}
              value={issueForm.challengeType}
            />
            {issueForm.challengeType === "dns-01" && (
              <Input label="DNS Provider" onChange={(v) => setIssueForm({ ...issueForm, dnsProvider: v })} placeholder="cloudflare / route53 / gandi" value={issueForm.dnsProvider} />
            )}
            <p className="ui-hint">
              HTTP-01 requires each domain to already resolve to this panel; use DNS-01 for wildcards.
              Issued certificates renew automatically.
            </p>
          </div>
          <ModalFooter
            confirmLabel={issueMutation.isPending ? "Requesting…" : "Request"}
            disabled={issueMutation.isPending || issueDomains.length === 0 || !!issueMutation.error}
            onCancel={() => setShowIssueModal(false)}
            onConfirm={() => issueMutation.mutate()}
          />
        </Modal>
      )}

      {showUploadModal && (
        <Modal description="Import a certificate you already hold and bind it to one of your domains." onClose={() => setShowUploadModal(false)} title="Upload a certificate">
          <div className="space-y-4">
            <Input label="Domain ID" mono onChange={(v) => setUploadForm({ ...uploadForm, domainId: v })} placeholder="Proxy domain UUID" value={uploadForm.domainId} />
            <label className="block">
              <span className="ui-label">Certificate (PEM)</span>
              <textarea
                autoComplete="off"
                className="ui-input mt-1.5 h-24 w-full font-mono text-xs"
                onChange={(e) => setUploadForm({ ...uploadForm, certificate: e.target.value })}
                placeholder="-----BEGIN CERTIFICATE-----"
                spellCheck={false}
                value={uploadForm.certificate}
              />
            </label>
            <label className="block">
              <span className="ui-label">Private Key (PEM)</span>
              <textarea
                autoComplete="off"
                className="ui-input mt-1.5 h-24 w-full font-mono text-xs"
                onChange={(e) => setUploadForm({ ...uploadForm, privateKey: e.target.value })}
                placeholder="-----BEGIN PRIVATE KEY-----"
                spellCheck={false}
                value={uploadForm.privateKey}
              />
            </label>
            <Input label="Issuer" onChange={(v) => setUploadForm({ ...uploadForm, issuer: v })} placeholder="Recorded on the certificate row" value={uploadForm.issuer} />
            <p className="ui-hint">
              An imported certificate is never renewed automatically — the panel has no ACME order behind
              it, so there is nothing to reissue. When it expires, upload a replacement or request one
              from the CA instead.
            </p>
            {uploadError && uploadForm.certificate ? <p className="text-xs text-danger">{uploadError}</p> : null}
          </div>
          <ModalFooter
            confirmLabel={uploadMutation.isPending ? "Uploading…" : "Upload"}
            destructive={false}
            disabled={uploadMutation.isPending || !!uploadError}
            onCancel={() => setShowUploadModal(false)}
            onConfirm={() => uploadMutation.mutate()}
          />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
