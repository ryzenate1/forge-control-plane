"use client";

import { useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plug, Plus, Trash2, Zap, Upload, Store, Compass, Settings2, Wrench, Search } from "lucide-react";
import { type ApiPlugin } from "@/lib/api";
import {
  deletePlugin,
  fetchDiscoveredPlugins,
  fetchMarketplacePlugins,
  fetchPluginHooks,
  fetchPlugins,
  importPluginFile,
  importPluginFromURL,
  installPlugin,
  togglePluginLifecycle,
  updatePlugin,
  updatePluginSettings,
  type PluginDiscoverItem,
  type PluginMarketplaceItem,
} from "@/lib/api/plugins";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader, AdminFormSection, AdminTabs } from "./admin-ui";
import { useToast } from "@/components/ui/toast";

type PluginTab = "installed" | "marketplace" | "discover";

type MarketplaceItem = PluginMarketplaceItem;
type DiscoverItem = PluginDiscoverItem;

function useMarketplaceQuery(enabled: boolean) {
  return useQuery({
    queryKey: ["plugins-marketplace"],
    queryFn: fetchMarketplacePlugins,
    enabled,
  });
}

function useDiscoverQuery(enabled: boolean) {
  return useQuery({
    queryKey: ["plugins-discover"],
    queryFn: fetchDiscoveredPlugins,
    enabled,
  });
}

function usePluginHooks(pluginId: string | null) {
  return useQuery({
    queryKey: ["plugin-hooks", pluginId],
    queryFn: () => fetchPluginHooks(pluginId!),
    enabled: !!pluginId,
  });
}

