"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { SearchAddon } from "@xterm/addon-search";
import { Copy, Minus, Plus, RotateCw, Send, Trash2, type LucideIcon } from "lucide-react";
import { connectServerWebSocket } from "@/lib/api";
import "@xterm/xterm/css/xterm.css";

interface EnhancedServerConsoleProps {
  serverId: string;
  serverName: string;
}

type ConnectionStatus = "connecting" | "connected" | "disconnected" | "reconnecting";

const STATUS_CONFIG: Record<ConnectionStatus, { label: string; color: string; icon: LucideIcon | null }> = {
  connecting: { label: "Connecting", color: "bg-yellow-900/30 text-yellow-300", icon: null },
  connected: { label: "Connected", color: "bg-emerald-900/30 text-emerald-300", icon: null },
  disconnected: { label: "Disconnected", color: "bg-red-900/30 text-red-300", icon: null },
  reconnecting: { label: "Reconnecting", color: "bg-amber-900/30 text-amber-300", icon: null },
};

const TERMINAL_THEME = {
  background: "#020617",
  foreground: "#f1f5f9",
  cursor: "#94a3b8",
  black: "#0f172a",
  red: "#ef4444",
  green: "#22c55e",
  yellow: "#eab308",
  blue: "#3b82f6",
  magenta: "#a855f7",
  cyan: "#06b6d4",
  white: "#cbd5e1",
  brightBlack: "#475569",
  brightRed: "#f87171",
  brightGreen: "#4ade80",
  brightYellow: "#facc15",
  brightBlue: "#60a5fa",
  brightMagenta: "#c084fc",
  brightCyan: "#22d3ee",
  brightWhite: "#f8fafc",
};

const FONT_SIZES = [11, 12, 13, 14, 15, 16, 18, 20];

