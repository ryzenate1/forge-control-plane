"use client";

import { DeploymentHistoryView } from "@/components/server/deployment-history-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";
import type { ServerTab } from "@/components/server/server-nav";

export default function ServerDeploymentsPage() {
  return (
    <ServerConsoleLayout activeTab={"deployments" as ServerTab}>
      {(server) => <DeploymentHistoryView server={server} />}
    </ServerConsoleLayout>
  );
}
