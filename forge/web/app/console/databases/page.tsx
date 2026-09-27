"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Database, Plus } from "lucide-react";
import { Card, Pill, SectionHeader } from "@/components/admin/admin-ui";
import { LoadingSpinner } from "@/components/ui/loading-skeleton";
import { SearchInput } from "@/components/ui/primitives";
import { listDatabaseServices } from "@/lib/api/database-services";

/**
 * /console/databases — customer-facing database instances overview.
 */
export default function ConsoleDatabasesPage() {
  const [search, setSearch] = useState("");

  const { data: databases = [], isLoading } = useQuery({
    queryKey: ["database-services"],
    queryFn: listDatabaseServices,
    staleTime: 30_000,
    retry: 1,
  });

  const filtered = useMemo(() => {
    if (!search.trim()) return databases;
    const q = search.toLowerCase();
    return databases.filter((db) => db.name?.toLowerCase().includes(q) || db.type?.toLowerCase().includes(q));
  }, [databases, search]);

  if (isLoading) return <LoadingSpinner />;

  return (
    <div className="space-y-5">
      <SectionHeader
        title="Databases"
        sub={`${databases.length} database instance${databases.length !== 1 ? "s" : ""}`}
        action={
          <Link href="/admin/databases" className="inline-flex items-center gap-2 rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-bold text-white transition-colors hover:bg-[var(--brand-dark)]">
            <Plus size={14} /> New Database
          </Link>
        }
      />

      <SearchInput value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search databases…" />

      {filtered.length === 0 ? (
        <Card>
          <div className="flex flex-col items-center gap-3 py-12 text-center">
            <Database size={32} className="text-slate-600" />
            <p className="text-sm text-slate-400">{search ? "No databases match your search." : "No databases provisioned yet."}</p>
          </div>
        </Card>
      ) : (
        <div className="overflow-x-auto rounded-xl border border-white/[0.06]">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-white/[0.06] bg-white/[0.02]">
              <tr>
                <th className="px-4 py-3 font-semibold text-slate-300">Name</th>
                <th className="px-4 py-3 font-semibold text-slate-300">Engine</th>
                <th className="px-4 py-3 font-semibold text-slate-300">Type</th>
                <th className="px-4 py-3 font-semibold text-slate-300">Status</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]">
              {filtered.map((db) => (
                <tr key={db.id} className="transition-colors hover:bg-white/[0.02]">
                  <td className="px-4 py-3 font-medium text-white">{db.name ?? db.id}</td>
                  <td className="px-4 py-3 text-slate-400">{db.type ?? "—"}</td>
                  <td className="px-4 py-3 text-slate-400 capitalize">service</td>
                  <td className="px-4 py-3"><Pill tone={db.status === "running" ? "success" : "neutral"}>{db.status ?? "unknown"}</Pill></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
