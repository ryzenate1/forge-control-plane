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
    <div className="ui-card flex items-center justify-between gap-4">
      <div className="min-w-0">
        <h3 className="font-bold text-white">{pt.processType}</h3>
        {pt.command ? <code className="mt-1 block font-mono text-xs text-slate-400">{pt.command}</code> : null}
      </div>
      <div className="flex items-center gap-3">
        <button className="ui-icon-button" disabled={isPending || pt.quantity <= 0} onClick={() => onScale(pt.quantity - 1)} type="button"><Minus size={14} /></button>
        <span className="min-w-[2ch] text-center font-mono text-lg font-bold text-white">{pt.quantity}</span>
        <button className="ui-icon-button" disabled={isPending} onClick={() => onScale(pt.quantity + 1)} type="button"><Plus size={14} /></button>
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
    <div className="ui-card space-y-4">
      <h3 className="flex items-center gap-2 font-bold text-white"><Upload size={16} /> Procfile</h3>
      <textarea
        className="ui-input min-h-32 resize-y font-mono"
        placeholder="web: gunicorn app:app&#10;worker: celery worker&#10;clock: celery beat&#10;release: ./migrate.sh"
        value={content}
        onChange={(e) => { setContent(e.target.value); parseMut.reset(); }}
      />
      <div className="flex gap-2">
        <button className="ui-button ui-button-secondary" disabled={!content.trim() || parseMut.isPending} onClick={() => parseMut.mutate(content)} type="button">Parse</button>
        <button className="ui-button ui-button-primary" disabled={!canSave || saveMut.isPending} onClick={() => saveMut.mutate(entries!)} type="button">Apply</button>
      </div>
      {parseMut.isError ? <p className="ui-alert ui-alert-error" role="alert">{message(parseMut.error)}</p> : null}
      {saveMut.isError ? <p className="ui-alert ui-alert-error" role="alert">{message(saveMut.error)}</p> : null}
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
    <div className="ui-card space-y-3">
      <h3 className="flex items-center gap-2 font-bold text-white"><Play size={16} /> Run one-off task</h3>
      <div className="flex gap-2">
        <input
          className="ui-input min-w-0 flex-1 font-mono"
          placeholder="python manage.py migrate"
          value={cmd}
          onChange={(e) => setCmd(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter" && cmd.trim()) runMut.mutate(cmd.trim()); }}
        />
        <button className="ui-button ui-button-primary" disabled={!cmd.trim() || runMut.isPending} onClick={() => runMut.mutate(cmd.trim())} type="button"><Terminal size={14} /> Run</button>
      </div>
      {runMut.isError ? <p className="ui-alert ui-alert-error" role="alert">{message(runMut.error)}</p> : null}
      {runMut.data ? <p className="ui-alert ui-alert-success" role="status">Task <span className="font-mono">{runMut.data.id.slice(0, 8)}</span> created ({runMut.data.status})</p> : null}
    </div>
  );
}

function TaskHistory({ tasks }: { tasks: OneOffTask[] }) {
  if (!tasks.length) return <p className="text-sm text-slate-500">No one-off tasks yet.</p>;
  return (
    <ul className="space-y-2">
      {tasks.map((t) => (
        <li key={t.id} className="rounded-lg border border-white/[0.06] bg-white/[0.02] p-3">
          <div className="flex items-center justify-between gap-3">
            <code className="truncate font-mono text-xs text-slate-200">{t.command}</code>
            <span className={`ui-status-pill ${
              t.status === "completed" ? "ui-status-pill-success" :
              t.status === "failed" ? "ui-status-pill-danger" :
              "ui-status-pill-warning"
            }`}>{t.status}</span>
          </div>
          {t.output ? <pre className="mt-2 max-h-20 overflow-auto rounded bg-surface-input p-2 font-mono text-[10px] text-slate-400">{t.output}</pre> : null}
          <p className="mt-1 font-mono text-[10px] text-slate-500">{new Date(t.createdAt).toLocaleString()}</p>
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
        <li key={e.id} className="flex items-center gap-3 rounded-lg border border-white/[0.06] bg-white/[0.02] p-3 text-sm">
          <History size={14} className="shrink-0 text-slate-400" />
          <span className="font-bold text-white">{e.processType}</span>
          <span className="font-mono text-slate-400">{e.oldQuantity} → {e.newQuantity}</span>
          <span className="ml-auto font-mono text-[10px] text-slate-500">{new Date(e.createdAt).toLocaleString()}</span>
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
          <button className={`ui-button ${showTasks ? "ui-button-danger" : "ui-button-secondary"}`} onClick={() => setShowTasks(!showTasks)} type="button"><Terminal size={13} /> Tasks</button>
          <button className={`ui-button ${showHistory ? "ui-button-danger" : "ui-button-secondary"}`} onClick={() => setShowHistory(!showHistory)} type="button"><History size={13} /> History</button>
          <button className={`ui-button ${showProcfile ? "ui-button-danger" : "ui-button-secondary"}`} onClick={() => setShowProcfile(!showProcfile)} type="button"><Upload size={13} /> Procfile</button>
        </div>
      </div>

      {processesQ.isLoading ? <p className="text-sm text-slate-400">Loading processes…</p> : null}
      {processesQ.isError ? <div className="ui-alert ui-alert-error" role="alert"><p className="text-sm">{message(processesQ.error)}</p></div> : null}

      {processes.length === 0 && !processesQ.isLoading ? (
        <div className="ui-empty">
          <div className="ui-empty-icon"><Terminal size={18} /></div>
          <h3 className="mt-3 text-sm font-semibold text-slate-200">No process types configured</h3>
          <p className="mt-1 max-w-md text-sm leading-6 text-slate-400">Paste a Procfile to get started, or check the egg configuration of this server if processes are missing.</p>
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
