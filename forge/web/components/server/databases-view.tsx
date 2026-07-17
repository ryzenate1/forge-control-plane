"use client";

import { useEffect, useState } from "react";
import { AlertTriangle, Check, Clipboard, Database, Eye, EyeOff, KeyRound, RefreshCw, Trash2 } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ApiDatabase, type ApiServer, createServerDatabase, deleteServerDatabase, fetchServerDatabases, rotateServerDatabasePassword } from "@/lib/api";
import { hasServerPermission, useOptionalServerContext } from "./server-context";
import { errorMessage as message } from "@/lib/utils";

const field = "mt-1 h-10 w-full rounded-lg border border-white/10 bg-[#111722] px-3 text-sm text-white outline-none focus:border-red-500";
type State = "pending" | "ready" | "failed" | "unknown";

function databaseState(database: ApiDatabase): State {
  if (database.provisioningState === "pending" || database.provisioningState === "ready" || database.provisioningState === "failed") return database.provisioningState;
  return "unknown";
}

function StateBadge({ state }: { state: State }) {
  const styles = state === "ready" ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-200" : state === "failed" ? "border-red-500/30 bg-red-500/10 text-red-200" : state === "pending" ? "border-amber-500/30 bg-amber-500/10 text-amber-200" : "border-slate-500/30 bg-slate-500/10 text-slate-200";
  return <span className={`inline-flex rounded-full border px-2 py-0.5 text-[11px] font-semibold capitalize ${styles}`}>{state}</span>;
}

