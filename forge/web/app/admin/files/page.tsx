"use client";

import { HostFilesView } from "@/components/admin/host-files-view";
import { Card, AdminPageLayout, AdminPageHeader } from "@/components/admin/admin-ui";
import { FolderSearch, Server, Upload } from "lucide-react";

export default function AdminFilesPage() {
  return (
    <AdminPageLayout>
      <AdminPageHeader
        info={{
          description: "Browse and edit the filesystem of one named Beacon node. Paths are node-local — switching nodes returns to root.",
          eyebrow: "Architecture & Semantics",
          sections: [
            {
              title: "One named node",
              icon: Server,
              content:
                "Every listing, edit, upload and permission change carries the node it targets; the host file API rejects a request that does not name one, so this page fetches nothing until you pick a node. A path on one node means nothing on another, so node switches reset directory, selection and any open editor.",
            },
            {
              title: "Uploads",
              icon: Upload,
              content:
                "Uploads stream through the panel to the node, with drag-and-drop and progress for batches. Pull-from-URL downloads here in the browser and then uploads the result into the current directory.",
            },
            {
              title: "No host archive surface",
              icon: FolderSearch,
              content:
                "Beacon exposes no archive or decompress endpoint for host paths, so this page offers neither — the file actions here are read, write, rename, copy, delete, permissions, download and upload. For tar.gz lifecycle use a server container's file manager.",
            },
          ],
          title: "Host files",
          triggerLabel: "About Host Files",
        }}
      />
      <Card>
        <div className="p-4 sm:p-5">
          <HostFilesView />
        </div>
      </Card>
    </AdminPageLayout>
  );
}
