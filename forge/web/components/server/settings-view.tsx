"use client";
import { queryKeys } from "@/lib/api/query-keys";

import { useEffect, useState } from "react";
import { Check, Clipboard, Flag, KeyRound, Save, Server, Settings, Wrench } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { type ApiNode, type ApiServer, fetchServerFlags, reinstallServer, updateServer, updateServerFlags } from "@/lib/api";
import { copySecret } from "@/lib/clipboard";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { errorMessage as message } from "@/lib/utils";

const field = "mt-2 w-full rounded-lg border border-white/10 bg-[var(--surface)] px-3 text-sm text-white outline-none focus:border-red-500 disabled:opacity-60";

export function ServerSettingsView({ server, node: nodeProp }: { server?: ApiServer; node?: ApiNode }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: null, isAdmin: false, isOwner: false };
  const canRename = hasServerPermission(access, "settings.rename");
  const canReinstall = hasServerPermission(access, "settings.reinstall");
  const canSftp = hasServerPermission(access, "file.sftp");
  const qc = useQueryClient();
  const node = nodeProp;
  const [name, setName] = useState(server?.name ?? "");
  const [description, setDescription] = useState(server?.description ?? "");
  const [copied, setCopied] = useState(false);
  useEffect(() => { setName(server?.name ?? ""); setDescription(server?.description ?? ""); }, [server?.id, server?.name, server?.description]);
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const save = useMutation({ mutationFn: () => updateServer(server?.id ?? "", { name: name.trim(), description }), onSuccess: async () => { await qc.invalidateQueries({ queryKey: queryKeys.servers.detail(server?.id ?? "") }); await qc.invalidateQueries({ queryKey: ["servers"] }); await context?.refreshServer(); }, onError: (error) => toast({ tone: "error", title: "Save failed", message: error instanceof Error ? error.message : "Could not save server details" }) });
  const flagsQuery = useQuery({ queryKey: ["server-flags", server?.id], queryFn: () => fetchServerFlags(server?.id ?? ""), enabled: Boolean(server?.id) });
  const flagsMutation = useMutation({
    mutationFn: (flags: { autoStart?: boolean; autoRestart?: boolean }) => updateServerFlags(server?.id ?? "", flags),
    onSuccess: async () => { await qc.invalidateQueries({ queryKey: ["server-flags", server?.id] }); await context?.refreshServer(); },
    onError: (error) => toast({ tone: "error", title: "Flags not saved", message: error instanceof Error ? error.message : "Could not save lifecycle flags" }),
  });
  const flags = flagsQuery.data;
  const reinstall = useMutation({ mutationFn: () => reinstallServer(server?.id ?? ""), onSuccess: async () => { await qc.invalidateQueries({ queryKey: ["servers"] }); await context?.refreshServer(); }, onError: (error) => toast({ tone: "error", title: "Reinstall failed", message: error instanceof Error ? error.message : "Could not reinstall server" }) });
  const host = server?.sftpHost?.replace(/^https?:\/\//, "").replace(/\/$/, "") || node?.fqdn || node?.baseUrl?.replace(/^https?:\/\//, "").replace(/\/$/, "") || null;
  // Beacon's native default is 127.0.0.1:2022 (beacon/config/config.go
  // SystemSFTPBindAddressEntry/SystemSFTPBindPortEntry, daemon :2022). A missing
  // report is not that default — it is shown as not reported with the default
  // named, never as a measured endpoint and never as node.name (not dialable).
  const port = server?.sftpPort ?? node?.daemonSftp ?? null;
  const hostLabel = host ?? "Not reported";
  const portLabel = port ?? "Not reported (beacon default 2022)";
  const username = access.user && server ? `${access.user.email}.${server.id}` : server ? `${server.owner}.${server.id}` : "Unavailable";
  const command = host && port ? `sftp -P ${port} ${username}@${host}` : "SFTP endpoint unavailable";
  const changed = name.trim() !== (server?.name ?? "") || description !== (server?.description ?? "");
  const actionError = save.error ?? reinstall.error;

  return <div className="grid gap-5 lg:grid-cols-2"><section className="rounded-xl border border-white/[0.07] bg-[var(--surface-raised)] p-5"><h2 className="flex items-center gap-2 font-bold text-white"><Settings size={18} />Server details</h2><div className="mt-4 space-y-4"><label className="block text-xs font-semibold text-slate-300">Server name<input className={`${field} h-10`} disabled={!canRename || save.isPending} maxLength={191} onChange={(event) => setName(event.target.value)} value={name} /></label><label className="block text-xs font-semibold text-slate-300">Description<textarea className={`${field} min-h-28 py-3`} disabled={!canRename || save.isPending} maxLength={1000} onChange={(event) => setDescription(event.target.value)} value={description} /></label><p className="text-xs text-slate-500">Only the name and description are submitted. Resource limits, owner, allocation, image, and startup fields are never included in this update.</p>{save.error ? <p className="text-sm text-red-300" role="alert">{message(save.error, "Details could not be saved.")}</p> : null}<div className="flex justify-end"><button className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-bold text-white disabled:opacity-40" disabled={!canRename || !server || !name.trim() || !changed || save.isPending} onClick={() => save.mutate()} type="button"><Save size={15} />{save.isPending ? "Saving…" : save.isSuccess && !changed ? "Saved" : "Save details"}</button></div></div></section>
    <section className="rounded-xl border border-white/[0.07] bg-[var(--surface-raised)] p-5"><h2 className="flex items-center gap-2 font-bold text-white"><KeyRound size={18} />SFTP details</h2>{!canSftp ? <p className="mt-4 text-sm text-amber-200">You do not have SFTP permission.</p> : <div className="mt-4 space-y-4"><dl className="grid gap-3 text-sm sm:grid-cols-2"><div><dt className="text-xs text-slate-500">Host</dt><dd className="mt-1 break-all font-mono">{hostLabel}</dd></div><div><dt className="text-xs text-slate-500">Port</dt><dd className="mt-1 font-mono">{portLabel}</dd></div><div className="sm:col-span-2"><dt className="text-xs text-slate-500">Username</dt><dd className="mt-1 break-all font-mono">{username}</dd></div></dl><div className="break-all rounded-lg bg-[var(--canvas)] p-3 font-mono text-xs text-slate-300">{command}</div><p className="text-xs text-slate-500">Beacon listens on 127.0.0.1:2022 by default (system.sftp.bind_address/bind_port); a “Not reported” field means the panel has no measured endpoint, not that default.</p><div className="flex items-center justify-between gap-3"><p className="text-xs text-slate-500">Use your panel password unless your administrator configured different SFTP authentication.</p><button className="inline-flex shrink-0 items-center gap-2 rounded-lg border border-white/10 px-3 py-2 text-xs font-bold disabled:opacity-40" disabled={command.startsWith("SFTP endpoint")} onClick={async () => { if (await copySecret(command)) { setCopied(true); window.setTimeout(() => setCopied(false), 1500); } }} type="button">{copied ? <Check size={14} /> : <Clipboard size={14} />}{copied ? "Copied" : "Copy"}</button></div></div>}</section>
    <section className="rounded-xl border border-red-500/20 bg-[var(--surface-raised)] p-5"><h2 className="flex items-center gap-2 font-bold text-white"><Wrench size={18} />Reinstall server</h2><p className="mt-3 text-sm text-slate-400">Re-runs the installation workflow. Installation scripts may overwrite server files; backups are not created automatically.</p><button className="mt-4 rounded-lg bg-red-700 px-4 py-2 text-sm font-bold text-white disabled:opacity-40" disabled={!canReinstall || !server || reinstall.isPending || server.status === "installing"} onClick={async () => { if (await confirm({ title: "Reinstall server?", description: "Installation scripts may overwrite server data. This action cannot be undone.", danger: true, confirmLabel: "Reinstall" })) reinstall.mutate(); }} type="button">{reinstall.isPending ? "Requesting reinstall…" : "Reinstall server"}</button>{actionError && actionError === reinstall.error ? <p className="mt-3 text-sm text-red-300" role="alert">{message(actionError, "Reinstall failed.")}</p> : null}</section>
    <section className="rounded-xl border border-white/[0.07] bg-[var(--surface-raised)] p-5"><h2 className="flex items-center gap-2 font-bold text-white"><Server size={18} />Server information</h2><dl className="mt-4 grid gap-4 text-sm sm:grid-cols-2"><div><dt className="text-xs text-slate-500">Identifier</dt><dd className="mt-1 break-all font-mono">{server?.id ?? "—"}</dd></div><div><dt className="text-xs text-slate-500">Node</dt><dd className="mt-1">{server?.node ?? "—"}</dd></div><div><dt className="text-xs text-slate-500">Template</dt><dd className="mt-1">{server?.template ?? "—"}</dd></div><div><dt className="text-xs text-slate-500">State</dt><dd className="mt-1 uppercase">{server?.suspended ? "Suspended" : server?.status ?? "Unknown"}</dd></div></dl></section>
    <section className="rounded-xl border border-white/[0.07] bg-[var(--surface-raised)] p-5"><h2 className="flex items-center gap-2 font-bold text-white"><Flag size={18} />Lifecycle flags</h2>
      {flagsQuery.isLoading ? <p className="mt-4 text-sm text-slate-500">Loading flags…</p> : null}
      {flagsQuery.isError ? <p className="mt-4 text-sm text-red-300" role="alert">Flags could not be loaded.</p> : null}
      {flags ? <div className="mt-4 space-y-3">
        <label className="flex cursor-pointer items-center justify-between gap-3 rounded-lg border border-white/[0.06] bg-white/[0.02] px-3 py-2 text-sm">
          <span><span className="block font-semibold text-slate-200">Auto-start</span><span className="block text-xs text-slate-500">Start the server automatically when the node boots.</span></span>
          <input checked={flags.autoStart} className="h-5 w-5 accent-red-600" disabled={!canRename || flagsMutation.isPending} onChange={(event) => flagsMutation.mutate({ autoStart: event.target.checked })} type="checkbox" />
        </label>
        <label className="flex cursor-pointer items-center justify-between gap-3 rounded-lg border border-white/[0.06] bg-white/[0.02] px-3 py-2 text-sm">
          <span><span className="block font-semibold text-slate-200">Auto-restart</span><span className="block text-xs text-slate-500">Restart the server automatically after an unexpected stop.</span></span>
          <input checked={flags.autoRestart} className="h-5 w-5 accent-red-600" disabled={!canRename || flagsMutation.isPending} onChange={(event) => flagsMutation.mutate({ autoRestart: event.target.checked })} type="checkbox" />
        </label>
        {!canRename ? <p className="text-xs text-amber-200">You need the settings.rename permission to change flags.</p> : null}
      </div> : null}
    </section>
    {renderConfirm()}
  </div>;
}
