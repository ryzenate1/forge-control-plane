"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plug, Plus, Trash2, Zap } from "lucide-react";
import { deleteJSON, fetchJSON, postJSON, type ApiPlugin } from "@/modules/infrastructure/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "./admin-ui";

export function AdminPlugins() {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: ["plugins"],
    queryFn: () => fetchJSON<ApiPlugin[]>("/admin/plugins"),
  });
  const [open, setOpen] = useState(false);
  const [url, setUrl] = useState("");
  const importMut = useMutation({
    mutationFn: () => postJSON<ApiPlugin>("/admin/plugins/import/url", { url: url.trim() }),
    onSuccess: () => {
      setOpen(false);
      setUrl("");
      void qc.invalidateQueries({ queryKey: ["plugins"] });
    },
  });
  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteJSON(`/admin/plugins/${encodeURIComponent(id)}`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["plugins"] }),
  });
  const plugins = query.data ?? [];

  return <div>
    <SectionHeader
      title="Plugins"
      sub="Plugin manifest registry. Forge has no plugin runtime, so install, enable, disable, update, and uninstall return HTTP 501 and are intentionally unavailable."
      action={
        <Btn onClick={() => setOpen(true)}><Plus size={14}/> Import Manifest URL</Btn>
      }
    />
    <div className="mb-4 rounded-lg border border-amber-500/30 bg-amber-950/20 p-3 text-sm text-amber-200">
      <div className="flex items-start gap-2">
        <Zap className="h-4 w-4 mt-0.5 flex-shrink-0" />
        <div>
          <p className="font-semibold">Plugin Runtime Status</p>
          <p className="text-xs mt-1">Imported manifests are metadata only and do not execute code. A plugin runtime is required before install, enable, disable, update, or uninstall can succeed.</p>
        </div>
      </div>
    </div>
    <Card>
      <CardHeader title={`${plugins.length} installed manifests`} icon={Plug}/>
      {query.isError ? <p className="p-4 text-sm text-red-300">Plugin manifests could not be loaded.</p> :
       plugins.length === 0 ? <EmptyState icon={Plug} message="No plugin manifests registered."/> :
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
             {plugins.map((plugin) => (
               <tr key={plugin.id}>
                 <td className="px-4 py-3">
                   <p className="font-semibold">{plugin.name}</p>
                   <p className="text-xs text-slate-500">{plugin.description}</p>
                 </td>
                 <td className="px-4 py-3">{plugin.kind}</td>
                 <td className="px-4 py-3 font-mono text-xs">{plugin.version}</td>
                 <td className="px-4 py-3"><Pill tone="yellow">Unavailable (501)</Pill></td>
                 <td className="px-4 py-3 text-right">
                   <Btn size="sm" tone="danger" onClick={() => { if (confirm(`Delete metadata for ${plugin.name}?`)) deleteMut.mutate(plugin.id); }}>
                     <Trash2 size={12}/>
                   </Btn>
                 </td>
               </tr>
             ))}
           </tbody>
         </table>
       </div>
      }
    </Card>

    {/* Import URL Modal */}
    {open ? (
      <Modal title="Import Plugin Manifest" onClose={() => setOpen(false)}>
        <Input label="HTTPS manifest URL" value={url} onChange={setUrl} placeholder="https://example.com/plugin.json"/>
        <p className="mt-3 text-xs text-slate-400">The backend fetches this URL and stores JSON manifest metadata. Review network and trust implications before importing.</p>
        {importMut.error ? <p className="mt-3 text-sm text-red-300">{importMut.error.message}</p> : null}
        <ModalFooter
          onCancel={() => setOpen(false)}
          onConfirm={() => importMut.mutate()}
          disabled={!/^https:\/\//i.test(url.trim()) || importMut.isPending}
          confirmLabel="Import Metadata"
        />
      </Modal>
    ) : null}


  </div>;
}
