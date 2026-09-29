"use client";

import { AdminMounts } from "@/components/admin/AdminMounts";

/**
 * Bare wrapper: `AdminMounts` owns its own `AdminPageLayout`, like every other
 * page in this group. It used to be wrapped here as well, so the mount list
 * nested two page frames and inherited two different vertical rhythms
 * (`space-y-6` inside `AdminPageLayout`'s `space-y-5`).
 */
export default function AdminMountsPage() {
  return <AdminMounts />;
}
