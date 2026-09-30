"use client";

import { AdminMonitoring } from "@/components/admin/AdminMonitoring";
import { AdminPageLayout } from "@/components/admin/admin-ui";

export default function AdminMonitoringPage() {
  return (
    <AdminPageLayout className="space-y-6">
      <AdminMonitoring />
    </AdminPageLayout>
  );
}
