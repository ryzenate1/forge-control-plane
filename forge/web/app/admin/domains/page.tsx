"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { Globe, Plus, Trash2, ShieldCheck, ShieldAlert, RotateCw, Network, CircleAlert } from "lucide-react";
import { fetchServers } from "@/lib/api/servers";
import { fetchServerDomains, addServerDomain, removeServerDomain, verifyDomain, checkDNS as checkDNSApi } from "@/lib/api/domains";
import { AdminPageLayout, AdminSelect, AdminTable, AdminTBody, AdminTd, AdminTh, AdminTHead, AdminTr, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, AdminLoadingState, AdminErrorState } from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { formatDate } from "@/lib/utils";
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

/** A hostname goes into gateway config, so it is checked before submit rather
 *  than rejected by the backend one click later. A wildcard is only meaningful
 *  as the leading label, which is what a TLS SAN can express. */
const HOSTNAME = /^(\*\.)?([a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$/i;

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

  const selectedServer = servers.find((s) => s.id === serverFilter);
  const addDomainError = addForm.domain && !HOSTNAME.test(addForm.domain.trim())
    ? "Enter a hostname such as example.com or *.example.com."
    : "";

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
        status={<FreshnessBadge state={sourceState(domainsQuery)} />}
        action={
          <div className="flex gap-2">
            <Link href="/admin/dns"><Btn tone="ghost" className="border border-[color-mix(in_srgb,var(--brand)_20%,transparent)] hover:bg-[color-mix(in_srgb,var(--brand)_10%,transparent)]"><ShieldCheck size={12} /> DNS Providers</Btn></Link>
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
        <CardHeader
          action={selectedServer ? <Link className="text-xs text-text-subtle underline hover:text-text" href={`/admin/servers/${selectedServer.id}`}>{selectedServer.name}</Link> : undefined}
          icon={Globe}
          title={selectedServer ? `Domains · ${selectedServer.name}` : "Domains"}
        />
        <div className="flex items-center gap-3 p-4">
          <div className="w-64"><AdminSelect label="Server" value={serverFilter} onChange={setServerFilter} placeholder="Select a server…" options={Array.isArray(servers) ? servers.map((s) => ({ value: s.id, label: `${s.name} (${s.id})` })) : []} /></div>
          <Input label="Search" placeholder="Search domains…" value={search} onChange={setSearch} />
        </div>

        {!serverFilter ? (
          <EmptyState icon={Globe} title="No server selected" message="Domains are stored per server, so pick a server above to list its domains. A fleet-wide domain view needs a server-less list endpoint." />
        ) : domainsQuery.isLoading ? (
          <div className="p-4"><AdminLoadingState label="Loading domains…" /></div>
        ) : domainsQuery.isError ? (
          <div className="p-4"><AdminErrorState message={domainsQuery.error instanceof Error ? domainsQuery.error.message : "Failed to load domains"} retry={() => void domainsQuery.refetch()} /></div>
        ) : filteredDomains.length === 0 ? (
          <EmptyState icon={Globe} title={search ? "No domains match the search" : "No domains"} message={search ? `No domain on ${selectedServer?.name ?? "this server"} contains “${search}”.` : "This server has no custom domains configured."} />
        ) : (
          <AdminTable label={`Domains for ${selectedServer?.name ?? serverFilter}`}>
            <AdminTHead><AdminTh>Domain</AdminTh><AdminTh>Type</AdminTh><AdminTh>Status</AdminTh><AdminTh>Verified At</AdminTh><AdminTh></AdminTh></AdminTHead>
            <AdminTBody>
                {filteredDomains.map((d) => (
                  <AdminTr key={d.id}>
                    <AdminTd className="font-mono text-xs font-medium text-text">
                      {d.domain}
                    </AdminTd>
                    <AdminTd>
                      <Pill tone={d.wildcard ? "blue" : "neutral"}>
                        {d.wildcard ? "Wildcard" : "Standard"}
                      </Pill>
                    </AdminTd>
                    <AdminTd>
                      <div className="flex items-center gap-1.5">
                        {d.verified ? (
                          <ShieldCheck aria-hidden="true" className="text-ok" size={14} />
                        ) : (
                          <ShieldAlert aria-hidden="true" className="text-warn" size={14} />
                        )}
                        <Pill tone={d.verified ? "green" : "yellow"}>
                          {d.verified ? "Verified" : "Not verified"}
                        </Pill>
                      </div>
                    </AdminTd>
                    <AdminTd className="text-xs text-text-subtle">
                      {d.verified ? (d.verifiedAt ? formatDate(d.verifiedAt) : "Verified — time not reported") : "Never verified"}
                    </AdminTd>
                    <AdminTd>
                      <div className="flex gap-1">
                        {!d.verified && (
                          <Btn
                            size="sm"
                            tone="ghost"
                            loading={verifyMutation.isPending && verifyMutation.variables === d.id}
                            onClick={() => verifyMutation.mutate(d.id)}
                            disabled={verifyMutation.isPending && verifyMutation.variables !== d.id}
                          >
                            <RotateCw size={12} /> Verify
                          </Btn>
                        )}
                        <Btn
                          ariaLabel={`Remove domain ${d.domain}`}
                          size="sm"
                          tone="danger"
                          disabled={deleteMutation.isPending && deleteMutation.variables?.id !== d.id}
                          loading={deleteMutation.isPending && deleteMutation.variables?.id === d.id}
                          onClick={() => {
                            void (async () => { if (await confirm({ title: `Remove domain ${d.domain}?`, description: `${selectedServer?.name ?? d.serverId} stops serving ${d.domain}, and any gateway route, certificate or header override attached to it stops matching. This cannot be undone.`, danger: true, confirmLabel: "Remove" })) {
                              deleteMutation.mutate({ serverId: d.serverId, id: d.id });
                            } })();
                          }}
                        >
                          <Trash2 size={12} />
                        </Btn>
                      </div>
                    </AdminTd>
                  </AdminTr>
                ))}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>

      {showAddModal && (
        <Modal onClose={() => setShowAddModal(false)} title="Add Domain">
          <div className="space-y-4">
            <AdminSelect
              disabled={!Array.isArray(servers) || servers.length === 0}
              label="Server"
              onChange={(v) => setAddForm({ ...addForm, serverId: v })}
              options={Array.isArray(servers) ? servers.map((s) => ({ value: s.id, label: s.name })) : []}
              placeholder="Select server…"
              value={addForm.serverId}
            />
            <Input
              label="Domain"
              mono
              onChange={(v) => setAddForm({ ...addForm, domain: v })}
              placeholder="example.com or *.example.com"
              value={addForm.domain}
            />
            {addDomainError ? <p className="text-xs text-danger">{addDomainError}</p> : null}
            {addForm.domain.startsWith("*.") && !addDomainError ? (
              <p className="ui-hint">A wildcard covers one label below {addForm.domain.replace("*.", "the apex ")}; verify it by resolving {addForm.domain.replace("*.", "test.")}.</p>
            ) : null}
          </div>
          <ModalFooter
            confirmLabel={addMutation.isPending ? "Adding…" : "Add Domain"}
            disabled={addMutation.isPending || !addForm.serverId || !addForm.domain.trim() || !!addDomainError}
            onCancel={() => setShowAddModal(false)}
            onConfirm={() => addMutation.mutate()}
          />
        </Modal>
      )}

      {showDNSModal && (
        <Modal onClose={() => { setShowDNSModal(false); setDnsResult(null); }} title="Check DNS Resolution">
          <div className="space-y-4">
            <Input
              label="Domain"
              mono
              onChange={(v) => setDnsForm({ ...dnsForm, domain: v })}
              placeholder="example.com"
              value={dnsForm.domain}
            />
            <Input
              label="Expected IP (optional)"
              mono
              onChange={(v) => setDnsForm({ ...dnsForm, expectedIp: v })}
              placeholder="1.2.3.4"
              value={dnsForm.expectedIp}
            />
            {dnsResult && (() => {
              // Four outcomes, four different colours: a failed probe is not a
              // warning about the domain, and an unmatched IP is not an error.
              const verdict = dnsResult.error
                ? { tone: "danger", text: `The lookup failed: ${dnsResult.error}`, className: "ui-alert ui-alert-danger" }
                : !dnsResult.resolved || !dnsResult.ips || dnsResult.ips.length === 0
                  ? { tone: "warn", text: "No address was returned for this hostname — it does not resolve yet.", className: "ui-alert ui-alert-warning" }
                  : dnsResult.expectedIp && !dnsResult.match
                    ? { tone: "warn", text: "It resolves, but not to the expected address.", className: "ui-alert ui-alert-warning" }
                    : { tone: "ok", text: "DNS matches the expected address.", className: "ui-alert ui-alert-success" };
              return (
                <div className={verdict.className}>
                  <p className="flex items-center gap-2 text-sm font-medium">
                    {verdict.tone === "danger" ? <CircleAlert aria-hidden="true" size={14} /> : verdict.tone === "ok" ? <ShieldCheck aria-hidden="true" size={14} /> : <ShieldAlert aria-hidden="true" size={14} />}
                    {verdict.text}
                  </p>
                  {dnsResult.ips && dnsResult.ips.length > 0 ? (
                    <p className="mt-1 font-mono text-xs">Resolved: {dnsResult.ips.join(", ")}</p>
                  ) : null}
                  {dnsResult.expectedIp ? <p className="mt-1 text-xs">Expected: {dnsResult.expectedIp}</p> : null}
                </div>
              );
            })()}
          </div>
          <ModalFooter
            confirmLabel={checkDNSMutation.isPending ? "Checking…" : "Check DNS"}
            disabled={checkDNSMutation.isPending || !dnsForm.domain.trim() || !HOSTNAME.test(dnsForm.domain.trim())}
            onCancel={() => { setShowDNSModal(false); setDnsResult(null); }}
            onConfirm={() => checkDNSMutation.mutate(dnsForm)}
          />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}

