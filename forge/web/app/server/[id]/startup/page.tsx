"use client";


import { StartupView } from "@/components/server/startup-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerStartupPage() {

  return (
    <ServerConsoleLayout activeTab="startup">
      {(server) => <StartupView server={server} />}
    </ServerConsoleLayout>
  );
}
