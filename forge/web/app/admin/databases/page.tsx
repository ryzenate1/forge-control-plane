"use client";

import { useState } from "react";
import { Download, Plus } from "lucide-react";
import { AdminDatabases } from "@/components/admin/AdminDatabases";
import { DBContainerView } from "@/components/database/container-view";
import { ManagedDatabaseView } from "@/components/database/managed-database-view";
import { DatabaseServicesView } from "@/components/database/database-services-view";
import { CreateDatabaseModal, DatabasesOverview, type DatabaseTab } from "@/components/database/databases-overview";
import { AdminPageLayout, AdminTabs, SectionHeader } from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";
import { Btn } from "@/components/admin/admin-ui";
import { adminPageGuides } from "@/components/admin/admin-page-guides";

export default function AdminDatabasesPage() {
  const [activeTab, setActiveTab] = useState<DatabaseTab>("overview");
  const [showCreate, setShowCreate] = useState(false);

  const tabs = [
    { id: "overview" as DatabaseTab, label: "Overview" },
    { id: "hosts" as DatabaseTab, label: "Database Hosts" },
    { id: "containers" as DatabaseTab, label: "DB Containers" },
    { id: "managed" as DatabaseTab, label: "Managed DBs" },
    { id: "services" as DatabaseTab, label: "Services" },
  ];

  return (
    <AdminPageLayout>
      <SectionHeader
        title="Databases"
        sub="Provision and operate external hosts, containers, managed databases, and database services."
        info={adminPageGuides.databases}
        action={<div className="flex flex-wrap items-center gap-2">
          <Btn tone="ghost" onClick={() => setActiveTab("hosts")}><Download size={14} /> Import</Btn>
          <Btn tone="primary" onClick={() => setShowCreate(true)}><Plus size={14} /> Create Database</Btn>
        </div>}
      />
      <OfflineBanner onRetry={() => window.location.reload()} />
      <AdminTabs active={activeTab} onChange={(id) => setActiveTab(id as DatabaseTab)} tabs={tabs} />
      {activeTab === "overview" && <DatabasesOverview onOpenTab={setActiveTab} />}
      {activeTab === "hosts" && <AdminDatabases embedded />}
      {activeTab === "containers" && <DBContainerView />}
      {activeTab === "managed" && <ManagedDatabaseView />}
      {activeTab === "services" && <DatabaseServicesView />}
      {showCreate && <CreateDatabaseModal onClose={() => setShowCreate(false)} onSelect={setActiveTab} />}
    </AdminPageLayout>
  );
}
