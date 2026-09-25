"use client";

import { useEffect } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { AdminPageLayout, AdminPageHeader } from "@/components/admin/admin-ui";

/**
 * Legacy route — Database Services now live as a tab inside the unified
 * Databases page (`/admin/databases?tab=services` + Overview inventory).
 * This stub keeps old bookmarks/API docs working and forwards admins to
 * the single place where every created DB is shown.
 */
export default function AdminDatabaseServicesRedirect() {
  const router = useRouter();

  useEffect(() => {
    router.replace("/admin/databases?tab=services");
  }, [router]);

  return (
    <AdminPageLayout>
      <AdminPageHeader
        title="Database Services"
        description="Moved — services now live inside Databases → Services, with every DB visible under Overview."
      />
      <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-6 text-sm text-slate-300">
        <p>This page moved to keep one inventory instead of two.</p>
        <Link href="/admin/databases?tab=services" className="mt-3 inline-block rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-semibold text-white">
          Open Databases → Services
        </Link>
      </div>
    </AdminPageLayout>
  );
}
