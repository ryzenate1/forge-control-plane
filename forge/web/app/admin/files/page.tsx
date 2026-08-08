"use client";

import { HostFilesView } from "@/components/admin/host-files-view";
import { Card, AdminPageLayout, AdminPageHeader } from "@/components/admin/admin-ui";

export default function AdminFilesPage() {
  return (
    <AdminPageLayout>
      <AdminPageHeader title="Host File Manager" description="Browse and manage files on the host system" />
      <Card>
        <div className="p-4 sm:p-6">
          <HostFilesView />
        </div>
      </Card>
    </AdminPageLayout>
  );
}
