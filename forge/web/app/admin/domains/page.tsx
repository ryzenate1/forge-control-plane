"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { Globe, Plus, Trash2, ShieldCheck, ShieldAlert, RotateCw, Network } from "lucide-react";
import { fetchServers } from "@/lib/api/servers";
import { fetchServerDomains, addServerDomain, removeServerDomain, verifyDomain, checkDNS as checkDNSApi } from "@/lib/api/domains";
import { AdminPageLayout, AdminSelect, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, AdminLoadingState, AdminErrorState } from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import Link from "next/link";

type DomainRecord = {
  id: string;
  serverId: string;
  domain: string;
  wildcard: boolean;
  verified: boolean;
  verifiedAt?: string;
  verificationToken?: string;
  createdAt: string;
};

type DNSResult = {
  domain: string;
  resolved: boolean;
  ips?: string[];
  expectedIp?: string;
  match: boolean;
  error?: string;
};

export default function AdminDomainsPage() {
  const [confirm, renderConfirm] = useConfirm();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [search, setSearch] = useState("");
  const [serverFilter, setServerFilter] = useState("");
  const [showAddModal, setShowAddModal] = useState(false);
  const [addForm, setAddForm] = useState({ serverId: "", domain: "" });
  const [dnsForm, setDnsForm] = useState({ domain: "", expectedIp: "" });
  const [showDNSModal, setShowDNSModal] = useState(false);
  const [dnsResult, setDnsResult] = useState<DNSResult | null>(null);

  const domainsQuery = useQuery<DomainRecord[]>({
    queryKey: ["domains", serverFilter || "all"],
    queryFn: async () => (await fetchServerDomains(serverFilter)) as unknown as DomainRecord[],
    enabled: !!serverFilter,
  });

  const serversQuery = useQuery({
    queryKey: ["admin", "servers", "list"],
    queryFn: () => fetchServers(),
  });

  const domains = useMemo(() => domainsQuery.data ?? [], [domainsQuery.data]);
  const servers = useMemo(() => serversQuery.data ?? [], [serversQuery.data]);

  const filteredDomains = domains.filter((d) =>
    !search || d.domain.toLowerCase().includes(search.toLowerCase())
  );

  const addMutation = useMutation({
    mutationFn: () => addServerDomain(addForm.serverId, addForm.domain),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["domains"] });
      setShowAddModal(false);
      setAddForm({ serverId: "", domain: "" });
      toast({ tone: "success", title: "Domain added" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to add domain", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const deleteMutation = useMutation({
    mutationFn: ({ serverId, id }: { serverId: string; id: string }) =>
      removeServerDomain(serverId, id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["domains"] });
      toast({ tone: "success", title: "Domain removed" });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to delete domain", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const verifyMutation = useMutation({
    mutationFn: (id: string) => verifyDomain(id),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["domains"] });
      const reason = (result as unknown as { error?: string }).error ?? result.message;
      if (result.verified) {
        toast({ tone: "success", title: "Domain verified" });
      } else {
        toast({ tone: "error", title: "Verification failed", message: reason ?? "Ownership could not be confirmed." });
      }
    },
    onError: (err) => toast({ tone: "error", title: "Verification failed", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const checkDNSMutation = useMutation({
    mutationFn: (data: { domain: string; expectedIp: string }) =>
      checkDNSApi(data.domain, data.expectedIp || undefined),
    onSuccess: (raw) => {
      const result = raw as unknown as DNSResult;
      setDnsResult({
        domain: result.domain ?? dnsForm.domain,
        resolved: result.resolved ?? false,
        ips: result.ips ?? [],
        expectedIp: result.expectedIp,
        match: result.match ?? false,
        error: result.error,
      });
    },
    onError: (err, data) => setDnsResult({
      domain: data.domain,
      resolved: false,
      ips: [],
      expectedIp: data.expectedIp,
      match: false,
      error: err instanceof Error ? err.message : "DNS check failed",
    }),
  });

  return (
    <AdminPageLayout>
      <SectionHeader
        title="Domains"
        sub="Custom domains with DNS and TLS status."
        action={
          <div className="flex gap-2">
            <Link href="/admin/dns"><Btn tone="ghost" className="border border-[color-mix(in_srgb,var(--brand)_20%,transparent)] hover:bg-[color-mix(in_srgb,var(--brand)_10%,transparent)]"><ShieldCheck size={12} /> DNS Providers</Btn></Link>
            <Link href="/admin/security"><Btn tone="ghost" className="border border-[color-mix(in_srgb,var(--brand)_20%,transparent)] hover:bg-[color-mix(in_srgb,var(--brand)_10%,transparent)]">Security Headers</Btn></Link>
            <Btn tone="ghost" onClick={() => setShowDNSModal(true)}>
              <Network size={14} /> Check DNS
            </Btn>
            <Btn size="sm" tone="primary" onClick={() => setShowAddModal(true)} className="bg-[var(--brand)] hover:bg-[color-mix(in_srgb,var(--brand)_90%,transparent)] text-white">
              <Plus size={12} /> Add Domain
            </Btn>
          </div>
        }
      />

      <Card>
        <CardHeader title="Domains" icon={Globe} />
        <div className="flex items-center gap-3 p-4">
          <div className="w-64"><AdminSelect value={serverFilter} onChange={setServerFilter} placeholder="Select a server..." options={Array.isArray(servers) ? servers.map((s) => ({ value: s.id, label: `${s.name} (${s.id})` })) : []} /></div>
          <Input placeholder="Search domains..." value={search} onChange={setSearch} />
        </div>

        {!serverFilter ? (
          <EmptyState icon={Globe} message="Select a server to view its domains." />
        ) : domainsQuery.isLoading ? (
          <AdminLoadingState label="Loading domains…" />
        ) : domainsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={domainsQuery.error instanceof Error ? domainsQuery.error.message : "Failed to load domains"}
              retry={() => void domainsQuery.refetch()}
            />
          </div>
        ) : filteredDomains.length === 0 ? (
          <EmptyState icon={Globe} message="No domains configured for this server." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-line text-left text-[10px] uppercase tracking-widest text-text-muted">
                  <th className="px-4 py-3">Domain</th>
                  <th className="px-4 py-3">Type</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Verified At</th>
                  <th className="px-4 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {Array.isArray(filteredDomains) && filteredDomains.map((d) => (
                  <tr key={d.id} className="hover:bg-overlay-subtle">
                    <td className="px-4 py-3 font-mono text-xs font-medium text-text">
                      {d.domain}
                    </td>
                    <td className="px-4 py-3">
                      <Pill tone={d.wildcard ? "blue" : "neutral"}>
                        {d.wildcard ? "Wildcard" : "Standard"}
                      </Pill>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-1.5">
                        {d.verified ? (
                          <ShieldCheck size={14} className="text-ok" />
                        ) : (
                          <ShieldAlert size={14} className="text-warn" />
                        )}
                        <Pill tone={d.verified ? "green" : "yellow"}>
                          {d.verified ? "Verified" : "Unverified"}
                        </Pill>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-xs text-text-subtle">
                      {d.verifiedAt ? new Date(d.verifiedAt).toLocaleString() : "—"}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex gap-1">
                        {!d.verified && (
                          <Btn
                            size="sm"
                            tone="ghost"
                            onClick={() => verifyMutation.mutate(d.id)}
                            disabled={verifyMutation.isPending}
                          >
                            <RotateCw size={12} /> Verify
                          </Btn>
                        )}
                        <Btn
                          size="sm"
                          tone="danger"
                          onClick={() => {
                            void (async () => { if (await confirm({ title: `Remove domain ${d.domain}?`, description: "The domain mapping will be removed. This cannot be undone.", danger: true, confirmLabel: "Remove" })) {
                              deleteMutation.mutate({ serverId: d.serverId, id: d.id });
                            } })();
                          }}
                        >
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

      {showAddModal && (
        <Modal title="Add Domain" onClose={() => setShowAddModal(false)}>
          <div className="space-y-4">
            <div>
              <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-text-subtle">Server</label>
              <select
                className="h-9 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 text-sm text-text outline-none focus:border-[color-mix(in_srgb,var(--brand)_60%,transparent)] focus:ring-1 focus:ring-[color-mix(in_srgb,var(--brand)_30%,transparent)]"
                value={addForm.serverId}
                onChange={(e) => setAddForm({ ...addForm, serverId: e.target.value })}
              >
                <option value="">Select server...</option>
                {Array.isArray(servers) && servers.map((s) => (
                  <option key={s.id} value={s.id}>
                    {s.name}
                  </option>
                ))}
              </select>
            </div>
            <Input
              label="Domain"
              value={addForm.domain}
              onChange={(v) => setAddForm({ ...addForm, domain: v })}
              placeholder="example.com or *.example.com"
            />
            {addForm.domain?.startsWith("*.") && (
              <p className="text-xs text-text-subtle">Wildcard domain detected. DNS verification will use test.{addForm.domain.replace("*.", "")}</p>
            )}
          </div>
          <ModalFooter
            onCancel={() => setShowAddModal(false)}
            onConfirm={() => addMutation.mutate()}
            confirmLabel={addMutation.isPending ? "Adding..." : "Add Domain"}
            disabled={addMutation.isPending || !addForm.serverId || !addForm.domain}
          />
        </Modal>
      )}

      {showDNSModal && (
        <Modal title="Check DNS Resolution" onClose={() => { setShowDNSModal(false); setDnsResult(null); }}>
          <div className="space-y-4">
            <Input
              label="Domain"
              value={dnsForm.domain}
              onChange={(v) => setDnsForm({ ...dnsForm, domain: v })}
              placeholder="example.com"
            />
            <Input
              label="Expected IP (optional)"
              value={dnsForm.expectedIp}
              onChange={(v) => setDnsForm({ ...dnsForm, expectedIp: v })}
              placeholder="1.2.3.4"
            />
            {dnsResult && (
              <div className={`p-4 rounded-lg border ${dnsResult.match ? "border-ok-line bg-ok-subtle" : "border-warn-line bg-warn-subtle"}`}>
                <p className={`text-sm font-medium ${dnsResult.match ? "text-ok" : "text-warn"}`}>
                  {dnsResult.match ? "DNS matches expected IP" : "DNS mismatch or not verified"}
                </p>
                {dnsResult.error && <p className="text-sm text-danger">{dnsResult.error}</p>}
                {dnsResult.ips && dnsResult.ips.length > 0 && (
                  <p className="text-xs text-text-subtle mt-1">
                    Resolved IPs: {dnsResult.ips.join(", ")}
                  </p>
                )}
                {dnsResult.expectedIp && (
                  <p className="text-xs text-text-muted mt-1">Expected: {dnsResult.expectedIp}</p>
                )}
              </div>
            )}
          </div>
          <ModalFooter
            onCancel={() => { setShowDNSModal(false); setDnsResult(null); }}
            onConfirm={() => checkDNSMutation.mutate(dnsForm)}
            confirmLabel={checkDNSMutation.isPending ? "Checking..." : "Check DNS"}
            disabled={checkDNSMutation.isPending || !dnsForm.domain}
          />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}

