"use client";
import { queryKeys } from "@/lib/api/query-keys";

import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { fetchServer } from "@/lib/api/servers";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";
import { LoadingSpinner } from "@/components/ui/loading-skeleton";
import { AdminErrorState } from "@/components/admin/admin-ui";
import { ConsoleView } from "@/components/server/console-view";

export default function Page() {
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

  return (
    <ServerConsoleLayout activeTab="terminal" >
      {(server) => <ConsoleView server={server} />}
    </ServerConsoleLayout>
  );
}
