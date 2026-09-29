"use client";

import { useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { Plus, RefreshCw } from "lucide-react";
import { AdminDatabases } from "@/components/admin/AdminDatabases";
import { DBContainerView } from "@/components/database/container-view";
import { ManagedDatabaseView } from "@/components/database/managed-database-view";
import { DatabaseServicesView } from "@/components/database/database-services-view";
import { CreateDatabaseModal, DatabasesOverview, type DatabaseTab } from "@/components/database/databases-overview";
import { AdminPageLayout, AdminTabs, Btn, SectionHeader } from "@/components/admin/admin-ui";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
import { OfflineBanner } from "@/components/shared/states-offline";

/**
 * Databases is one route with five tabs.
 *
 * The frame — and therefore the `<h1>`, the description and the header glyph — is
 * `SectionHeader`, which resolves all three from `admin-registry.ts`. This page
 * used to hand-roll its own `<h1>` **and** a `Database` chip **and** a second
 * breadcrumb, while each active tab rendered another `SectionHeader` on top of
 * it: two `<h1>`s per route, the visible one changing whenever a tab changed. The
 * tab views now head themselves with `<h2>` (`AdminSection`), so there is exactly
 * one page title, and it never disagrees with the sidebar row.
 *
 * The selected tab is a URL (`?tab=`), the same mechanism
 * `app/admin/apps/[id]/page.tsx` already uses. It was plain `useState` before,
 * which meant no database view could be deep-linked or restored after a reload,
 * and the `/admin/database-services` stub's `?tab=services` forward landed on
 * Overview while promising Services.
 */
const TABS: Array<{ id: DatabaseTab; label: string }> = [
  { id: "overview", label: "Overview" },
  { id: "hosts", label: "Database Hosts" },
  { id: "containers", label: "DB Containers" },
  { id: "managed", label: "Managed DBs" },
  { id: "services", label: "Services" },
];

/**
 * Every list query this route reads. Refreshing invalidates these rather than
 * remounting the active tab, so the operator's filters, sort and page survive the
 * refetch and only the data moves underneath them.
 */
const DATABASE_LIST_KEYS = [
  "database-hosts",
  "db-containers",
  "managed-databases",
  "database-services",
  "service-templates",
  "databases-overview-catalog",
  "databases-overview-serverdb",
];

export default function AdminDatabasesPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const qc = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  const rawTab = searchParams.get("tab");
  const activeTab: DatabaseTab = rawTab && TABS.some((t) => t.id === rawTab) ? (rawTab as DatabaseTab) : "overview";

  const setActiveTab = (tab: DatabaseTab) => {
    router.replace(`/admin/databases?tab=${encodeURIComponent(tab)}`, { scroll: false });
  };

  async function refresh() {
    setRefreshing(true);
    try {
      await Promise.all(DATABASE_LIST_KEYS.map((key) => qc.invalidateQueries({ queryKey: [key] })));
    } finally {
      setRefreshing(false);
    }
  }

  return (
    <AdminPageLayout>
      <SectionHeader
        info={adminPageGuides.databases}
        action={
          <>
            <Btn
              ariaLabel="Refresh the database inventory"
              loading={refreshing}
              onClick={() => void refresh()}
              tone="ghost"
            >
              <RefreshCw size={14} /> Refresh
            </Btn>
            {/* Labelled "Add database…" rather than "Create Database", because this
                control creates nothing: it opens a picker that routes you to the one
                of the six surfaces where the chosen kind is actually created. The real
                create form lives inside the Managed tab and keeps that label. */}
            <Btn onClick={() => setShowCreate(true)} tone="primary">
              <Plus size={14} /> Add database…
            </Btn>
          </>
        }
      />
      <OfflineBanner onRetry={() => window.location.reload()} />
      <AdminTabs
        active={activeTab}
        label="Database views"
        onChange={(id) => setActiveTab(id as DatabaseTab)}
        tabs={TABS}
      />
      {activeTab === "overview" && <DatabasesOverview onOpenTab={setActiveTab} />}
      {activeTab === "hosts" && <AdminDatabases />}
      {activeTab === "containers" && <DBContainerView />}
      {activeTab === "managed" && <ManagedDatabaseView />}
      {activeTab === "services" && <DatabaseServicesView />}
      {showCreate && <CreateDatabaseModal onClose={() => setShowCreate(false)} onSelect={setActiveTab} />}
    </AdminPageLayout>
  );
}
