"use client";

import { BuildsView } from "@/components/server/builds-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerBuildsPage() {
  return (
    <ServerConsoleLayout activeTab="builds">
      {(server) => <BuildsView server={server} />}
    </ServerConsoleLayout>
  );
}
