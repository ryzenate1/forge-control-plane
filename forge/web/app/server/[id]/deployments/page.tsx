"use client";

import { DeploymentsView } from "@/components/server/deployments-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";
import type { ServerTab } from "@/components/server/server-nav";

export default function ServerDeploymentsPage() {
  return (
    <ServerConsoleLayout activeTab={"deployments" as ServerTab}>
      {(server) => <DeploymentsView server={server} />}
    </ServerConsoleLayout>
  );
}
