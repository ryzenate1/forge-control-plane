"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Minus, Plus, Play, RotateCcw, Terminal, History, Upload } from "lucide-react";
import { ServerConsoleLayout } from "@/components/server/server-console-layout";
import { fetchProcesses, scaleProcess, runOneOffTask, fetchOneOffTasks, fetchScalingHistory, parseProcfile, setProcesses } from "@/lib/api/servers";
import type { ProcessType, OneOffTask, ProcessScalingEvent, ProcfileEntry } from "@/lib/api/types";

function message(e: unknown) {
  if (e instanceof Error) return e.message;
  return String(e);
}

function ProcessCard({ pt, onScale, isPending }: { pt: ProcessType; onScale: (qty: number) => void; isPending: boolean }) {
  return (
    <div className="flex items-center justify-between rounded-xl border border-white/[0.07] bg-[#151b27] p-5">
      <div>
        <h3 className="font-bold text-white">{pt.processType}</h3>
        {pt.command ? <code className="mt-1 block text-xs text-slate-400">{pt.command}</code> : null}
      </div>
      <div className="flex items-center gap-3">
        <button className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-white/10 bg-[#111722] text-slate-300 hover:border-red-500 disabled:opacity-40" disabled={isPending || pt.quantity <= 0} onClick={() => onScale(pt.quantity - 1)} type="button"><Minus size={14} /></button>
        <span className="min-w-[2ch] text-center font-mono text-lg font-bold text-white">{pt.quantity}</span>
        <button className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-white/10 bg-[#111722] text-slate-300 hover:border-red-500 disabled:opacity-40" disabled={isPending} onClick={() => onScale(pt.quantity + 1)} type="button"><Plus size={14} /></button>
      </div>
    </div>
  );
}

function ProcfileInput({ serverId, onDone }: { serverId: string; onDone: () => void }) {
  const [content, setContent] = useState("");
  const qc = useQueryClient();
  const parseMut = useMutation({
    mutationFn: (c: string) => parseProcfile(serverId, c),
    onError: () => {},
  });
  const saveMut = useMutation({
    mutationFn: (entries: ProcfileEntry[]) => setProcesses(serverId, entries),
    onSuccess: () => { setContent(""); void qc.invalidateQueries({ queryKey: ["processes", serverId] }); onDone(); },
  });

  const entries = parseMut.data;
  const canSave = entries && entries.length > 0;

  return (
    <div className="space-y-4 rounded-xl border border-white/[0.07] bg-[#151b27] p-5">
      <h3 className="flex items-center gap-2 font-bold text-white"><Upload size={16} /> Procfile</h3>
      <textarea
        className="min-h-32 w-full rounded-lg border border-white/10 bg-[#080c13] p-4 font-mono text-sm text-slate-200 outline-none focus:border-red-500"
        placeholder="web: gunicorn app:app&#10;worker: celery worker&#10;clock: celery beat&#10;release: ./migrate.sh"
        value={content}
        onChange={(e) => { setContent(e.target.value); parseMut.reset(); }}
      />
      <div className="flex gap-2">
        <button className="inline-flex items-center gap-2 rounded-lg bg-slate-700 px-4 py-2 text-xs font-bold text-white hover:bg-slate-600 disabled:opacity-40" disabled={!content.trim() || parseMut.isPending} onClick={() => parseMut.mutate(content)} type="button">Parse</button>
        <button className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-xs font-bold text-white hover:bg-red-500 disabled:opacity-40" disabled={!canSave || saveMut.isPending} onClick={() => saveMut.mutate(entries!)} type="button">Apply</button>
      </div>
      {parseMut.isError ? <p className="text-xs text-red-400">{message(parseMut.error)}</p> : null}
      {saveMut.isError ? <p className="text-xs text-red-400">{message(saveMut.error)}</p> : null}
      {entries && entries.length > 0 ? (
        <ul className="space-y-1 text-xs text-slate-300">
          {entries.map((e) => <li key={e.processType}><span className="font-bold text-white">{e.processType}</span>: {e.command}</li>)}
        </ul>
      ) : null}
    </div>
  );
}

function OneOffRunner({ serverId }: { serverId: string }) {
  const [cmd, setCmd] = useState("");
  const qc = useQueryClient();
  const runMut = useMutation({
    mutationFn: (c: string) => runOneOffTask(serverId, c),
    onSuccess: () => { setCmd(""); void qc.invalidateQueries({ queryKey: ["one-off-tasks", serverId] }); },
  });

  return (
    <div className="space-y-3 rounded-xl border border-white/[0.07] bg-[#151b27] p-5">
      <h3 className="flex items-center gap-2 font-bold text-white"><Play size={16} /> Run one-off task</h3>
      <div className="flex gap-2">
        <input
          className="h-10 flex-1 rounded-lg border border-white/10 bg-[#111722] px-3 font-mono text-sm text-white outline-none focus:border-red-500"
          placeholder="python manage.py migrate"
          value={cmd}
          onChange={(e) => setCmd(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter" && cmd.trim()) runMut.mutate(cmd.trim()); }}
        />
        <button className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-xs font-bold text-white hover:bg-red-500 disabled:opacity-40" disabled={!cmd.trim() || runMut.isPending} onClick={() => runMut.mutate(cmd.trim())} type="button"><Terminal size={14} /> Run</button>
      </div>
      {runMut.isError ? <p className="text-xs text-red-400">{message(runMut.error)}</p> : null}
      {runMut.data ? <p className="text-xs text-emerald-400">Task {runMut.data.id.slice(0, 8)} created ({runMut.data.status})</p> : null}
    </div>
  );
}

function TaskHistory({ tasks }: { tasks: OneOffTask[] }) {
  if (!tasks.length) return <p className="text-sm text-slate-500">No one-off tasks yet.</p>;
  return (
    <ul className="space-y-2">
      {tasks.map((t) => (
        <li key={t.id} className="rounded-lg border border-white/[0.06] bg-[#111722] p-3">
          <div className="flex items-center justify-between">
            <code className="truncate font-mono text-xs text-slate-200">{t.command}</code>
            <span className={`rounded-full px-2 py-0.5 text-[10px] font-bold uppercase ${
              t.status === "completed" ? "bg-emerald-500/20 text-emerald-300" :
              t.status === "failed" ? "bg-red-500/20 text-red-300" :
              "bg-amber-500/20 text-amber-300"
            }`}>{t.status}</span>
          </div>
          {t.output ? <pre className="mt-2 max-h-20 overflow-auto rounded bg-[#080c13] p-2 font-mono text-[10px] text-slate-400">{t.output}</pre> : null}
          <p className="mt-1 text-[10px] text-slate-500">{new Date(t.createdAt).toLocaleString()}</p>
        </li>
      ))}
    </ul>
  );
}

function ScalingTimeline({ events }: { events: ProcessScalingEvent[] }) {
  if (!events.length) return <p className="text-sm text-slate-500">No scaling events yet.</p>;
  return (
    <ul className="space-y-2">
      {events.map((e) => (
        <li key={e.id} className="flex items-center gap-3 rounded-lg border border-white/[0.06] bg-[#111722] p-3 text-sm">
          <History size={14} className="shrink-0 text-slate-400" />
          <span className="font-bold text-white">{e.processType}</span>
          <span className="text-slate-400">{e.oldQuantity} → {e.newQuantity}</span>
          <span className="ml-auto text-[10px] text-slate-500">{new Date(e.createdAt).toLocaleString()}</span>
        </li>
      ))}
    </ul>
  );
}

export default function ProcessesPage() {
  const [showProcfile, setShowProcfile] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const [showTasks, setShowTasks] = useState(false);

  return (
    <ServerConsoleLayout activeTab="processes">
      {(server) => <ProcessesInner serverId={server.id} showProcfile={showProcfile} setShowProcfile={setShowProcfile} showHistory={showHistory} setShowHistory={setShowHistory} showTasks={showTasks} setShowTasks={setShowTasks} />}
    </ServerConsoleLayout>
  );
}

function ProcessesInner({
  serverId, showProcfile, setShowProcfile, showHistory, setShowHistory, showTasks, setShowTasks,
}: {
  serverId: string;
  showProcfile: boolean;
  setShowProcfile: (v: boolean) => void;
  showHistory: boolean;
  setShowHistory: (v: boolean) => void;
  showTasks: boolean;
  setShowTasks: (v: boolean) => void;
}) {
  const qc = useQueryClient();
  const processesQ = useQuery({ queryKey: ["processes", serverId], queryFn: () => fetchProcesses(serverId) });
  const tasksQ = useQuery({ queryKey: ["one-off-tasks", serverId], queryFn: () => fetchOneOffTasks(serverId), enabled: showTasks });
  const historyQ = useQuery({ queryKey: ["scaling-history", serverId], queryFn: () => fetchScalingHistory(serverId), enabled: showHistory });
  const scaleMut = useMutation({
    mutationFn: ({ pt, qty }: { pt: string; qty: number }) => scaleProcess(serverId, pt, qty),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["processes", serverId] }),
  });

  const processes = processesQ.data ?? [];

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-bold text-white">Processes</h1>
        <div className="flex gap-2">
          <button className={`inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-bold ${showTasks ? "border-red-500 bg-red-500/20 text-red-200" : "border-white/10 bg-[#151b27] text-slate-300 hover:border-red-500"}`} onClick={() => setShowTasks(!showTasks)} type="button"><Terminal size={13} /> Tasks</button>
          <button className={`inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-bold ${showHistory ? "border-red-500 bg-red-500/20 text-red-200" : "border-white/10 bg-[#151b27] text-slate-300 hover:border-red-500"}`} onClick={() => setShowHistory(!showHistory)} type="button"><History size={13} /> History</button>
          <button className={`inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-xs font-bold ${showProcfile ? "border-red-500 bg-red-500/20 text-red-200" : "border-white/10 bg-[#151b27] text-slate-300 hover:border-red-500"}`} onClick={() => setShowProcfile(!showProcfile)} type="button"><Upload size={13} /> Procfile</button>
        </div>
      </div>

      {processesQ.isLoading ? <p className="text-sm text-slate-400">Loading processes…</p> : null}
      {processesQ.isError ? <p className="text-sm text-red-400">{message(processesQ.error)}</p> : null}

      {processes.length === 0 && !processesQ.isLoading ? (
        <div className="rounded-xl border border-dashed border-white/10 p-8 text-center text-sm text-slate-500">
          No process types configured. Paste a Procfile to get started.
        </div>
      ) : (
        <div className="space-y-3">
          {processes.map((pt) => (
            <ProcessCard key={pt.id} pt={pt} onScale={(qty) => scaleMut.mutate({ pt: pt.processType, qty })} isPending={scaleMut.isPending} />
          ))}
        </div>
      )}

      {showProcfile ? <ProcfileInput serverId={serverId} onDone={() => setShowProcfile(false)} /> : null}

      {showTasks ? (
        <section>
          <OneOffRunner serverId={serverId} />
          <div className="mt-4">
            <h3 className="mb-3 flex items-center gap-2 font-bold text-white"><RotateCcw size={15} /> Task history</h3>
            {tasksQ.isLoading ? <p className="text-sm text-slate-500">Loading…</p> : tasksQ.isError ? <p className="text-sm text-red-400">{message(tasksQ.error)}</p> : <TaskHistory tasks={tasksQ.data ?? []} />}
          </div>
        </section>
      ) : null}

      {showHistory ? (
        <section>
          <h3 className="mb-3 flex items-center gap-2 font-bold text-white"><History size={15} /> Scaling history</h3>
          {historyQ.isLoading ? <p className="text-sm text-slate-500">Loading…</p> : historyQ.isError ? <p className="text-sm text-red-400">{message(historyQ.error)}</p> : <ScalingTimeline events={historyQ.data ?? []} />}
        </section>
      ) : null}
    </div>
  );
}
