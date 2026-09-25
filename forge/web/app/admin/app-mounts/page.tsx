"use client";

import { AppMountsManager } from "@/components/admin/app-mounts-manager";
import { AdminPageLayout } from "@/components/admin/admin-ui";

export default function AdminAppMountsPage() {
  return (
    <AdminPageLayout>
      <AppMountsManager />
    </AdminPageLayout>
  );
}
