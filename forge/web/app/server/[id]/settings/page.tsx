"use client";


import { ServerSettingsView } from "@/components/server/settings-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerSettingsPage() {

  return (
    <ServerConsoleLayout activeTab="settings">
      {(server) => <ServerSettingsView server={server} />}
    </ServerConsoleLayout>
  );
}
