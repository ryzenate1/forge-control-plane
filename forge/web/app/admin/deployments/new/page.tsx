"use client";

import { useState } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  ArrowLeft, Layers, Play, Loader2,
} from "lucide-react";
import { fetchJSON, postJSON } from "@/modules/infrastructure/api";
import { Btn, Card, CardHeader, Input, SectionHeader } from "@/components/admin/admin-ui";
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
  const [healthCheckPath, setHealthCheckPath] = useState("/health");
  const [healthCheckPort, setHealthCheckPort] = useState("8080");
  const healthCheckPortNumber = Number(healthCheckPort);
  const hasValidPort = Number.isInteger(healthCheckPortNumber) && healthCheckPortNumber > 0 && healthCheckPortNumber <= 65_535;

  const createMutation = useMutation({
    mutationFn: () =>
      postJSON<DeploymentResponse>("/admin/deployments/blue-green", {
        serverId,
        image,
        healthCheckPath,
        healthCheckPort: healthCheckPortNumber,
      }),
    onSuccess: ({ data }) => {
      toast({ tone: "success", title: "Deployment started", message: "The blue-green deployment has been created." });
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
          title="New Blue-Green Deployment"
          sub="Create a blue-green deployment for a game server."
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

          <Input
            label="Container Image"
            value={image}
            onChange={setImage}
            placeholder="e.g. nginx:latest"
          />

          <div className="rounded-lg border border-sky-500/20 bg-sky-500/[0.06] px-3.5 py-3 text-sm text-sky-100">
            <p className="font-semibold">Blue-green strategy</p>
            <p className="mt-1 text-xs text-sky-200/75">A replacement instance is started and checked before traffic is switched.</p>
          </div>

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
