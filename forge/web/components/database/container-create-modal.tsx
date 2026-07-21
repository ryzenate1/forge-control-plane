"use client";

import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Database, Server, HardDrive, Cpu, DatabaseZap, Rabbit, Shell } from "lucide-react";
import { type DBContainerEngine, provisionDBContainer, fetchDBEngines } from "@/lib/api/database-containers";
import { Modal, ModalFooter } from "@/components/admin/admin-ui";
import { useToast } from "@/components/ui/toast";
import { cn } from "@/lib/utils";

const engineMeta: Record<DBContainerEngine, { label: string; icon: typeof Database; color: string; ring: string; desc: string }> = {
  postgresql: { label: "PostgreSQL", icon: DatabaseZap, color: "text-blue-400", ring: "ring-blue-500/30", desc: "Advanced relational database" },
  mysql: { label: "MySQL", icon: Database, color: "text-orange-400", ring: "ring-orange-500/30", desc: "Popular open-source RDBMS" },
  mariadb: { label: "MariaDB", icon: Shell, color: "text-cyan-400", ring: "ring-cyan-500/30", desc: "MySQL-compatible fork" },
  redis: { label: "Redis", icon: Rabbit, color: "text-red-400", ring: "ring-red-500/30", desc: "In-memory key-value store" },
  mongodb: { label: "MongoDB", icon: Server, color: "text-emerald-400", ring: "ring-emerald-500/30", desc: "Document-oriented NoSQL" },
};

