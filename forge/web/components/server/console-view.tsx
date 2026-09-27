"use client";

import { FormEvent, KeyboardEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle, ArrowDown, Clock, Cpu, Download, Loader2, MemoryStick, Network, Play, Power,
  RefreshCw, RotateCcw, Search, Send, ShieldAlert, Square, Terminal, Trash2, Upload, WifiOff,
} from "lucide-react";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  type ApiServer,
  type ApiStats,
  type PowerSignal,
  connectServerWebSocket,
  fetchOperation,
  fetchServerLogs,
  reinstallServer,
  sendPowerSignal,
} from "@/lib/api";
import { WebSocketManager } from "@/lib/api/ws/websocket-manager";
import { cn, formatBytes } from "@/lib/utils";
import { hasServerPermission, useServerContext } from "./server-context";
import { CrashBanner } from "./crash-banner";
import { useConfirm } from "@/components/ui/confirm-dialog";

const MAX_LINES = 500;
const MAX_POINTS = 60;
const OPERATION_POLL_MS = 1200;

/* -------------------------------------------------------------------------- */
/*  Console output entries                                                    */
/* -------------------------------------------------------------------------- */

/**
 * `output` is bytes the workload produced. `notice` and `error` are this
 * client's own annotations and are rendered distinctly, so nothing the panel
 * says is ever mistaken for something the workload said.
 */
export type ConsoleEntryKind = "output" | "notice" | "error";

type ConsoleEntry = { text: string; ts: number; serverTs: string | null; kind: ConsoleEntryKind };

export function extractServerTimestamp(line: string): string | null {
  const bracket = line.match(/^\[(\d{2}:\d{2}:\d{2}(?:\.\d+)?)\]/);
  if (bracket) return bracket[1];
  const iso = line.match(/^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)/);
  return iso?.[1] ?? null;
}

function consoleEntries(text: string, kind: ConsoleEntryKind = "output"): ConsoleEntry[] {
  // Freeze the receipt time once per batch, not on every render.
  const ts = Date.now();
  return text.split("\n").filter(Boolean).map((line) => ({
    text: line,
    ts,
    serverTs: kind === "output" ? extractServerTimestamp(line) : null,
    kind,
  }));
}

/* -------------------------------------------------------------------------- */
/*  Runtime lifecycle — what the node actually reported                       */
/* -------------------------------------------------------------------------- */

/**
 * A reading of the workload from `runtime.Inspect` on the node, as carried by
 * beacon's console `type: "state"` frames and by every stats frame.
 *
 * `null` fields mean the node did not report the value. They are never
 * defaulted to zero: an absent start time is not "started at the epoch" and an
 * absent uptime is not "up for 0 ms".
 */
export type RuntimeLifecycle = {
  exists: boolean;
  running: boolean;
  status: string | null;
  startedAt: string | null;
  uptimeMs: number | null;
  /** Client receipt time, used to age the reading rather than to invent one. */
  observedAt: number;
};

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function readLifecycle(frame: Record<string, unknown>, observedAt: number): RuntimeLifecycle | null {
  if (typeof frame.running !== "boolean") return null;
  return {
    exists: typeof frame.exists === "boolean" ? frame.exists : true,
    running: frame.running,
    status: typeof frame.status === "string" && frame.status ? frame.status : null,
    startedAt: typeof frame.startedAt === "string" && frame.startedAt ? frame.startedAt : null,
    uptimeMs: typeof frame.uptimeMs === "number" && Number.isFinite(frame.uptimeMs) ? frame.uptimeMs : null,
    observedAt,
  };
}

function frameMessage(frame: Record<string, unknown>, fallback: string): string {
  if (typeof frame.error === "string" && frame.error) return frame.error;
  if (typeof frame.data === "string" && frame.data) return frame.data;
  return fallback;
}

/* -------------------------------------------------------------------------- */
/*  Frame decoding                                                            */
/* -------------------------------------------------------------------------- */

export type ConsoleFrame =
  | { kind: "output"; text: string }
  | { kind: "state"; lifecycle: RuntimeLifecycle }
  | { kind: "error"; code: string; message: string }
  | { kind: "ignored" };

/**
 * Decodes one console websocket frame.
 *
 * `WebSocketManager` hands us the parsed JSON object, so the frames beacon
 * emits (`{type, data}`) arrive as objects — stringifying them here is what
 * used to render every line as `[object Object]`. A raw string still arrives
 * for non-JSON output (see `deliverRawText`) and is treated as output, because
 * dropping it would lose real workload bytes.
 */
export function decodeConsoleFrame(data: unknown, observedAt: number = Date.now()): ConsoleFrame {
  if (typeof data === "string") {
    return data ? { kind: "output", text: data } : { kind: "ignored" };
  }
  const frame = asRecord(data);
  if (!frame) return { kind: "ignored" };

  const type = typeof frame.type === "string" ? frame.type : "";
  if (type === "error") {
    const code = typeof frame.code === "string" && frame.code ? frame.code : "stream_error";
    return { kind: "error", code, message: frameMessage(frame, "The console stream reported an error.") };
  }
  if (type === "state") {
    const lifecycle = readLifecycle(frame, observedAt);
    return lifecycle ? { kind: "state", lifecycle } : { kind: "ignored" };
  }
  if (type === "output" || type === "log" || type === "status") {
    return typeof frame.data === "string" && frame.data
      ? { kind: "output", text: frame.data }
      : { kind: "ignored" };
  }
  // Untyped shapes emitted by the logs stream and by older daemon builds.
  if (typeof frame.logs === "string" && frame.logs) return { kind: "output", text: frame.logs };
  if (typeof frame.data === "string" && frame.data) return { kind: "output", text: frame.data };
  if (typeof frame.error === "string" && frame.error) {
    return { kind: "error", code: "stream_error", message: frame.error };
  }
  const lifecycle = readLifecycle(frame, observedAt);
  return lifecycle ? { kind: "state", lifecycle } : { kind: "ignored" };
}

export type StatsFrame =
  | { kind: "sample"; stats: ApiStats; lifecycle: RuntimeLifecycle | null }
  | { kind: "lifecycle"; lifecycle: RuntimeLifecycle }
  | { kind: "error"; code: string; message: string }
  | { kind: "ignored" };

