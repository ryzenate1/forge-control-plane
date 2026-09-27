"use client";

import { ConsoleView } from "@/components/server/console-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerConsolePage() {
  // The tab is named "terminal"; its route segment is the older "console".
  return <ServerConsoleLayout activeTab="terminal">{(server) => <ConsoleView server={server} />}</ServerConsoleLayout>;
}
