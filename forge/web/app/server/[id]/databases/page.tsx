"use client";


import { DatabasesView } from "@/components/server/databases-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerDatabasesPage() {

  return (
    <ServerConsoleLayout activeTab="databases">
      {(server) => <DatabasesView server={server} />}
    </ServerConsoleLayout>
  );
}
