"use client";


import { FilesView } from "@/components/server/files-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerFilesPage() {


  return (
    <ServerConsoleLayout activeTab="files">
      {(server) => <FilesView server={server} />}
    </ServerConsoleLayout>
  );
}