/**
 * Decodes one stats websocket frame. A frame with `metrics: false` is a
 * lifecycle report from a workload that has no metrics to sample — it carries
 * no numbers, and must not be plotted as zeros.
 */
export function decodeStatsFrame(data: unknown, observedAt: number = Date.now()): StatsFrame {
  const frame = asRecord(data);
  if (!frame) return { kind: "ignored" };

  if (frame.type === "error" || (typeof frame.error === "string" && frame.error)) {
    const code = typeof frame.code === "string" && frame.code ? frame.code : "stream_error";
    return { kind: "error", code, message: frameMessage(frame, "Telemetry reported an error.") };
  }

  const lifecycle = readLifecycle(frame, observedAt);
  if (frame.metrics === false) {
    return lifecycle ? { kind: "lifecycle", lifecycle } : { kind: "ignored" };
  }
  if (typeof frame.cpuPercent === "number" && Number.isFinite(frame.cpuPercent)) {
    return { kind: "sample", stats: frame as unknown as ApiStats, lifecycle };
  }
  return lifecycle ? { kind: "lifecycle", lifecycle } : { kind: "ignored" };
}

/* -------------------------------------------------------------------------- */
/*  Session state                                                            */
/* -------------------------------------------------------------------------- */

export type LinkState = "connecting" | "connected" | "reconnecting" | "disconnected";

export type SessionState =
  | "forbidden"
  | "unavailable"
  | "connecting"
  | "reconnecting"
  | "disconnected"
  | "stream-error"
  | "workload-stopped"
  | "connected";

/**
 * Collapses everything known about the session into the one state shown to the
 * operator. Transport health and workload lifecycle are distinct facts, so a
 * stopped workload reads as stopped rather than as a broken connection — the
 * daemon keeps that socket open on purpose.
 */
export function deriveSessionState(input: {
  permitted: boolean;
  blockedReason: string | null;
  link: LinkState;
  streamError: string | null;
  lifecycle: RuntimeLifecycle | null;
}): SessionState {
  if (!input.permitted) return "forbidden";
  if (input.blockedReason) return "unavailable";
  if (input.link !== "connected") return input.link;
  if (input.streamError) return "stream-error";
  if (input.lifecycle && !input.lifecycle.running) return "workload-stopped";
  return "connected";
}

type SessionPresentation = {
  label: string;
  detail: string;
  tone: "ok" | "warn" | "bad" | "idle";
  busy: boolean;
};

function describeSession(state: SessionState, context: { blockedReason: string | null; streamError: string | null; lifecycle: RuntimeLifecycle | null }): SessionPresentation {
  switch (state) {
    case "forbidden":
      return { label: "No access", detail: "You do not have permission to attach to this console.", tone: "bad", busy: false };
    case "unavailable":
      return { label: "Unavailable", detail: context.blockedReason ?? "The console is unavailable for this workload.", tone: "bad", busy: false };
    case "connecting":
      return { label: "Connecting", detail: "Opening the console stream to the node.", tone: "warn", busy: true };
    case "reconnecting":
      return { label: "Reconnecting", detail: "The stream dropped. Retrying with backoff.", tone: "warn", busy: true };
    case "disconnected":
      return { label: "Disconnected", detail: "The console stream is closed. Reconnect to attach again.", tone: "bad", busy: false };
    case "stream-error":
      return { label: "Stream error", detail: context.streamError ?? "The node reported a stream error.", tone: "bad", busy: false };
    case "workload-stopped":
      return {
        label: "Workload stopped",
        detail: context.lifecycle?.exists === false
          ? "The node reports no container for this workload. Attached and waiting — output resumes when it starts."
          : `Attached and waiting. The runtime reports ${context.lifecycle?.status ?? "not running"}; output resumes when it starts.`,
        tone: "idle",
        busy: false,
      };
    case "connected":
      return { label: "Attached", detail: "Streaming live output from the workload.", tone: "ok", busy: false };
  }
}

const TONE_DOT: Record<SessionPresentation["tone"], string> = {
  ok: "bg-emerald-400",
  warn: "bg-amber-400 animate-pulse",
  bad: "bg-[var(--danger)]",
  idle: "bg-[var(--text-muted)]",
};

const TONE_TEXT: Record<SessionPresentation["tone"], string> = {
  ok: "text-emerald-300",
  warn: "text-amber-200",
  bad: "text-[var(--danger)]",
  idle: "text-[var(--text-subtle)]",
};

/* -------------------------------------------------------------------------- */
/*  Telemetry maths                                                           */
/* -------------------------------------------------------------------------- */

export function computeNetworkDelta(rx: number, tx: number, prevRx: number | null, prevTx: number | null) {
  let delta = 0;
  if (prevRx !== null && prevTx !== null) {
    const drx = rx - prevRx;
    const dtx = tx - prevTx;
    // Each counter can reset independently. Never add an unchanged lifetime total.
    delta = Math.max(0, drx < 0 ? rx : drx) + Math.max(0, dtx < 0 ? tx : dtx);
  }
  return { delta, nextPrevRx: rx, nextPrevTx: tx };
}

export function getChartMax(values: number[], limit?: number): number {
  const observedMax = values.length ? Math.max(...values) : 0;
  const ceiling = typeof limit === "number" && Number.isFinite(limit) && limit > 0 ? limit : 100;
  return Math.max(observedMax, ceiling, 1);
}

