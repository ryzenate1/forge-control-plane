"use client";

import { useState } from "react";
import { Globe, Plus, ShieldAlert, ShieldCheck, Trash2 } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  type ApiServer,
  addServerDomain,
  checkDNS,
  fetchServerDomains,
  removeServerDomain,
  verifyDomain,
  type CheckDNSResult,
  type ServerDomain,
} from "@/lib/api";
import { PanelCard } from "@/components/ui/panel-card";
import { EmptyState } from "@/components/ui/primitives";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { Skeleton } from "@/components/ui/loading-skeleton";
import { useToast } from "@/components/ui/toast";
import { useOptionalServerContext } from "./server-context";

/**
 * ServerDomainsView — per-server custom domains.
 *
 * The backend routes are admin-gated (list/remove require the admin role),
 * so this tab is marked `adminOnly` in the workload registry and never
 * renders for customers. The view itself still refuses to fetch without an
 * admin context rather than showing an empty list that looks like "no
 * domains".
 */
export function ServerDomainsView({ server }: { server?: ApiServer }) {
  const context = useOptionalServerContext();
  const isAdmin = Boolean(context?.access.isAdmin);
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [domain, setDomain] = useState("");
  const [dnsResults, setDnsResults] = useState<Record<string, CheckDNSResult>>({});

  const serverId = server?.id ?? "";
  const domainsQuery = useQuery({
    queryKey: ["server-domains", serverId],
    queryFn: () => fetchServerDomains(serverId),
    enabled: Boolean(serverId) && isAdmin,
  });

  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ["server-domains", serverId] });

  const addMutation = useMutation({
    mutationFn: () => addServerDomain(serverId, domain.trim().toLowerCase()),
    onSuccess: () => {
      setDomain("");
      invalidate();
      toast({ tone: "success", title: "Domain added", message: "Add the verification token to your DNS, then verify." });
    },
    onError: (error) => toast({ tone: "error", title: "Could not add domain", message: error instanceof Error ? error.message : "Check the domain and try again." }),
  });

  const removeMutation = useMutation({
    mutationFn: (id: string) => removeServerDomain(serverId, id),
    onSuccess: () => {
      invalidate();
      toast({ tone: "success", title: "Domain removed" });
    },
    onError: (error) => toast({ tone: "error", title: "Could not remove domain", message: error instanceof Error ? error.message : "Try again." }),
  });

  const verifyMutation = useMutation({
    mutationFn: (id: string) => verifyDomain(id),
    onSuccess: (result) => {
      invalidate();
      if (result.verified) {
        toast({ tone: "success", title: "Domain verified" });
      } else {
        toast({ tone: "error", title: "Verification failed", message: result.error ?? result.message ?? "Ownership could not be confirmed." });
      }
    },
    onError: (error) => toast({ tone: "error", title: "Verification failed", message: error instanceof Error ? error.message : "Try again." }),
  });

  const dnsMutation = useMutation({
    mutationFn: (entry: ServerDomain) => checkDNS(entry.domain, expectedIp(server?.allocation) ?? undefined),
    onSuccess: (result, entry) => setDnsResults((current) => ({ ...current, [entry.id]: result })),
    onError: (error, entry) => toast({ tone: "error", title: `DNS check failed for ${entry.domain}`, message: error instanceof Error ? error.message : "Try again." }),
  });

  if (!isAdmin) {
    return (
      <PanelCard title="Domains" icon={Globe}>
        <p className="text-sm text-amber-200">Domain management requires an administrator account.</p>
      </PanelCard>
    );
  }

  const domains = domainsQuery.data ?? [];
  const busy = addMutation.isPending || removeMutation.isPending || verifyMutation.isPending || dnsMutation.isPending;

  return (
    <div className="space-y-5">
      {renderConfirm()}
      <PanelCard title="Custom Domains" icon={Globe}>
        <div className="space-y-4">
          <div className="flex flex-col gap-2 sm:flex-row">
            <input
              aria-label="Domain name"
              className="ui-input flex-1"
              disabled={addMutation.isPending}
              onChange={(event) => setDomain(event.target.value)}
              placeholder="play.example.com"
              value={domain}
            />
            <button
              className="ui-button ui-button-primary"
              disabled={addMutation.isPending || domain.trim() === ""}
              onClick={() => addMutation.mutate()}
              type="button"
            >
              <Plus size={15} /> {addMutation.isPending ? "Adding…" : "Add domain"}
            </button>
          </div>
          {domainsQuery.isLoading ? <div className="space-y-3">{Array.from({ length: 2 }).map((_, index) => <Skeleton className="h-20 w-full" key={index} />)}</div> : null}
          {domainsQuery.isError ? <p className="text-sm text-red-300">Domains could not be loaded. Check the API connection and try again.</p> : null}
          {!domainsQuery.isLoading && !domainsQuery.isError && domains.length === 0 ? (
            <EmptyState icon={<Globe size={20} />} title="No custom domains" description="Add a domain above, point its DNS at this server, then verify ownership." />
          ) : null}
          <div className="space-y-3">
            {domains.map((entry) => {
              const dns = dnsResults[entry.id];
              return (
                <div className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-3" key={entry.id}>
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex min-w-0 flex-wrap items-center gap-2">
                      <span className="truncate font-semibold text-slate-100">{entry.domain}</span>
                      {entry.verified ? (
                        <span className="ui-status-pill ui-status-pill-success">Verified</span>
                      ) : (
                        <span className="ui-status-pill ui-status-pill-warning">Unverified</span>
                      )}
                    </div>
                    <div className="flex items-center gap-2">
                      {!entry.verified ? (
                        <button className="ui-button ui-button-secondary" disabled={busy} onClick={() => verifyMutation.mutate(entry.id)} type="button">
                          <ShieldCheck size={13} /> Verify
                        </button>
                      ) : null}
                      <button className="ui-button ui-button-secondary" disabled={busy} onClick={() => dnsMutation.mutate(entry)} type="button">
                        Check DNS
                      </button>
                      <button
                        aria-label={`Remove ${entry.domain}`}
                        className="ui-button ui-button-danger"
                        disabled={busy}
                        onClick={async () => {
                          if (await confirm({ title: `Remove ${entry.domain}?`, description: "Traffic will stop routing to this server. This cannot be undone.", danger: true, confirmLabel: "Remove" })) removeMutation.mutate(entry.id);
                        }}
                        type="button"
                      >
                        <Trash2 size={14} />
                      </button>
                    </div>
                  </div>
                  {!entry.verified && entry.verificationToken ? (
                    <p className="mt-2 break-all font-mono text-xs text-slate-400">
                      TXT verification token: <span className="text-slate-200">{entry.verificationToken}</span>
                    </p>
                  ) : null}
                  {dns ? (
                    <p className="mt-2 text-xs text-slate-400">
                      {dns.resolved ? (
                        <>DNS resolves{dns.ips?.length ? ` to ${dns.ips.join(", ")}` : ""}{dns.expectedIp ? ` (expected ${dns.expectedIp}${dns.match ? ", match" : ", mismatch"})` : ""}.</>
                      ) : (
                        <span className="inline-flex items-center gap-1 text-amber-200"><ShieldAlert size={13} /> {dns.error ?? dns.message ?? "DNS is not pointing at this server yet."}</span>
                      )}
                    </p>
                  ) : null}
                </div>
              );
            })}
          </div>
        </div>
      </PanelCard>
    </div>
  );
}

/** Best-effort expected IP from the server allocation (host[:port]). */
function expectedIp(allocation?: string | null): string | null {
  if (!allocation) return null;
  const host = allocation.trim().split(":")[0];
  if (!host || host === "0.0.0.0") return null;
  return host;
}