export function DatabasesView({ server }: { server?: ApiServer }) {
  const context = useOptionalServerContext();
  const access = context?.access ?? { user: null, permissions: [], isAdmin: true, isOwner: true };
  const canCreate = hasServerPermission(access, "database.create");
  const canUpdate = hasServerPermission(access, "database.update");
  const canDelete = hasServerPermission(access, "database.delete");

  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [remote, setRemote] = useState("%");
  const [maxConnections, setMaxConnections] = useState("");
  const [secrets, setSecrets] = useState<Record<string, string>>({});
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});
  const [copied, setCopied] = useState("");
  const [mutationMessage, setMutationMessage] = useState<{ tone: "error" | "success"; text: string } | null>(null);
  const [forceDeleteId, setForceDeleteId] = useState<string | null>(null);
  const query = useQuery({ queryKey: ["server-databases", server?.id], queryFn: () => fetchServerDatabases(server?.id ?? ""), enabled: Boolean(server?.id), refetchInterval: (current) => current.state.data?.some((database) => databaseState(database) === "pending") ? 5_000 : false });
  const refresh = () => void qc.invalidateQueries({ queryKey: ["server-databases", server?.id] });
  const captureSecret = (database: { id: string; password?: string }) => { if (database.password) { setSecrets((current) => ({ ...current, [database.id]: database.password! })); setRevealed((current) => ({ ...current, [database.id]: true })); } };
  const create = useMutation({
    mutationFn: () => createServerDatabase(server?.id ?? "", { database: name.trim(), remote: remote.trim() || "%", maxConnections: maxConnections ? Number(maxConnections) : undefined }),
    onMutate: () => setMutationMessage(null),
    onSuccess: (database) => { captureSecret(database); setName(""); setMaxConnections(""); },
    onError: (error) => setMutationMessage({ tone: "error", text: `Database creation did not complete: ${message(error, "Database action failed.")} The API can retain a failed record after this error; the list has been refreshed so you can review it.` }),
    onSettled: refresh,
  });
  const rotate = useMutation({
    mutationFn: (id: string) => rotateServerDatabasePassword(server?.id ?? "", id).then((result) => ({ ...result, id })),
    onMutate: () => setMutationMessage(null),
    onSuccess: (database) => captureSecret(database),
    onError: (error) => setMutationMessage({ tone: "error", text: `Password rotation failed: ${message(error, "Database action failed.")} The database status has been refreshed.` }),
    onSettled: refresh,
  });
  const remove = useMutation({
    mutationFn: ({ id, force }: { id: string; force: boolean }) => deleteServerDatabase(server?.id ?? "", id, force),
    onMutate: () => setMutationMessage(null),
    onSuccess: (_, variables) => {
      setSecrets((current) => { const next = { ...current }; delete next[variables.id]; return next; });
      setForceDeleteId(null);
      if (variables.force) setMutationMessage({ tone: "success", text: "The panel database record was force-deleted. If remote cleanup failed, an orphan-remediation task was recorded." });
    },
    onError: (error, variables) => {
      if (!variables.force) setForceDeleteId(variables.id);
      setMutationMessage({ tone: "error", text: variables.force ? `Force deletion failed: ${message(error, "Database action failed.")}` : `Database deletion did not complete: ${message(error, "Database action failed.")} The panel record may have been retained; its status has been refreshed. You can force-delete it if remote cleanup is unavailable.` });
    },
    onSettled: refresh,
  });
  const rows = query.data ?? [];
  const limit = server?.databaseLimit ?? 0;
  const limitReached = limit > 0 && rows.length >= limit;
  const copy = async (key: string, value: string) => { try { await navigator.clipboard.writeText(value); setCopied(key); window.setTimeout(() => setCopied(""), 1500); } catch { /* Clipboard access can be blocked. */ } };
  useEffect(() => () => setSecrets({}), []);

  const deleteDatabase = (row: ApiDatabase, force = false) => {
    const databaseName = row.database || row.name || row.id;
    const prompt = force
      ? `Force-delete ${databaseName}. The panel record will be removed even if the remote database cannot be cleaned up, and an orphan-remediation task will be created. Type ${databaseName} to continue.`
      : `Type ${databaseName} to permanently delete this database.`;
    if (window.prompt(prompt) === databaseName) remove.mutate({ id: row.id, force });
  };

  return <div className="space-y-5">
    <section className="rounded-xl border border-white/[0.07] bg-[#151b27] p-5"><div className="flex flex-wrap items-center justify-between gap-2"><div><h2 className="flex items-center gap-2 font-bold text-white"><Database size={18} />Create database</h2><p className="mt-1 text-sm text-slate-400">Credentials are shown only after creation or rotation.</p></div><span className="text-xs text-slate-400">{limit > 0 ? `${rows.length} / ${limit} used` : `${rows.length} created`}</span></div><div className="mt-4 grid gap-3 md:grid-cols-[1fr_180px_180px_auto]"><label className="text-xs font-semibold text-slate-300">Database name<input className={field} onChange={(event) => setName(event.target.value)} placeholder="game_data" value={name} /></label><label className="text-xs font-semibold text-slate-300">Connections from<input className={field} onChange={(event) => setRemote(event.target.value)} value={remote} /></label><label className="text-xs font-semibold text-slate-300">Max connections<input className={field} min="1" onChange={(event) => setMaxConnections(event.target.value)} placeholder="Backend default" type="number" value={maxConnections} /></label><button className="self-end rounded-lg bg-red-600 px-4 py-2.5 text-sm font-bold text-white disabled:opacity-40" disabled={!canCreate || create.isPending || !name.trim() || limitReached || (maxConnections !== "" && Number(maxConnections) < 1)} onClick={() => create.mutate()} type="button">{create.isPending ? "Creating…" : limitReached ? "Limit reached" : "Create"}</button></div></section>
    {query.isError ? <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200" role="alert">{message(query.error, "Unable to load databases.")}</div> : null}
    {mutationMessage ? <div className={`rounded-lg border p-4 text-sm ${mutationMessage.tone === "error" ? "border-red-500/30 bg-red-500/10 text-red-200" : "border-emerald-500/30 bg-emerald-500/10 text-emerald-200"}`} role={mutationMessage.tone === "error" ? "alert" : "status"}>{mutationMessage.text}</div> : null}
    <div className="space-y-3">{query.isLoading ? <div className="rounded-xl bg-[#151b27] p-5 text-sm text-slate-400">Loading databases…</div> : null}{!query.isLoading && !query.isError && rows.length === 0 ? <div className="rounded-xl bg-[#151b27] p-8 text-center text-sm text-slate-400">No databases have been created.</div> : null}{rows.map((row) => {
      const state = databaseState(row); const secret = secrets[row.id]; const visible = revealed[row.id]; const databaseName = row.database || row.name || row.id; const isReady = state === "ready"; const endpoint = isReady && row.host && typeof row.port === "number" ? `${row.host}:${row.port}` : null; const hasUsername = isReady && Boolean(row.username); const isPending = state === "pending"; const canForceDelete = canDelete && (state === "failed" || forceDeleteId === row.id);
      return <article className="rounded-xl border border-white/[0.07] bg-[#151b27] p-5" key={row.id}><div className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between"><div className="min-w-0"><div className="flex items-center gap-2"><h3 className="truncate font-mono font-bold text-white">{databaseName}</h3><StateBadge state={state} /></div><p className="mt-1 text-xs uppercase text-slate-500">{row.engine}{row.maxConnections ? ` · ${row.maxConnections} max connections` : ""}</p></div><dl className="grid flex-1 gap-3 text-sm sm:grid-cols-3 xl:max-w-3xl"><div><dt className="text-xs text-slate-500">Endpoint</dt><dd className="mt-1 flex items-center gap-2 font-mono text-slate-200">{endpoint ?? "Available when ready"}{endpoint ? <button aria-label={`Copy endpoint for ${databaseName}`} onClick={() => void copy(`endpoint-${row.id}`, endpoint)} type="button">{copied === `endpoint-${row.id}` ? <Check size={14} /> : <Clipboard size={14} />}</button> : null}</dd></div><div><dt className="text-xs text-slate-500">Username</dt><dd className="mt-1 flex items-center gap-2 font-mono text-slate-200">{hasUsername ? row.username : "Available when ready"}{hasUsername ? <button aria-label={`Copy username for ${databaseName}`} onClick={() => void copy(`user-${row.id}`, row.username)} type="button">{copied === `user-${row.id}` ? <Check size={14} /> : <Clipboard size={14} />}</button> : null}</dd></div><div><dt className="text-xs text-slate-500">Connections from</dt><dd className="mt-1 font-mono text-slate-200">{row.remote || "—"}</dd></div></dl><div className="flex flex-wrap gap-2"><button className="inline-flex items-center gap-2 rounded-lg border border-white/10 px-3 py-2 text-xs font-bold disabled:opacity-40" disabled={!canUpdate || !isReady || rotate.isPending} onClick={() => { if (window.confirm(`Rotate the password for ${databaseName}? Existing clients will stop connecting.`)) rotate.mutate(row.id); }} type="button"><KeyRound size={14} />{rotate.isPending && rotate.variables === row.id ? "Rotating…" : "Rotate password"}</button><button aria-label={`Delete ${databaseName}`} className="rounded-lg border border-red-500/40 p-2 text-red-300 disabled:opacity-40" disabled={!canDelete || remove.isPending} onClick={() => deleteDatabase(row)} type="button"><Trash2 size={15} /></button>{canForceDelete ? <button className="inline-flex items-center gap-1 rounded-lg border border-amber-500/40 px-3 py-2 text-xs font-bold text-amber-200 disabled:opacity-40" disabled={remove.isPending} onClick={() => deleteDatabase(row, true)} type="button"><AlertTriangle size={14} />Force delete</button> : null}</div></div>
      {isPending ? <div className="mt-4 flex items-center justify-between gap-3 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-100"><span>Provisioning is still in progress. Password rotation is unavailable until the database is ready.</span><button className="inline-flex shrink-0 items-center gap-1 font-semibold hover:text-white" onClick={refresh} type="button"><RefreshCw size={13} />Refresh status</button></div> : null}
      {state === "unknown" ? <div className="mt-4 rounded-lg border border-slate-500/30 bg-slate-500/10 p-3 text-xs text-slate-200">The API did not report a recognized provisioning state. Password rotation and credentials remain unavailable until the database state can be verified.</div> : null}
      {state === "failed" ? <div className="mt-4 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-xs text-red-100"><p className="font-semibold">Provisioning failed</p><p className="mt-1 break-words text-red-200">{row.provisioningError || "The API did not provide an error detail."}</p><p className="mt-2 text-red-200/80">Retry is not available because this API does not expose a provisioning retry endpoint. Delete the record after resolving the host issue; administrators can force-delete it if remote cleanup cannot complete.</p></div> : null}
      {secret && isReady ? <div className="mt-4 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3"><div className="flex flex-wrap items-center justify-between gap-2"><div><p className="text-xs font-bold uppercase text-amber-200">New password — save it now</p><p className="mt-1 font-mono text-sm text-amber-50">{visible ? secret : "••••••••••••••••"}</p></div><div className="flex gap-2"><button aria-label={`${visible ? "Hide" : "Reveal"} password for ${row.database}`} className="rounded p-2 hover:bg-white/5" onClick={() => setRevealed((current) => ({ ...current, [row.id]: !visible }))} type="button">{visible ? <EyeOff size={16} /> : <Eye size={16} />}</button><button className="inline-flex items-center gap-2 rounded border border-amber-400/30 px-3 py-2 text-xs font-bold" onClick={() => void copy(`password-${row.id}`, secret)} type="button"><Clipboard size={14} />{copied === `password-${row.id}` ? "Copied" : "Copy"}</button><button className="rounded border border-amber-400/30 px-3 py-2 text-xs font-bold" onClick={() => { setSecrets((current) => { const next = { ...current }; delete next[row.id]; return next; }); }} type="button">Dismiss permanently</button></div></div></div> : isReady ? <p className="mt-3 text-xs text-slate-500">The password is not retrievable. Rotate it to generate and reveal a new one once.</p> : null}</article>;
    })}</div>
  </div>;
}
