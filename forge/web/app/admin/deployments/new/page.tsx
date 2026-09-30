"use client";

import { useState } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { ArrowRightLeft, Check, GitBranch, Loader2, Play, RefreshCw, Split } from "lucide-react";
import { postJSON } from "@/lib/api";
import { fetchServers } from "@/lib/api/servers";
import type { ApiServer } from "@/lib/api/types";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
  AdminPageLayout,
  AdminSection,
  AdminSelect,
  Btn,
  Card,
  Input,
  Pill,
} from "@/components/admin/admin-ui";
import { Alert } from "@/components/ui/primitives";
import { useToast } from "@/components/ui/toast";
import { errorMessage } from "@/lib/utils";

type Deployment = { id: string };
type DeploymentResponse = { data: Deployment };

/**
 * The four strategies the deployment service actually implements:
 * `rollout.go:57-68` switches on `recreate | rolling | blue-green | canary`,
 * all hyphenated. Canary is real (`canaryRollout`), so the form keeps it and the
 * list page can filter on it — both come from this one list rather than from a
 * vocabulary invented at each call site.
 */
const strategies = [
  { key: "recreate", label: "Recreate", icon: RefreshCw, desc: "Stop all instances, then start new ones. Downtime expected." },
  { key: "rolling", label: "Rolling", icon: ArrowRightLeft, desc: "Update instances one at a time with health check gating." },
  { key: "blue-green", label: "Blue-Green", icon: Split, desc: "Start new stack, health check, switch traffic, stop old." },
  { key: "canary", label: "Canary", icon: GitBranch, desc: "Start new alongside old, route a percentage of traffic, verify, promote." },
] as const;

