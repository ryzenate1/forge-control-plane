"use client";

import { useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plug, Plus, Trash2, Upload, Settings2, Wrench, Search } from "lucide-react";
import { type ApiPlugin } from "@/lib/api";
import {
  deletePlugin,
  fetchPluginHooks,
  fetchPluginRuntimeRecords,
  fetchPlugins,
  importPluginFile,
  importPluginFromURL,
  togglePluginLifecycle,
  updatePlugin,
  type PluginRuntimeRecord,
} from "@/lib/api/plugins";
import { useConfirm } from "@/components/ui/confirm-dialog";
import {
  AdminErrorState,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  CardHeader,
  Input,
  Modal,
  ModalFooter,
  Pill,
  SectionHeader,
  AdminFormSection,
} from "./admin-ui";
import { DataState, Reading, FreshnessBadge } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { PageInfoDisclosure } from "@/components/ui/page-info-disclosure";
import { useToast } from "@/components/ui/toast";
import { errorMessage, formatDate } from "@/lib/utils";

/**
 * Plugins are **manifest metadata**. Forge has no third-party catalogue: the
 * route historically called `/marketplace` returns the plugin service's own
 * list of registered rows, and `/discover` is a scan that registers every
 * manifest it finds as it goes. An "Install" control against either therefore
 * always hits the service's duplicate-name rejection, so this page offers
 * none — see `lib/api/plugins.ts` for the endpoint evidence.
 */

function runtimeOf(records: PluginRuntimeRecord[], id: string): PluginRuntimeRecord | undefined {
  return records.find((record) => record.id === id);
}

/**
 * The lifecycle `state` the `/enable` and `/disable` handlers write. A plugin
 * with no reported state renders as `unknown` — never as "Disabled", which is
 * what the old page showed for every row because it read the `enabled` column
 * these routes do not touch.
 */
function stateView(state?: string): { tone: "ok" | "warn" | "danger" | "neutral" | "unknown"; label: string } {
  switch (state) {
    case "enabled": return { tone: "ok", label: "Enabled" };
    case "disabled": return { tone: "neutral", label: "Disabled" };
    case "installed": return { tone: "neutral", label: "Registered, never enabled" };
    case "updating": return { tone: "warn", label: "Updating" };
    case "error": return { tone: "danger", label: "Error" };
    default: return { tone: "unknown", label: "State not reported" };
  }
}

