"use client";

import { MountsView } from "@/components/server/mounts-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerMountsPage() {
  return <ServerConsoleLayout activeTab="mounts">{(server) => <MountsView server={server} />}</ServerConsoleLayout>;
}
