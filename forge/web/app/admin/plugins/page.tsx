"use client";

import { AdminPageLayout } from "@/components/admin/admin-ui";
import { AdminPlugins } from "@/components/admin/AdminPlugins";

export default function PluginsPage() {
  return (
    <AdminPageLayout>
      <AdminPlugins />
    </AdminPageLayout>
  );
}
