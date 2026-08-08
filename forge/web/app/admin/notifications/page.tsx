"use client";

import { AdminNotifications } from "@/components/admin/AdminNotifications";
import { AdminPageLayout } from "@/components/admin/admin-ui";

export default function NotificationsPage() {
  return (
    <AdminPageLayout>
      <AdminNotifications />
    </AdminPageLayout>
  );
}
