"use client";

import { useState } from "react";
import { AdminDatabases } from "@/components/admin/AdminDatabases";
import { DBContainerView } from "@/components/database/container-view";
import { ManagedDatabaseView } from "@/components/database/managed-database-view";
import { cn } from "@/lib/utils";

type Tab = "hosts" | "containers" | "managed";

const tabs: { key: Tab; label: string }[] = [
  { key: "hosts", label: "Database Hosts" },
  { key: "containers", label: "DB Containers" },
  { key: "managed", label: "Managed DBs" },
];

export default function AdminDatabasesPage() {
  const [activeTab, setActiveTab] = useState<Tab>("containers");

  return (
    <div>
      <div className="mb-6 flex items-center gap-1 rounded-lg bg-[#161b28] p-1 w-fit">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            className={cn(
              "rounded-md px-4 py-2 text-sm font-medium transition-colors",
              activeTab === tab.key
                ? "bg-[#1e2536] text-slate-100 shadow-sm"
                : "text-slate-400 hover:text-slate-200"
            )}
            onClick={() => setActiveTab(tab.key)}
            type="button"
          >
            {tab.label}
          </button>
        ))}
      </div>

      {activeTab === "hosts" && <AdminDatabases />}
      {activeTab === "containers" && <DBContainerView />}
      {activeTab === "managed" && <ManagedDatabaseView />}
    </div>
  );
}
