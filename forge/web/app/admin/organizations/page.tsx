"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { Building2, Plus } from "lucide-react";
import { Btn, Card, CardHeader, SectionHeader, EmptyState } from "@/components/admin/admin-ui";
import type { Organization } from "@/lib/api/tenancy";

export default function AdminOrganizationsPage() {
  const router = useRouter();
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [loading, setLoading] = useState(true);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");

  const fetchOrgs = async () => {
    setLoading(true);
    try {
      const res = await fetch("/api/v1/organizations", { credentials: "include" });
      if (res.ok) setOrgs(await res.json());
    } finally { setLoading(false); }
  };

  useEffect(() => { fetchOrgs(); }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    const res = await fetch("/api/v1/organizations", {
      method: "POST", credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: name.trim(), slug: slug.trim() || undefined }),
    });
    if (res.ok) {
      setName(""); setSlug("");
      await fetchOrgs();
    }
  };

  return (
    <div>
      <SectionHeader title="Organizations" sub="Manage multi-tenant organizations" action={
        <form onSubmit={handleCreate} className="flex gap-2">
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Name" className="rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none" required />
          <input value={slug} onChange={(e) => setSlug(e.target.value.toLowerCase().replace(/\s+/g, "-"))} placeholder="slug" className="w-32 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none" />
          <Btn type="submit" tone="primary"><Plus size={14} /> Create</Btn>
        </form>
      } />

      <Card>
        <CardHeader title="All Organizations" icon={Building2} />
        {loading ? <div className="p-6 text-sm text-slate-400">Loading...</div> :
         orgs.length === 0 ? <EmptyState message="No organizations yet. Create one to get started." /> :
         <div className="divide-y divide-white/[0.06]">
           {orgs.map((org) => (
            <div key={org.id} className="flex items-center justify-between px-4 py-3 hover:bg-white/[0.02] cursor-pointer" onClick={() => router.push(`/organizations/${org.slug}`)}>
              <div>
                <span className="text-sm font-medium text-slate-200">{org.name}</span>
                <span className="ml-2 text-xs text-slate-500">{org.slug}</span>
              </div>
              <span className="text-xs text-slate-500">Owner: {org.ownerName}</span>
            </div>
          ))}
        </div>}
      </Card>
    </div>
  );
}
