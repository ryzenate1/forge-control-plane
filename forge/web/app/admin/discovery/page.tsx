"use client";

import { AdminDiscovery } from "@/components/admin/AdminDiscovery";
import { AdminPageLayout } from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";

export default function AdminDiscoveryPage() {
  return (
    <AdminPageLayout>
      <AdminDiscovery />
      <OfflineBanner onRetry={() => window.location.reload()} />
    </AdminPageLayout>
  );
}
