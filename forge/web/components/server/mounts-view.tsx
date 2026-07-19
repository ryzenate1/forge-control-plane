"use client";

import { useQuery } from "@tanstack/react-query";
import { Folder } from "lucide-react";
import { type ApiMount, type ApiServer, fetchServerMounts } from "@/modules/gameservers/api";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { errorMessage as message } from "@/lib/utils";


export function MountsView({ server }: { server: ApiServer }) {
  const ctx = useOptionalServerContext();
  const access = ctx?.access ?? { user: null, permissions: null, isOwner: false, isAdmin: false };
  const { data: mounts, isLoading, isError, error } = useQuery<ApiMount[]>({
    queryKey: ["server-mounts", server.id],
    queryFn: () => fetchServerMounts(server.id),
    enabled: hasServerPermission(access, "mount.read"),
  });

  if (isLoading) {
    return <div className="flex flex-col items-center justify-center py-16 text-slate-400"><div className="h-8 w-8 animate-spin rounded-full border-2 border-slate-600 border-t-red-500" /><p className="mt-4 text-sm">Loading mounts…</p></div>;
  }

  if (isError) {
    return <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-5 text-sm text-red-200" role="alert"><p>{message(error, "Mounts could not be loaded.")}</p></div>;
  }

  if (!mounts || mounts.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-slate-400">
        <Folder size={48} className="mb-4 opacity-30" />
        <p className="text-lg font-medium">No mounts assigned</p>
        <p className="mt-1 text-sm">This server does not have any mounts configured.</p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <h2 className="text-lg font-bold text-white">Server Mounts</h2>
      <div className="grid gap-3 sm:grid-cols-2">
        {mounts.map((mount: ApiMount) => (
          <div key={mount.id} className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-4">
            <div className="flex items-center justify-between">
              <h3 className="font-medium text-white">{mount.name}</h3>
              {mount.readOnly !== false && (
                <span className="rounded bg-amber-500/10 px-2 py-0.5 text-[10px] font-medium text-amber-300">Read-only</span>
              )}
            </div>
            {mount.description && <p className="mt-1 text-xs text-slate-400">{mount.description}</p>}
            <div className="mt-3 space-y-1 text-xs text-slate-400">
              <p><span className="text-slate-500">Source:</span> <code className="font-mono text-slate-300">{mount.source}</code></p>
              <p><span className="text-slate-500">Target:</span> <code className="font-mono text-slate-300">{mount.target}</code></p>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
