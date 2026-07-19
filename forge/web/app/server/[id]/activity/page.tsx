"use client";


import { ActivityView } from "@/components/server/activity-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerActivityPage() {

  return (
    <ServerConsoleLayout activeTab="activity">
      {(server) => <ActivityView server={server} />}
    </ServerConsoleLayout>
  );
}
