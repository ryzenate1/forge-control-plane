"use client";


import { SchedulesView } from "@/components/server/schedules-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerSchedulesPage() {

  return (
    <ServerConsoleLayout activeTab="schedules">
      {(server) => <SchedulesView server={server} />}
    </ServerConsoleLayout>
  );
}
