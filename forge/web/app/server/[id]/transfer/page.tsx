"use client";

import { TransferView } from "@/components/server/transfer-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerTransferPage() {
  return (
    <ServerConsoleLayout activeTab="transfer">
      {(server) => <TransferView server={server} />}
    </ServerConsoleLayout>
  );
}
