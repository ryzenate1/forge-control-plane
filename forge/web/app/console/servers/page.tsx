"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Plus, Search, Server } from "lucide-react";
import { fetchServers } from "@/lib/api/servers";
import type { ApiServer } from "@forge/shared-types";
import { Card, Pill, SectionHeader } from "@/components/admin/admin-ui";
import { LoadingSpinner } from "@/components/ui/loading-skeleton";
import { SearchInput } from "@/components/ui/primitives";

/**
 * /console/servers — customer-facing game server list.
 * Mirrors /servers but rendered inside the console shell with richer actions.
 */
export default function ConsoleServersPage() {
  const [search, setSearch] = useState("");

  const { data: servers = [], isLoading } = useQuery({
    queryKey: ["servers"],
    queryFn: fetchServers,
    staleTime: 30_000,
    refetchInterval: 30_000,
    retry: 1,
  });

  const filtered = useMemo(() => {
    if (!search.trim()) return servers;
    const q = search.toLowerCase();
    return servers.filter((s) => s.name.toLowerCase().includes(q) || s.id.includes(q));
  }, [servers, search]);

  if (isLoading) return <LoadingSpinner />;

  return (
    <div className="space-y-5">
      <SectionHeader
        title="Game Servers"
        sub={`${servers.length} server${servers.length !== 1 ? "s" : ""} provisioned`}
        action={
          <Link href="/admin/servers?new=true" className="inline-flex items-center gap-2 rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-bold text-white transition-colors hover:bg-[var(--brand-dark)]">
            <Plus size={14} /> New Server
          </Link>
        }
      />

      <SearchInput value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search servers…" />

      {filtered.length === 0 ? (
        <Card>
          <div className="flex flex-col items-center gap-3 py-12 text-center">
            <Server size={32} className="text-slate-600" />
            <p className="text-sm text-slate-400">{search ? "No servers match your search." : "No servers yet. Create one to get started."}</p>
          </div>
        </Card>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          {filtered.map((server) => (
            <ServerCard key={server.id} server={server} />
          ))}
        </div>
      )}
    </div>
  );
}

function ServerCard({ server }: { server: ApiServer }) {
  const statusTone = server.suspended ? "danger" : server.status === "running" ? "success" : server.status === "installing" ? "warning" : "neutral";
  const statusLabel = server.suspended ? "Suspended" : server.transferring ? "Transferring" : server.status || "Offline";

  return (
    <Link href={`/console/servers/${server.id}`} className="group">
      <Card className="h-full transition-all group-hover:border-red-400/20 group-hover:bg-white/[0.02]">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold text-white group-hover:text-red-200">{server.name}</p>
            <p className="mt-0.5 truncate font-mono text-[11px] text-slate-500">{server.allocation || server.id}</p>
          </div>
          <Pill tone={statusTone as "success" | "danger" | "warning" | "neutral"}>{statusLabel}</Pill>
        </div>
        <div className="mt-3 flex items-center gap-3 text-[11px] text-slate-500">
          {server.node && <span>Node: {server.node}</span>}
          {server.template && <span>· {server.template}</span>}
        </div>
      </Card>
    </Link>
  );
}
