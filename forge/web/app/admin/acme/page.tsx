"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Plus, Trash2 } from "lucide-react";
import { listAcmeAccounts, createAcmeAccount, deleteAcmeAccount, listDNSAccounts } from "@/lib/api/acme";
import { AdminPageLayout, SectionHeader, Card, CardHeader, Btn, Input, Modal, ModalFooter, EmptyState, Pill, AdminLoadingState, AdminErrorState, AdminTable, AdminTHead, AdminTh, AdminTBody, AdminTr, AdminTd } from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { formatDate } from "@/lib/utils";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";

const EMAIL = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

function isHttpUrl(value: string): boolean {
  try {
    const u = new URL(value);
    return u.protocol === "https:" || u.protocol === "http:";
  } catch {
    return false;
  }
}

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
    mutationFn: () => createAcmeAccount({ email: email.trim(), caUrl: caUrl.trim() || undefined }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["acme", "accounts"] }); setShowCreate(false); setEmail(""); setCaUrl(""); toast({ tone: "success", title: "ACME account created" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Creation failed", message: e.message }),
  });
  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteAcmeAccount(id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["acme", "accounts"] }); toast({ tone: "success", title: "ACME account deleted" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  // A typo in either field produces an account that cannot be diagnosed from
  // this page (there is no verify action), so both are checked before submit.
  const emailError = email && !EMAIL.test(email.trim()) ? "Enter an email address the CA can reach." : "";
  const caUrlError = caUrl && !isHttpUrl(caUrl.trim()) ? "Enter an absolute http(s) directory URL, for example https://acme-v02.api.letsencrypt.org/directory." : "";

  const accountCount = Array.isArray(accountsQuery.data) ? accountsQuery.data.length : null;

  return (
    <AdminPageLayout>
      <SectionHeader
        status={<FreshnessBadge state={sourceState(accountsQuery)} />}
        action={<Btn tone="primary" onClick={() => setShowCreate(true)}><Plus size={12} /> New ACME Account</Btn>}
      />
      <Card>
        <CardHeader
          action={<span className="text-xs text-text-subtle">{accountsQuery.isPending ? "Counting…" : accountCount === null ? "Count not available" : `${accountCount} account${accountCount === 1 ? "" : "s"}`}</span>}
          icon={KeyRound}
          title="ACME Accounts"
        />
        {accountsQuery.isPending ? <div className="p-4"><AdminLoadingState label="Loading ACME accounts…" /></div>
          : accountsQuery.isError ? <div className="p-4"><AdminErrorState message={accountsQuery.error instanceof Error ? accountsQuery.error.message : "ACME accounts could not be loaded"} retry={() => void accountsQuery.refetch()} /></div>
          : accountCount === 0 ? <EmptyState icon={KeyRound} title="No ACME accounts" message="No ACME account is registered. Certificate issuance needs one, with a contact email and optionally its own CA directory." />
          : (
            <AdminTable label="ACME accounts">
              <AdminTHead><AdminTh>Email</AdminTh><AdminTh>CA directory</AdminTh><AdminTh>Default</AdminTh><AdminTh>Created</AdminTh><AdminTh></AdminTh></AdminTHead>
              <AdminTBody>
                  {accountsQuery.data?.map((a) => (
                    <AdminTr key={a.id}>
                      <AdminTd className="font-mono text-xs text-text">{a.email}</AdminTd>
                      <AdminTd className="max-w-xs truncate font-mono text-xs text-text-subtle" title={a.caUrl ?? undefined}>{a.caUrl || "Provider default"}</AdminTd>
                      <AdminTd>{a.isDefault ? <Pill tone="green">Default</Pill> : <Pill tone="neutral">Not default</Pill>}</AdminTd>
                      <AdminTd className="text-xs text-text-subtle">{formatDate(a.createdAt, "Not reported")}</AdminTd>
                      <AdminTd><div className="flex justify-end"><Btn ariaLabel={`Delete ACME account ${a.email}`} disabled={deleteMut.isPending && deleteMut.variables === a.id} loading={deleteMut.isPending && deleteMut.variables === a.id} onClick={() => { void (async () => { if (await confirm({ title: `Delete ACME account ${a.email}?`, description: "Certificates already issued under this account stay valid, but issuance and renewal needs an account — with none left, Request Certificate on the Certificates page will fail.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(a.id); })(); }} size="sm" tone="danger"><Trash2 size={12} /></Btn></div></AdminTd>
                    </AdminTr>
                  ))}
              </AdminTBody>
            </AdminTable>
          )}
        <div className="mt-4 border-t border-line px-4 py-2 text-xs text-text-subtle">
          DNS provider accounts used for DNS-01 challenges are listed below; they are managed on the
          DNS Providers page.
        </div>
      </Card>

      <Card>
        <CardHeader title="DNS Provider Accounts" icon={KeyRound} />
        {dnsQuery.isPending ? <div className="p-4"><AdminLoadingState label="Loading DNS accounts…" /></div>
          : dnsQuery.isError ? <div className="p-4"><AdminErrorState message={dnsQuery.error instanceof Error ? dnsQuery.error.message : "DNS accounts could not be loaded"} retry={() => void dnsQuery.refetch()} /></div>
          : (
            <div className="p-4">
              {(dnsQuery.data?.length ?? 0) === 0 ? <EmptyState icon={KeyRound} title="No DNS accounts" message="No DNS provider account is configured. Add one from DNS Providers to issue certificates over DNS-01." />
                : <div className="space-y-2">{dnsQuery.data?.map((d) => <div key={d.id} className="flex justify-between rounded border border-line px-3 py-2 text-xs"><span className="font-mono text-text">{d.name} · {d.provider || "Provider not recorded"}</span><span className="text-text-subtle">{formatDate(d.createdAt, "Date not reported")}</span></div>)}</div>}
            </div>
          )}
      </Card>

      {showCreate && (
        <Modal description="The account identity a CA issues certificates against." onClose={() => setShowCreate(false)} title="New ACME Account">
          <div className="space-y-4">
            <Input autoComplete="off" label="Email" onChange={setEmail} placeholder="admin@example.com" type="email" value={email} />
            {emailError ? <p className="text-xs text-danger">{emailError}</p> : null}
            <Input autoComplete="off" label="CA directory URL (optional)" mono onChange={setCaUrl} placeholder="https://acme-v02.api.letsencrypt.org/directory" value={caUrl} />
            {caUrlError ? <p className="text-xs text-danger">{caUrlError}</p> : null}
            <p className="ui-hint">Leave the CA directory empty to use the configured default authority.</p>
          </div>
          <ModalFooter confirmLabel={createMut.isPending ? "Creating…" : "Create"} disabled={createMut.isPending || !email.trim() || !!emailError || !!caUrlError} onCancel={() => setShowCreate(false)} onConfirm={() => createMut.mutate()} />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
