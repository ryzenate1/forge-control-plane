"use client";

import { AdminNotifications } from "@/components/admin/AdminNotifications";
import { NotificationsManager } from "@/components/admin/notifications-manager";
import { AdminPageLayout } from "@/components/admin/admin-ui";

export default function NotificationsPage() {
  return (
    <AdminPageLayout>
      <div className="space-y-8">
        <NotificationsManager />
        {/* Legacy admin console: global channels, per-channel event toggles
            and the delivery log. The engine above is the user-scoped view. */}
        <AdminNotifications />
      </div>
    </AdminPageLayout>
  );
}
