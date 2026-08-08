"use client";
import { AdminActivityLog } from "@/components/admin/AdminActivityLog";
import { AdminPageLayout } from "@/components/admin/admin-ui";

export default function AdminActivityPage() {
  return (
    <AdminPageLayout>
      <AdminActivityLog />
    </AdminPageLayout>
  );
}
