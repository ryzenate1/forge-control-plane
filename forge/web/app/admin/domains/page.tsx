"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Globe, Plus, Trash2, ShieldCheck, ShieldAlert, RotateCw, Network } from "lucide-react";
import { fetchJSON, postJSON, deleteJSON } from "@/lib/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "@/components/admin/admin-ui";

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

type VerificationResult = {
  domain: string;
  verified: boolean;
  dnsResolved: boolean;
  expectedIp?: string;
  resolvedIps?: string[];
  error?: string;
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
  const queryClient = useQueryClient();
  const [search, setSearch] = useState("");
  const [serverFilter, setServerFilter] = useState("");
  const [showAddModal, setShowAddModal] = useState(false);
  const [addForm, setAddForm] = useState({ serverId: "", domain: "" });
  const [dnsForm, setDnsForm] = useState({ domain: "", expectedIp: "" });
  const [showDNSModal, setShowDNSModal] = useState(false);
  const [dnsResult, setDnsResult] = useState<DNSResult | null>(null);

  const domainsQuery = useQuery({
    queryKey: ["domains", serverFilter || "all"],
    queryFn: async () => {
      if (serverFilter) {
        const result = await fetchJSON<DomainRecord[]>("/servers/" + serverFilter + "/domains");
        return result;
      }
      return [] as DomainRecord[];
    },
    enabled: !!serverFilter,
  });

  const serversQuery = useQuery<Array<{ id: string; name: string }>>({
    queryKey: ["admin", "servers", "list"],
    queryFn: () => fetchJSON<Array<{ id: string; name: string }>>("/servers"),
  });

  const domains = useMemo(() => domainsQuery.data ?? [], [domainsQuery.data]);
  const servers = useMemo(() => serversQuery.data ?? [], [serversQuery.data]);

  const filteredDomains = domains.filter((d) =>
    !search || d.domain.toLowerCase().includes(search.toLowerCase())
  );

  const addMutation = useMutation({
    mutationFn: () =>
      postJSON<DomainRecord>("/servers/" + addForm.serverId + "/domains", {
        domain: addForm.domain,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["domains"] });
      setShowAddModal(false);
      setAddForm({ serverId: "", domain: "" });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: ({ serverId, id }: { serverId: string; id: string }) =>
      deleteJSON("/servers/" + serverId + "/domains/" + id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["domains"] });
    },
  });

  const verifyMutation = useMutation({
    mutationFn: (id: string) => postJSON<VerificationResult>("/domains/verify", { id }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["domains"] });
    },
  });

  const checkDNSMutation = useMutation({
    mutationFn: (data: { domain: string; expectedIp: string }) =>
      postJSON<DNSResult>("/domains/check-dns", data),
    onSuccess: (result) => {
      setDnsResult(result);
    },
  });

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Domain Management"
        sub="Manage custom domains for game servers. Verify ownership via HTTP challenge."
        action={
          <div className="flex gap-2">
            <Btn tone="ghost" onClick={() => setShowDNSModal(true)}>
              <Network size={14} /> Check DNS
            </Btn>
            <Btn size="sm" tone="primary" onClick={() => setShowAddModal(true)}>
              <Plus size={12} /> Add Domain
            </Btn>
          </div>
        }
      />

      <Card>
        <CardHeader title="Domains" icon={Globe} />
        <div className="flex items-center gap-3 p-4">
          <select
            className="h-9 w-64 rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none focus:border-[#dc2626]/60 focus:ring-1 focus:ring-[#dc2626]/30"
            value={serverFilter}
            onChange={(e) => {
              setServerFilter(e.target.value);
            }}
          >
            <option value="">Select a server...</option>
            {servers.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name} ({s.id})
              </option>
            ))}
          </select>
          <Input placeholder="Search domains..." value={search} onChange={setSearch} />
        </div>

        {!serverFilter ? (
          <EmptyState icon={Globe} message="Select a server to view its domains." />
        ) : domainsQuery.isLoading ? (
          <div className="p-8 text-center text-sm text-slate-500">Loading domains...</div>
        ) : filteredDomains.length === 0 ? (
          <EmptyState icon={Globe} message="No domains configured for this server." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Domain</th>
                  <th className="px-4 py-3">Type</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Verified At</th>
                  <th className="px-4 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filteredDomains.map((d) => (
                  <tr key={d.id} className="hover:bg-white/[0.02]">
                    <td className="px-4 py-3 font-mono text-xs font-medium text-slate-200">
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
                          <ShieldCheck size={14} className="text-emerald-400" />
                        ) : (
                          <ShieldAlert size={14} className="text-amber-400" />
                        )}
                        <Pill tone={d.verified ? "green" : "yellow"}>
                          {d.verified ? "Verified" : "Unverified"}
                        </Pill>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-400">
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
                            if (confirm("Remove domain " + d.domain + "?")) {
                              deleteMutation.mutate({ serverId: d.serverId, id: d.id });
                            }
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
          <div className="grid gap-4">
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1.5">Server</label>
              <select
                className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none focus:border-[#dc2626]/60 focus:ring-1 focus:ring-[#dc2626]/30"
                value={addForm.serverId}
                onChange={(e) => setAddForm({ ...addForm, serverId: e.target.value })}
              >
                <option value="">Select server...</option>
                {servers.map((s) => (
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
              <p className="text-xs text-blue-400">Wildcard domain detected. DNS verification will use test.{addForm.domain.replace("*.", "")}</p>
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
          <div className="grid gap-4">
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
              <div className={`p-4 rounded-lg border ${dnsResult.match ? "border-emerald-500/30 bg-emerald-500/10" : "border-amber-500/30 bg-amber-500/10"}`}>
                <p className={`text-sm font-medium ${dnsResult.match ? "text-emerald-400" : "text-amber-400"}`}>
                  {dnsResult.match ? "DNS matches expected IP" : "DNS mismatch or not verified"}
                </p>
                {dnsResult.error && <p className="text-xs text-red-400 mt-1">{dnsResult.error}</p>}
                {dnsResult.ips && dnsResult.ips.length > 0 && (
                  <p className="text-xs text-slate-400 mt-1">
                    Resolved IPs: {dnsResult.ips.join(", ")}
                  </p>
                )}
                {dnsResult.expectedIp && (
                  <p className="text-xs text-slate-500 mt-1">Expected: {dnsResult.expectedIp}</p>
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
    </div>
  );
}
