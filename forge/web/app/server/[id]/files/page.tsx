"use client";

import { useState } from "react";
import { ContainerFilesView } from "@/components/server/container-files-view";
import { FilesView } from "@/components/server/files-view";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";

type FileSource = "volume" | "container";

export default function ServerFilesPage() {
  const [source, setSource] = useState<FileSource>("volume");

  return (
    <ServerConsoleLayout activeTab="files">
      {(server) => (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="inline-flex rounded-lg border border-[var(--line)] bg-[var(--surface-input)] p-1" role="tablist" aria-label="File source">
              <button
                aria-selected={source === "volume"}
                className={source === "volume" ? "rounded-md bg-[var(--brand)] px-3 py-1.5 text-xs font-bold text-white" : "rounded-md px-3 py-1.5 text-xs font-semibold text-[var(--text-subtle)] hover:text-[var(--text)]"}
                onClick={() => setSource("volume")}
                role="tab"
                type="button"
              >
                Server volume
              </button>
              <button
                aria-selected={source === "container"}
                className={source === "container" ? "rounded-md bg-[var(--brand)] px-3 py-1.5 text-xs font-bold text-white" : "rounded-md px-3 py-1.5 text-xs font-semibold text-[var(--text-subtle)] hover:text-[var(--text)]"}
                onClick={() => setSource("container")}
                role="tab"
                type="button"
              >
                Container filesystem
              </button>
            </div>
          </div>
          {source === "volume" ? <FilesView server={server} /> : <ContainerFilesView server={server} />}
        </div>
      )}
    </ServerConsoleLayout>
  );
}
