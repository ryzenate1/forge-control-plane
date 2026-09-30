"use client";

import { ResourceLimitsView } from "@/components/server/resource-limits-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerResourceLimitsPage() {
  return (
    <ServerConsoleLayout activeTab="resource-limits">
      {(server) => <ResourceLimitsView server={server} />}
    </ServerConsoleLayout>
  );
}
