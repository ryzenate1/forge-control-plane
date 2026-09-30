"use client";


import { ServerDomainsView } from "@/components/server/domains-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerDomainsPage() {

  return (
    <ServerConsoleLayout activeTab="domains">
      {(server) => <ServerDomainsView server={server} />}
    </ServerConsoleLayout>
  );
}
