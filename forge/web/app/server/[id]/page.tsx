"use client";

import { ConsoleView } from "@/components/server/console-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerConsolePage() {
  return <ServerConsoleLayout activeTab="console">{(server) => <ConsoleView server={server} />}</ServerConsoleLayout>;
}
