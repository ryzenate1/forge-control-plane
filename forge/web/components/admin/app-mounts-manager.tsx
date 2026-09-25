"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { HardDrive, Plus, Trash2 } from "lucide-react";
import { fetchApps, type ApiApp } from "@/lib/api/apps";
import {
  createAppMount,
  deleteAppMount,
  fetchAppMounts,
  validateAppMount,
  type AppMount,
  type AppMountType,
  type CreateAppMountInput,
} from "@/lib/api/mounts";
import { errorMessage } from "@/lib/utils";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
  AdminErrorState,
  AdminSelect,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Badge,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  SectionHeader,
  Textarea,
} from "./admin-ui";

/**
 * Admin view for *per-application* mounts (`/apps/:id/mounts`) — the declarative
 * storage a single app carries through every redeploy. This is deliberately not
 * the admin `mounts` catalogue (node/egg-eligible host paths) that lives at
 * /admin/mounts; the two backends never overlap.
 */

type MountTypeMeta = {
  value: AppMountType;
  label: string;
  hint: string;
  /** The type reads a source path/name at all. */
  usesSource: boolean;
  /** The source is mandatory rather than optional. */
  requiresSource: boolean;
  usesContent: boolean;
  requiresContent: boolean;
};

// Keyed by the union so TypeScript forces every mount type to be described and
// `metaFor` never has to reason about a missing entry.
const MOUNT_TYPE_META: Record<AppMountType, MountTypeMeta> = {
  volume: {
    value: "volume",
    label: "Volume",
    hint: "Named Docker volume. Leave the source empty to let the platform allocate one for you.",
    usesSource: true,
    requiresSource: false,
    usesContent: false,
    requiresContent: false,
  },
  bind: {
    value: "bind",
    label: "Bind mount",
    hint: "A host directory mapped into the container. The source must be an absolute path the node allows.",
    usesSource: true,
    requiresSource: true,
    usesContent: false,
    requiresContent: false,
  },
  tmpfs: {
    value: "tmpfs",
    label: "Tmpfs",
    hint: "Ephemeral in-memory filesystem. Nothing is persisted across restarts, so there is no source.",
    usesSource: false,
    requiresSource: false,
    usesContent: false,
    requiresContent: false,
  },
  "seed-file": {
    value: "seed-file",
    label: "Seed file",
    hint: "A file body stored in the database and materialised at the target path on every deploy.",
    usesSource: false,
    requiresSource: false,
    usesContent: true,
    requiresContent: true,
  },
};

const MOUNT_TYPES: MountTypeMeta[] = [
  MOUNT_TYPE_META.volume,
  MOUNT_TYPE_META.bind,
  MOUNT_TYPE_META.tmpfs,
  MOUNT_TYPE_META["seed-file"],
];

const typeTone: Record<AppMountType, string> = {
  volume: "bg-sky-500/15 text-sky-300",
  bind: "bg-violet-500/15 text-violet-300",
  tmpfs: "bg-amber-500/15 text-amber-300",
  "seed-file": "bg-emerald-500/15 text-emerald-300",
};

const metaFor = (type: AppMountType): MountTypeMeta => MOUNT_TYPE_META[type];

type MountForm = {
  appId: string;
  name: string;
  type: AppMountType;
  source: string;
  target: string;
  readOnly: boolean;
  content: string;
};

type MountErrors = Partial<Record<keyof MountForm, string>>;

/** A per-app slice of the page: the mounts we loaded, or why loading failed. */
type AppMountGroup = { app: ApiApp; mounts: AppMount[]; error: string | null };

/** Cheap client-side checks only — the authoritative rules (reserved targets,
 * disallowed bind prefixes) are enforced by the backend validate endpoint. */
function validateForm(form: MountForm): MountErrors {
  const errors: MountErrors = {};
  const meta = metaFor(form.type);
  if (!form.appId) errors.appId = "Choose the application this mount belongs to.";
  if (!form.name.trim()) errors.name = "Name is required.";
  const target = form.target.trim();
  if (!target) errors.target = "Target path is required.";
  else if (!target.startsWith("/")) errors.target = "The target must be an absolute container path.";
  else if (target.split("/").includes("..")) errors.target = "The target must not traverse outside its root.";
  if (meta.requiresSource && !form.source.trim()) {
    errors.source = "A bind mount needs a host source path.";
  }
  if (meta.requiresContent && !form.content.trim()) {
    errors.content = "A seed-file mount needs the file content.";
  }
  return errors;
}