export function EnhancedServerConsole({ serverId, serverName }: EnhancedServerConsoleProps) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectAttempt = useRef(0);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [command, setCommand] = useState("");
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>("disconnected");
  const [nonce, setNonce] = useState(0);
  const [history, setHistory] = useState<string[]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);
  const [fontSize, setFontSize] = useState(13);

  const status = STATUS_CONFIG[connectionStatus];

  useEffect(() => {
    if (!terminalRef.current || xtermRef.current) return;

    const terminal = new Terminal({
      theme: TERMINAL_THEME,
      fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace',
      fontSize,
      cursorBlink: true,
      cursorStyle: "block",
      allowTransparency: true,
      rows: 30,
      scrollback: 5000,
    });

    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon();
    const searchAddon = new SearchAddon();

    terminal.loadAddon(fitAddon);
    terminal.loadAddon(webLinksAddon);
    terminal.loadAddon(searchAddon);

    terminal.open(terminalRef.current);
    fitAddon.fit();

    xtermRef.current = terminal;
    fitAddonRef.current = fitAddon;

    const handleResize = () => fitAddon.fit();
    window.addEventListener("resize", handleResize);

    terminal.attachCustomKeyEventHandler((e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "c") {
        const selection = terminal.getSelection();
        if (selection) navigator.clipboard.writeText(selection).catch(() => {});
        return false;
      }
      return true;
    });

    return () => {
      window.removeEventListener("resize", handleResize);
      terminal.dispose();
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (xtermRef.current) {
      (xtermRef.current.options as { fontSize: number }).fontSize = fontSize;
    }
    fitAddonRef.current?.fit();
  }, [fontSize]);

  useEffect(() => {
    if (!xtermRef.current) return;

    const terminal = xtermRef.current;
    setConnectionStatus("connecting");
    terminal.writeln("\x1b[1;33mRequesting secure console ticket...\x1b[0m");

    let aborted = false;
    let socket: WebSocket | null = null;

    void connectServerWebSocket(serverId, "console")
      .then((ws) => {
        if (aborted) { ws.close(); return; }
        socket = ws;
        wsRef.current = ws;

        ws.onopen = () => {
          setConnectionStatus("connected");
          reconnectAttempt.current = 0;
          terminal.writeln(`\x1b[1;32mConnected to ${serverName}\x1b[0m`);
          terminal.writeln("");
        };

        ws.onmessage = (event) => {
          terminal.write(event.data);
        };

        ws.onerror = () => {
          if (aborted) return;
          terminal.writeln("\x1b[1;31mConsole WebSocket connection failed.\x1b[0m");
          ws.close();
        };

        ws.onclose = () => {
          if (aborted) return;
          setConnectionStatus("reconnecting");
          terminal.writeln("");
          terminal.writeln("\x1b[1;31mDisconnected from server\x1b[0m");

          const delay = Math.min(1000 * Math.pow(2, reconnectAttempt.current), 30000);
          reconnectAttempt.current += 1;
          reconnectTimer.current = setTimeout(() => {
            if (!aborted) {
              setConnectionStatus("connecting");
              setNonce((v) => v + 1);
            }
          }, delay);
        };
      })
      .catch((error) => {
        if (aborted) return;
        const message = error instanceof Error ? error.message : "Unable to authorize the console connection.";
        terminal.writeln(`\x1b[1;31m${message}\x1b[0m`);

        const delay = Math.min(1000 * Math.pow(2, reconnectAttempt.current), 30000);
        reconnectAttempt.current += 1;
        reconnectTimer.current = setTimeout(() => {
          if (!aborted) {
            setConnectionStatus("connecting");
            setNonce((v) => v + 1);
          }
        }, delay);
      });

    return () => {
      aborted = true;
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
      socket?.close();
      if (wsRef.current === socket) wsRef.current = null;
    };
  }, [serverId, serverName, nonce]);

  const sendCommand = useCallback((cmd: string) => {
    if (!cmd.trim() || connectionStatus !== "connected" || !wsRef.current) return;
    wsRef.current.send(cmd);
    setHistory((prev) => [cmd, ...prev].slice(0, 50));
    setHistoryIndex(-1);
    setCommand("");
  }, [connectionStatus]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    sendCommand(command);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "ArrowUp") {
      e.preventDefault();
      const newIndex = Math.min(historyIndex + 1, history.length - 1);
      setHistoryIndex(newIndex);
      setCommand(history[newIndex] || "");
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      const newIndex = Math.max(historyIndex - 1, -1);
      setHistoryIndex(newIndex);
      setCommand(history[newIndex] || "");
    }
  };

  const clearConsole = () => {
    xtermRef.current?.clear();
  };

  const handleReconnect = () => {
    if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
    reconnectAttempt.current = 0;
    setConnectionStatus("connecting");
    setNonce((v) => v + 1);
  };

  const handleCopyAll = async () => {
    const content = xtermRef.current?.buffer.active.getLine(0)?.translateToString() ?? "";
    if (content) {
      try {
        await navigator.clipboard.writeText(content);
      } catch { /* ignore */ }
    }
  };

  return (
    <div className="rounded-lg border border-white/[0.06] bg-[#1e2536] overflow-hidden">
      <div className="flex items-center justify-between border-b border-white/[0.06] px-4 py-2.5 bg-[#161b28]">
        <div className="flex items-center gap-2">
          <h3 className="text-sm font-semibold text-slate-200">Console</h3>
          <span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider ${status.color}`}>
            <span className={`h-1.5 w-1.5 rounded-full bg-current ${connectionStatus === "connecting" || connectionStatus === "reconnecting" ? "animate-pulse" : ""}`} />
            {status.label}
          </span>
        </div>
        <div className="flex items-center gap-0.5">
          <div className="flex items-center mr-1 gap-0.5 border-r border-white/[0.06] pr-2">
            <button
              className="p-1 text-slate-500 hover:text-slate-200 rounded disabled:opacity-30"
              disabled={fontSize <= FONT_SIZES[0]}
              onClick={() => setFontSize((s) => Math.max(FONT_SIZES[0], s - 1))}
              title="Decrease font size"
              type="button"
            >
              <Minus size={13} />
            </button>
            <span className="text-[10px] text-slate-500 min-w-[1.5rem] text-center">{fontSize}</span>
            <button
              className="p-1 text-slate-500 hover:text-slate-200 rounded disabled:opacity-30"
              disabled={fontSize >= FONT_SIZES[FONT_SIZES.length - 1]}
              onClick={() => setFontSize((s) => Math.min(FONT_SIZES[FONT_SIZES.length - 1], s + 1))}
              title="Increase font size"
              type="button"
            >
              <Plus size={13} />
            </button>
          </div>
          <button
            className="p-1.5 text-slate-500 hover:text-slate-200 rounded"
            onClick={handleCopyAll}
            title="Copy terminal contents"
            type="button"
          >
            <Copy size={13} />
          </button>
          <button
            className="p-1.5 text-slate-500 hover:text-slate-200 rounded"
            onClick={handleReconnect}
            title="Reconnect"
            type="button"
          >
            <RotateCw size={13} />
          </button>
          <button
            className="p-1.5 text-slate-500 hover:text-slate-200 rounded"
            onClick={clearConsole}
            title="Clear terminal"
            type="button"
          >
            <Trash2 size={13} />
          </button>
        </div>
      </div>

      <div ref={terminalRef} className="h-[500px] bg-slate-950" style={{ minHeight: "500px" }} />

      <form onSubmit={handleSubmit} className="flex gap-2 border-t border-white/[0.06] p-3 bg-[#161b28]">
        <input
          type="text"
          value={command}
          onChange={(e) => setCommand(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={connectionStatus !== "connected"}
          placeholder={connectionStatus === "connected" ? "Type a command..." : "Waiting for connection..."}
          className="flex-1 px-3 py-2 border border-white/10 rounded-md font-mono text-sm bg-[#0f1419] text-slate-200
                     focus:outline-none focus:ring-2 focus:ring-[#dc2626] disabled:bg-white/[0.03] disabled:text-slate-500"
        />
        <button
          type="submit"
          disabled={connectionStatus !== "connected" || !command.trim()}
          className="px-4 py-2 bg-[#dc2626] text-white rounded-md font-medium
                     hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed
                     flex items-center gap-2"
        >
          <Send size={16} />
          Send
        </button>
      </form>
    </div>
  );
}
