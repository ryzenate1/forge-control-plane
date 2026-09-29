"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertCircle, Boxes, Plus, Trash2, ArrowUpRight } from "lucide-react";
import { ApiError, createEndpoint, deleteEndpoint, fetchEndpoints } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
  AdminFormSection, AdminPageLayout, AdminSelect, AdminTable, AdminTBody, AdminTd, AdminTh, AdminTHead, AdminTr, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, PermissionDeniedState, Pill, SectionHeader, AdminLoadingState, AdminErrorState,
} from "./admin-ui";
import { FreshnessBadge } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import Link from "next/link";

// Status and type chips go through `Pill tone`, never a call-site class map: a
// value these tables did not know used to render as a borderless, colourless
// chip containing the raw wire word. `resolveTone` maps an unrecognised status
// to `unknown`, which is a visible grey chip and an honest one.
const EP_TYPE_TONE: Record<string, "blue" | "yellow" | "neutral" | "info" | "green"> = {
  docker: "blue",
  swarm: "yellow",
  kubernetes: "info",
  edge: "green",
};

const EP_STATUS_TONE: Record<string, "green" | "yellow" | "red" | "unknown" | "info"> = {
  online: "green",
  degraded: "yellow",
  offline: "red",
  provisioning: "info",
  unknown: "unknown",
};

/** `reachable` is a probe result, not a boolean fact: an endpoint nobody probed
 *  is unknown, and unknown must not read as an outage. */
function Reachability({ value }: { value?: boolean }) {
  if (value === true) return <Pill tone="green">Reachable</Pill>;
  if (value === false) return <Pill tone="red">Unreachable</Pill>;
  return <Pill tone="unknown">Not probed</Pill>;
}

const URL_SHAPE = /^https?:\/\/[^\s/]+(:\d+)?(\/.*)?$/i;