export function AppMountsManager() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();

  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState<MountForm | null>(null);
  const [errors, setErrors] = useState<MountErrors>({});

  const appsQuery = useQuery({ queryKey: ["apps"], queryFn: fetchApps });
  const apps = useMemo(() => appsQuery.data ?? [], [appsQuery.data]);
  const appIds = useMemo(() => apps.map((app) => app.id), [apps]);

  // One request per app, settled independently so a single failing app cannot
  // blank the whole page.
  const mountsQuery = useQuery({
    queryKey: ["admin", "app-mounts", appIds],
    enabled: apps.length > 0,
    queryFn: async () =>
      Promise.all(
        apps.map(async (app): Promise<AppMountGroup> => {
          try {
            return { app, mounts: await fetchAppMounts(app.id), error: null };
          } catch (error) {
            return { app, mounts: [], error: errorMessage(error, "Mounts could not be loaded.") };
          }
        }),
      ),
  });

  // Memoised off `data`, not the `[]` fallback: a fresh array identity each
  // render would invalidate every derived memo below on every keystroke.
  const groups = useMemo(() => mountsQuery.data ?? [], [mountsQuery.data]);
  const rows = useMemo(() => {
    const out: Array<{ key: string; app: ApiApp; mount: AppMount }> = [];
    for (const group of groups) {
      for (const mount of group.mounts) {
        out.push({ key: `${group.app.id}:${mount.id}`, app: group.app, mount });
      }
    }
    return out;
  }, [groups]);

  const failingGroups = groups.filter((group) => group.error);
  const mountedAppCount = groups.filter((group) => group.mounts.length > 0).length;

  const invalidateMounts = () => void qc.invalidateQueries({ queryKey: ["admin", "app-mounts"] });

  const openCreate = () => {
    setErrors({});
    setForm({
      appId: apps[0]?.id ?? "",
      name: "",
      type: "volume",
      source: "",
      target: "",
      readOnly: false,
      content: "",
    });
    setShowForm(true);
  };

  const createMut = useMutation({
    mutationFn: async (payload: { appId: string; input: CreateAppMountInput }) => {
      // Ask the backend to vet the draft first: it returns the concrete rule the
      // definition violates instead of a bare 400 from the insert.
      const check = await validateAppMount(payload.appId, payload.input);
      if (!check.valid) throw new Error(check.error ?? "The mount definition failed validation.");
      return createAppMount(payload.appId, payload.input);
    },
    onSuccess: (mount) => {
      setShowForm(false);
      setForm(null);
      invalidateMounts();
      toast({ tone: "success", title: "Mount created", message: `${mount.name} → ${mount.target}` });
    },
    onError: (error) => {
      toast({ tone: "error", title: "Mount not created", message: errorMessage(error) });
    },
  });

  const deleteMut = useMutation({
    mutationFn: (payload: { appId: string; mountId: string }) => deleteAppMount(payload.appId, payload.mountId),
    onSuccess: () => {
      invalidateMounts();
      toast({ tone: "success", title: "Mount removed" });
    },
    onError: (error) => {
      toast({ tone: "error", title: "Failed to remove mount", message: errorMessage(error) });
    },
  });

  const handleSubmit = () => {
    if (!form) return;
    const nextErrors = validateForm(form);
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) return;
    const meta = metaFor(form.type);
    const source = form.source.trim();
    const input: CreateAppMountInput = {
      name: form.name.trim(),
      type: form.type,
      target: form.target.trim(),
      readOnly: form.readOnly,
      ...(meta.usesSource && source ? { source } : {}),
      // Send the body verbatim for seed files; omit it otherwise so the backend
      // never stores content against a mount that cannot have any.
      ...(meta.usesContent ? { content: form.content } : {}),
    };
    createMut.mutate({ appId: form.appId, input });
  };

  // Only consumed inside the modal, which renders solely when a draft form
  // exists; the volume entry is a harmless default outside of it.
  const activeMeta: MountTypeMeta = form ? metaFor(form.type) : MOUNT_TYPE_META.volume;

  return (
    <div className="space-y-6">
      <SectionHeader
        title="App Mounts"
        sub="Persistent storage declared on individual applications — named volumes, host bind mounts, tmpfs and seed files. The deploy pipeline injects each one into the app's compose document so data survives redeploys. For node-wide mount allowlists use Storage Mounts instead."
        action={
          <Btn onClick={openCreate} disabled={apps.length === 0} tone="primary">
            <Plus size={14} /> New Mount
          </Btn>
        }
      />

      <Card>
        <CardHeader
          title="Declared Mounts"
          icon={HardDrive}
          action={
            rows.length > 0 ? (
              <span className="text-xs normal-case tracking-normal text-slate-400">
                {rows.length} mount{rows.length === 1 ? "" : "s"} across {mountedAppCount} app
                {mountedAppCount === 1 ? "" : "s"}
              </span>
            ) : null
          }
        />

        {appsQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-500">Loading applications…</div>
        ) : appsQuery.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={`Applications could not be loaded: ${errorMessage(appsQuery.error)}`}
              retry={() => void appsQuery.refetch()}
            />
          </div>
        ) : apps.length === 0 ? (
          <EmptyState
            icon={HardDrive}
            title="No applications yet"
            message="Mounts are declared per application. Create an application first, then attach its persistent storage here."
          />
        ) : mountsQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-500">Loading mounts…</div>
        ) : (
          <div className="space-y-4">
            {failingGroups.map((group) => (
              <div key={`error-${group.app.id}`} className="px-4 pt-4">
                <AdminErrorState
                  message={`${group.app.name}: ${group.error ?? "Mounts could not be loaded."}`}
                  retry={() => void mountsQuery.refetch()}
                />
              </div>
            ))}

            {rows.length === 0 && failingGroups.length === 0 ? (
              <EmptyState
                icon={HardDrive}
                title="No mounts declared"
                message="No application has persistent storage attached yet. Add a mount to keep data, uploads or config files across deploys."
              />
            ) : rows.length > 0 ? (
              <AdminTable label="Application mounts">
                <AdminTHead>
                  <AdminTh>Application</AdminTh>
                  <AdminTh>Mount</AdminTh>
                  <AdminTh>Type</AdminTh>
                  <AdminTh>Target</AdminTh>
                  <AdminTh>Source</AdminTh>
                  <AdminTh>Mode</AdminTh>
                  <AdminTh></AdminTh>
                </AdminTHead>
                <AdminTBody>
                  {rows.map(({ key, app, mount }) => (
                    <AdminTr key={key}>
                      <AdminTd>
                        <div className="min-w-0">
                          <p className="truncate text-sm font-semibold text-white">{app.name}</p>
                          <p className="truncate text-[10px] uppercase tracking-wider text-slate-500">{app.type}</p>
                        </div>
                      </AdminTd>
                      <AdminTd>
                        <span className="text-sm font-medium text-slate-100">{mount.name}</span>
                      </AdminTd>
                      <AdminTd>
                        <Badge className={typeTone[mount.type]}>{mount.type}</Badge>
                      </AdminTd>
                      <AdminTd>
                        <code className="block max-w-[24ch] truncate font-mono text-xs text-slate-300" title={mount.target}>
                          {mount.target}
                        </code>
                      </AdminTd>
                      <AdminTd>
                        {mount.source ? (
                          <code className="block max-w-[24ch] truncate font-mono text-xs text-slate-400" title={mount.source}>
                            {mount.source}
                          </code>
                        ) : (
                          <span className="text-xs text-slate-600">—</span>
                        )}
                      </AdminTd>
                      <AdminTd>
                        <span className={`text-xs ${mount.readOnly ? "text-amber-300" : "text-slate-400"}`}>
                          {mount.readOnly ? "Read-only" : "Read-write"}
                        </span>
                      </AdminTd>
                      <AdminTd>
                        <div className="flex justify-end">
                          <Btn
                            size="sm"
                            tone="danger"
                            disabled={deleteMut.isPending}
                            ariaLabel={`Remove mount ${mount.name}`}
                            onClick={() => {
                              void (async () => {
                                if (
                                  await confirm({
                                    title: `Remove mount “${mount.name}”?`,
                                    description:
                                      "The mount stops being injected into the next deploy of this application. The underlying volume or host files are not deleted.",
                                    danger: true,
                                    confirmLabel: "Remove",
                                  })
                                ) {
                                  deleteMut.mutate({ appId: app.id, mountId: mount.id });
                                }
                              })();
                            }}
                          >
                            <Trash2 size={12} /> Remove
                          </Btn>
                        </div>
                      </AdminTd>
                    </AdminTr>
                  ))}
                </AdminTBody>
              </AdminTable>
            ) : null}
          </div>
        )}
      </Card>

      {showForm && form ? (
        <Modal
          title="Declare an application mount"
          description="The definition is validated by the server before it is stored, so reserved paths and disallowed bind prefixes are rejected here rather than at deploy time."
          onClose={() => {
            setShowForm(false);
            setForm(null);
          }}
        >
          <div className="space-y-4">
            <AdminSelect
              label="Application"
              value={form.appId}
              onChange={(v) => setForm((current) => (current ? { ...current, appId: v } : current))}
              options={apps.map((app) => ({ value: app.id, label: app.name }))}
              placeholder="Select an application"
            />
            {errors.appId ? <p className="-mt-2 text-xs text-red-400">{errors.appId}</p> : null}

            <Input
              label="Name"
              value={form.name}
              onChange={(v) => {
                setForm((current) => (current ? { ...current, name: v } : current));
                setErrors((current) => ({ ...current, name: undefined }));
              }}
              placeholder="uploads"
            />
            {errors.name ? <p className="-mt-2 text-xs text-red-400">{errors.name}</p> : null}

            <AdminSelect
              label="Type"
              value={form.type}
              onChange={(v) => {
                const next = v as AppMountType;
                setForm((current) =>
                  current
                    ? { ...current, type: next, source: metaFor(next).usesSource ? current.source : "" }
                    : current,
                );
                setErrors({});
              }}
              options={MOUNT_TYPES.map(({ value, label }) => ({ value, label }))}
            />
            <p className="-mt-2 text-xs leading-5 text-slate-400">{activeMeta.hint}</p>

            {activeMeta.usesSource ? (
              <Input
                label={activeMeta.requiresSource ? "Host source path" : "Volume name (optional)"}
                value={form.source}
                onChange={(v) => {
                  setForm((current) => (current ? { ...current, source: v } : current));
                  setErrors((current) => ({ ...current, source: undefined }));
                }}
                placeholder={activeMeta.requiresSource ? "/srv/data/app" : "app-uploads"}
                mono
              />
            ) : null}
            {errors.source ? <p className="-mt-2 text-xs text-red-400">{errors.source}</p> : null}

            <Input
              label="Target path in the container"
              value={form.target}
              onChange={(v) => {
                setForm((current) => (current ? { ...current, target: v } : current));
                setErrors((current) => ({ ...current, target: undefined }));
              }}
              placeholder="/data/uploads"
              mono
            />
            {errors.target ? <p className="-mt-2 text-xs text-red-400">{errors.target}</p> : null}

            {activeMeta.usesContent ? (
              <Textarea
                label="File content"
                value={form.content}
                onChange={(v) => {
                  setForm((current) => (current ? { ...current, content: v } : current));
                  setErrors((current) => ({ ...current, content: undefined }));
                }}
                rows={5}
                placeholder="The exact bytes written to the target path on deploy."
              />
            ) : null}
            {errors.content ? <p className="-mt-2 text-xs text-red-400">{errors.content}</p> : null}

            <label className="flex cursor-pointer items-center gap-2 text-sm text-slate-300">
              <input
                type="checkbox"
                className="h-4 w-4 accent-[var(--brand)]"
                checked={form.readOnly}
                onChange={(event) =>
                  setForm((current) => (current ? { ...current, readOnly: event.target.checked } : current))
                }
              />
              Read-only
            </label>

            {createMut.isError ? (
              <p className="ui-alert ui-alert-error" role="alert">
                {errorMessage(createMut.error)}
              </p>
            ) : null}

            <ModalFooter
              onCancel={() => {
                setShowForm(false);
                setForm(null);
              }}
              onConfirm={handleSubmit}
              disabled={!form.appId || !form.name.trim() || !form.target.trim() || createMut.isPending}
              confirmLabel={createMut.isPending ? "Validating…" : "Create mount"}
            />
          </div>
        </Modal>
      ) : null}

      {renderConfirm()}
    </div>
  );
}

export default AppMountsManager;