export default function NewDeploymentPage() {
  const router = useRouter();
  const { toast } = useToast();

  const serversQuery = useQuery({
    queryKey: ["servers"],
    queryFn: () => fetchServers(),
  });

  const servers: ApiServer[] = serversQuery.data ?? [];

  const [serverId, setServerId] = useState("");
  const [image, setImage] = useState("");
  const [strategy, setStrategy] = useState<string>("blue-green");
  const [canaryPercent, setCanaryPercent] = useState(10);
  const [healthCheckPath, setHealthCheckPath] = useState("/health");
  const [healthCheckPort, setHealthCheckPort] = useState("8080");
  const healthCheckPortNumber = Number(healthCheckPort);
  const hasValidPort =
    Number.isInteger(healthCheckPortNumber) && healthCheckPortNumber > 0 && healthCheckPortNumber <= 65_535;

  const createMutation = useMutation({
    mutationFn: () =>
      postJSON<DeploymentResponse>(`/admin/deployments/${encodeURIComponent(serverId)}/rollout`, {
        strategy,
        image,
        healthCheckPath,
        healthCheckPort: healthCheckPortNumber,
        canaryPercent: strategy === "canary" ? canaryPercent : undefined,
      }),
    onSuccess: ({ data }) => {
      toast({
        tone: "success",
        title: "Deployment started",
        message: `A ${strategy.replace("-", " ")} rollout was created for the selected server.`,
      });
      router.push(`/admin/deployments/${data.id}`);
    },
    onError: (err) =>
      toast({ tone: "error", title: "Deployment not started", message: errorMessage(err) }),
  });

  const canSubmit =
    Boolean(serverId) && Boolean(image.trim()) && hasValidPort && !createMutation.isPending;

  return (
    <AdminPageLayout>
      <AdminPageHeader
        title="New Deployment"
        description="Start a rollout for one server. The server, image and health check are sent as-is; the strategy decides how traffic moves."
        backAction={() => router.push("/admin/deployments")}
        backLabel="Deployments"
      />

      <AdminSection title="Target">
        <Card>
          <div className="space-y-4 p-4">
            {serversQuery.isPending ? (
              <AdminLoadingState label="Loading servers…" />
            ) : serversQuery.isError ? (
              <AdminErrorState
                message={`Servers could not be listed: ${errorMessage(serversQuery.error)}`}
                retry={() => void serversQuery.refetch()}
              />
            ) : servers.length === 0 ? (
              <p className="text-sm text-text-subtle">
                No server is registered with the control plane, so there is nothing to deploy to. Create a
                server first; this form will not guess a target for you.
              </p>
            ) : (
              /* `AdminSelect` owns its `<label>`; wrapping it in `AdminFormField`
                 (also a `<label>`) would nest them. */
              <>
                <AdminSelect
                  label="Server"
                  value={serverId}
                  onChange={setServerId}
                  placeholder="Select a server"
                  options={servers.map((s) => ({
                    value: s.id,
                    label: s.name ? `${s.name} · ${s.id.slice(0, 8)}` : s.id,
                  }))}
                />
                <p className="ui-hint block">
                  Required. The rollout runs against the server you pick here — none is chosen for you.
                </p>
              </>
            )}
          </div>
        </Card>
      </AdminSection>

      <AdminSection title="Rollout strategy">
        <Card className="p-4">
          <div
            aria-label="Rollout strategy"
            className="grid gap-3 sm:grid-cols-2"
            role="radiogroup"
          >
            {strategies.map((s) => {
              const Icon = s.icon;
              const isActive = strategy === s.key;
              return (
                <button
                  key={s.key}
                  aria-checked={isActive}
                  className={`rounded-lg border p-4 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] ${
                    isActive
                      ? "border-[var(--focus)] bg-overlay-strong"
                      : "border-line bg-overlay-subtle hover:border-line-strong"
                  }`}
                  onClick={() => setStrategy(s.key)}
                  role="radio"
                  tabIndex={isActive ? 0 : -1}
                  type="button"
                >
                  <span className="mb-1 flex items-center gap-2">
                    <Icon aria-hidden="true" className={isActive ? "text-text" : "text-text-muted"} size={14} />
                    <span className="text-sm font-medium text-text">{s.label}</span>
                    {isActive ? (
                      <span className="ml-auto inline-flex items-center gap-1 text-meta text-text-subtle">
                        <Check aria-hidden="true" size={12} /> Selected
                      </span>
                    ) : null}
                  </span>
                  <span className="block text-meta text-text-subtle">{s.desc}</span>
                </button>
              );
            })}
          </div>

          {strategy === "canary" ? (
            <div className="mt-4">
              <label className="ui-label mb-1.5 block" htmlFor="canary-percent">
                Canary traffic share: {canaryPercent}%
              </label>
              <input
                className="w-full cursor-pointer"
                id="canary-percent"
                max={50}
                min={1}
                onChange={(e) => setCanaryPercent(Number(e.target.value))}
                style={{ accentColor: "var(--brand)" }}
                type="range"
                value={canaryPercent}
              />
              <p className="ui-hint mt-1.5 block">
                Sent as <code className="font-mono">canaryPercent</code>. Promotion happens on the
                zero-downtime release surface, not here.
              </p>
            </div>
          ) : null}
        </Card>
      </AdminSection>

      <AdminSection title="Workload and health gate">
        <Card className="p-4">
          <div className="grid gap-3.5">
            <Input
              label="Container image"
              value={image}
              onChange={setImage}
              placeholder="e.g. nginx:latest"
              required
            />
            <Input
              label="Health check path"
              value={healthCheckPath}
              onChange={setHealthCheckPath}
              placeholder="/health"
            />
            <div>
              <Input
                label="Health check port"
                type="number"
                value={healthCheckPort}
                onChange={setHealthCheckPort}
                placeholder="8080"
              />
              {!hasValidPort ? (
                <span className="ui-field-error mt-1.5 block" role="alert">
                  Enter a port between 1 and 65535.
                </span>
              ) : null}
            </div>
          </div>
        </Card>
      </AdminSection>

      {createMutation.isError ? (
        <Alert tone="error" title="Deployment not started">
          {errorMessage(createMutation.error)}
        </Alert>
      ) : null}

      <div className="flex items-center justify-end gap-3 border-t border-line pt-4">
        {!canSubmit && !createMutation.isPending ? (
          <Pill tone="neutral">
            {!serverId
              ? "Select a server to continue"
              : !image.trim()
                ? "Enter the image to deploy"
                : "Health check port must be 1–65535"}
          </Pill>
        ) : null}
        <Btn tone="ghost" onClick={() => router.push("/admin/deployments")}>
          Cancel
        </Btn>
        <Btn tone="primary" onClick={() => createMutation.mutate()} disabled={!canSubmit}>
          {createMutation.isPending ? (
            <>
              <Loader2 aria-hidden="true" className="animate-spin" size={14} /> Starting…
            </>
          ) : (
            <>
              <Play aria-hidden="true" size={14} /> Start deployment
            </>
          )}
        </Btn>
      </div>
    </AdminPageLayout>
  );
}