export function DBContainerCreateModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const { toast } = useToast();
  const [engine, setEngine] = useState<DBContainerEngine>("postgresql");
  const [version, setVersion] = useState("16");
  const [memoryMb, setMemoryMb] = useState("256");
  const [cpuShares, setCpuShares] = useState("0");

  const enginesQuery = useQuery({
    queryKey: ["db-engines"],
    queryFn: fetchDBEngines,
    staleTime: 5 * 60 * 1000,
  });

  const availableVersions = enginesQuery.data?.[engine] ?? [];
  const selectedMeta = engineMeta[engine];

  const createMut = useMutation({
    mutationFn: () => provisionDBContainer({
      engine,
      version,
      memoryMb: memoryMb ? parseInt(memoryMb, 10) : 256,
      cpuShares: cpuShares ? parseInt(cpuShares, 10) : 0,
    }),
    onSuccess: () => {
      onCreated();
      onClose();
      toast({ tone: "success", title: "Database container provisioning started" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Provisioning failed", message: e.message }),
  });

  return (
    <Modal title={<span className="text-base font-semibold text-slate-100">Create Database Container</span>} onClose={onClose} wide>
      <div className="space-y-6">

        {/* Engine Type Selection */}
        <div>
          <label className="mb-3 block text-xs font-semibold uppercase tracking-wider text-slate-400">Engine Type</label>
          <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-3 md:grid-cols-5">
            {(Object.entries(engineMeta) as [DBContainerEngine, typeof engineMeta[DBContainerEngine]][]).map(([value, meta]) => {
              const Icon = meta.icon;
              const active = engine === value;
              return (
                <button
                  key={value}
                  type="button"
                  onClick={() => {
                    setEngine(value);
                    const versions = enginesQuery.data?.[value] ?? [];
                    if (versions.length > 0 && !versions.includes(version)) {
                      setVersion(versions[versions.length - 1]);
                    }
                  }}
                  className={cn(
                    "group relative flex flex-col items-center gap-1.5 rounded-xl border px-3 py-3 text-center transition-all duration-150",
                    "outline-none focus-visible:ring-2 focus-visible:ring-red-400/60",
                    active
                      ? "border-slate-600 bg-slate-800/60 shadow-sm"
                      : "border-white/[0.06] bg-white/[0.02] hover:border-white/20 hover:bg-white/[0.04]"
                  )}
                >
                  <Icon className={cn("h-5 w-5 transition-colors", active ? meta.color : "text-slate-500 group-hover:text-slate-300")} strokeWidth={1.5} />
                  <span className={cn("text-xs font-medium leading-tight", active ? "text-slate-100" : "text-slate-400 group-hover:text-slate-300")}>
                    {meta.label}
                  </span>
                  {active && <span className="absolute -right-1 -top-1 h-3 w-3 rounded-full bg-red-500 ring-2 ring-[#0d1117]" />}
                </button>
              );
            })}
          </div>
        </div>

        {/* Version + Resources row */}
        <div className="grid gap-5 sm:grid-cols-3">
          {/* Version */}
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">Version</label>
            <select
              className="h-10 w-full rounded-lg border border-white/10 bg-[#0f141f] px-3 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15"
              value={version}
              onChange={(e) => setVersion(e.target.value)}
            >
              {enginesQuery.isLoading ? (
                <option value="">Loading...</option>
              ) : (
                availableVersions.map((v) => (
                  <option key={v} value={v}>{v}</option>
                ))
              )}
            </select>
          </div>

          {/* Memory */}
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">
              Memory <span className="font-normal normal-case text-slate-500">(MB)</span>
            </label>
            <div className="relative">
              <HardDrive size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" strokeWidth={1.5} />
              <input
                type="number"
                min={64}
                max={65536}
                step={64}
                className="h-10 w-full rounded-lg border border-white/10 bg-[#0f141f] pl-9 pr-3 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15 [&::-webkit-inner-spin-button]:appearance-none"
                value={memoryMb}
                onChange={(e) => setMemoryMb(e.target.value)}
                placeholder="256"
              />
            </div>
            <p className="mt-1 text-[11px] text-slate-500">Min 64 MB. Leave empty for default (256 MB).</p>
          </div>

          {/* CPU Shares */}
          <div>
            <label className="mb-1.5 block text-xs font-semibold uppercase tracking-wider text-slate-400">
              CPU <span className="font-normal normal-case text-slate-500">(shares)</span>
            </label>
            <div className="relative">
              <Cpu size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" strokeWidth={1.5} />
              <input
                type="number"
                min={0}
                max={1024}
                step={1}
                className="h-10 w-full rounded-lg border border-white/10 bg-[#0f141f] pl-9 pr-3 text-sm text-slate-100 shadow-inner shadow-black/10 outline-none transition hover:border-white/20 focus:border-red-400/70 focus:ring-2 focus:ring-red-500/15 [&::-webkit-inner-spin-button]:appearance-none"
                value={cpuShares}
                onChange={(e) => setCpuShares(e.target.value)}
                placeholder="0"
              />
            </div>
            <p className="mt-1 text-[11px] text-slate-500">Relative CPU weight. Default (0) = 1024 shares.</p>
          </div>
        </div>

        {/* Summary card */}
        <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-4">
          <div className="flex items-center gap-3">
            <div className={cn("flex h-10 w-10 items-center justify-center rounded-lg", selectedMeta.color.replace("text-", "bg-").replace("400", "950") + "/50")}>
              <selectedMeta.icon className={cn("h-5 w-5", selectedMeta.color)} strokeWidth={1.5} />
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-slate-200">{selectedMeta.label} {version}</p>
              <p className="text-xs text-slate-500">{selectedMeta.desc} &middot; {memoryMb || "256"} MB &middot; {cpuShares || "1024"} CPU shares</p>
            </div>
          </div>
        </div>

        {/* Error */}
        {createMut.isError && (
          <div className="flex items-start gap-2.5 rounded-lg border border-red-500/20 bg-red-950/10 p-3.5 text-sm text-red-200">
            <span>{createMut.error?.message || "An unexpected error occurred."}</span>
          </div>
        )}
      </div>

      <ModalFooter
        onCancel={onClose}
        onConfirm={() => createMut.mutate()}
        disabled={createMut.isPending || !engine || !version}
        confirmLabel={createMut.isPending ? "Provisioning..." : "Create Container"}
      />
    </Modal>
  );
}