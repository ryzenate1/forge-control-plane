"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
import { LocationPinIcon } from "@/components/ui/forge-icons";
import { type ApiLocation, createLocation, deleteLocation, fetchLocations, updateLocation } from "@/lib/api";
import { REFRESH, sourceState } from "@/lib/admin/telemetry";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { AdminErrorState, AdminFormSection, AdminLoadingState, AdminPageLayout, AdminTable, AdminTBody, AdminTd, AdminTh, AdminTHead, AdminTr, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "./admin-ui";
import { formatDate } from "@/lib/utils";

export function AdminLocations() {
  const qc = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const { toast } = useToast();
  const locationsQuery = useQuery({
    queryKey: ["locations"],
    queryFn: fetchLocations,
    refetchInterval: REFRESH.inventory,
    retry: false,
  });
  const locationsSource = sourceState(locationsQuery, REFRESH.inventory);
  const locations = useMemo(() => Array.isArray(locationsQuery.data) ? locationsQuery.data : [], [locationsQuery.data]);

  const [modal, setModal] = useState<null | "create" | { id: string; short: string; long: string }>(null);
  const [short, setShort] = useState("");
  const [long, setLong] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  // The API only requires a non-empty `short`, so a duplicate is accepted — but
  // the code is what the Nodes table and a node's legacy region string display,
  // so two identical codes read as one place. Blocked here, with the reason.
  const trimmedShort = short.trim();
  const duplicateShort = useMemo(() => {
    if (!trimmedShort) return false;
    const editingId = modal && modal !== "create" ? modal.id : null;
    return locations.some((loc) => loc.id !== editingId && loc.short.trim().toLowerCase() === trimmedShort.toLowerCase());
  }, [locations, trimmedShort, modal]);

  const createMut = useMutation({
    mutationFn: () => createLocation({ short: trimmedShort, long: long.trim() }),
    onSuccess: (created) => {
      qc.invalidateQueries({ queryKey: ["locations"] });
      setModal(null); setShort(""); setLong(""); setFormError(null);
      toast({ tone: "success", title: "Location created", message: `${created.short} is available when registering a node.` });
    },
    onError: (e: Error) => { setFormError(e.message || "Failed to create location"); },
  });

  const updateMut = useMutation({
    mutationFn: (id: string) => updateLocation(id, { short: trimmedShort, long: long.trim() }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["locations"] });
      setModal(null); setFormError(null);
      toast({ tone: "success", title: "Location updated", message: `${trimmedShort} saved.` });
    },
    onError: (e: Error) => { setFormError(e.message); },
  });

  const deleteMut = useMutation({
    mutationFn: deleteLocation,
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["locations"] }); toast({ tone: "success", title: "Location deleted" }); },
    // One notify policy for this group: the shared admin toast, not a bare
    // library call the rest of the slice does not use.
    onError: (e: Error) => toast({ tone: "error", title: "Failed to delete location", message: e.message || "Failed to delete location" }),
  });

  const openEdit = (loc: ApiLocation) => {
    setShort(loc.short);
    setLong(loc.long);
    setFormError(null);
    setModal({ id: loc.id, short: loc.short, long: loc.long });
  };

  const openCreate = () => { setShort(""); setLong(""); setFormError(null); setModal("create"); };

  return (
    <AdminPageLayout>
      <SectionHeader
        status={<FreshnessBadge state={locationsSource} />}
        info={{
          title: "Locations",
          triggerLabel: "About Locations",
          eyebrow: "Architecture & Semantics",
          description: "Physical and logical node locations for grouping capacity.",
          sections: [
            { title: "Grouping nodes", content: "Locations group nodes by datacenter or logical site. Nodes pick a location at creation; placement and filtering read it back. The short code is also copied into a node's legacy free-text region field, which is why two locations sharing a code are ambiguous." },
            { title: "Counts", content: "Node and server counts are live inventory rollups from the same read, so a number only appears once the counts have actually arrived." },
            { title: "Removal", content: "A location with nodes cannot be removed until its nodes move; the delete control is withheld on those rows and states the count, rather than failing after the click." },
          ],
        }}
        action={<Btn onClick={openCreate}><Plus size={14} /> New Location</Btn>}
      />

      <Card>
        <CardHeader action={<FreshnessBadge state={locationsSource} />} icon={LocationPinIcon} title="All locations" />
        {locationsQuery.isPending ? (
          <div className="p-4"><AdminLoadingState label="Loading locations…" /></div>
        ) : locationsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState message={`Locations could not be loaded: ${locationsSource.message ?? "the request failed"}. The list below is not empty — it is unread.`} retry={() => void locationsQuery.refetch()} />
          </div>
        ) : locations.length === 0 ? (
          <EmptyState icon={LocationPinIcon} title="No locations" message="No locations yet. Create one to group nodes geographically." />
        ) : (
          <AdminTable label="Locations">
            <AdminTHead>
              <AdminTh>Short</AdminTh>
              <AdminTh>Description</AdminTh>
              <AdminTh>Nodes</AdminTh>
              <AdminTh>Servers</AdminTh>
              <AdminTh>ID</AdminTh>
              <AdminTh>Created</AdminTh>
              <AdminTh><span className="sr-only">Actions</span></AdminTh>
            </AdminTHead>
            <AdminTBody>
              {locations.map((loc) => {
                const blockedByNodes = (loc.nodeCount ?? 0) > 0;
                return (
                  <AdminTr key={loc.id}>
                    {/* Was `text-[var(--brand)]`, the link colour, on a cell with
                        no click handler: a colour promising navigation that was
                        not there. The code stays monospace and neutral. */}
                    <AdminTd className="font-mono font-semibold text-text">{loc.short}</AdminTd>
                    <AdminTd className="text-text">{loc.long || <span className="text-text-muted">No description</span>}</AdminTd>
                    <AdminTd><Pill tone={blockedByNodes ? "blue" : "neutral"}>{loc.nodeCount ?? 0}</Pill></AdminTd>
                    <AdminTd><Pill tone={(loc.serverCount ?? 0) > 0 ? "green" : "neutral"}>{loc.serverCount ?? 0}</Pill></AdminTd>
                    <AdminTd className="font-mono text-xs text-text-subtle">{loc.id.slice(0, 8)}</AdminTd>
                    <AdminTd className="font-mono text-xs text-text-subtle">{loc.createdAt ? formatDate(loc.createdAt) : <span title="The API returned no creation timestamp">not reported</span>}</AdminTd>
                    <AdminTd>
                      <div className="flex flex-col items-end gap-1">
                        {blockedByNodes ? (
                          <span className="text-right text-xs leading-4 text-warn">{loc.nodeCount} node{loc.nodeCount === 1 ? "" : "s"} here — move them first.</span>
                        ) : null}
                        <div className="flex items-center gap-1">
                          <Btn size="sm" tone="ghost" onClick={() => openEdit(loc)}>Edit</Btn>
                          <Btn
                            ariaLabel={blockedByNodes ? `Delete location ${loc.short}, unavailable while it has nodes` : `Delete location ${loc.short}`}
                            disabled={blockedByNodes || deleteMut.isPending}
                            size="sm"
                            tone="danger"
                            onClick={() => { void (async () => { if (await confirm({ title: `Delete location "${loc.short}"?`, description: loc.long ? `"${loc.long}" and its placement metadata will be removed. This cannot be undone.` : "This placement will be removed. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(loc.id); })(); }}
                          >
                            <Trash2 size={12} /> Delete
                          </Btn>
                        </div>
                      </div>
                    </AdminTd>
                  </AdminTr>
                );
              })}
            </AdminTBody>
          </AdminTable>
        )}
        {deleteMut.isError ? (
          <div className="p-4"><AdminErrorState message={`Could not delete the location: ${(deleteMut.error as Error).message}`} /></div>
        ) : null}
      </Card>

      {modal !== null ? (
        <Modal title={modal === "create" ? "Create Location" : "Edit Location"} onClose={() => setModal(null)}>
          <div className="space-y-4">
            <AdminFormSection title="Location Details">
              <Input label="Short code (e.g. US)" value={short} onChange={setShort} placeholder="US" mono required />
              <Input label="Description (e.g. United States)" value={long} onChange={setLong} placeholder="United States" required />
            </AdminFormSection>
            {trimmedShort === "" || long.trim() === "" ? (
              <p className="text-xs text-warn" role="alert">Short code and description are both required.</p>
            ) : null}
            {duplicateShort ? (
              <p className="text-xs text-danger" role="alert">
                Another location already uses “{trimmedShort}”. Nodes list this code as their location and legacy region, so two identical codes would be indistinguishable — choose a different one.
              </p>
            ) : null}
            {formError ? <AdminErrorState message={formError} /> : null}
          </div>
          <ModalFooter
            onCancel={() => setModal(null)}
            onConfirm={() => {
              if (trimmedShort === "" || long.trim() === "") {
                setFormError("Short code and description are required.");
                return;
              }
              if (duplicateShort) {
                setFormError("That short code is already used by another location.");
                return;
              }
              setFormError(null);
              if (modal === "create") createMut.mutate();
              else updateMut.mutate((modal as { id: string }).id);
            }}
            disabled={createMut.isPending || updateMut.isPending || trimmedShort === "" || long.trim() === "" || duplicateShort}
            confirmLabel={modal === "create" ? "Create" : "Save"}
          />
        </Modal>
      ) : null}
      {renderConfirm()}
    </AdminPageLayout>
  );
}
