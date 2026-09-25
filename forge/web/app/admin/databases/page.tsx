"use client";

import { useState } from "react";
import { Database, Download, Plus } from "lucide-react";
import { AdminDatabases } from "@/components/admin/AdminDatabases";
import { DBContainerView } from "@/components/database/container-view";
import { ManagedDatabaseView } from "@/components/database/managed-database-view";
import { DatabaseServicesView } from "@/components/database/database-services-view";
import { CreateDatabaseModal, DatabasesOverview, type DatabaseTab } from "@/components/database/databases-overview";
import { AdminPageLayout, AdminTabs } from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";
import { Btn } from "@/components/admin/admin-ui";

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
      <nav aria-label="Breadcrumb" className="flex items-center gap-2 font-mono text-[11px] text-slate-500">
        <span>Workloads</span>
        <span aria-hidden="true">/</span>
        <span className="font-semibold text-slate-200">Databases</span>
      </nav>
      <div className="mt-2 flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="flex items-center gap-2.5 text-2xl font-bold tracking-tight text-slate-100">
            Databases
            <span className="grid h-7 w-7 place-items-center rounded-lg border border-white/[0.08] bg-white/[0.03] text-slate-400"><Database size={14} /></span>
          </h1>
          <p className="mt-1 max-w-2xl text-sm text-slate-400">
            Single inventory across external hosts, raw containers, managed DBs, services, catalog instances and server databases.
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Btn tone="ghost" onClick={() => setActiveTab("hosts")}><Download size={14} /> Import</Btn>
          <Btn tone="primary" onClick={() => setShowCreate(true)}><Plus size={14} /> Create Database</Btn>
        </div>
      </div>
      <OfflineBanner onRetry={() => window.location.reload()} />
      <AdminTabs active={activeTab} onChange={(id) => setActiveTab(id as DatabaseTab)} tabs={tabs} />
      {activeTab === "overview" && <DatabasesOverview onOpenTab={setActiveTab} />}
      {activeTab === "hosts" && <AdminDatabases />}
      {activeTab === "containers" && <DBContainerView />}
      {activeTab === "managed" && <ManagedDatabaseView />}
      {activeTab === "services" && <DatabaseServicesView />}
      {showCreate && <CreateDatabaseModal onClose={() => setShowCreate(false)} onSelect={setActiveTab} />}
    </AdminPageLayout>
  );
}
