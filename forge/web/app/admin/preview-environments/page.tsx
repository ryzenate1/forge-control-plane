"use client";

import { AdminPageLayout } from "@/components/admin/admin-ui";
import { PreviewDeploymentsView } from "@/components/admin/preview-deployments-view";

export default function PreviewEnvironmentsPage() {
  return (
    <AdminPageLayout>
      <PreviewDeploymentsView />
    </AdminPageLayout>
  );
}
