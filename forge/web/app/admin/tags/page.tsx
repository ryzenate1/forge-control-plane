"use client";

import { TagsManager } from "@/components/admin/tags-manager";
import { AdminPageLayout } from "@/components/admin/admin-ui";

export default function AdminTagsPage() {
  return (
    <AdminPageLayout>
      <TagsManager />
    </AdminPageLayout>
  );
}
