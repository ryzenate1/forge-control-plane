"use client";

import { AdminEndpoints } from "@/components/admin/AdminEndpoints";
import { AdminPageLayout } from "@/components/admin/admin-ui";

export default function AdminEndpointsPage() {
  return (
    <AdminPageLayout>
      <AdminEndpoints />
    </AdminPageLayout>
  );
}
