"use client";

import { useState } from "react";
import { AdminDatabases } from "@/components/admin/AdminDatabases";
import { DBContainerView } from "@/components/database/container-view";
import { ManagedDatabaseView } from "@/components/database/managed-database-view";
import { AdminPageLayout, AdminPageHeader, AdminTabs } from "@/components/admin/admin-ui";

type Tab = "hosts" | "containers" | "managed";

export default function AdminDatabasesPage() {
  const [activeTab, setActiveTab] = useState<Tab>("containers");

  const tabs = [
    { id: "hosts" as Tab, label: "Database Hosts" },
    { id: "containers" as Tab, label: "DB Containers" },
    { id: "managed" as Tab, label: "Managed DBs" },
  ];

  return (
    <AdminPageLayout>
      <AdminPageHeader
        title="Databases"
        description="Database hosts, containers, and managed database services"
      />
      <AdminTabs active={activeTab} onChange={(id) => setActiveTab(id as Tab)} tabs={tabs} />
      {activeTab === "hosts" && <AdminDatabases />}
      {activeTab === "containers" && <DBContainerView />}
      {activeTab === "managed" && <ManagedDatabaseView />}
    </AdminPageLayout>
  );
}
