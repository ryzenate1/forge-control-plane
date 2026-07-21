"use client";

import { useEffect, useRef, useState } from "react";
import { Pause, Play, Search, Terminal, X } from "lucide-react";
import { cn } from "@/lib/utils";

interface LogEntry {
  timestamp: string;
  message: string;
  level?: "info" | "warn" | "error" | "debug";
}

interface DeploymentLogViewerProps {
  deploymentId: string;
  wsUrl: string | (() => Promise<string>);
  initialLogs?: LogEntry[];
}

const ANSI_PATTERN = /[\u001b\u009b][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]/g;

function stripAnsi(str: string): string {
  return str.replace(ANSI_PATTERN, "");
}

function getLevelColor(level?: string): string {
  switch (level) {
    case "error": return "text-red-400";
    case "warn": return "text-amber-400";
    case "info": return "text-blue-400";
    case "debug": return "text-slate-500";
    default: return "text-slate-300";
  }
}

export function DeploymentLogViewer({ deploymentId, wsUrl, initialLogs }: DeploymentLogViewerProps) {
  const [logs, setLogs] = useState<LogEntry[]>(initialLogs ?? []);
  const [autoScroll, setAutoScroll] = useState(true);
  const [searchQuery, setSearchQuery] = useState("");
  const [connected, setConnected] = useState(false);
  const [showSearch, setShowSearch] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const reconnectAttempt = useRef(0);
  const [nonce, setNonce] = useState(0);

  const filteredLogs = searchQuery
    ? logs.filter((log) =>
        stripAnsi(log.message).toLowerCase().includes(searchQuery.toLowerCase()),
      )
    : logs;

  useEffect(() => {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [filteredLogs, autoScroll]);

  useEffect(() => {
    let aborted = false;
    let socket: WebSocket | null = null;

    async function connect() {
      try {
        const url = typeof wsUrl === "function" ? await wsUrl() : wsUrl;
        socket = new WebSocket(url);
        wsRef.current = socket;

        socket.onopen = () => {
          if (aborted) { socket?.close(); return; }
          setConnected(true);
          reconnectAttempt.current = 0;
        };

        socket.onmessage = (event) => {
          if (aborted) return;
          try {
            const entry = JSON.parse(event.data) as LogEntry;
            setLogs((prev) => [...prev.slice(-999), entry]);
          } catch {
            setLogs((prev) => [
              ...prev.slice(-999),
              { timestamp: new Date().toISOString(), message: event.data },
            ]);
          }
        };

        socket.onclose = () => {
          if (aborted) return;
          setConnected(false);
          const delay = Math.min(1000 * Math.pow(2, reconnectAttempt.current), 30000);
          reconnectAttempt.current += 1;
          reconnectTimer.current = setTimeout(() => {
            if (!aborted) setNonce((v) => v + 1);
          }, delay);
        };

        socket.onerror = () => {
          socket?.close();
        };
      } catch {
        const delay = Math.min(1000 * Math.pow(2, reconnectAttempt.current), 30000);
        reconnectAttempt.current += 1;
        reconnectTimer.current = setTimeout(() => {
          if (!aborted) setNonce((v) => v + 1);
        }, delay);
      }
    }

    void connect();

    return () => {
      aborted = true;
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
      socket?.close();
      if (wsRef.current === socket) wsRef.current = null;
    };
  }, [deploymentId, wsUrl, nonce]);

  return (
    <div className="rounded-lg border border-white/[0.06] bg-[#0f1419] overflow-hidden">
      <div className="flex items-center justify-between border-b border-white/[0.06] bg-[#161b28] px-4 py-2">
        <div className="flex items-center gap-2">
          <Terminal size={14} className="text-slate-400" />
          <span className="text-sm font-semibold text-slate-200">Deployment Logs</span>
          <span className={cn(
            "ml-2 inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-bold uppercase",
            connected
              ? "bg-emerald-900/30 text-emerald-300"
              : "bg-red-900/30 text-red-300",
          )}>
            {connected ? "Live" : "Disconnected"}
          </span>
          <span className="text-xs text-slate-500">{logs.length} lines</span>
        </div>
        <div className="flex items-center gap-1">
          <button
            className="p-1.5 text-slate-500 hover:text-slate-200 rounded"
            onClick={() => setShowSearch(!showSearch)}
            type="button"
            title="Search logs"
          >
            <Search size={14} />
          </button>
          <button
            className={cn("p-1.5 rounded", autoScroll ? "text-blue-400" : "text-slate-500 hover:text-slate-200")}
            onClick={() => setAutoScroll(!autoScroll)}
            type="button"
            title={autoScroll ? "Pause auto-scroll" : "Resume auto-scroll"}
          >
            {autoScroll ? <Pause size={14} /> : <Play size={14} />}
          </button>
          <button
            className="p-1.5 text-slate-500 hover:text-red-400 rounded"
            onClick={() => setLogs([])}
            type="button"
            title="Clear logs"
          >
            <X size={14} />
          </button>
        </div>
      </div>

      {showSearch && (
        <div className="border-b border-white/[0.06] px-4 py-2">
          <div className="relative">
            <Search size={12} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-500" />
            <input
              className="w-full rounded border border-white/10 bg-[#0d131d] py-1.5 pl-7 pr-8 text-xs text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-red-500/50"
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search logs..."
              type="text"
              value={searchQuery}
            />
            {searchQuery && (
              <button
                className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-500 hover:text-slate-300"
                onClick={() => setSearchQuery("")}
                type="button"
              >
                <X size={12} />
              </button>
            )}
          </div>
        </div>
      )}

      <div
        ref={containerRef}
        className="h-96 overflow-y-auto font-mono text-xs leading-relaxed"
      >
        {filteredLogs.length === 0 ? (
          <div className="flex h-full items-center justify-center text-slate-500">
            {searchQuery ? "No matching logs found" : "Waiting for logs..."}
          </div>
        ) : (
          <div className="p-3 space-y-0">
            {filteredLogs.map((log, index) => (
              <div key={index} className="flex gap-3 hover:bg-white/[0.02] px-1 py-0.5 rounded">
                <span className="shrink-0 text-slate-600 select-none w-8 text-right">
                  {index + 1}
                </span>
                <span className="shrink-0 text-slate-600 w-20">
                  {new Date(log.timestamp).toLocaleTimeString()}
                </span>
                <span className={cn("shrink-0 w-10 uppercase text-[10px]", getLevelColor(log.level))}>
                  {log.level ?? "log"}
                </span>
                <span className="text-slate-300 whitespace-pre-wrap break-all">
                  {stripAnsi(log.message)}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
