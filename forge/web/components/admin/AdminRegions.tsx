"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2 } from "lucide-react";
import { GlobeGridIcon } from "@/components/ui/forge-icons";
import { createRegion, deleteRegion, fetchRegions, updateRegion, type ApiRegion } from "@/lib/api";
import { REFRESH, sourceState } from "@/lib/admin/telemetry";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/components/ui/toast";
import { AdminErrorState, AdminFormSection, AdminLoadingState, AdminPageLayout, AdminTable, AdminTBody, AdminTd, AdminTh, AdminTHead, AdminTr, Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, Textarea, cn } from "./admin-ui";
import { formatDate } from "@/lib/utils";

const normalizeSlug = (value: string): string => value.trim().toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
const validateSlug = (value: string): boolean => /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(normalizeSlug(value));

export function AdminRegions() {
  const qc = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const { toast } = useToast();
  // Inventory cadence, the same one the Nodes page uses: these rows change on
  // operator action, and a page that shows a node count should notice.
  const regionsQuery = useQuery({
    queryKey: ["regions"],
    queryFn: fetchRegions,
    refetchInterval: REFRESH.inventory,
    retry: false,
  });
  const regionsSource = sourceState(regionsQuery, REFRESH.inventory);
  const regions = useMemo(() => Array.isArray(regionsQuery.data) ? regionsQuery.data : [], [regionsQuery.data]);

  const [search, setSearch] = useState("");
  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return regions;
    return regions.filter((region) =>
      region.name.toLowerCase().includes(q)
      || region.slug.toLowerCase().includes(q)
      || (region.description ?? "").toLowerCase().includes(q));
  }, [regions, search]);

  const [editing, setEditing] = useState<"new" | ApiRegion | null>(null);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [description, setDescription] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [formError, setFormError] = useState<string | null>(null);

  const close = () => setEditing(null);
  const open = (region?: ApiRegion) => {
    setEditing(region ?? "new"); setName(region?.name ?? ""); setSlug(region?.slug ?? "");
    setDescription(region?.description ?? ""); setEnabled(region?.enabled ?? true);
    setFormError(null);
  };

  const createMut = useMutation({
    mutationFn: () => createRegion({
      name: name.trim(),
      slug: normalizeSlug(slug),
      description: description.trim(),
      enabled
    }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["regions"] });
      close();
      toast({ tone: "success", title: "Region created", message: `${name.trim()} is available for placement.` });
    },
    onError: (e: Error) => { setFormError(e.message || "Could not create the region."); },
  });
  const updateMut = useMutation({
    mutationFn: () => updateRegion((editing as ApiRegion).id, {
      name: name.trim(),
      slug: normalizeSlug(slug),
      description: description.trim(),
      enabled
    }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["regions"] });
      close();
      toast({ tone: "success", title: "Region updated", message: `${name.trim()} saved.` });
    },
    onError: (e: Error) => { setFormError(e.message || "Could not update the region."); },
  });
  const deleteMut = useMutation({
    mutationFn: deleteRegion,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["regions"] });
      toast({ tone: "success", title: "Region deleted" });
    },
    // The refused reason used to go only to the console, so a failed delete left
    // no trace on screen; it now also surfaces in the row's own error box.
    onError: (e: Error) => { setFormError(e.message || "Could not delete the region."); toast({ tone: "error", title: "Failed to delete region", message: e.message }); },
  });

  return <AdminPageLayout>
    <SectionHeader
      status={<FreshnessBadge state={regionsSource} />}
      info={{
        title: "Regions",
        triggerLabel: "About Regions",
        eyebrow: "Architecture & Semantics",
        description: "Cluster regions and placement zones for scheduling and recovery planning.",
        sections: [
          { title: "Placement zones", content: "Regions group nodes into placement zones. The scheduler prefers nodes in enabled regions when choosing where a workload lands." },
          { title: "Recovery planning", content: "Regions scope capacity and recovery planning. A region with nodes cannot be deleted until its nodes are moved or removed." },
          { title: "How a node joins one", content: "A node counts here only when its region was chosen in the create-node form, which writes the region link. A node carrying only the older free-text region field is not counted, and a count of zero therefore means 'not linked' rather than 'empty'. Region cannot be changed after creation: the node update endpoint has no region field." },
        ],
      }}
      action={<Btn onClick={() => open()}><Plus size={14}/> New Region</Btn>}
    />

    <Card>
      <CardHeader
        icon={GlobeGridIcon}
        title={regionsSource.status === "ready" ? `${regions.length} region${regions.length === 1 ? "" : "s"}` : "Regions"}
        action={<FreshnessBadge state={regionsSource} />}
      />
      <div className="mb-4 flex flex-wrap items-end gap-3">
        <div className="min-w-[220px] flex-1">
          <Input label="Search regions" placeholder="Name, slug or description" value={search} onChange={setSearch} />
        </div>
        {regionsSource.status === "ready" ? (
          <span className="pb-2 text-xs text-text-subtle">{filtered.length} of {regions.length} shown</span>
        ) : null}
      </div>
      {/* loading → error → empty, in that precedence. `isError` used to render a
          banner *and* fall through to the empty branch, so a failed read said
          "No regions configured" about regions that may exist. */}
      {regionsQuery.isPending ? (
        <div className="p-4"><AdminLoadingState label="Loading regions…" /></div>
      ) : regionsQuery.isError ? (
        <div className="p-4">
          <AdminErrorState message={`Regions could not be loaded: ${regionsSource.message ?? "the request failed"}. The list below is not empty — it is unread.`} retry={() => void regionsQuery.refetch()} />
        </div>
      ) : filtered.length === 0 ? (
        <EmptyState
          icon={GlobeGridIcon}
          title={search ? "No matching regions" : "No regions configured"}
          message={search ? `No region matches “${search}”.` : "Create a region to group nodes into a placement zone."}
        />
      ) : (
        <AdminTable label="Regions">
          <AdminTHead>
            <AdminTh>Region</AdminTh>
            <AdminTh>Slug</AdminTh>
            <AdminTh>Linked nodes</AdminTh>
            <AdminTh>Status</AdminTh>
            <AdminTh>Updated</AdminTh>
            <AdminTh><span className="sr-only">Actions</span></AdminTh>
          </AdminTHead>
          <AdminTBody>
            {filtered.map((region) => {
              const blockedByNodes = (region.nodeCount ?? 0) > 0;
              return (
                <AdminTr key={region.id}>
                  <AdminTd>
                    <button className="text-left font-semibold text-text hover:text-brand" onClick={() => open(region)} type="button">{region.name}</button>
                    <p className="text-xs text-text-subtle">{region.description || <span className="text-text-muted">No description</span>}</p>
                  </AdminTd>
                  <AdminTd className="font-mono text-xs">{region.slug}</AdminTd>
                  <AdminTd>
                    <span title={blockedByNodes ? undefined : "No node has this region linked. A node links it only when chosen in the create-node form."}>
                      <Pill tone={blockedByNodes ? "blue" : "neutral"}>{region.nodeCount ?? 0}</Pill>
                    </span>
                  </AdminTd>
                  <AdminTd><Pill tone={region.enabled ? "green" : "yellow"}>{region.enabled ? "Enabled" : "Disabled"}</Pill></AdminTd>
                  <AdminTd className="font-mono text-xs text-text-subtle">{region.updatedAt ? formatDate(region.updatedAt) : region.createdAt ? formatDate(region.createdAt) : <span title="The API returned no timestamp">not reported</span>}</AdminTd>
                  <AdminTd className="text-right">
                    {/* The delete control is named, and when it is withheld the
                        reason is on screen rather than in a hover state. */}
                    {blockedByNodes ? (
                      <span className="text-xs leading-5 text-warn">{region.nodeCount} node{region.nodeCount === 1 ? "" : "s"} still linked — move or delete them first.</span>
                    ) : null}
                    <Btn
                      ariaLabel={blockedByNodes ? `Delete region ${region.name}, unavailable while it has nodes` : `Delete region ${region.name}`}
                      disabled={blockedByNodes || deleteMut.isPending}
                      onClick={() => { void (async () => { if (await confirm({ title: `Delete region ${region.name}?`, description: `${region.slug} will be removed from placement and recovery planning. Nodes that were never linked to it are unaffected. This cannot be undone.`, danger: true, confirmLabel: "Delete" })) deleteMut.mutate(region.id); })(); }}
                      size="sm"
                      tone="danger"
                    >
                      <Trash2 size={12} /> Delete
                    </Btn>
                  </AdminTd>
                </AdminTr>
              );
            })}
          </AdminTBody>
        </AdminTable>
      )}
      {deleteMut.isError ? (
        <div className="p-4">
          <AdminErrorState message={`Could not delete the region: ${(deleteMut.error as Error).message}`} />
        </div>
      ) : null}
    </Card>

    {editing ? (
      <Modal onClose={close} title={editing === "new" ? "Create Region" : "Edit Region"}>
        <div className="space-y-4">
          <AdminFormSection title="Region Details">
            <Input label="Name" value={name} onChange={setName} />
            <Input label="Slug (lowercase letters, numbers, and hyphens)" mono onChange={setSlug} placeholder="us-east" value={slug} />
            <Textarea label="Description" value={description} onChange={setDescription} />
            <label className={cn("flex items-center gap-2 text-sm text-text")}>
              <input checked={enabled} className="accent-[var(--brand)]" onChange={(event) => setEnabled(event.target.checked)} type="checkbox" />
              Enabled for placement
            </label>
          </AdminFormSection>
          {/* A validation hint must not wear the same colour as body copy. */}
          {!validateSlug(slug) && slug.trim() !== "" ? (
            <p className="text-xs text-danger" role="alert">Slug may contain lowercase letters, numbers, and single hyphens only.</p>
          ) : null}
          {formError ? <AdminErrorState message={formError} /> : null}
        </div>
        <ModalFooter
          onCancel={close}
          confirmLabel={editing === "new" ? "Create" : "Save"}
          disabled={!name.trim() || !validateSlug(slug) || createMut.isPending || updateMut.isPending}
          onConfirm={() => editing === "new" ? createMut.mutate() : updateMut.mutate()}
        />
      </Modal>
    ) : null}
    {renderConfirm()}
  </AdminPageLayout>;
}
