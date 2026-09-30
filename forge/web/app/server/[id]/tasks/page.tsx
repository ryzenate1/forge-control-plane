"use client";


import { ScheduledTasksView } from "@/components/server/scheduled-tasks-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerScheduledTasksPage() {

  return (
    <ServerConsoleLayout activeTab="tasks">
      {(server) => <ScheduledTasksView server={server} />}
    </ServerConsoleLayout>
  );
}
