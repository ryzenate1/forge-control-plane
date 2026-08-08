"use client";

import { useQuery } from "@tanstack/react-query";
import { Folder } from "lucide-react";
import { type ApiMount, type ApiServer, fetchServerMounts } from "@/lib/api";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { errorMessage as message } from "@/lib/utils";
import { EmptyState } from "@/components/ui/primitives";
import { Skeleton } from "@/components/ui/loading-skeleton";


export function MountsView({ server }: { server: ApiServer }) {
  const ctx = useOptionalServerContext();
  const access = ctx?.access ?? { user: null, permissions: null, isOwner: false, isAdmin: false };
  const { data: mounts, isLoading, isError, error, refetch } = useQuery<ApiMount[]>({
    queryKey: ["server-mounts", server.id],
    queryFn: () => fetchServerMounts(server.id),
    enabled: hasServerPermission(access, "mount.read"),
  });

  if (isLoading) {
    return <div className="space-y-3"><div className="ui-card"><div className="flex items-center justify-between gap-3"><Skeleton className="h-4 w-32" /><Skeleton className="h-5 w-16" /></div><div className="mt-3 space-y-2"><Skeleton className="h-3 w-full" /><Skeleton className="h-3 w-2/3" /></div></div><div className="ui-card"><div className="flex items-center justify-between gap-3"><Skeleton className="h-4 w-40" /><Skeleton className="h-5 w-16" /></div><div className="mt-3 space-y-2"><Skeleton className="h-3 w-full" /><Skeleton className="h-3 w-3/4" /></div></div></div>;
  }

  if (isError) {
    return (
      <div className="ui-card">
        <div className="ui-alert ui-alert-error" role="alert">
          <p className="text-sm">{message(error, "Mounts could not be loaded.")}</p>
          <p className="mt-1 text-xs text-red-300/80">This usually means the daemon is unreachable or the server lacks the mount.read permission. Verify the daemon is online, then retry.</p>
          <button className="ui-button ui-button-secondary mt-3" onClick={() => void refetch()} type="button">Retry</button>
        </div>
      </div>
    );
  }

  if (!mounts || mounts.length === 0) {
    return (
      <EmptyState
        icon={<Folder size={20} />}
        title="No mounts assigned"
        description="This server does not have any mounts configured. Add mounts from the node or daemon configuration, then reload this page."
      />
    );
  }

  return (
    <div className="space-y-3">
      <h2 className="text-lg font-bold text-white">Server Mounts</h2>
      <div className="grid gap-3 sm:grid-cols-2">
        {mounts.map((mount: ApiMount) => (
          <div key={mount.id} className="ui-card">
            <div className="flex items-center justify-between gap-3">
              <h3 className="truncate font-medium text-white">{mount.name}</h3>
              {mount.readOnly !== false && (
                <span className="ui-status-pill ui-status-pill-warning">Read-only</span>
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
