"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Shield, Plus, Trash2, RotateCw, ExternalLink, AlertTriangle, CheckCircle, XCircle } from "lucide-react";
import { fetchJSON, postJSON, deleteJSON } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "@/components/admin/admin-ui";

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
  const [search, setSearch] = useState("");
  const [showUploadModal, setShowUploadModal] = useState(false);
  const [uploadForm, setUploadForm] = useState({ domainId: "", certificate: "", privateKey: "", issuer: "custom", autoRenew: false });

  const certsQuery = useQuery({
    queryKey: ["admin", "certificates"],
    queryFn: () => fetchJSON<{ data: Certificate[] }>("/certificates"),
  });

  const certificates = useMemo(() => certsQuery.data?.data ?? [], [certsQuery.data]);

  const filtered = certificates.filter((c) =>
    !search || c.domains.some((d) => d.toLowerCase().includes(search.toLowerCase())) || c.provider.toLowerCase().includes(search.toLowerCase())
  );

  const uploadMutation = useMutation({
    mutationFn: () => postJSON("/certificates", uploadForm),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "certificates"] });
      setShowUploadModal(false);
      setUploadForm({ domainId: "", certificate: "", privateKey: "", issuer: "custom", autoRenew: false });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteJSON(`/certificates/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "certificates"] }),
  });

  const renewMutation = useMutation({
    mutationFn: (id: string) => postJSON(`/certificates/${id}/renew`, {}),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["admin", "certificates"] }),
  });

  const isExpiring = (expiresAt: string) => {
    const days = (new Date(expiresAt).getTime() - Date.now()) / (1000 * 86400);
    return days < 30;
  };

  const isExpired = (expiresAt: string) => new Date(expiresAt) < new Date();

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Certificate Management"
        sub="Manage TLS/SSL certificates for proxy domains. Upload custom certificates or use Let's Encrypt auto-provisioning."
        action={
          <Btn size="sm" tone="primary" onClick={() => setShowUploadModal(true)}>
            <Plus size={12} /> Upload Certificate
          </Btn>
        }
      />

      <Card>
        <CardHeader title="Certificates" icon={Shield} />
        <div className="flex items-center gap-3 p-4">
          <Input placeholder="Search certificates..." value={search} onChange={setSearch} />
        </div>

        {certsQuery.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading certificates...</div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Shield} message="No certificates configured. Upload a certificate or use the ACME service to provision one." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Domains</th>
                  <th className="px-4 py-3">Provider</th>
                  <th className="px-4 py-3">Expiry</th>
                  <th className="px-4 py-3">Auto-Renew</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filtered.map((cert) => (
                  <tr key={cert.id} className="hover:bg-white/[0.02]">
                    <td className="px-4 py-3">
                      <div className="flex flex-col gap-0.5">
                        {cert.domains.map((d, i) => (
                          <span key={i} className="font-mono text-xs font-medium text-slate-200">{d}</span>
                        ))}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <Pill tone={cert.provider === "letsencrypt" ? "blue" : "neutral"}>
                        {cert.provider}
                      </Pill>
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
                      {cert.expiresAt ? new Date(cert.expiresAt).toLocaleDateString() : "—"}
                    </td>
                    <td className="px-4 py-3">
                      <Pill tone={cert.autoRenew ? "green" : "neutral"}>
                        {cert.autoRenew ? "Enabled" : "Disabled"}
                      </Pill>
                    </td>
                    <td className="px-4 py-3">
                      {isExpired(cert.expiresAt) ? (
                        <div className="flex items-center gap-1.5"><XCircle size={14} className="text-red-400" /><Pill tone="red">Expired</Pill></div>
                      ) : isExpiring(cert.expiresAt) ? (
                        <div className="flex items-center gap-1.5"><AlertTriangle size={14} className="text-amber-400" /><Pill tone="yellow">Expiring</Pill></div>
                      ) : (
                        <div className="flex items-center gap-1.5"><CheckCircle size={14} className="text-emerald-400" /><Pill tone="green">Valid</Pill></div>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex gap-1">
                        {cert.autoRenew && (
                          <Btn size="sm" tone="ghost" onClick={() => renewMutation.mutate(cert.id)} disabled={renewMutation.isPending}>
                            <RotateCw size={12} /> Renew
                          </Btn>
                        )}
                        <Btn size="sm" tone="danger" onClick={() => { if (confirm("Delete certificate?")) deleteMutation.mutate(cert.id); }}>
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

      {showUploadModal && (
        <Modal title="Upload Custom Certificate" onClose={() => setShowUploadModal(false)}>
          <div className="grid gap-4">
            <Input label="Domain ID" value={uploadForm.domainId} onChange={(v) => setUploadForm({ ...uploadForm, domainId: v })} placeholder="Domain UUID" />
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1.5">Certificate (PEM)</label>
              <textarea
                className="h-24 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 py-2 text-xs font-mono text-slate-100 outline-none focus:border-[#dc2626]/60 focus:ring-1 focus:ring-[#dc2626]/30"
                value={uploadForm.certificate}
                onChange={(e) => setUploadForm({ ...uploadForm, certificate: e.target.value })}
                placeholder="-----BEGIN CERTIFICATE-----"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1.5">Private Key (PEM)</label>
              <textarea
                className="h-24 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 py-2 text-xs font-mono text-slate-100 outline-none focus:border-[#dc2626]/60 focus:ring-1 focus:ring-[#dc2626]/30"
                value={uploadForm.privateKey}
                onChange={(e) => setUploadForm({ ...uploadForm, privateKey: e.target.value })}
                placeholder="-----BEGIN PRIVATE KEY-----"
              />
            </div>
            <Input label="Issuer" value={uploadForm.issuer} onChange={(v) => setUploadForm({ ...uploadForm, issuer: v })} placeholder="custom" />
            <label className="flex items-center gap-2 text-sm font-medium text-slate-300">
              <input type="checkbox" checked={uploadForm.autoRenew} onChange={(e) => setUploadForm({ ...uploadForm, autoRenew: e.target.checked })} className="rounded border-white/10 bg-[#161b28]" />
              Auto-renew (Caddy managed)
            </label>
          </div>
          <ModalFooter
            onCancel={() => setShowUploadModal(false)}
            onConfirm={() => uploadMutation.mutate()}
            confirmLabel={uploadMutation.isPending ? "Uploading..." : "Upload"}
            disabled={uploadMutation.isPending || !uploadForm.domainId || !uploadForm.certificate}
          />
        </Modal>
      )}
    </div>
  );
}