export function AdminPlugins() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [tab, setTab] = useState<PluginTab>("installed");
  const [search, setSearch] = useState("");

  const query = useQuery({
    queryKey: ["plugins"],
    queryFn: fetchPlugins,
  });
  const marketplaceQuery = useMarketplaceQuery(tab === "marketplace");
  const discoverQuery = useDiscoverQuery(tab === "discover");

  const [open, setOpen] = useState(false);
  const [url, setUrl] = useState("");
  const [showFile, setShowFile] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const [editingPlugin, setEditingPlugin] = useState<ApiPlugin | null>(null);
  const [hooksPluginId, setHooksPluginId] = useState<string | null>(null);
  const hooksQuery = usePluginHooks(hooksPluginId);

  const importMut = useMutation({
    mutationFn: () => importPluginFromURL(url.trim()),
    onSuccess: () => {
      setOpen(false);
      setUrl("");
      void qc.invalidateQueries({ queryKey: ["plugins"] });
      toast({ tone: "success", title: "Manifest imported" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Import failed", message: e.message }),
  });

  const fileImportMut = useMutation({
    mutationFn: (file: File) => importPluginFile(file),
    onSuccess: () => {
      setShowFile(false);
      if (fileRef.current) fileRef.current.value = "";
      void qc.invalidateQueries({ queryKey: ["plugins"] });
      toast({ tone: "success", title: "File imported" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "File import failed", message: e.message }),
  });

  const installMut = useMutation({
    mutationFn: (item: MarketplaceItem | DiscoverItem) => installPlugin({
      name: item.name,
      source: (item as MarketplaceItem).source ?? `marketplace:${item.id ?? item.name}`,
      manifest: (item as MarketplaceItem).manifest ?? JSON.stringify(item),
    }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["plugins"] });
      toast({ tone: "success", title: "Plugin installed" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Install failed", message: e.message }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deletePlugin(id),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["plugins"] }); toast({ tone: "success", title: "Plugin deleted" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });
  const lifecycleMut = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => togglePluginLifecycle(id, enabled),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["plugins"] }); toast({ tone: "success", title: "State updated" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Lifecycle failed", message: e.message }),
  });

  const patchMut = useMutation({
    mutationFn: ({ id, data }: { id: string; data: Record<string, unknown> }) => updatePlugin(id, data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["plugins"] });
      setEditingPlugin(null);
      toast({ tone: "success", title: "Plugin updated (PATCH)" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Update failed", message: e.message }),
  });

  const updateSettingsMut = useMutation({
    mutationFn: ({ id, settings }: { id: string; settings: Record<string, unknown> }) =>
      updatePluginSettings(id, settings),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ["plugins"] }); toast({ tone: "success", title: "Settings updated" }); },
    onError: (e: Error) => toast({ tone: "error", title: "Settings update failed", message: e.message }),
  });

  const plugins = useMemo(() => Array.isArray(query.data) ? query.data : [], [query.data]);
  const marketplace = useMemo(() => marketplaceQuery.data ?? [], [marketplaceQuery.data]);
  const discover = useMemo(() => discoverQuery.data ?? [], [discoverQuery.data]);

  const filteredInstalled = useMemo(() => {
    if (!search) return plugins;
    const q = search.toLowerCase();
    return plugins.filter((p) => `${p.name} ${p.id} ${p.kind ?? ""} ${p.version ?? ""}`.toLowerCase().includes(q));
  }, [plugins, search]);

  const filteredMarketplace = useMemo(() => {
    if (!search) return marketplace;
    const q = search.toLowerCase();
    return marketplace.filter((p) => `${p.name} ${p.description ?? ""}`.toLowerCase().includes(q));
  }, [marketplace, search]);

  const filteredDiscover = useMemo(() => {
    if (!search) return discover;
    const q = search.toLowerCase();
    return discover.filter((p) => `${p.name} ${p.id}`.toLowerCase().includes(q));
  }, [discover, search]);

  return <div className="space-y-6">
    <SectionHeader
      title="Platform — Plugins"
      sub="PLATFORM · Integrations: plugin manifest registry with install/update/enable/disable, marketplace, discover and hooks. Distinct from Deploy (Compose) and Infrastructure."
      action={
        <div className="flex gap-2">
          <Btn tone="ghost" onClick={() => setShowFile(true)} className="border border-[var(--brand)]/30 hover:bg-[var(--brand)]/10"><Upload size={14}/> Import File</Btn>
          <Btn onClick={() => setOpen(true)} className="bg-[var(--brand)] hover:bg-[var(--brand)]/90 text-white"><Plus size={14}/> Import Manifest URL</Btn>
        </div>
      }
    />
    <div className="rounded-xl border border-white/[0.06] bg-white/[0.015] px-4 py-2 text-xs leading-5 text-slate-400">
      <span className="font-semibold text-slate-300">PLATFORM</span> · <span className="font-semibold text-slate-200">Integrations</span> — <code className="font-mono text-[11px]">Plugins</code> (this page) · <code className="font-mono">Webhooks</code> · <code className="font-mono">API Keys</code> + <code className="font-mono">Settings</code> for panel. Plugin hooks fire on workload lifecycle — see <code className="font-mono">GET /admin/plugins/:id/hooks</code>.
    </div>
    <div className="rounded-lg border border-[var(--brand)]/30 bg-[var(--brand)]/10 p-3 text-sm text-slate-200">
      <div className="flex items-start gap-2">
        <Zap className="h-4 w-4 mt-0.5 flex-shrink-0 text-[var(--brand)]" />
        <div>
          <p className="font-semibold">Plugin lifecycle active — <span className="font-mono text-xs">var(--brand)</span> themed</p>
          <p className="text-xs mt-1 text-slate-400">Manifests remain permission-scoped. Enable only reviewed plugins. Marketplace / Discover use <code className="font-mono">GET /admin/plugins/marketplace</code> & <code className="font-mono">/discover</code>; hooks via <code className="font-mono">/:id/hooks</code>; updates via <code className="font-mono">PATCH /:id</code>.</p>
        </div>
      </div>
    </div>

    <div className="flex items-center gap-3">
      <div className="relative w-64">
        <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-slate-400" />
        <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search plugins…" className="h-9 w-full rounded-lg border border-white/10 bg-[var(--surface)] pl-9 pr-3 text-sm text-slate-100 placeholder:text-slate-500 outline-none focus:border-[var(--brand)]/50" />
      </div>
      <span className="text-xs text-slate-500">{tab === "installed" ? `${filteredInstalled.length} installed` : tab === "marketplace" ? `${filteredMarketplace.length} marketplace` : `${filteredDiscover.length} discovered`}</span>
    </div>

    <AdminTabs tabs={[{ id: "installed", label: "Installed" }, { id: "marketplace", label: "Marketplace" }, { id: "discover", label: "Discover" }]} active={tab} onChange={(v) => setTab(v as PluginTab)} />

    {tab === "installed" && (
      <Card>
        <CardHeader title={`${plugins.length} installed manifests`} icon={Plug}/>
        {query.isError ? <div className="p-4"><div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200"><span>Could not load plugin manifests: {query.error.message}</span><Btn size="sm" tone="ghost" onClick={() => void query.refetch()}>Retry</Btn></div></div> :
         plugins.length === 0 ? <EmptyState icon={Plug} message="No plugin manifests registered. Use Import File or Manifest URL, or install from Marketplace/Discover."/> :
         <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.06] text-left text-xs uppercase text-slate-500">
                <th className="px-4 py-3">Plugin</th>
                <th className="px-4 py-3">Kind</th>
                <th className="px-4 py-3">Version</th>
                <th className="px-4 py-3">Runtime</th>
                <th className="px-4 py-3 text-right">Actions</th>
              </tr>
            </thead>
             <tbody className="divide-y divide-white/[0.04]">
               {filteredInstalled.map((plugin) => (
                <tr key={plugin.id} className="hover:bg-white/[0.02]">
                  <td className="px-4 py-3">
                    <p className="font-semibold text-slate-100">{plugin.name}</p>
                    <p className="text-xs text-slate-500">{plugin.description}</p>
                  </td>
                  <td className="px-4 py-3 text-slate-300">{plugin.kind}</td>
                  <td className="px-4 py-3 font-mono text-xs text-slate-400">{plugin.version}</td>
                  <td className="px-4 py-3"><Pill tone={plugin.enabled ? "green" : "yellow"}>{plugin.enabled ? "Enabled" : "Disabled"}</Pill></td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex justify-end gap-1.5 flex-wrap">
                      <Btn size="sm" tone="ghost" onClick={() => setHooksPluginId(plugin.id)} title="GET /:id/hooks"><Compass size={12}/> Hooks</Btn>
                      <Btn size="sm" tone="ghost" onClick={() => setEditingPlugin(plugin)} className="border border-[var(--brand)]/20"><Settings2 size={12}/> Edit (PATCH)</Btn>
                      <Btn size="sm" tone="ghost" onClick={() => lifecycleMut.mutate({ id: plugin.id, enabled: plugin.enabled })}>
                        {plugin.enabled ? "Disable" : "Enable"}
                      </Btn>
                      <Btn size="sm" tone="danger" onClick={() => { void (async () => { if (await confirm({ title: `Delete metadata for ${plugin.name}?`, description: "The plugin record will be removed from the panel. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(plugin.id); })(); }}>
                        <Trash2 size={12}/>
                      </Btn>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
       }
      </Card>
    )}

    {tab === "marketplace" && (
      <Card>
        <CardHeader title="Marketplace — GET /admin/plugins/marketplace" icon={Store} action={<Btn size="sm" tone="ghost" onClick={() => void marketplaceQuery.refetch()}>Refresh</Btn>} />
        {marketplaceQuery.isLoading ? <div className="p-8 text-center text-sm text-slate-400">Loading marketplace via GET /admin/plugins/marketplace…</div>
         : marketplaceQuery.isError ? <div className="p-4"><div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">Failed: {(marketplaceQuery.error as Error).message} <Btn size="sm" tone="ghost" onClick={() => void marketplaceQuery.refetch()} className="ml-2">Retry</Btn></div></div>
         : filteredMarketplace.length === 0 ? <EmptyState icon={Store} message="No marketplace plugins. Backend returns { marketplace: [] }." />
         : (
          <div className="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-3">
            {filteredMarketplace.map((item) => (
              <div key={item.id ?? item.name} className="rounded-xl border border-white/[0.06] bg-[var(--surface)] p-4 hover:border-[var(--brand)]/30 transition">
                <div className="flex items-start justify-between gap-2">
                  <h4 className="text-sm font-semibold text-slate-100 line-clamp-1">{item.name}</h4>
                  <Pill tone="blue">{item.kind ?? "integration"}</Pill>
                </div>
                <p className="mt-1 line-clamp-2 text-xs text-slate-400">{item.description ?? "No description"}</p>
                <p className="mt-2 font-mono text-[11px] text-slate-500">v{item.version ?? "0.0.0"} {item.author ? `· ${item.author}` : ""}</p>
                <div className="mt-3 flex gap-2">
                  <Btn size="sm" tone="primary" disabled={installMut.isPending} onClick={() => installMut.mutate(item)} className="bg-[var(--brand)] hover:bg-[var(--brand)]/90 text-white"><Plus size={12}/> Install (POST /install)</Btn>
                  <Btn size="sm" tone="ghost" onClick={() => setHooksPluginId(item.id)}>Hooks</Btn>
                </div>
              </div>
            ))}
          </div>
         )}
      </Card>
    )}

    {tab === "discover" && (
      <Card>
        <CardHeader title="Discover — GET /admin/plugins/discover" icon={Compass} action={<Btn size="sm" tone="ghost" onClick={() => void discoverQuery.refetch()}>Refresh</Btn>} />
        {discoverQuery.isLoading ? <div className="p-8 text-center text-sm text-slate-400">Discovering via GET /admin/plugins/discover…</div>
         : discoverQuery.isError ? <div className="p-4"><div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">Failed: {(discoverQuery.error as Error).message} <Btn size="sm" tone="ghost" onClick={() => void discoverQuery.refetch()} className="ml-2">Retry</Btn></div></div>
         : filteredDiscover.length === 0 ? <EmptyState icon={Compass} message="No discovered plugins. Requires local plugin search path configured on server." />
         : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead><tr className="border-b border-white/[0.06] text-left text-xs uppercase text-slate-500"><th className="px-4 py-3">Plugin</th><th className="px-4 py-3">Source</th><th className="px-4 py-3"></th></tr></thead>
              <tbody className="divide-y divide-white/[0.04]">
                {filteredDiscover.map((item) => (
                  <tr key={item.id ?? item.name} className="hover:bg-white/[0.02]">
                    <td className="px-4 py-3"><p className="font-medium text-slate-200">{item.name}</p><p className="text-xs text-slate-500">{item.id}</p></td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-400">{(item as DiscoverItem).path ?? (item as MarketplaceItem).source ?? "—"}</td>
                    <td className="px-4 py-3 text-right flex justify-end gap-2">
                      <Btn size="sm" tone="primary" disabled={installMut.isPending} onClick={() => installMut.mutate(item)} className="bg-[var(--brand)] hover:bg-[var(--brand)]/90 text-white"><Wrench size={12}/> Install</Btn>
                      <Btn size="sm" tone="ghost" onClick={() => setHooksPluginId(item.id)}>Hooks</Btn>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
         )}
      </Card>
    )}

    {hooksPluginId && (
      <Card>
        <CardHeader title={`Hooks — GET /admin/plugins/${hooksPluginId}/hooks`} icon={Wrench} action={<Btn size="sm" tone="ghost" onClick={() => setHooksPluginId(null)}>Close</Btn>} />
        {hooksQuery.isLoading ? <div className="p-4 text-sm text-slate-400">Loading hooks…</div>
         : hooksQuery.isError ? <div className="p-4 text-sm text-red-300">Failed: {(hooksQuery.error as Error).message}</div>
         : !hooksQuery.data || hooksQuery.data.length === 0 ? <div className="p-4 text-sm text-slate-400">No hooks. PUT /:id/settings can configure plugin settings.</div>
         : (
          <div className="p-4 space-y-2">
            {hooksQuery.data.map((h, idx) => (
              <pre key={idx} className="rounded-lg bg-black/30 p-3 text-xs text-slate-300 overflow-auto">{JSON.stringify(h, null, 2)}</pre>
            ))}
            <div className="flex gap-2 pt-2">
              <Btn size="sm" tone="ghost" onClick={() => updateSettingsMut.mutate({ id: hooksPluginId, settings: { note: "example" } })}>PUT /:id/settings (example)</Btn>
            </div>
            {updateSettingsMut.isError && <p className="text-xs text-red-300">{(updateSettingsMut.error as Error).message}</p>}
            {updateSettingsMut.isSuccess && <p className="text-xs text-emerald-300">Settings updated.</p>}
          </div>
         )}
      </Card>
    )}

    {/* Import URL Modal */}
    {open ? (
      <Modal title="Import Plugin Manifest" description="POST /admin/plugins/import/url" onClose={() => setOpen(false)}>
        <AdminFormSection title="Manifest URL">
        <Input label="HTTPS manifest URL" value={url} onChange={setUrl} placeholder="https://example.com/plugin.json"/>
        <p className="text-xs text-slate-400">The backend fetches this URL and stores JSON manifest metadata. Review network and trust implications before importing.</p>
        </AdminFormSection>
        {importMut.error ? <p className="mt-3 text-sm text-red-300">{importMut.error.message}</p> : null}
        <ModalFooter
          onCancel={() => setOpen(false)}
          onConfirm={() => importMut.mutate()}
          disabled={!/^https:\/\//i.test(url.trim()) || importMut.isPending}
          confirmLabel={importMut.isPending ? "Importing…" : "Import Metadata"}
        />
      </Modal>
    ) : null}

    {/* Import File Modal */}
    {showFile ? (
      <Modal title="Import Plugin Manifest" description="POST /admin/plugins/import/file" onClose={() => { setShowFile(false); if (fileRef.current) fileRef.current.value = ""; }}>
        <AdminFormSection title="Manifest File">
          <input ref={fileRef} type="file" accept=".json,application/json" className="block w-full text-sm text-slate-300 file:mr-3 file:rounded-lg file:border file:border-[var(--brand)]/30 file:bg-[var(--brand)]/10 file:px-3 file:py-2 file:text-sm file:text-slate-100 hover:file:bg-[var(--brand)]/20" onChange={(e) => {
            const f = e.target.files?.[0];
            if (f) fileImportMut.mutate(f);
          }} />
          <p className="text-xs text-slate-400">Upload a JSON manifest file. The field name is <code className="font-mono">file</code> (multipart/form-data) — wired to <code className="font-mono">POST /admin/plugins/import/file</code>.</p>
          {fileImportMut.isPending && <p className="text-xs text-slate-400">Uploading…</p>}
          {fileImportMut.error && <p className="text-sm text-red-300">{(fileImportMut.error as Error).message}</p>}
          {fileImportMut.isSuccess && <p className="text-sm text-emerald-300">Imported — check Installed tab.</p>}
        </AdminFormSection>
        <ModalFooter onCancel={() => { setShowFile(false); if (fileRef.current) fileRef.current.value = ""; }} onConfirm={() => {
          const f = fileRef.current?.files?.[0];
          if (f) fileImportMut.mutate(f);
        }} disabled={fileImportMut.isPending || !fileRef.current?.files?.[0]} confirmLabel={fileImportMut.isPending ? "Uploading…" : "Import File"} />
      </Modal>
    ) : null}

    {/* Edit PATCH Modal */}
    {editingPlugin ? (
      <PatchPluginModal plugin={editingPlugin} onClose={() => setEditingPlugin(null)} onSave={(data) => patchMut.mutate({ id: editingPlugin.id, data })} isPending={patchMut.isPending} error={patchMut.error as Error | null} />
    ) : null}
    {renderConfirm()}


  </div>;
}

function PatchPluginModal({ plugin, onClose, onSave, isPending, error }: { plugin: ApiPlugin; onClose: () => void; onSave: (data: Record<string, unknown>) => void; isPending: boolean; error: Error | null }) {
  const [name, setName] = useState(plugin.name);
  const [description, setDescription] = useState(plugin.description ?? "");
  const [version, setVersion] = useState(plugin.version ?? "");
  const [kind, setKind] = useState(plugin.kind ?? "");

  return (
    <Modal title="Edit Plugin" description={`PATCH /admin/plugins/${plugin.id}`} onClose={onClose}>
      <div className="space-y-4">
        <Input label="Name" value={name} onChange={setName} placeholder="Plugin name" />
        <Input label="Description" value={description} onChange={setDescription} placeholder="Description" />
        <div className="grid grid-cols-2 gap-3">
          <Input label="Version" value={version} onChange={setVersion} placeholder="1.0.0" />
          <Input label="Kind" value={kind} onChange={setKind} placeholder="integration" />
        </div>
        <p className="text-xs text-slate-400">Wires <code className="font-mono">PATCH /admin/plugins/:id</code> with {"{name, description, kind, version}"} — backend handler UpdatePlugin.</p>
        {error && <p className="text-sm text-red-300">{error.message}</p>}
      </div>
      <ModalFooter onCancel={onClose} onConfirm={() => onSave({
        ...(name !== plugin.name ? { name } : {}),
        ...(description !== (plugin.description ?? "") ? { description } : {}),
        ...(version !== (plugin.version ?? "") ? { version } : {}),
        ...(kind !== (plugin.kind ?? "") ? { kind } : {}),
      })} disabled={isPending || !name.trim()} confirmLabel={isPending ? "Saving…" : "Save"} />
    </Modal>
  );
}
