"use client";


import { ServerUsersView } from "@/components/server/users-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerUsersPage() {

  return (
    <ServerConsoleLayout activeTab="users">
      {(server) => <ServerUsersView server={server} />}
    </ServerConsoleLayout>
  );
}
