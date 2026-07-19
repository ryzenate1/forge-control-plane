"use client";


import { NetworkView } from "@/components/server/network-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerNetworkPage() {

  return (
    <ServerConsoleLayout activeTab="network">
      {(server) => <NetworkView server={server} />}
    </ServerConsoleLayout>
  );
}