export function AdminPlugins() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [search, setSearch] = useState("");

  const query = useQuery({ queryKey: ["plugins"], queryFn: fetchPlugins });
  // The service projection carries the lifecycle state; the registry list
  // carries the editable metadata. Both are needed for the table to be true.
  const runtimeQuery = useQuery({ queryKey: ["plugins-runtime"], queryFn: fetchPluginRuntimeRecords });

  const [open, setOpen] = useState(false);
  const [url, setUrl] = useState("");
  const [showFile, setShowFile] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const [editingPlugin, setEditingPlugin] = useState<ApiPlugin | null>(null);
  const [hooksPlugin, setHooksPlugin] = useState<{ id: string; name: string } | null>(null);
  const hooksQuery = useQuery({
    queryKey: ["plugin-hooks", hooksPlugin?.id],
    queryFn: () => fetchPluginHooks(hooksPlugin!.id),
    enabled: Boolean(hooksPlugin),
  });

  const importMut = useMutation({
    mutationFn: () => importPluginFromURL(url.trim()),
    onSuccess: () => {
      setOpen(false);
      setUrl("");
      void qc.invalidateQueries({ queryKey: ["plugins"] });
      void qc.invalidateQueries({ queryKey: ["plugins-runtime"] });
      toast({ tone: "success", title: "Manifest imported", message: "Registered as metadata. It does not run code." });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Import failed", message: e.message }),
  });

  const fileImportMut = useMutation({
    mutationFn: (file: File) => importPluginFile(file),
    onSuccess: () => {
      setShowFile(false);
      if (fileRef.current) fileRef.current.value = "";
      void qc.invalidateQueries({ queryKey: ["plugins"] });
      void qc.invalidateQueries({ queryKey: ["plugins-runtime"] });
      toast({ tone: "success", title: "File imported", message: "Registered as metadata. It does not run code." });
    },
    onError: (e: Error) => toast({ tone: "error", title: "File import failed", message: e.message }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deletePlugin(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["plugins"] });
      void qc.invalidateQueries({ queryKey: ["plugins-runtime"] });
      toast({ tone: "success", title: "Plugin record deleted" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  const lifecycleMut = useMutation({
    mutationFn: ({ id, action }: { id: string; action: "enable" | "disable" }) => togglePluginLifecycle(id, action),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["plugins-runtime"] });
      toast({ tone: "success", title: "Lifecycle state updated", message: "Verify the status pill once the list reloads." });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Lifecycle failed", message: e.message }),
  });

  const patchMut = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Record<string, unknown> }) => updatePlugin(id, data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["plugins"] });
      setEditingPlugin(null);
      toast({ tone: "success", title: "Plugin metadata updated" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Update failed", message: e.message }),
  });

  const plugins = useMemo(() => (Array.isArray(query.data) ? query.data : []), [query.data]);
  const runtime = useMemo(() => (Array.isArray(runtimeQuery.data) ? runtimeQuery.data : []), [runtimeQuery.data]);

  const filtered = useMemo(() => {
    if (!search) return plugins;
    const q = search.toLowerCase();
    return plugins.filter((p) => `${p.name} ${p.id} ${p.kind ?? ""} ${p.version ?? ""}`.toLowerCase().includes(q));
  }, [plugins, search]);

  const listState = sourceState(query);
  const lifecycleReadFailed = runtimeQuery.isError || runtimeQuery.data === undefined;

  return <div className="space-y-6">
    <SectionHeader
      sub="Manifest metadata registered with the control plane. Import, edit and remove records; enabling a plugin only gates its hooks."
      status={<FreshnessBadge state={listState} />}
      action={
        <div className="flex gap-2">
          <Btn tone="ghost" onClick={() => setShowFile(true)}><Upload size={14} /> Import file</Btn>
          <Btn onClick={() => setOpen(true)}><Plus size={14} /> Import manifest URL</Btn>
        </div>
      }
      info={{
        title: "Plugins",
        triggerLabel: "About plugins",
        description: "What this page can and cannot do about plugin manifests.",
        sections: [
          {
            title: "Metadata only",
            content: "Importing stores the manifest and lists it here. Forge does not load plugin code from these records, and there is no third-party catalogue to browse: the endpoints historically named marketplace and discover both describe plugins this panel already has.",
          },
          {
            title: "What Enable does",
            content: "Enable and disable write the plugin service's lifecycle state. Hooks registered in-process run only for a plugin in the enabled state; nothing else changes. The status pill below reads that state from the service, so a change is visible after the reload — when the service cannot be reached the pill reads \"State not reported\" and the buttons are disabled.",
          },
          {
            title: "Settings",
            content: "The API can replace a plugin's settings document, but no read endpoint returns the current settings, so this page offers no settings editor rather than one that opens blind.",
          },
        ],
      }}
    />

    <Card>
      <CardHeader
        title={search ? `Plugins · ${filtered.length} of ${plugins.length} match` : `Plugins · ${query.data === undefined ? "—" : `${plugins.length} registered`}`}
        icon={Plug}
        action={
          <div className="relative">
            <Search aria-hidden="true" size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-text-muted" />
            <label className="sr-only" htmlFor="plugin-search">Search registered plugins by name, id, kind or version</label>
            <input
              aria-label="Search registered plugins"
              className="ui-input h-9 w-56 pl-9"
              id="plugin-search"
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search plugins…"
              value={search}
            />
          </div>
        }
      />
      <DataState
        emptyMessage="No plugin manifests are registered. Use Import file or Import manifest URL."
        emptyTitle="No plugins registered"
        isEmpty={plugins.length === 0}
        loadingLabel="Loading plugin manifests…"
        onRetry={() => void query.refetch()}
        state={listState}
      >
        {runtimeQuery.isError ? (
          <div className="p-4">
            <AdminErrorState
              message={`Lifecycle state is unavailable (${errorMessage(runtimeQuery.error, "the plugin service did not respond")}), so status shows as not reported and Enable/Disable are disabled.`}
              retry={() => void runtimeQuery.refetch()}
            />
          </div>
        ) : null}
        <AdminTable label="Registered plugins">
          <AdminTHead>
            <AdminTh>Plugin</AdminTh>
            <AdminTh>Kind</AdminTh>
            <AdminTh>Version</AdminTh>
            <AdminTh>Lifecycle state</AdminTh>
            <AdminTh>Updated</AdminTh>
            <AdminTh className="text-right">Actions</AdminTh>
          </AdminTHead>
          <AdminTBody>
            {filtered.map((plugin) => {
              const record = runtimeOf(runtime, plugin.id);
              const view = stateView(record?.state);
              const isPending = lifecycleMut.isPending && lifecycleMut.variables?.id === plugin.id;
              return (
                <AdminTr key={plugin.id}>
                  <AdminTd>
                    <p className="font-medium text-text">{plugin.name}</p>
                    {plugin.description ? <p className="text-meta text-text-subtle">{plugin.description}</p> : null}
                  </AdminTd>
                  <AdminTd className="text-text-subtle"><Reading className="text-text-subtle" reason="Manifest did not report a kind" value={plugin.kind} /></AdminTd>
                  <AdminTd><Reading reason="Manifest did not report a version" value={plugin.version} /></AdminTd>
                  <AdminTd>
                    <div className="flex flex-col items-start gap-1">
                      <Pill tone={view.tone}>{view.label}</Pill>
                      {record?.state === "error" && record.error ? <span className="max-w-prose text-[11px] text-danger">{record.error}</span> : null}
                    </div>
                  </AdminTd>
                  <AdminTd className="text-text-subtle"><Reading reason="Not reported" value={record?.updatedAt ? formatDate(record.updatedAt) : plugin.installedAt ? formatDate(plugin.installedAt) : undefined} /></AdminTd>
                  <AdminTd className="text-right">
                    <div className="flex justify-end gap-1.5 flex-wrap">
                      <Btn size="sm" tone="ghost" onClick={() => setHooksPlugin({ id: plugin.id, name: plugin.name })} title="Run this plugin's info hook">
                        <Wrench size={12} /> Hooks
                      </Btn>
                      <Btn size="sm" tone="ghost" onClick={() => setEditingPlugin(plugin)} ariaLabel={`Edit metadata for ${plugin.name}`}>
                        <Settings2 size={12} /> Edit
                      </Btn>
                      <Btn
                        size="sm"
                        tone="ghost"
                        disabled={lifecycleReadFailed || isPending}
                        loading={isPending}
                        onClick={() => lifecycleMut.mutate({ id: plugin.id, action: record?.state === "enabled" ? "disable" : "enable" })}
                        title={lifecycleReadFailed ? "Unavailable while the plugin service cannot be read" : undefined}
                      >
                        {record?.state === "enabled" ? "Disable" : "Enable"}
                      </Btn>
                      <Btn
                        ariaLabel={`Delete ${plugin.name}`}
                        size="sm"
                        tone="danger"
                        onClick={() => {
                          void (async () => {
                            const ok = await confirm({
                              title: `Delete the record for ${plugin.name}?`,
                              description: "The manifest metadata is removed from the panel. Deployments already running are unaffected. This cannot be undone.",
                              danger: true,
                              confirmLabel: "Delete",
                            });
                            if (ok) deleteMut.mutate(plugin.id);
                          })();
                        }}
                      >
                        <Trash2 size={12} />
                      </Btn>
                    </div>
                  </AdminTd>
                </AdminTr>
              );
            })}
          </AdminTBody>
        </AdminTable>
      </DataState>
    </Card>

    {hooksPlugin ? (
      <Card>
        <CardHeader
          title={`Hooks · ${hooksPlugin.name}`}
          icon={Wrench}
          action={<Btn size="sm" tone="ghost" onClick={() => setHooksPlugin(null)}>Close</Btn>}
        />
        <div className="p-4">
          <DataState
            emptyMessage="No handler returned a result. Hooks run only for an enabled plugin that registered one in the control plane process."
            emptyTitle="No hooks ran"
            isEmpty={(hooksQuery.data ?? []).length === 0}
            loadingLabel="Running the info hook…"
            onRetry={() => void hooksQuery.refetch()}
            state={sourceState(hooksQuery)}
          >
            <div className="space-y-2">
              {(hooksQuery.data ?? []).map((hook, idx) => (
                <pre className="overflow-auto rounded-lg border border-line bg-overlay-subtle p-3 text-xs text-text-subtle" key={idx}>{JSON.stringify(hook, null, 2)}</pre>
              ))}
            </div>
          </DataState>
        </div>
      </Card>
    ) : null}

    {/* Import URL Modal */}
    {open ? (
      <Modal title="Import plugin manifest" description="Fetches a JSON manifest and stores its metadata." onClose={() => setOpen(false)}>
        <AdminFormSection title="Manifest URL">
          <Input label="HTTPS manifest URL" value={url} onChange={setUrl} placeholder="https://example.com/plugin.json" />
          <p className="text-meta text-text-subtle">The control plane fetches the URL and stores the JSON as metadata. Review the source and its permissions before importing; importing does not run anything.</p>
        </AdminFormSection>
        {importMut.error ? <AdminErrorState message={errorMessage(importMut.error, "The manifest could not be imported.")} /> : null}
        <ModalFooter
          onCancel={() => setOpen(false)}
          onConfirm={() => importMut.mutate()}
          disabled={!/^https:\/\//i.test(url.trim()) || importMut.isPending}
          confirmLabel={importMut.isPending ? "Importing…" : "Import metadata"}
        />
      </Modal>
    ) : null}

    {/* Import File Modal */}
    {showFile ? (
      <Modal title="Import plugin manifest file" description="Uploads a JSON manifest for storage as metadata." onClose={() => { setShowFile(false); if (fileRef.current) fileRef.current.value = ""; }}>
        <AdminFormSection title="Manifest file">
          <label className="ui-label" htmlFor="plugin-manifest-file">Manifest file (JSON)</label>
          <input
            accept=".json,application/json"
            aria-label="Plugin manifest file"
            className="block w-full text-sm text-text-subtle"
            id="plugin-manifest-file"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) fileImportMut.mutate(f);
            }}
            ref={fileRef}
            type="file"
          />
          <p className="text-meta text-text-subtle">The file is uploaded as <code className="font-mono">multipart/form-data</code> and stored as metadata. Selecting a file uploads it immediately.</p>
          {fileImportMut.isPending && <p className="text-meta text-text-subtle" role="status">Uploading…</p>}
          {fileImportMut.isError && <AdminErrorState message={errorMessage(fileImportMut.error, "The file could not be imported.")} />}
        </AdminFormSection>
        <ModalFooter onCancel={() => { setShowFile(false); if (fileRef.current) fileRef.current.value = ""; }} onConfirm={() => {
          const f = fileRef.current?.files?.[0];
          if (f) fileImportMut.mutate(f);
        }} disabled={fileImportMut.isPending || !fileRef.current?.files?.[0]} confirmLabel={fileImportMut.isPending ? "Uploading…" : "Import file"} />
      </Modal>
    ) : null}

    {editingPlugin ? (
      <PatchPluginModal
        error={patchMut.error as Error | null}
        isPending={patchMut.isPending}
        onClose={() => setEditingPlugin(null)}
        onSave={(data) => patchMut.mutate({ id: editingPlugin.id, data })}
        plugin={editingPlugin}
      />
    ) : null}
    {renderConfirm()}
  </div>;
}

