"use client";

import { useState } from "react";
import { Container, Image, Network, HardDrive } from "lucide-react";
import { SectionHeader, cn } from "@/components/admin/admin-ui";
import { ContainersView } from "@/components/docker/containers-view";
import { ImagesView } from "@/components/docker/images-view";
import { NetworksView } from "@/components/docker/networks-view";
import { VolumesView } from "@/components/docker/volumes-view";

type DockerTab = "containers" | "images" | "networks" | "volumes";

const TABS: Array<{ id: DockerTab; label: string; icon: typeof Container }> = [
  { id: "containers", label: "Containers", icon: Container },
  { id: "images", label: "Images", icon: Image },
  { id: "networks", label: "Networks", icon: Network },
  { id: "volumes", label: "Volumes", icon: HardDrive },
];

export default function DockerPage() {
  const [tab, setTab] = useState<DockerTab>("containers");

  return (
    <div className="space-y-6">
      <SectionHeader title="Docker" sub="Manage containers, images, networks, and volumes across all nodes." />
      <div className="flex gap-1 border-b border-white/[0.06]">
        {TABS.map((t) => {
          const Icon = t.icon;
          const active = tab === t.id;
          return (
            <button
              key={t.id}
              aria-current={active ? "page" : undefined}
              className={cn(
                "flex items-center gap-2 px-4 py-2.5 text-sm font-medium transition border-b-2 -mb-px",
                active
                  ? "border-[#dc2626] text-[#dc2626]"
                  : "border-transparent text-slate-400 hover:text-slate-200",
              )}
              onClick={() => setTab(t.id)}
              type="button"
            >
              <Icon size={15} />
              {t.label}
            </button>
          );
        })}
      </div>
      {tab === "containers" && <ContainersView />}
      {tab === "images" && <ImagesView />}
      {tab === "networks" && <NetworksView />}
      {tab === "volumes" && <VolumesView />}
    </div>
  );
}
