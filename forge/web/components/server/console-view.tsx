"use client";

import { FormEvent, KeyboardEvent, useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, ArrowDown, Clock, Cpu, Download, MemoryStick, Network, PlugZap, RefreshCw, Search, Send, Server, Trash2, Upload } from "lucide-react";
import { useMutation } from "@tanstack/react-query";
import { type ApiServer, type ApiStats, connectServerWebSocket, fetchServerLogs, reinstallServer, sendPowerSignal } from "@/modules/gameservers/api";
import { cn, formatBytes } from "@/lib/utils";
import { hasServerPermission, useServerContext } from "./server-context";
import { CrashBanner } from "./crash-banner";

const MAX_LINES = 500;
const MAX_POINTS = 60;

/* -------------------------------------------------------------------------- */
/*  Uptime display                                                            */
/* -------------------------------------------------------------------------- */

function formatUptime(ms: number | undefined | null): string {
  if (!ms || ms <= 0) return "—";
  const totalSeconds = Math.floor(ms / 1000);
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (days > 0) return `${days}d ${hours}h ${minutes}m`;
  if (hours > 0) return `${hours}h ${minutes}m ${seconds}s`;
  if (minutes > 0) return `${minutes}m ${seconds}s`;
  return `${seconds}s`;
}

/* -------------------------------------------------------------------------- */
/*  Install / Transfer state banners                                         */
/* -------------------------------------------------------------------------- */

function InstallBanner({ server }: { server: ApiServer }) {
  return (
    <div className="flex items-start gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 p-4">
      <Download className="mt-0.5 shrink-0 text-amber-300" size={19} />
      <div>
        <p className="text-sm font-semibold text-amber-100">This server is being installed</p>
        <p className="mt-1 text-xs text-amber-200/70">
          The installation process is running. The console will display output from the installation script.
          Do not restart or power off the server during this process.
        </p>
        {server.installationState && (
          <p className="mt-2 text-xs font-mono text-amber-300/60">
            State: {server.installationState}
          </p>
        )}
      </div>
    </div>
  );
}

function TransferBanner({ server }: { server: ApiServer }) {
  return (
    <div className="flex items-start gap-3 rounded-xl border border-sky-500/30 bg-sky-500/10 p-4">
      <Upload className="mt-0.5 shrink-0 text-sky-300 animate-pulse" size={19} />
      <div>
        <p className="text-sm font-semibold text-sky-100">This server is being transferred</p>
        <p className="mt-1 text-xs text-sky-200/70">
          The server is migrating to another node. During this process, the console may be unavailable
          and the server cannot be started, stopped, or modified.
        </p>
        {server.transferTargetNodeId && (
          <p className="mt-2 text-xs font-mono text-sky-300/60">
            Target node: {server.transferTargetNodeId}
            {server.transferState ? ` · ${server.transferState}` : ""}
          </p>
        )}
      </div>
    </div>
  );
}