function formatUptime(ms: number | undefined | null): string {
  if (typeof ms !== "number" || !Number.isFinite(ms) || ms <= 0) return "—";
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
/*  Workload state banners                                                    */
/* -------------------------------------------------------------------------- */

function InstallBanner() {
  return (
    <div className="flex items-start gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 p-4">
      <Download className="mt-0.5 shrink-0 text-amber-300" size={19} />
      <div>
        <p className="text-sm font-semibold text-amber-100">This server is being installed</p>
        <p className="mt-1 text-xs text-amber-200/70">
          The installation process is running. The console will display output from the installation script.
          Do not restart or power off the server during this process.
        </p>
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

function Chart({ label, value, detail, values, limit, icon: Icon }: { label: string; value: string; detail: string; values: number[]; limit?: number; icon: typeof Cpu }) {
  const max = getChartMax(values, limit);
  const points = values.map((point, index) => `${values.length < 2 ? 0 : (index / (values.length - 1)) * 100},${100 - (point / max) * 92}`).join(" ");
  return (
    <section aria-label={`${label} chart`} className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="flex items-center gap-2 text-[11px] font-bold uppercase tracking-wider text-[var(--text-subtle)]">
          <Icon size={13} className="text-[var(--brand)]" />{label}
        </div>
        <div className="text-right">
          <p className="font-mono text-sm font-bold text-[var(--text)]">{value}</p>
          <p className="font-mono text-[10px] text-[var(--text-muted)]">{detail}</p>
        </div>
      </div>
      <svg aria-hidden="true" className="mt-3 h-14 w-full" preserveAspectRatio="none" viewBox="0 0 100 100">
        <line stroke="var(--line-strong)" strokeWidth=".5" x1="0" x2="100" y1="50" y2="50" />
        <polygon fill="color-mix(in srgb, var(--brand) 15%, transparent)" points={`0,100 ${points} 100,100`} />
        <polyline fill="none" points={points} stroke="var(--brand)" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.5" />
      </svg>
    </section>
  );
}

/* -------------------------------------------------------------------------- */
/*  Power controls                                                            */
/* -------------------------------------------------------------------------- */

type PowerAction = {
  signal: PowerSignal;
  label: string;
  icon: typeof Play;
  intent: "go" | "neutral" | "danger";
  /** Destructive actions require confirmation before dispatch. */
  confirm?: { title: string; description: string; confirmLabel: string };
};

const POWER_ACTIONS: PowerAction[] = [
  { signal: "start", label: "Start", icon: Play, intent: "go" },
  {
    signal: "restart", label: "Restart", icon: RotateCcw, intent: "neutral",
    confirm: { title: "Restart workload?", description: "The workload is stopped and started again. Active player sessions and in-flight work are dropped.", confirmLabel: "Restart" },
  },
  {
    signal: "stop", label: "Stop", icon: Square, intent: "neutral",
    confirm: { title: "Stop workload?", description: "The runtime is asked to stop the workload gracefully. It will stay offline until started again.", confirmLabel: "Stop" },
  },
  {
    signal: "kill", label: "Kill", icon: Power, intent: "danger",
    confirm: { title: "Kill workload?", description: "The process is terminated immediately with no chance to shut down. Unsaved data may be lost.", confirmLabel: "Kill" },
  },
];

/**
 * A dispatched power signal, tracked from the request through the durable
 * operation the API created for it.
 *
 * `accepted` is deliberately distinct from `succeeded`: the API returning 202
 * means the operation was recorded, not that the workload obeyed. When no
 * operation id comes back there is nothing to poll, and that is reported as
 * untrackable rather than as success.
 */
type OperationTrack = {
  signal: string;
  phase: "dispatching" | "accepted" | "pending" | "running" | "succeeded" | "failed" | "cancelled" | "unknown" | "untracked";
  operationId: string | null;
  mode?: string;
  message?: string;
};

const OPERATION_LABEL: Record<OperationTrack["phase"], string> = {
  dispatching: "Dispatching",
  accepted: "Accepted",
  pending: "Queued",
  running: "Running",
  succeeded: "Completed",
  failed: "Failed",
  cancelled: "Cancelled",
  unknown: "Unrecognised status",
  untracked: "Accepted, progress not trackable",
};

/* -------------------------------------------------------------------------- */
/*  Console workspace                                                         */
/* -------------------------------------------------------------------------- */

export function ConsoleView({ server }: { server: ApiServer }) {
  const { access, refreshServer } = useServerContext();

  const [lines, setLines] = useState<ConsoleEntry[]>([]);
  const [command, setCommand] = useState("");
  const [history, setHistory] = useState<string[]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);
  const [link, setLink] = useState<LinkState>("connecting");
  const [streamError, setStreamError] = useState<string | null>(null);
  const [historyError, setHistoryError] = useState<string | null>(null);
  const [lifecycle, setLifecycle] = useState<RuntimeLifecycle | null>(null);
  const [nonce, setNonce] = useState(0);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchOpen, setSearchOpen] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const [showTimestamps, setShowTimestamps] = useState(false);
  const [track, setTrack] = useState<OperationTrack | null>(null);
  const [confirm, renderConfirm] = useConfirm();

  const [stats, setStats] = useState<ApiStats | null>(null);
  const [cpuHistory, setCpuHistory] = useState<number[]>([]);
  const [memoryHistory, setMemoryHistory] = useState<number[]>([]);
  const [networkHistory, setNetworkHistory] = useState<number[]>([]);
  const [networkDelta, setNetworkDelta] = useState<number | null>(null);
  const [clock, setClock] = useState(() => Date.now());

  const messageCount = useRef(0);
  const prevRxRef = useRef<number | null>(null);
  const prevTxRef = useRef<number | null>(null);
  const outputRef = useRef<HTMLDivElement>(null);
  const workspaceRef = useRef<HTMLDivElement>(null);
  const commandRef = useRef<HTMLInputElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const managerRef = useRef<WebSocketManager | null>(null);
  const HISTORY_KEY = `console-history-${server.id}`;

  // The proxy gates the console stream on websocket.connect *and*
  // control.console (see realtime.go), so attaching needs both. Asking for
  // either here would show an attached console that the API then refuses.
  const canConsole = hasServerPermission(access, "websocket.connect") && hasServerPermission(access, "control.console");
  // Telemetry is read-only and gated on websocket.connect alone.
  const canStats = hasServerPermission(access, "websocket.connect");
  const canPower = (signal: PowerSignal) => hasServerPermission(access, signal === "start" ? "control.start" : signal === "restart" ? "control.restart" : "control.stop");
  const canReinstall = hasServerPermission(access, "settings.reinstall");

  // A suspended or transferring workload has no console to attach to. An
  // installing one does: the installer's output is exactly what to watch.
  const consoleBlockedReason = server.suspended
    ? "This workload is suspended. Runtime streams are disabled."
    : server.transferring
      ? "This workload is transferring to another node. Its console lives on the node that ends up owning it."
      : null;
  const controlsBlocked = Boolean(consoleBlockedReason) || server.status === "installing";

  const appendEntries = useCallback((entries: ConsoleEntry[]) => {
    if (!entries.length) return;
    setLines((current) => [...current, ...entries].slice(-MAX_LINES));
  }, []);

  /* ---- console stream ---------------------------------------------------- */

  useEffect(() => {
    if (!canConsole || consoleBlockedReason) {
      setLink("disconnected");
      return;
    }
    setLink("connecting");
    setStreamError(null);
    setHistoryError(null);
    let aborted = false;

    void fetchServerLogs(server.id)
      .then((logs) => {
        if (aborted) return;
        setLines(consoleEntries(logs).slice(-MAX_LINES));
      })
      .catch((error) => {
        if (aborted) return;
        // Recent history is a separate fetch from the live stream. Failing to
        // load it does not mean the stream is broken, and must not be reported
        // as though it were.
        setHistoryError(error instanceof Error ? error.message : "Previous logs could not be loaded.");
      });

    const manager = new WebSocketManager({
      maxRetries: 20,
      baseDelay: 1000,
      maxDelay: 30000,
      deliverRawText: true,
      factory: () => connectServerWebSocket(server.id, "console"),
      onMessage: (data) => {
        if (aborted) return;
        messageCount.current += 1;
        const frame = decodeConsoleFrame(data);
        switch (frame.kind) {
          case "output":
            appendEntries(consoleEntries(frame.text));
            break;
          case "state":
            setLifecycle(frame.lifecycle);
            // The stream reporting a lifecycle means it is healthy.
            setStreamError(null);
            // The workload's recorded status in the panel lags the node's.
            void refreshServer();
            break;
          case "error":
            if (frame.code === "not_running") {
              // Answer to a command sent to a stopped workload: a lifecycle
              // fact, surfaced inline where the command was typed.
              appendEntries(consoleEntries(frame.message, "error"));
            } else if (frame.code === "command_failed") {
              appendEntries(consoleEntries(frame.message, "error"));
            } else {
              setStreamError(frame.message);
            }
            break;
          case "ignored":
            break;
        }
      },
      onStatusChange: (status) => {
        if (aborted) return;
        setLink(status);
        if (status === "connected") {
          messageCount.current = 0;
          setStreamError(null);
        }
      },
      onError: () => {
        if (!aborted) setStreamError("The console connection failed.");
      },
    });
    managerRef.current = manager;
    void manager.connect();
    return () => {
      aborted = true;
      manager.disconnect();
      managerRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [canConsole, consoleBlockedReason, nonce, server.id]);

  /* ---- telemetry stream -------------------------------------------------- */

  useEffect(() => {
    prevRxRef.current = null;
    prevTxRef.current = null;
    setStats(null);
    setCpuHistory([]);
    setMemoryHistory([]);
    setNetworkHistory([]);
    setNetworkDelta(null);
    if (!canStats || consoleBlockedReason) return;
    let aborted = false;

    const clearSample = () => {
      // A stopped workload has no current usage. Leaving the last sample on
      // screen would present a stale reading as a live one; plotting zeros
      // would invent one. Drop the current values and keep the history.
      setStats(null);
      setNetworkDelta(null);
      prevRxRef.current = null;
      prevTxRef.current = null;
    };

    const statsManager = new WebSocketManager({
      maxRetries: 20,
      baseDelay: 1000,
      maxDelay: 30000,
      factory: () => connectServerWebSocket(server.id, "stats"),
      onMessage: (data) => {
        if (aborted) return;
        const frame = decodeStatsFrame(data);
        if (frame.kind === "error") {
          setStreamError(frame.message);
          clearSample();
          return;
        }
        if (frame.kind === "lifecycle") {
          setLifecycle(frame.lifecycle);
          if (!frame.lifecycle.running) clearSample();
          return;
        }
        if (frame.kind !== "sample") return;
        if (frame.lifecycle) setLifecycle(frame.lifecycle);
        const sample = frame.stats;
        setStats(sample);
        const memory = sample.memoryLimit > 0 ? (sample.memoryBytes / sample.memoryLimit) * 100 : 0;
        // Plot bytes transferred between samples, not lifetime counters or bytes/second.
        const hasBaseline = prevRxRef.current !== null && prevTxRef.current !== null;
        const network = computeNetworkDelta(sample.networkRxBytes, sample.networkTxBytes, prevRxRef.current, prevTxRef.current);
        prevRxRef.current = network.nextPrevRx;
        prevTxRef.current = network.nextPrevTx;
        setNetworkDelta(hasBaseline ? network.delta : null);
        setCpuHistory((items) => [...items.slice(-(MAX_POINTS - 1)), sample.cpuPercent]);
        setMemoryHistory((items) => [...items.slice(-(MAX_POINTS - 1)), memory]);
        setNetworkHistory((items) => [...items.slice(-(MAX_POINTS - 1)), network.delta]);
      },
      onError: () => {
        if (!aborted) setStreamError("Telemetry connection failed.");
      },
    });
    void statsManager.connect();
    return () => {
      aborted = true;
      statsManager.disconnect();
    };
  }, [canStats, consoleBlockedReason, nonce, server.id]);

  /* ---- command history --------------------------------------------------- */

  useEffect(() => {
    try {
      const saved = localStorage.getItem(HISTORY_KEY);
      if (saved) {
        const parsed = JSON.parse(saved);
        if (Array.isArray(parsed)) setHistory(parsed.filter((item): item is string => typeof item === "string"));
      }
    } catch { /* history is a convenience; a broken store is not an error */ }
  }, [HISTORY_KEY]);

  /* ---- scrolling --------------------------------------------------------- */

  useEffect(() => {
    if (autoScroll) requestAnimationFrame(() => outputRef.current?.scrollTo({ top: outputRef.current.scrollHeight }));
  }, [autoScroll, lines]);

  /* ---- live uptime ------------------------------------------------------- */

  useEffect(() => {
    if (!lifecycle?.running) return;
    const timer = setInterval(() => setClock(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [lifecycle?.running]);

  const liveUptimeMs = useMemo(() => {
    if (!lifecycle?.running) return null;
    if (lifecycle.startedAt) {
      const started = Date.parse(lifecycle.startedAt);
      if (Number.isFinite(started)) return Math.max(0, clock - started);
    }
    // No start time reported: age the reported uptime by how long ago we read it.
    if (lifecycle.uptimeMs !== null) return Math.max(0, lifecycle.uptimeMs + (clock - lifecycle.observedAt));
    if (typeof stats?.uptimeMs === "number") return stats.uptimeMs;
    return null;
  }, [clock, lifecycle, stats?.uptimeMs]);

  /* ---- operation progress ------------------------------------------------ */

  const trackedId = track && !["succeeded", "failed", "cancelled", "untracked", "unknown"].includes(track.phase) ? track.operationId : null;
  const operationQuery = useQuery({
    queryKey: ["server-operation", server.id, trackedId],
    queryFn: () => fetchOperation(trackedId as string),
    enabled: Boolean(trackedId),
    refetchInterval: OPERATION_POLL_MS,
    retry: false,
  });

  useEffect(() => {
    const progress = operationQuery.data;
    if (!progress) return;
    setTrack((current) => {
      if (!current || current.operationId !== progress.id) return current;
      return {
        ...current,
        phase: progress.status,
        message: progress.error
          ?? (progress.status === "unknown" ? `The API reported status "${progress.rawStatus}", which this console does not recognise.` : undefined),
      };
    });
    if (progress.terminal) void refreshServer();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [operationQuery.data]);

  useEffect(() => {
    const error = operationQuery.error;
    if (!error) return;
    setTrack((current) => current ? {
      ...current,
      phase: "unknown",
      message: `Progress could not be read: ${error instanceof Error ? error.message : "the operation lookup failed"}.`,
    } : current);
  }, [operationQuery.error]);

  const power = useMutation({
    mutationFn: (signal: PowerSignal) => sendPowerSignal(server.id, signal),
    onSuccess: (dispatch, signal) => {
      void refreshServer();
      if (!dispatch.accepted) {
        setTrack({ signal, phase: "failed", operationId: null, message: "The API did not accept the signal." });
        return;
      }
      if (!dispatch.operationId) {
        setTrack({
          signal,
          phase: "untracked",
          operationId: null,
          mode: dispatch.mode,
          message: "The API accepted the signal but reported no operation id, so its progress cannot be followed here. Watch the console and runtime state.",
        });
        return;
      }
      setTrack({ signal, phase: "accepted", operationId: dispatch.operationId, mode: dispatch.mode });
    },
    onError: (error, signal) => {
      setTrack({ signal, phase: "failed", operationId: null, message: error instanceof Error ? error.message : "The power signal was rejected." });
    },
  });

  const install = useMutation({
    mutationFn: () => reinstallServer(server.id),
    onSuccess: (result) => {
      void refreshServer();
      setTrack({
        signal: "reinstall",
        phase: result.accepted ? "untracked" : "failed",
        operationId: null,
        message: result.accepted
          ? "Reinstall accepted by the node. Its progress appears as installer output in the console."
          : "The node did not accept the reinstall request.",
      });
    },
    onError: (error) => {
      setTrack({ signal: "reinstall", phase: "failed", operationId: null, message: error instanceof Error ? error.message : "The reinstall request failed." });
    },
  });

  /* ---- derived session state --------------------------------------------- */

  const session = deriveSessionState({
    permitted: canConsole,
    blockedReason: consoleBlockedReason,
    link,
    streamError,
    lifecycle,
  });
  const presentation = describeSession(session, { blockedReason: consoleBlockedReason, streamError, lifecycle });
  const attached = session === "connected";
  const runningNow = lifecycle ? lifecycle.running : server.status === "running" ? true : ["offline", "stopped", "crashed", "terminated"].includes(server.status) ? false : null;
  const canType = attached && runningNow !== false;

  /* ---- command submission ------------------------------------------------ */

  const runCommand = (value: string) => {
    const manager = managerRef.current;
    if (!manager || manager.status !== "connected") {
      // Buffering the command would run it at an unpredictable later moment.
      appendEntries(consoleEntries("Not sent: the console is not attached.", "error"));
      return;
    }
    if (runningNow === false) {
      appendEntries(consoleEntries("Not sent: the workload is not running.", "error"));
      return;
    }
    appendEntries(consoleEntries(`> ${value}`, "notice"));
    manager.send(value);
    setHistory((items) => {
      const next = [value, ...items.filter((item) => item !== value)].slice(0, 50);
      try { localStorage.setItem(HISTORY_KEY, JSON.stringify(next)); } catch { /* quota or private mode */ }
      return next;
    });
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const value = command.trim();
    if (!value) return;
    runCommand(value);
    setHistoryIndex(-1);
    setCommand("");
  };

  const historyKey = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "l" && (event.ctrlKey || event.metaKey)) {
      event.preventDefault();
      setLines([]);
      return;
    }
    if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;
    event.preventDefault();
    const next = event.key === "ArrowUp" ? Math.min(historyIndex + 1, history.length - 1) : Math.max(historyIndex - 1, -1);
    setHistoryIndex(next);
    setCommand(next < 0 ? "" : history[next] ?? "");
  };

  /* ---- workspace keyboard shortcuts -------------------------------------- */

  useEffect(() => {
    const onKeyDown = (event: globalThis.KeyboardEvent) => {
      const workspace = workspaceRef.current;
      if (!workspace) return;
      const focusInside = workspace.contains(document.activeElement);
      const mod = event.ctrlKey || event.metaKey;
      if (mod && event.key.toLowerCase() === "k") {
        event.preventDefault();
        commandRef.current?.focus();
        return;
      }
      if (!focusInside) return;
      if (mod && event.shiftKey && event.key.toLowerCase() === "f") {
        event.preventDefault();
        setSearchOpen((open) => {
          if (!open) requestAnimationFrame(() => searchRef.current?.focus());
          return !open;
        });
        return;
      }
      if (event.key === "Escape" && searchOpen) {
        event.preventDefault();
        setSearchOpen(false);
        setSearchQuery("");
        commandRef.current?.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [searchOpen]);

  /* ---- render ------------------------------------------------------------ */

  const filteredLines = searchQuery ? lines.filter((entry) => entry.text.toLowerCase().includes(searchQuery.toLowerCase())) : lines;
  const memoryPercent = stats && stats.memoryLimit > 0 ? (stats.memoryBytes / stats.memoryLimit) * 100 : null;
  const noTelemetryReason = runningNow === false ? "Not running" : "Waiting for telemetry";
  const runtimeStatusLabel = lifecycle?.status ?? (lifecycle ? (lifecycle.running ? "running" : lifecycle.exists ? "stopped" : "absent") : null);

  return (
    <div className="flex flex-col gap-4" ref={workspaceRef}>
      {renderConfirm()}

      {server.suspended ? <SuspendedBanner /> : null}
      {server.transferring ? <TransferBanner server={server} /> : null}
      {server.status === "installing" && !server.suspended && !server.transferring ? <InstallBanner /> : null}
      {!server.suspended && !server.transferring && server.status !== "installing" ? <CrashBanner serverId={server.id} /> : null}

      {/* ---- command bar: identity, live runtime state, power rail -------- */}
      <section
        aria-label="Workload controls"
        className="flex flex-col gap-3 rounded-xl border border-[var(--line)] bg-[var(--surface)] p-3 lg:flex-row lg:items-center lg:justify-between"
      >
        <div className="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1">
          <div className="flex min-w-0 items-center gap-2">
            <Terminal className="shrink-0 text-[var(--brand)]" size={16} />
            <h2 className="truncate text-sm font-bold tracking-tight text-[var(--text)]">{server.name}</h2>
          </div>
          <dl className="flex flex-wrap items-center gap-x-4 gap-y-1 font-mono text-[11px] text-[var(--text-muted)]">
            <div className="flex items-center gap-1.5">
              <dt className="uppercase tracking-wider">Runtime</dt>
              <dd className={cn("font-semibold", runningNow ? "text-emerald-300" : runningNow === false ? "text-[var(--text-subtle)]" : "text-amber-200")}>
                {runtimeStatusLabel ?? `${server.status} (panel record)`}
              </dd>
            </div>
            <div className="flex items-center gap-1.5">
              <dt className="uppercase tracking-wider">Uptime</dt>
              <dd className="font-semibold text-[var(--text)]">
                {liveUptimeMs === null ? (runningNow === false ? "—" : "Not reported") : formatUptime(liveUptimeMs)}
              </dd>
            </div>
            {server.node ? (
              <div className="flex items-center gap-1.5">
                <dt className="uppercase tracking-wider">Node</dt>
                <dd className="font-semibold text-[var(--text)]">{server.node}</dd>
              </div>
            ) : null}
            {server.runtimeProvider ? (
              <div className="flex items-center gap-1.5">
                <dt className="uppercase tracking-wider">Engine</dt>
                <dd className="font-semibold text-[var(--text)]">{server.runtimeProvider}</dd>
              </div>
            ) : null}
          </dl>
        </div>

        <div className="flex flex-wrap items-center gap-1.5">
          {POWER_ACTIONS.map((action) => {
            const expected = action.signal === "start" ? runningNow !== true : runningNow !== false;
            const disabled = !canPower(action.signal) || controlsBlocked || power.isPending || !expected;
            const Icon = action.icon;
            return (
              <button
                aria-label={`${action.label} workload`}
                className={cn(
                  "inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-[11px] font-bold uppercase tracking-wider transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] disabled:cursor-not-allowed disabled:opacity-40",
                  action.intent === "go" ? "bg-emerald-600 text-white hover:bg-emerald-500"
                    : action.intent === "danger" ? "bg-[var(--danger)] text-white hover:bg-[var(--danger-hover)]"
                      : "border border-[var(--line)] bg-[var(--surface-raised)] text-[var(--text)] hover:bg-white/[0.06]",
                )}
                disabled={disabled}
                key={action.signal}
                onClick={async () => {
                  if (action.confirm && !(await confirm({ ...action.confirm, danger: action.intent === "danger" }))) return;
                  setTrack({ signal: action.signal, phase: "dispatching", operationId: null });
                  power.mutate(action.signal);
                }}
                title={disabled && !expected ? `The workload is ${runningNow ? "running" : "not running"}` : undefined}
                type="button"
              >
                {power.isPending && power.variables === action.signal ? <Loader2 className="animate-spin" size={12} /> : <Icon size={12} />}
                {action.label}
              </button>
            );
          })}
          <button
            aria-label="Reinstall workload"
            className="inline-flex items-center gap-1.5 rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-2.5 py-1.5 text-[11px] font-bold uppercase tracking-wider text-[var(--text-subtle)] transition-colors hover:bg-white/[0.06] hover:text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] disabled:cursor-not-allowed disabled:opacity-40"
            disabled={!canReinstall || install.isPending || controlsBlocked}
            onClick={async () => {
              if (!(await confirm({
                title: "Reinstall this workload?",
                description: "The install script runs again on the node and may overwrite workload files. This cannot be undone.",
                confirmLabel: "Reinstall",
                danger: true,
              }))) return;
              install.mutate();
            }}
            type="button"
          >
            {install.isPending ? <Loader2 className="animate-spin" size={12} /> : <Download size={12} />}
            Reinstall
          </button>
        </div>
      </section>

      {/* ---- operation progress ------------------------------------------- */}
      {track ? (
        <div
          aria-live="polite"
          className={cn(
            "flex flex-wrap items-center gap-x-3 gap-y-1 rounded-xl border px-3 py-2 text-xs",
            track.phase === "failed" || track.phase === "cancelled" ? "border-[var(--danger)]/30 bg-[var(--danger)]/10 text-[var(--danger)]"
              : track.phase === "succeeded" ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-200"
                : "border-[var(--line)] bg-[var(--surface-raised)] text-[var(--text-subtle)]",
          )}
          role="status"
        >
          {["dispatching", "accepted", "pending", "running"].includes(track.phase) ? <Loader2 className="animate-spin" size={13} /> : null}
          <span className="font-mono font-bold uppercase tracking-wider">{track.signal}</span>
          <span className="font-semibold">{OPERATION_LABEL[track.phase]}</span>
          {track.operationId ? <span className="font-mono text-[10px] text-[var(--text-muted)]">operation {track.operationId}</span> : null}
          {track.mode ? <span className="font-mono text-[10px] text-[var(--text-muted)]">mode {track.mode}</span> : null}
          {track.message ? <span className="min-w-0 basis-full font-mono text-[11px]">{track.message}</span> : null}
          <button
            aria-label="Dismiss operation status"
            className="ml-auto rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-[var(--text-muted)] hover:text-[var(--text)]"
            onClick={() => setTrack(null)}
            type="button"
          >
            Dismiss
          </button>
        </div>
      ) : null}

      {/* ---- workspace: terminal + telemetry rail ------------------------- */}
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_20rem]">
        <section aria-label="Server console" className="flex min-w-0 flex-col overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--canvas)] shadow-xl">
          <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--line)] bg-[var(--surface)] px-3 py-2">
            <div aria-live="polite" className="flex min-w-0 items-center gap-2 text-xs" role="status">
              <span className={cn("h-2 w-2 shrink-0 rounded-full", TONE_DOT[presentation.tone])} />
              <span className={cn("font-semibold", TONE_TEXT[presentation.tone])}>{presentation.label}</span>
              {attached ? <span className="font-mono text-[11px] text-[var(--text-muted)]">· {messageCount.current} frames</span> : null}
              <span className="hidden min-w-0 truncate font-mono text-[11px] text-[var(--text-muted)] sm:inline">· {presentation.detail}</span>
            </div>
            <div className="flex items-center gap-1">
              <button aria-label="Toggle search" aria-pressed={searchOpen} className={cn("rounded-md p-1.5 transition-colors hover:bg-white/5", searchOpen ? "bg-[var(--brand)]/10 text-[var(--brand)]" : "text-[var(--text-subtle)] hover:text-[var(--text)]")} onClick={() => setSearchOpen((v) => !v)} type="button"><Search size={14} /></button>
              <button aria-label={autoScroll ? "Freeze scroll" : "Auto-scroll"} aria-pressed={autoScroll} className={cn("rounded-md p-1.5 transition-colors hover:bg-white/5", autoScroll ? "text-emerald-400" : "text-[var(--text-muted)] hover:text-[var(--text)]")} onClick={() => setAutoScroll((v) => !v)} type="button"><ArrowDown size={14} /></button>
              <button aria-label={showTimestamps ? "Hide timestamps" : "Show timestamps"} aria-pressed={showTimestamps} className={cn("rounded-md p-1.5 transition-colors hover:bg-white/5", showTimestamps ? "text-emerald-400" : "text-[var(--text-muted)] hover:text-[var(--text)]")} onClick={() => setShowTimestamps((v) => !v)} type="button"><Clock size={14} /></button>
              <button aria-label="Reconnect console" className="rounded-md p-1.5 text-[var(--text-subtle)] transition-colors hover:bg-white/5 hover:text-[var(--text)]" onClick={() => setNonce((value) => value + 1)} type="button"><RefreshCw size={14} /></button>
              <button aria-label="Clear console" className="rounded-md p-1.5 text-[var(--text-subtle)] transition-colors hover:bg-white/5 hover:text-[var(--text)]" onClick={() => setLines([])} type="button"><Trash2 size={14} /></button>
            </div>
          </div>

          {/* Session state strip — explains the state and offers the way out. */}
          {session !== "connected" ? (
            <div className={cn(
              "flex flex-wrap items-center gap-x-3 gap-y-1 border-b px-3 py-2 text-xs",
              presentation.tone === "bad" ? "border-[var(--danger)]/30 bg-[var(--danger)]/10 text-[var(--danger)]"
                : presentation.tone === "warn" ? "border-amber-500/30 bg-amber-500/10 text-amber-100"
                  : "border-[var(--line)] bg-[var(--surface-raised)] text-[var(--text-subtle)]",
            )}>
              {presentation.busy ? <Loader2 className="animate-spin" size={13} />
                : session === "forbidden" ? <ShieldAlert size={13} />
                  : session === "workload-stopped" ? <Clock size={13} />
                    : <WifiOff size={13} />}
              <span className="min-w-0 font-mono text-[11px]">{presentation.detail}</span>
              {session === "disconnected" || session === "stream-error" ? (
                <button
                  className="ml-auto rounded-md border border-current px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider"
                  onClick={() => setNonce((value) => value + 1)}
                  type="button"
                >
                  Reconnect
                </button>
              ) : null}
              {session === "workload-stopped" && canPower("start") && !controlsBlocked ? (
                <button
                  className="ml-auto inline-flex items-center gap-1 rounded-md bg-emerald-600 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider text-white hover:bg-emerald-500"
                  onClick={() => {
                    setTrack({ signal: "start", phase: "dispatching", operationId: null });
                    power.mutate("start");
                  }}
                  type="button"
                >
                  <Play size={11} /> Start
                </button>
              ) : null}
            </div>
          ) : null}

          {historyError ? (
            <p className="border-b border-amber-500/30 bg-amber-500/10 px-3 py-1.5 font-mono text-[11px] text-amber-100">
              Recent history unavailable: {historyError} Live output is unaffected.
            </p>
          ) : null}

          {searchOpen ? (
            <div className="flex items-center gap-2 border-b border-[var(--line)] bg-[var(--surface-input)] px-3 py-2">
              <Search className="shrink-0 text-[var(--text-muted)]" size={13} />
              <label className="sr-only" htmlFor="console-filter">Filter console output</label>
              <input
                autoComplete="off"
                className="w-full bg-transparent font-mono text-xs text-[var(--text)] outline-none placeholder:text-[var(--text-muted)]"
                id="console-filter"
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Filter console output…"
                ref={searchRef}
                value={searchQuery}
              />
              <span className="shrink-0 font-mono text-[10px] text-[var(--text-muted)]">{filteredLines.length}/{lines.length}</span>
            </div>
          ) : null}

          <div
            aria-label="Console output"
            aria-live="polite"
            aria-relevant="additions text"
            className="h-[48vh] min-h-72 flex-1 overflow-y-auto bg-[var(--canvas)] p-3 font-mono text-xs leading-5 text-[var(--text)] scrollbar-thin focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-[var(--focus)]"
            ref={outputRef}
            role="log"
            tabIndex={0}
          >
            {filteredLines.length ? filteredLines.map((entry, index) => (
              <div
                className={cn(
                  "rounded px-1 py-0.5 whitespace-pre-wrap break-words transition-colors hover:bg-white/[0.02]",
                  entry.kind === "notice" ? "text-[var(--brand)]" : entry.kind === "error" ? "text-[var(--danger)]" : null,
                )}
                key={`${index}-${entry.ts}-${entry.text}`}
              >
                {showTimestamps ? <span className="mr-2.5 select-none font-mono text-[11px] text-[var(--text-muted)]">{entry.serverTs ?? new Date(entry.ts).toLocaleTimeString()}</span> : null}
                <span>{entry.text}</span>
              </div>
            )) : (
              <p className="font-mono text-xs text-[var(--text-muted)]">
                {searchQuery ? "No matching console output."
                  : session === "workload-stopped" ? "No output. The workload is not running."
                    : presentation.busy ? "Attaching to the workload…"
                      : "Waiting for console output…"}
              </p>
            )}
          </div>

          <form className="flex items-center gap-2 border-t border-[var(--line)] bg-[var(--surface)] p-2" onSubmit={submit}>
            <span aria-hidden="true" className="ml-1 shrink-0 font-mono text-xs font-bold text-[var(--brand)]">›</span>
            <label className="sr-only" htmlFor="console-command">Console command</label>
            <input
              aria-describedby="console-shortcuts"
              autoComplete="off"
              className="min-w-0 flex-1 bg-transparent font-mono text-xs text-[var(--text)] outline-none placeholder:text-[var(--text-muted)]"
              disabled={!canType}
              id="console-command"
              onChange={(event) => setCommand(event.target.value)}
              onKeyDown={historyKey}
              placeholder={canType ? "Type a command; ↑ ↓ for history, Ctrl+L clears" : runningNow === false ? "The workload is not running" : "Console is not attached"}
              ref={commandRef}
              value={command}
            />
            <button
              aria-label="Send command"
              className="rounded-md bg-[var(--brand)] px-2.5 py-1.5 text-white transition-opacity hover:bg-[var(--brand-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] disabled:opacity-40"
              disabled={!canType || !command.trim()}
              type="submit"
            >
              <Send size={13} />
            </button>
          </form>
          <p className="border-t border-[var(--line)] bg-[var(--surface)] px-3 py-1.5 font-mono text-[10px] text-[var(--text-muted)]" id="console-shortcuts">
            Ctrl/⌘+K focus command · ↑ ↓ history · Ctrl+L clear · Ctrl/⌘+Shift+F filter · Esc close filter
          </p>
        </section>

        {/* ---- telemetry rail --------------------------------------------- */}
        <aside aria-label="Runtime telemetry" className="flex min-w-0 flex-col gap-3">
          <Chart
            detail={stats ? "Sampled by the node" : noTelemetryReason}
            icon={Cpu}
            label="CPU"
            limit={100}
            value={stats ? `${stats.cpuPercent.toFixed(1)}%` : noTelemetryReason}
            values={cpuHistory}
          />
          <Chart
            detail={stats ? `${formatBytes(stats.memoryBytes)} of ${formatBytes(stats.memoryLimit)}` : noTelemetryReason}
            icon={MemoryStick}
            label="Memory"
            limit={100}
            value={memoryPercent === null ? noTelemetryReason : `${memoryPercent.toFixed(1)}%`}
            values={memoryHistory}
          />
          <Chart
            detail={stats ? `Since previous sample · RX ${formatBytes(stats.networkRxBytes)} · TX ${formatBytes(stats.networkTxBytes)}` : noTelemetryReason}
            icon={Network}
            label="Network"
            value={networkDelta === null ? "Waiting for next sample" : formatBytes(networkDelta)}
            values={networkHistory}
          />
          <section aria-label="Runtime facts" className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3">
            <h3 className="flex items-center gap-2 text-[11px] font-bold uppercase tracking-wider text-[var(--text-subtle)]">
              <Clock size={13} className="text-[var(--brand)]" />Runtime
            </h3>
            <dl className="mt-2 space-y-1.5 font-mono text-[11px]">
              <div className="flex items-baseline justify-between gap-2">
                <dt className="text-[var(--text-muted)]">Reported state</dt>
                <dd className="text-right font-semibold text-[var(--text)]">{runtimeStatusLabel ?? "Not reported"}</dd>
              </div>
              <div className="flex items-baseline justify-between gap-2">
                <dt className="text-[var(--text-muted)]">Container</dt>
                <dd className="text-right font-semibold text-[var(--text)]">{lifecycle ? (lifecycle.exists ? "present" : "absent") : "Not reported"}</dd>
              </div>
              <div className="flex items-baseline justify-between gap-2">
                <dt className="text-[var(--text-muted)]">Started</dt>
                <dd className="text-right font-semibold text-[var(--text)]">{lifecycle?.startedAt ? new Date(lifecycle.startedAt).toLocaleString() : "Not reported"}</dd>
              </div>
              <div className="flex items-baseline justify-between gap-2">
                <dt className="text-[var(--text-muted)]">Uptime</dt>
                <dd className="text-right font-semibold text-[var(--text)]">{liveUptimeMs === null ? (runningNow === false ? "—" : "Not reported") : formatUptime(liveUptimeMs)}</dd>
              </div>
              <div className="flex items-baseline justify-between gap-2">
                <dt className="text-[var(--text-muted)]">Disk</dt>
                {/* No runtime adapter reports workload disk usage yet. */}
                <dd className="text-right font-semibold text-[var(--text)]">{typeof stats?.diskBytes === "number" ? formatBytes(stats.diskBytes) : "Not reported by node"}</dd>
              </div>
              <div className="flex items-baseline justify-between gap-2">
                <dt className="text-[var(--text-muted)]">Panel record</dt>
                <dd className="text-right font-semibold text-[var(--text)]">{server.status}</dd>
              </div>
            </dl>
          </section>
        </aside>
      </div>
    </div>
  );
}
