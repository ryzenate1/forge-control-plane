"use client";

import { useEffect, useState } from "react";
import { Check, Clipboard, KeyRound, Save, Server, Settings, Wrench } from "lucide-react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ApiNode, type ApiServer, reinstallServer, updateServer } from "@/modules/gameservers/api";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { errorMessage as message } from "@/lib/utils";

const field = "mt-2 w-full rounded-lg border border-white/10 bg-[#111722] px-3 text-sm text-white outline-none focus:border-red-500 disabled:opacity-60";

export function ServerSettingsView({ server, node: nodeProp }: { server?: ApiServer; node?: ApiNode }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: [], isAdmin: true, isOwner: true };
  const canRename = hasServerPermission(access, "settings.rename");
  const canReinstall = hasServerPermission(access, "settings.reinstall");
  const canSftp = hasServerPermission(access, "file.sftp");
  const qc = useQueryClient();
  const node = nodeProp;
  const [name, setName] = useState(server?.name ?? "");
  const [description, setDescription] = useState(server?.description ?? "");
  const [copied, setCopied] = useState(false);
  useEffect(() => { setName(server?.name ?? ""); setDescription(server?.description ?? ""); }, [server?.id, server?.name, server?.description]);
  const save = useMutation({ mutationFn: () => updateServer(server?.id ?? "", { name: name.trim(), description }), onSuccess: async () => { await qc.invalidateQueries({ queryKey: ["server", server?.id] }); await qc.invalidateQueries({ queryKey: ["servers"] }); await context?.refreshServer(); } });
  const reinstall = useMutation({ mutationFn: () => reinstallServer(server?.id ?? ""), onSuccess: async () => { await qc.invalidateQueries({ queryKey: ["servers"] }); await context?.refreshServer(); } });
  const host = server?.sftpHost?.replace(/^https?:\/\//, "").replace(/\/$/, "") || node?.fqdn || node?.baseUrl?.replace(/^https?:\/\//, "").replace(/\/$/, "") || node?.name || "Unavailable";
  const port = server?.sftpPort ?? node?.daemonSftp;
  const username = access.user && server ? `${access.user.email}.${server.id}` : server ? `${server.owner}.${server.id}` : "Unavailable";
  const command = host !== "Unavailable" && port ? `sftp -P ${port} ${username}@${host}` : "SFTP endpoint unavailable";
  const changed = name.trim() !== (server?.name ?? "") || description !== (server?.description ?? "");
  const actionError = save.error ?? reinstall.error;

  return <div className="grid gap-5 lg:grid-cols-2"><section className="rounded-xl border border-white/[0.07] bg-[#151b27] p-5"><h2 className="flex items-center gap-2 font-bold text-white"><Settings size={18} />Server details</h2><div className="mt-4 space-y-4"><label className="block text-xs font-semibold text-slate-300">Server name<input className={`${field} h-10`} disabled={!canRename || save.isPending} maxLength={191} onChange={(event) => setName(event.target.value)} value={name} /></label><label className="block text-xs font-semibold text-slate-300">Description<textarea className={`${field} min-h-28 py-3`} disabled={!canRename || save.isPending} maxLength={1000} onChange={(event) => setDescription(event.target.value)} value={description} /></label><p className="text-xs text-slate-500">Only the name and description are submitted. Resource limits, owner, allocation, image, and startup fields are never included in this update.</p>{save.error ? <p className="text-sm text-red-300" role="alert">{message(save.error, "Details could not be saved.")}</p> : null}<div className="flex justify-end"><button className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-bold text-white disabled:opacity-40" disabled={!canRename || !server || !name.trim() || !changed || save.isPending} onClick={() => save.mutate()} type="button"><Save size={15} />{save.isPending ? "Saving…" : save.isSuccess && !changed ? "Saved" : "Save details"}</button></div></div></section>
    <section className="rounded-xl border border-white/[0.07] bg-[#151b27] p-5"><h2 className="flex items-center gap-2 font-bold text-white"><KeyRound size={18} />SFTP details</h2>{!canSftp ? <p className="mt-4 text-sm text-amber-200">You do not have SFTP permission.</p> : <div className="mt-4 space-y-4"><dl className="grid gap-3 text-sm sm:grid-cols-2"><div><dt className="text-xs text-slate-500">Host</dt><dd className="mt-1 break-all font-mono">{host}</dd></div><div><dt className="text-xs text-slate-500">Port</dt><dd className="mt-1 font-mono">{port ?? "Unavailable"}</dd></div><div className="sm:col-span-2"><dt className="text-xs text-slate-500">Username</dt><dd className="mt-1 break-all font-mono">{username}</dd></div></dl><div className="break-all rounded-lg bg-[#080c13] p-3 font-mono text-xs text-slate-300">{command}</div><div className="flex items-center justify-between gap-3"><p className="text-xs text-slate-500">Use your panel password unless your administrator configured different SFTP authentication.</p><button className="inline-flex shrink-0 items-center gap-2 rounded-lg border border-white/10 px-3 py-2 text-xs font-bold disabled:opacity-40" disabled={command.startsWith("SFTP endpoint")} onClick={async () => { await navigator.clipboard.writeText(command); setCopied(true); window.setTimeout(() => setCopied(false), 1500); }} type="button">{copied ? <Check size={14} /> : <Clipboard size={14} />}{copied ? "Copied" : "Copy"}</button></div></div>}</section>
    <section className="rounded-xl border border-red-500/20 bg-[#151b27] p-5"><h2 className="flex items-center gap-2 font-bold text-white"><Wrench size={18} />Reinstall server</h2><p className="mt-3 text-sm text-slate-400">Re-runs the installation workflow. Installation scripts may overwrite server files; backups are not created automatically.</p><button className="mt-4 rounded-lg bg-red-700 px-4 py-2 text-sm font-bold text-white disabled:opacity-40" disabled={!canReinstall || !server || reinstall.isPending || server.status === "installing"} onClick={() => { if (window.confirm("Reinstall this server? Installation scripts may overwrite server data. This action cannot be undone.")) reinstall.mutate(); }} type="button">{reinstall.isPending ? "Requesting reinstall…" : "Reinstall server"}</button>{actionError && actionError === reinstall.error ? <p className="mt-3 text-sm text-red-300" role="alert">{message(actionError, "Reinstall failed.")}</p> : null}</section>
    <section className="rounded-xl border border-white/[0.07] bg-[#151b27] p-5"><h2 className="flex items-center gap-2 font-bold text-white"><Server size={18} />Server information</h2><dl className="mt-4 grid gap-4 text-sm sm:grid-cols-2"><div><dt className="text-xs text-slate-500">Identifier</dt><dd className="mt-1 break-all font-mono">{server?.id ?? "—"}</dd></div><div><dt className="text-xs text-slate-500">Node</dt><dd className="mt-1">{server?.node ?? "—"}</dd></div><div><dt className="text-xs text-slate-500">Template</dt><dd className="mt-1">{server?.template ?? "—"}</dd></div><div><dt className="text-xs text-slate-500">State</dt><dd className="mt-1 uppercase">{server?.suspended ? "Suspended" : server?.status ?? "Unknown"}</dd></div></dl></section>
  </div>;
}
