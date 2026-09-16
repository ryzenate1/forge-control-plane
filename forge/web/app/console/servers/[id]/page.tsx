"use client";
import { queryKeys } from "@/lib/api/query-keys";

import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { fetchServer } from "@/lib/api/servers";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";
import { LoadingSpinner } from "@/components/ui/loading-skeleton";
import { AdminErrorState } from "@/components/admin/admin-ui";

/**
 * /console/servers/[id] — overview tab of a workload inside the console shell.
 * Reuses ServerConsoleLayout (which renders the same view as /server/[id]) but
 * within the /console chrome. The sidebar tabs come from ConsoleNav via the
 * console-registry's workloadTabHref().
 */
export default function ConsoleServerOverviewPage() {
  const { id: serverId } = useParams<{ id: string }>();

  const serverQuery = useQuery({
    queryKey: queryKeys.servers.detail(serverId),
    queryFn: () => fetchServer(serverId),
    enabled: Boolean(serverId),
    staleTime: 30_000,
    retry: 1,
  });

  if (serverQuery.isPending) return <LoadingSpinner />;
  if (serverQuery.isError || !serverQuery.data) {
    return <AdminErrorState message="Server not found or access denied." />;
  }

  return <ServerConsoleLayout activeTab="overview">{(server) => <OverviewInline server={server} />}</ServerConsoleLayout>;
}

/* Lazy-import the overview view component to avoid circular deps */
import dynamic from "next/dynamic";
const OverviewView = dynamic(() => import("@/components/server/overview-view").then((m) => ({ default: m.OverviewView })), { ssr: false });

function OverviewInline({ server }: { server: import("@forge/shared-types").ApiServer }) {
  return <OverviewView server={server} />;
}
