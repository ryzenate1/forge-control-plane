"use client";

import { FolderOpen } from "lucide-react";
import { HostFilesView } from "@/components/admin/host-files-view";
import { Card, CardHeader, AdminPageLayout, SectionHeader } from "@/components/admin/admin-ui";

export default function AdminFilesPage() {
  return (
    <AdminPageLayout>
      <SectionHeader title="Host Files" sub="Browse and manage files on a node." />
      <Card>
        <CardHeader title="Host Files" icon={FolderOpen} />
        <div className="p-4 sm:p-6">
          <HostFilesView />
        </div>
      </Card>
    </AdminPageLayout>
  );
}