function PatchPluginModal({ plugin, onClose, onSave, isPending, error }: {
  plugin: ApiPlugin;
  onClose: () => void;
  onSave: (data: Record<string, unknown>) => void;
  isPending: boolean;
  error: Error | null;
}) {
  const [name, setName] = useState(plugin.name);
  const [description, setDescription] = useState(plugin.description ?? "");
  const [version, setVersion] = useState(plugin.version ?? "");
  const [kind, setKind] = useState(plugin.kind ?? "");

  return (
    <Modal title="Edit plugin metadata" description={`Stored metadata for ${plugin.name}. Runtime behaviour is unaffected.`} onClose={onClose}>
      <div className="space-y-4">
        <Input label="Name" onChange={setName} placeholder="Plugin name" required value={name} />
        <Input label="Description" onChange={setDescription} placeholder="Description" value={description} />
        <div className="grid grid-cols-2 gap-3">
          <Input label="Version" onChange={setVersion} placeholder="1.0.0" value={version} />
          <Input label="Kind" onChange={setKind} placeholder="integration" value={kind} />
        </div>
        <p className="text-meta text-text-subtle">Only changed fields are sent. The plugin id is not editable.</p>
        {error ? <AdminErrorState message={errorMessage(error, "The metadata could not be saved.")} /> : null}
      </div>
      <ModalFooter
        confirmLabel={isPending ? "Saving…" : "Save"}
        disabled={isPending || !name.trim()}
        onCancel={onClose}
        onConfirm={() => onSave({
          ...(name !== plugin.name ? { name } : {}),
          ...(description !== (plugin.description ?? "") ? { description } : {}),
          ...(version !== (plugin.version ?? "") ? { version } : {}),
          ...(kind !== (plugin.kind ?? "") ? { kind } : {}),
        })}
      />
    </Modal>
  );
}
