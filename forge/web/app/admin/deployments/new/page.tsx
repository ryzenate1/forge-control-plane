"use client";

import { useState } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  ArrowLeft, Layers, Play, Loader2, Split, GitBranch,
  RefreshCw, ArrowRightLeft,
} from "lucide-react";
import { fetchJSON, postJSON } from "@/lib/api";
import { Btn, Card, CardHeader, Input, SectionHeader, Pill } from "@/components/admin/admin-ui";
import { Alert } from "@/components/ui/primitives";
import { useToast } from "@/components/ui/toast";

type Server = {
  id: string;
  name?: string;
  uuid?: string;
};

type Deployment = {
  id: string;
};

type DeploymentResponse = {
  data: Deployment;
};

const strategies = [
  { key: "recreate", label: "Recreate", icon: RefreshCw, desc: "Stop all instances, then start new ones. Downtime expected." },
  { key: "rolling", label: "Rolling", icon: ArrowRightLeft, desc: "Update instances one at a time with health check gating." },
  { key: "blue-green", label: "Blue-Green", icon: Split, desc: "Start new stack, health check, switch traffic, stop old." },
  { key: "canary", label: "Canary", icon: GitBranch, desc: "Start new alongside old, route a percentage of traffic, verify, promote." },
];

export default function NewDeploymentPage() {
  const router = useRouter();
  const { toast } = useToast();

  const serversQuery = useQuery({
    queryKey: ["servers"],
    queryFn: () => fetchJSON<Server[]>("/servers"),
  });

  const servers = serversQuery.data ?? [];

  const [serverId, setServerId] = useState("");
  const [image, setImage] = useState("");
  const [strategy, setStrategy] = useState("blue-green");
  const [canaryPercent, setCanaryPercent] = useState(10);
  const [healthCheckPath, setHealthCheckPath] = useState("/health");
  const [healthCheckPort, setHealthCheckPort] = useState("8080");
  const healthCheckPortNumber = Number(healthCheckPort);
  const hasValidPort = Number.isInteger(healthCheckPortNumber) && healthCheckPortNumber > 0 && healthCheckPortNumber <= 65_535;

  const createMutation = useMutation({
    mutationFn: () =>
      postJSON<DeploymentResponse>(`/admin/deployments/${serverId}/rollout`, {
        strategy,
        image,
        healthCheckPath,
        healthCheckPort: healthCheckPortNumber,
        canaryPercent: strategy === "canary" ? canaryPercent : undefined,
      }),
    onSuccess: ({ data }) => {
      toast({ tone: "success", title: "Deployment started", message: "The " + strategy.replace("-", " ") + " deployment has been created." });
      router.push(`/admin/deployments/${data.id}`);
    },
  });

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-4">
        <Btn tone="ghost" onClick={() => router.push("/admin/deployments")}>
          <ArrowLeft size={14} /> Back
        </Btn>
        <SectionHeader
          title="New Deployment"
          sub="Create a deployment with rollout strategy for a game server."
        />
      </div>

      <Card>
        <CardHeader title="Deployment Configuration" icon={Layers} />
        <div className="grid gap-5 p-6 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <label className="block text-sm font-medium text-slate-300 mb-1.5">Server</label>
            <select
              className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none focus:border-[#dc2626]/60 focus:ring-1 focus:ring-[#dc2626]/30"
              value={serverId}
              onChange={(e) => setServerId(e.target.value)}
            >
              <option value="">Select a server...</option>
              {servers.map((s) => (
                <option key={s.id} value={s.id}>{s.name ?? s.uuid ?? s.id}</option>
              ))}
            </select>
          </div>

          <div className="sm:col-span-2">
            <label className="block text-sm font-medium text-slate-300 mb-1.5">Rollout Strategy</label>
            <div className="grid gap-3 sm:grid-cols-2">
              {strategies.map((s) => {
                const Icon = s.icon;
                const isActive = strategy === s.key;
                return (
                  <button
                    key={s.key}
                    type="button"
                    className={`rounded-lg border p-4 text-left transition-colors ${
                      isActive
                        ? "border-[#dc2626]/60 bg-[#dc2626]/[0.08]"
                        : "border-white/[0.06] bg-[#161b28] hover:border-white/[0.12]"
                    }`}
                    onClick={() => setStrategy(s.key)}
                  >
                    <div className="flex items-center gap-2 mb-1">
                      <Icon size={14} className={isActive ? "text-red-400" : "text-slate-400"} />
                      <span className="text-sm font-medium text-slate-200">{s.label}</span>
                      {isActive && <Pill tone="red">Selected</Pill>}
                    </div>
                    <p className="text-xs text-slate-500">{s.desc}</p>
                  </button>
                );
              })}
            </div>
          </div>

          <Input
            label="Container Image"
            value={image}
            onChange={setImage}
            placeholder="e.g. nginx:latest"
          />

          {strategy === "canary" && (
            <div className="sm:col-span-2">
              <label className="block text-sm font-medium text-slate-300 mb-1.5">Canary Traffic Percent: {canaryPercent}%</label>
              <input
                type="range"
                min={1}
                max={50}
                value={canaryPercent}
                onChange={(e) => setCanaryPercent(Number(e.target.value))}
                className="w-full h-2 bg-[#161b28] rounded-lg appearance-none cursor-pointer accent-[#dc2626]"
              />
            </div>
          )}

          <Input
            label="Health Check Path"
            value={healthCheckPath}
            onChange={setHealthCheckPath}
            placeholder="/health"
          />

          <Input
            label="Health Check Port"
            type="number"
            value={healthCheckPort}
            onChange={setHealthCheckPort}
            placeholder="8080"
          />
          {!hasValidPort ? <p className="-mt-3 text-xs text-red-300">Enter a port between 1 and 65535.</p> : null}
        </div>
        {createMutation.isError ? (
          <div className="px-6 pb-4">
            <Alert tone="error" title="Could not start deployment">{createMutation.error instanceof Error ? createMutation.error.message : "Try again after checking the deployment settings."}</Alert>
          </div>
        ) : null}
        <div className="flex items-center justify-end gap-3 border-t border-white/[0.06] px-6 py-4">
          <Btn tone="ghost" onClick={() => router.push("/admin/deployments")}>Cancel</Btn>
          <Btn
            tone="primary"
            onClick={() => createMutation.mutate()}
            disabled={createMutation.isPending || !serverId || !image || !hasValidPort}
          >
            {createMutation.isPending ? (
              <><Loader2 size={14} className="animate-spin" /> Starting...</>
            ) : (
              <><Play size={14} /> Start Deployment</>
            )}
          </Btn>
        </div>
      </Card>
    </div>
  );
}