export function AdminEndpoints() {
  const { toast } = useToast();
  const qc = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const endpointsQuery = useQuery({ queryKey: ["infra-endpoints"], queryFn: fetchEndpoints });
  const endpoints = useMemo(() => endpointsQuery.data ?? [], [endpointsQuery.data]);

  const [modal, setModal] = useState<null | "create" | { id: string }>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [epType, setEpType] = useState("docker");
  const [connMode, setConnMode] = useState("direct");
  const [url, setUrl] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  // An edge/direct API endpoint is an address a daemon will dial, so the shape is
  // checked here rather than failing as a connection error later.
  const urlRequired = connMode !== "edge";
  const trimmedUrl = url.trim();
  const urlError = trimmedUrl && !URL_SHAPE.test(trimmedUrl)
    ? "Enter a full endpoint URL, for example https://docker.example.com:2375."
    : urlRequired && !trimmedUrl
      ? "This connection mode needs the endpoint URL."
      : "";

  const createMut = useMutation({
    mutationFn: () =>
      createEndpoint({
        name: name.trim(),
        description: description.trim(),
        endpointType: epType,
        connectionMode: connMode,
        url: trimmedUrl || undefined,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["infra-endpoints"] });
      setModal(null);
      setName("");
      setDescription("");
      setEpType("docker");
      setConnMode("direct");
      setUrl("");
      setFormError(null);
      toast({ tone: "success", title: "Endpoint created" });
    },
    onError: (e: Error) => {
      setFormError(e.message || "Failed to create endpoint");
    },
  });

  const deleteMut = useMutation({
    mutationFn: deleteEndpoint,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["infra-endpoints"] });
      toast({ tone: "success", title: "Endpoint removed" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Failed to delete endpoint", message: e.message }),
  });

  return (
    <AdminPageLayout>
      <SectionHeader
        status={<FreshnessBadge state={sourceState(endpointsQuery)} />}
        action={<Btn onClick={() => { setName(""); setDescription(""); setEpType("docker"); setConnMode("direct"); setUrl(""); setFormError(null); setModal("create"); }} tone="primary"><Plus size={14} /> New Endpoint</Btn>}
      />
      <p className="ui-hint">
        These are container-runtime API endpoints — the Docker, Swarm, Kubernetes or edge host this
        panel talks to. They are not the public service endpoints a workload is reached on; those are
        under Domains, Traffic and Load Balancer.
      </p>

      <Card>
        <CardHeader title="All endpoints" icon={Boxes} />
        {endpointsQuery.isPending ? (
          <div className="p-4"><AdminLoadingState label="Loading endpoints…" /></div>
        ) : endpointsQuery.isError ? (
          endpointsQuery.error instanceof ApiError && endpointsQuery.error.status === 403 ? (
            <div className="p-4"><PermissionDeniedState message="Viewing the endpoint inventory needs the infrastructure read scope." /></div>
          ) : (
            <div className="p-4">
              <AdminErrorState
                message={endpointsQuery.error instanceof Error ? `Endpoints could not be loaded: ${endpointsQuery.error.message}` : "Endpoints could not be loaded"}
                retry={() => void endpointsQuery.refetch()}
              />
            </div>
          )
        ) : endpoints.length === 0 ? (
          <EmptyState icon={Boxes} title="No endpoints yet" message="No infrastructure endpoint is registered. Create one to group nodes for routing." />
        ) : (
          <AdminTable label="Infrastructure endpoints">
            <AdminTHead><AdminTh>Name</AdminTh><AdminTh>Type</AdminTh><AdminTh>Connection</AdminTh><AdminTh>Status</AdminTh><AdminTh>Reachable</AdminTh><AdminTh>Version</AdminTh><AdminTh></AdminTh></AdminTHead>
            <AdminTBody>
              {endpoints.map((ep) => (
                <AdminTr key={ep.id}>
                  <AdminTd>
                    <Link className="flex items-center gap-1 font-medium text-text hover:underline" href={`/admin/endpoints/${ep.id}`}>
                      {ep.name}
                      <ArrowUpRight aria-hidden="true" size={12} className="opacity-50" />
                    </Link>
                    {ep.description && <div className="mt-0.5 text-xs text-text-subtle">{ep.description}</div>}
                  </AdminTd>
                  <AdminTd><Pill tone={EP_TYPE_TONE[ep.endpointType] ?? "neutral"}>{ep.endpointType || "Type not reported"}</Pill></AdminTd>
                  <AdminTd className="text-text-subtle">{ep.connectionMode || "Not reported"}</AdminTd>
                  <AdminTd><Pill tone={EP_STATUS_TONE[ep.status] ?? "unknown"}>{ep.status || "Status not reported"}</Pill></AdminTd>
                  <AdminTd><Reachability value={ep.reachable} /></AdminTd>
                  <AdminTd className="text-xs text-text-subtle">{ep.version || "Not reported"}</AdminTd>
                  <AdminTd className="text-right">
                    <Btn
                      ariaLabel={`Delete endpoint ${ep.name}`}
                      disabled={deleteMut.isPending}
                      loading={deleteMut.isPending && deleteMut.variables === ep.id}
                      onClick={() => { void (async () => { if (await confirm({ title: `Delete endpoint "${ep.name}"?`, description: "Nodes this endpoint manages stop being reachable through it, and any workload placed on them loses its control path until the endpoint is re-added. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(ep.id); })(); }}
                      size="sm"
                      tone="danger"
                    >
                      <Trash2 size={14} />
                    </Btn>
                  </AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>

      {modal === "create" && (
        <Modal onClose={() => setModal(null)} title="New Endpoint">
          <div className="space-y-4">
            {formError && (
              <div className="ui-alert ui-alert-danger">
                <AlertCircle aria-hidden="true" size={14} /> <span>{formError}</span>
              </div>
            )}
            <AdminFormSection title="Endpoint Details">
              <Input label="Name" onChange={setName} placeholder="Production Cluster" required value={name} />
              <Input label="Description" onChange={setDescription} placeholder="Primary production environment" value={description} />
              <div className="grid grid-cols-2 gap-3">
                <AdminSelect label="Type" onChange={setEpType} options={[
                  { value: "docker", label: "Docker" },
                  { value: "swarm", label: "Swarm" },
                  { value: "kubernetes", label: "Kubernetes" },
                  { value: "edge", label: "Edge" },
                ]} value={epType} />
                <AdminSelect label="Connection Mode" onChange={setConnMode} options={[
                  { value: "direct", label: "Direct — dial the URL from this panel" },
                  { value: "tunnel", label: "Tunnel — dial the URL through a node" },
                  { value: "edge", label: "Edge — no URL, served by a beacon" },
                ]} value={connMode} />
              </div>
              <Input label={connMode === "edge" ? "URL (not used for edge endpoints)" : "URL"} onChange={setUrl} placeholder="https://docker.example.com:2375" value={url} />
              {urlError ? <p className="text-xs text-danger">{urlError}</p> : null}
            </AdminFormSection>
          </div>
          <ModalFooter
            confirmLabel={createMut.isPending ? "Creating…" : "Create"}
            disabled={createMut.isPending || !name.trim() || !!urlError}
            onCancel={() => setModal(null)}
            onConfirm={() => createMut.mutate()}
          />
        </Modal>
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
