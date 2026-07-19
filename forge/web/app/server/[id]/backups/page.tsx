"use client";


import { BackupsView } from "@/components/server/backups-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerBackupsPage() {

  return (
    <ServerConsoleLayout activeTab="backups">
      {(server) => <BackupsView server={server} />}
    </ServerConsoleLayout>
  );
}
