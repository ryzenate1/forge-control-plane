"use client";

import { HostFilesView } from "@/components/admin/host-files-view";
import { SectionHeader } from "@/components/admin/admin-ui";

export default function AdminFilesPage() {
  return (
    <div className="space-y-6">
      <SectionHeader title="Host File Manager" sub="Browse and manage files on the host system" />
      <div className="rounded-xl border border-white/[0.08] bg-[#111722] shadow-xl shadow-black/10">
        <div className="p-4 sm:p-6">
          <HostFilesView />
        </div>
      </div>
    </div>
  );
}
