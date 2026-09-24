"use client";

import { OverviewView } from "@/components/server/overview-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerOverviewPage() {
  return (
    <ServerConsoleLayout activeTab="overview">
      {(server) => <OverviewView server={server} />}
    </ServerConsoleLayout>
  );
}
