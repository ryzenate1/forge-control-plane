"use client";

import { AdminSettings } from "@/components/admin/AdminSettings";
import { AdminPageLayout } from "@/components/admin/admin-ui";

export default function AdminSettingsPage() {
  return (
    <AdminPageLayout>
      <AdminSettings />
    </AdminPageLayout>
  );
}
