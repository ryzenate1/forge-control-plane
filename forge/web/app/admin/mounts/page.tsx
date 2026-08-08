"use client";

import { AdminMounts } from "@/components/admin/AdminMounts";
import { AdminPageLayout } from "@/components/admin/admin-ui";

export default function AdminMountsPage() {
  return (
    <AdminPageLayout>
      <AdminMounts />
    </AdminPageLayout>
  );
}