function SuspendedBanner() {
  return (
    <div className="flex items-start gap-3 rounded-xl border border-rose-500/30 bg-rose-500/10 p-4">
      <AlertTriangle className="mt-0.5 shrink-0 text-rose-300" size={19} />
      <div>
        <p className="text-sm font-semibold text-rose-100">This server is suspended</p>
        <p className="mt-1 text-xs text-rose-200/70">
          All runtime actions are unavailable. Contact your server administrator to unsuspend this server.
        </p>
      </div>
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/*  Sparkline chart                                                            */
/* -------------------------------------------------------------------------- */

function Chart({ label, value, detail, values, icon: Icon }: { label: string; value: string; detail: string; values: number[]; icon: typeof Cpu }) {
  const max = Math.max(...values, 1);
  const points = values.map((point, index) => `${values.length < 2 ? 0 : (index / (values.length - 1)) * 100},${100 - (point / max) * 92}`).join(" ");
  return <section className="rounded-xl border border-white/[0.07] bg-[#151b27] p-4" aria-label={`${label} chart`}><div className="flex items-start justify-between gap-3"><div className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-slate-400"><Icon size={15} />{label}</div><div className="text-right"><p className="font-mono text-sm font-bold text-slate-100">{value}</p><p className="text-[10px] text-slate-500">{detail}</p></div></div><svg aria-hidden="true" className="mt-4 h-24 w-full" preserveAspectRatio="none" viewBox="0 0 100 100"><line stroke="#334155" strokeWidth=".5" x1="0" x2="100" y1="50" y2="50" /><polygon fill="rgba(220,38,38,.16)" points={`0,100 ${points} 100,100`} /><polyline fill="none" points={points} stroke="#ef4444" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.5" /></svg></section>;
}

/* -------------------------------------------------------------------------- */
/*  Console view                                                               */
/* -------------------------------------------------------------------------- */

export function ConsoleView({ server }: { server: ApiServer }) {
  const { access, refreshServer } = useServerContext();
  const [lines, setLines] = useState<string[]>([]);
  const [command, setCommand] = useState("");
  const [history, setHistory] = useState<string[]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);
  const [connection, setConnection] = useState<"connecting" | "connected" | "reconnecting" | "error">("connecting");
  const [connectionError, setConnectionError] = useState("");
  const [nonce, setNonce] = useState(0);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const [showTimestamps, setShowTimestamps] = useState(false);
  const reconnectAttempt = useRef(0);
  const messageCount = useRef(0);
  const connectedAt = useRef<number | null>(null);
  const HISTORY_KEY = `console-history-${server.id}`;
  const [stats, setStats] = useState<ApiStats | null>(null);
  const [cpuHistory, setCpuHistory] = useState<number[]>([]);
  const [memoryHistory, setMemoryHistory] = useState<number[]>([]);
  const [networkHistory, setNetworkHistory] = useState<number[]>([]);
  const outputRef = useRef<HTMLDivElement>(null);
  const socketRef = useRef<WebSocket | null>(null);
  const canConsole = hasServerPermission(access, ["websocket.connect", "control.console"]);
  const canPower = (signal: "start" | "stop" | "restart" | "kill") => hasServerPermission(access, signal === "start" ? "control.start" : signal === "restart" ? "control.restart" : "control.stop");
  const canReinstall = hasServerPermission(access, "settings.reinstall");

  const power = useMutation({ mutationFn: (signal: "start" | "stop" | "restart" | "kill") => sendPowerSignal(server.id, signal), onSuccess: () => void refreshServer() });
  const install = useMutation({ mutationFn: () => reinstallServer(server.id), onSuccess: () => void refreshServer() });

  useEffect(() => { if (autoScroll) requestAnimationFrame(() => outputRef.current?.scrollTo({ top: outputRef.current.scrollHeight })); }, [autoScroll, lines]);
  useEffect(() => {
    if (!canConsole) { setConnection("error"); setConnectionError("You do not have permission to access this server console."); return; }
    let closed = false;
    let socket: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | undefined;
    setConnection(nonce ? "reconnecting" : "connecting");
    setConnectionError("");
    void fetchServerLogs(server.id).then((logs) => setLines(logs.split("\n").filter(Boolean).slice(-MAX_LINES))).catch((error) => setConnectionError(error instanceof Error ? error.message : "Previous logs could not be loaded."));
    void connectServerWebSocket(server.id, "console").then((next) => {
      if (closed) { next.close(); return; }
      socket = next; socketRef.current = next;
      next.onopen = () => { setConnection("connected"); reconnectAttempt.current = 0; messageCount.current = 0; connectedAt.current = Date.now(); };
      next.onmessage = (event) => {
        messageCount.current += 1;
        let text = String(event.data);
        try { const payload = JSON.parse(text) as { data?: string; error?: string }; text = payload.data ?? payload.error ?? text; } catch { /* plain daemon output */ }
        if (text) setLines((current) => [...current, ...text.split("\n").filter(Boolean)].slice(-MAX_LINES));
      };
      next.onerror = () => { setConnection("error"); setConnectionError("The console connection failed."); };
      next.onclose = () => { if (!closed) { setConnection("reconnecting"); const delay = Math.min(1000 * Math.pow(2, reconnectAttempt.current), 30000); reconnectAttempt.current += 1; reconnectTimer = setTimeout(() => setNonce((value) => value + 1), delay); } };
    }).catch((error) => { if (!closed) { setConnection("error"); setConnectionError(error instanceof Error ? error.message : "Console authorization failed."); } });
    return () => { closed = true; if (reconnectTimer) clearTimeout(reconnectTimer); socket?.close(); socketRef.current = null; };
  }, [canConsole, nonce, server.id]);

  useEffect(() => {
    if (!canConsole) return;
    let closed = false; let socket: WebSocket | null = null;
    void connectServerWebSocket(server.id, "stats").then((next) => { if (closed) { next.close(); return; } socket = next; next.onmessage = (event) => { try { const data = JSON.parse(String(event.data)) as ApiStats & { error?: string }; if (data.error) return; setStats(data); const memory = data.memoryLimit > 0 ? (data.memoryBytes / data.memoryLimit) * 100 : 0; const network = data.networkRxBytes + data.networkTxBytes; setCpuHistory((items) => [...items.slice(-(MAX_POINTS - 1)), data.cpuPercent]); setMemoryHistory((items) => [...items.slice(-(MAX_POINTS - 1)), memory]); setNetworkHistory((items) => [...items.slice(-(MAX_POINTS - 1)), network]); } catch { /* ignore malformed telemetry without inventing values */ } }; next.onerror = () => { setConnectionError("Stats connection failed."); }; }).catch((error) => { if (!closed) setConnectionError(error instanceof Error ? error.message : "Stats connection failed."); });
    return () => { closed = true; socket?.close(); };
  }, [canConsole, server.id]);

  useEffect(() => {
    try {
      const saved = localStorage.getItem(HISTORY_KEY);
      if (saved) { const parsed = JSON.parse(saved); if (Array.isArray(parsed)) setHistory(parsed); }
    } catch { /* ignore */ }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const filteredLines = searchQuery ? lines.filter((l) => l.toLowerCase().includes(searchQuery.toLowerCase())) : lines;

  const memoryPercent = stats && stats.memoryLimit > 0 ? (stats.memoryBytes / stats.memoryLimit) * 100 : null;
  const stateLabel = connection === "connected" ? "Connected" : connection === "connecting" ? "Connecting" : connection === "reconnecting" ? "Reconnecting" : "Connection error";
  const submit = (event: FormEvent) => { event.preventDefault(); const value = command.trim(); if (!value || socketRef.current?.readyState !== WebSocket.OPEN) return; socketRef.current.send(value); setHistory((items) => { const next = [value, ...items.filter((item) => item !== value)].slice(0, 50); localStorage.setItem(HISTORY_KEY, JSON.stringify(next)); return next; }); setHistoryIndex(-1); setCommand(""); };
  const historyKey = (event: KeyboardEvent<HTMLInputElement>) => { if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return; event.preventDefault(); const next = event.key === "ArrowUp" ? Math.min(historyIndex + 1, history.length - 1) : Math.max(historyIndex - 1, -1); setHistoryIndex(next); setCommand(next < 0 ? "" : history[next] ?? ""); };
  const controls = useMemo(() => (["start", "restart", "stop", "kill"] as const), []);
  const blocked = server.suspended || server.transferring || server.status === "installing";

  return <div className="space-y-5">
    {/* State banners — install / transfer state */}
    {server.suspended ? <SuspendedBanner /> : null}
    {server.transferring ? <TransferBanner server={server} /> : null}
    {server.status === "installing" && !server.suspended && !server.transferring ? <InstallBanner server={server} /> : null}

    {!server.suspended && !server.transferring && server.status !== "installing" ? (
      <CrashBanner serverId={server.id} />
    ) : null}

    {(power.error || install.error) ? <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-4 text-sm text-red-200" role="alert">{[power.error, install.error].filter(Boolean).map((err) => err instanceof Error ? err.message : "The server action failed.").join("; ")}</div> : null}
    <div className="grid gap-4 lg:grid-cols-[1fr_auto] lg:items-center"><div><h2 className="text-xl font-bold text-white">Console</h2><p className="mt-1 text-sm text-slate-400">Live daemon output and telemetry for {server.name}.</p></div><div className="grid grid-cols-4 gap-2">{controls.map((signal) => <button className={cn("rounded-lg px-3 py-2 text-xs font-bold uppercase text-white disabled:cursor-not-allowed disabled:opacity-40", signal === "start" ? "bg-emerald-600" : signal === "stop" || signal === "kill" ? "bg-red-700" : "bg-slate-600")} disabled={!canPower(signal) || blocked || power.isPending || (signal === "start" ? server.status === "running" : server.status !== "running")} key={signal} onClick={() => { if (signal === "kill" && !window.confirm("Kill the server process immediately? Unsaved data may be lost.")) return; power.mutate(signal); }} type="button">{power.isPending && power.variables === signal ? "…" : signal}</button>)}</div></div>

    {/* Stats row with uptime */}
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Chart detail="Current process usage" icon={Cpu} label="CPU" value={stats ? `${stats.cpuPercent.toFixed(1)}%` : "Waiting for telemetry"} values={cpuHistory} />
      <Chart detail={stats ? `${formatBytes(stats.memoryBytes)} of ${formatBytes(stats.memoryLimit)}` : "No telemetry received"} icon={MemoryStick} label="Memory" value={memoryPercent === null ? "Waiting for telemetry" : `${memoryPercent.toFixed(1)}%`} values={memoryHistory} />
      <Chart detail={stats ? `RX ${formatBytes(stats.networkRxBytes)} · TX ${formatBytes(stats.networkTxBytes)}` : "No telemetry received"} icon={Network} label="Network transfer" value={stats ? formatBytes(stats.networkRxBytes + stats.networkTxBytes) : "Waiting for telemetry"} values={networkHistory} />
      {/* Uptime card */}
      <section className="rounded-xl border border-white/[0.07] bg-[#151b27] p-4" aria-label="Uptime">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-slate-400"><Clock size={15} />Uptime</div>
          <div className="text-right">
            <p className="font-mono text-sm font-bold text-slate-100">{formatUptime(stats?.uptimeMs)}</p>
            <p className="text-[10px] text-slate-500">{server.status === "running" ? "Server is online" : "Server is offline"}</p>
          </div>
        </div>
        <div className="mt-4 flex h-24 items-center justify-center">
          <div className={cn("h-16 w-16 rounded-full border-4 flex items-center justify-center", server.status === "running" ? "border-emerald-500/50" : "border-slate-600/50")}>
            <div className={cn("h-8 w-8 rounded-full", server.status === "running" ? "bg-emerald-500/30 animate-pulse" : "bg-slate-700")} />
          </div>
        </div>
      </section>
    </div>

    <section className="overflow-hidden rounded-xl border border-white/[0.08] bg-[#060a11] shadow-xl" aria-label="Server console">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-white/[0.07] bg-[#111722] px-4 py-3">
        <div className="flex items-center gap-2 text-sm font-semibold">
          <PlugZap size={16} className={connection === "connected" ? "text-emerald-400" : "text-amber-300"} />
          <span>{stateLabel}</span>
          {connection === "connected" && connectedAt.current ? (
            <span className="text-xs font-normal text-slate-400"> · {messageCount.current} msgs</span>
          ) : null}
          {connectionError ? <span className="font-normal text-red-300">· {connectionError}</span> : null}
        </div>
        <div className="flex gap-1">
          <button aria-label="Toggle search" className={cn("rounded p-2 hover:bg-white/5 hover:text-white", searchOpen ? "text-red-400" : "text-slate-400")} onClick={() => setSearchOpen((v) => !v)} type="button"><Search size={16} /></button>
          <button aria-label={autoScroll ? "Freeze scroll" : "Auto-scroll"} className={cn("rounded p-2 hover:bg-white/5 hover:text-white", autoScroll ? "text-emerald-400" : "text-slate-400")} onClick={() => setAutoScroll((v) => !v)} type="button"><ArrowDown size={16} /></button>
          <button aria-label={showTimestamps ? "Hide timestamps" : "Show timestamps"} className={cn("rounded p-2 hover:bg-white/5 hover:text-white", showTimestamps ? "text-emerald-400" : "text-slate-400")} onClick={() => setShowTimestamps((v) => !v)} type="button"><Clock size={16} /></button>
          <button aria-label="Reconnect console" className="rounded p-2 text-slate-400 hover:bg-white/5 hover:text-white" onClick={() => setNonce((value) => value + 1)} type="button"><RefreshCw size={16} /></button>
          <button aria-label="Clear console" className="rounded p-2 text-slate-400 hover:bg-white/5 hover:text-white" onClick={() => setLines([])} type="button"><Trash2 size={16} /></button>
        </div>
      </div>
      {searchOpen ? (
        <div className="border-b border-white/[0.07] bg-[#111722] px-4 py-2">
          <input
            autoComplete="off"
            className="w-full bg-transparent font-mono text-sm text-white outline-none placeholder:text-slate-600"
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Filter console output…"
            value={searchQuery}
          />
        </div>
      ) : null}
      <div aria-live="polite" className="h-[50vh] min-h-80 overflow-y-auto p-4 font-mono text-xs leading-5 text-slate-200 sm:text-[13px]" ref={outputRef} role="log" tabIndex={0}>
        {filteredLines.length ? filteredLines.map((line, index) => (
          <div className="whitespace-pre-wrap break-words" key={`${index}-${line}`}>
            {showTimestamps ? <span className="mr-2 text-slate-500">{new Date().toLocaleTimeString()}</span> : null}
            {line}
          </div>
        )) : (
          <p className="text-slate-500">{searchQuery ? "No matching console output." : connectionError || "Waiting for console output…"}</p>
        )}
      </div>
      <form className="flex items-center gap-2 border-t border-white/[0.07] bg-[#111722] p-3" onSubmit={submit}>
        <Server className="text-slate-500" size={16} />
        <label className="sr-only" htmlFor="console-command">Console command</label>
        <input autoComplete="off" className="min-w-0 flex-1 bg-transparent font-mono text-sm text-white outline-none placeholder:text-slate-600" disabled={connection !== "connected" || !canConsole} id="console-command" onChange={(event) => setCommand(event.target.value)} onKeyDown={historyKey} placeholder={connection === "connected" ? "Type a command; use ↑ and ↓ for history" : "Console is not connected"} value={command} />
        <button aria-label="Send command" className="rounded-lg bg-red-600 p-2 text-white disabled:opacity-40" disabled={connection !== "connected" || !command.trim()} type="submit"><Send size={16} /></button>
      </form>
    </section>
    <div className="flex justify-end"><button className="rounded-lg border border-white/10 px-4 py-2 text-sm font-semibold text-slate-200 hover:bg-white/5 disabled:opacity-40" disabled={!canReinstall || install.isPending || server.status === "installing"} onClick={() => { if (window.confirm("Reinstall this server? Installation scripts may overwrite server files.")) install.mutate(); }} type="button">{install.isPending ? "Reinstall requested…" : "Reinstall server"}</button></div>
  </div>;
}
