"use client";

import { AdminWebhooks } from "@/components/admin/AdminWebhooks";
import { AdminPageLayout } from "@/components/admin/admin-ui";

export default function WebhooksPage() {
  return (
    <AdminPageLayout>
      <AdminWebhooks />
    </AdminPageLayout>
  );
}
