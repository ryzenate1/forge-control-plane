"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Shield, Plus, Trash2 } from "lucide-react";
import { listAcmeAccounts, createAcmeAccount, deleteAcmeAccount, listDNSAccounts } from "@/lib/api/acme";
import { AdminPageLayout, SectionHeader, Card, CardHeader, Btn, Input, Modal, ModalFooter, EmptyState, Pill, AdminLoadingState, AdminErrorState } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";

export default function AdminAcmePage() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [showCreate, setShowCreate] = useState(false);
  const [email, setEmail] = useState("");
  const [caUrl, setCaUrl] = useState("");

  const accountsQuery = useQuery({ queryKey: ["acme", "accounts"], queryFn: listAcmeAccounts, retry: false });
  const dnsQuery = useQuery({ queryKey: ["acme", "dns-accounts"], queryFn: () => listDNSAccounts(), retry: false });

  const createMut = useMutation({
    mutationFn: () => createAcmeAccount({ email, caUrl: caUrl || undefined }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["acme", "accounts"] }); setShowCreate(false); setEmail(""); setCaUrl(""); toast({ tone: "success", title: "ACME account created" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Creation failed", message: e.message }),
  });
  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteAcmeAccount(id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["acme", "accounts"] }); toast({ tone: "success", title: "ACME account deleted" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  return (
    <AdminPageLayout>
      <SectionHeader
        title="ACME Accounts"
        sub="ACME accounts for automatic certificate issuance."
        action={<Btn tone="primary" onClick={() => setShowCreate(true)}><Plus size={12} /> New ACME Account</Btn>}
      />
      <Card>
        <CardHeader title="ACME Accounts" icon={Shield} action={<span className="text-xs text-text-subtle">{accountsQuery.data?.length ?? 0} accounts</span>} />
        {accountsQuery.isLoading ? <AdminLoadingState label="Loading ACME accounts…" />
          : accountsQuery.isError ? <div className="p-4"><AdminErrorState message={accountsQuery.error instanceof Error ? accountsQuery.error.message : "Failed"} retry={() => accountsQuery.refetch()} /></div>
          : (accountsQuery.data?.length ?? 0) === 0 ? <EmptyState icon={Shield} message="No ACME accounts. Create one with email and optional CA URL." />
          : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead><tr className="border-b border-line text-left text-[10px] uppercase tracking-widest text-text-subtle"><th className="px-4 py-3">Email</th><th className="px-4 py-3">CA</th><th className="px-4 py-3">Default</th><th className="px-4 py-3">Created</th><th className="px-4 py-3"></th></tr></thead>
                <tbody className="divide-y divide-line">
                  {accountsQuery.data?.map((a) => (
                    <tr key={a.id} className="hover:bg-overlay-subtle">
                      <td className="px-4 py-3 font-mono text-xs text-text">{a.email}</td>
                      <td className="px-4 py-3 text-xs text-text-subtle truncate max-w-[200px]">{a.caUrl}</td>
                      <td className="px-4 py-3">{a.isDefault ? <Pill tone="green">default</Pill> : <Pill>—</Pill>}</td>
                      <td className="px-4 py-3 text-xs text-text-subtle">{a.createdAt ? new Date(a.createdAt).toLocaleDateString() : "—"}</td>
                      <td className="px-4 py-3 text-right"><Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Delete ACME account ${a.email}?`, danger: true, confirmLabel: "Delete" })) deleteMut.mutate(a.id); })(); }}><Trash2 size={12} /></Btn></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        <div className="border-t border-line bg-overlay-subtle px-4 py-2 text-[11px] text-text-subtle">
          DNS provider accounts for DNS-01 challenges are listed below.
        </div>
      </Card>

      <Card>
        <CardHeader title="DNS Provider Accounts" icon={Shield} />
        {dnsQuery.isLoading ? <AdminLoadingState label="Loading DNS accounts…" />
          : dnsQuery.isError ? <div className="p-4"><AdminErrorState message={dnsQuery.error instanceof Error ? dnsQuery.error.message : "Failed to load DNS accounts"} retry={() => dnsQuery.refetch()} /></div>
          : (
            <div className="p-4">
              {(dnsQuery.data?.length ?? 0) === 0 ? <p className="text-sm text-text-subtle">No DNS accounts yet. Add one from DNS Providers.</p>
                : <div className="space-y-2">{dnsQuery.data?.map((d) => <div key={d.id} className="rounded border border-line px-3 py-2 text-xs flex justify-between"><span className="font-mono text-text">{d.name} · {d.provider}</span><span className="text-text-subtle">{new Date(d.createdAt).toLocaleDateString()}</span></div>)}</div>}
            </div>
          )}
      </Card>

      {showCreate && (
        <Modal title="Create ACME Account" onClose={() => setShowCreate(false)}>
          <div className="space-y-4">
            <Input label="Email *" value={email} onChange={setEmail} placeholder="admin@example.com" />
            <Input label="CA URL (optional)" value={caUrl} onChange={setCaUrl} placeholder="https://acme-v02.api.letsencrypt.org/directory" />
            <p className="text-xs text-text-subtle">Defaults to Let&apos;s Encrypt when no CA URL is given.</p>
          </div>
          <ModalFooter onCancel={() => setShowCreate(false)} onConfirm={() => createMut.mutate()} confirmLabel={createMut.isPending ? "Creating…" : "Create"} disabled={!email || createMut.isPending} />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
