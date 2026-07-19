"use client";

import { useEffect, useRef, useState, FormEvent } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { SearchAddon } from "@xterm/addon-search";
import { type ApiServer, connectServerWebSocket } from "@/modules/gameservers/api";
import { Send, Trash2 } from "lucide-react";
import "@xterm/xterm/css/xterm.css";

interface ServerConsoleProps {
  serverId: string;
  server: ApiServer;
}

const TERMINAL_THEME = {
  background: "#020617", // slate-950
  foreground: "#f1f5f9", // slate-100
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

export function ServerConsole({ serverId, server }: ServerConsoleProps) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectAttempt = useRef(0);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const serverNameRef = useRef(server.name);
  const [command, setCommand] = useState("");
  const [connected, setConnected] = useState(false);
  const [nonce, setNonce] = useState(0);
  const [history, setHistory] = useState<string[]>([]);
  const [historyIndex, setHistoryIndex] = useState(-1);

  // Initialize terminal
  useEffect(() => {
    if (!terminalRef.current || xtermRef.current) return;

    const terminal = new Terminal({
      theme: TERMINAL_THEME,
      fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace',
      fontSize: 13,
      cursorBlink: true,
      cursorStyle: "block",
      allowTransparency: true,
      rows: 30,
      scrollback: 1000,
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

    // Handle window resize
    const handleResize = () => {
      fitAddon.fit();
    };
    window.addEventListener("resize", handleResize);

    // Keyboard shortcuts
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
  }, []);

  useEffect(() => {
    serverNameRef.current = server.name;
  }, [server.name]);

  // WebSocket connection
  useEffect(() => {
    if (!xtermRef.current) return;

    const terminal = xtermRef.current;
    terminal.writeln("\x1b[1;33mRequesting a secure console ticket...\x1b[0m");
    setConnected(false);

    let aborted = false;
    let socket: WebSocket | null = null;
    void connectServerWebSocket(serverId, "console")
      .then((ws) => {
        if (aborted) {
          ws.close();
          return;
        }
        socket = ws;
        wsRef.current = ws;
        ws.onopen = () => {
          setConnected(true);
          reconnectAttempt.current = 0;
          terminal.writeln("\x1b[1;32mConnected to " + serverNameRef.current + "\x1b[0m");
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
          setConnected(false);
          terminal.writeln("");
          terminal.writeln("\x1b[1;31mDisconnected from server\x1b[0m");
          const delay = Math.min(1000 * Math.pow(2, reconnectAttempt.current), 30000);
          reconnectAttempt.current += 1;
          reconnectTimer.current = setTimeout(() => {
            if (!aborted) setNonce((v) => v + 1);
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
          if (!aborted) setNonce((v) => v + 1);
        }, delay);
      });

    return () => {
      aborted = true;
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
      socket?.close();
      if (wsRef.current === socket) wsRef.current = null;
    };
  }, [serverId, nonce]);

  const sendCommand = (cmd: string) => {
    if (!cmd.trim() || !connected || !wsRef.current) return;

    wsRef.current.send(cmd);
    setHistory((prev) => [cmd, ...prev].slice(0, 50));
    setHistoryIndex(-1);
    setCommand("");
  };

  const handleSubmit = (e: FormEvent) => {
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

  return (
    <div className="rounded-lg border border-white/[0.06] bg-[#1e2536] overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-white/[0.06] px-4 py-3 bg-[#161b28]">
        <h3 className="font-semibold text-slate-200">Console</h3>
        <div className="flex items-center gap-2">
          <span className={`px-2 py-1 rounded text-xs font-medium ${
            connected
              ? "bg-emerald-900/30 text-emerald-300"
              : "bg-red-900/30 text-red-300"
          }`}>
            {connected ? "Connected" : "Disconnected"}
          </span>
          <button
            onClick={clearConsole}
            className="p-2 text-slate-400 hover:text-slate-200 hover:bg-white/[0.06] rounded"
            title="Clear console"
          >
            <Trash2 size={16} />
          </button>
        </div>
      </div>

      {/* Terminal */}
      <div
        ref={terminalRef}
        className="h-[500px] bg-slate-950 p-4"
        style={{ minHeight: "500px" }}
      />

      {/* Command Input */}
      <form onSubmit={handleSubmit} className="flex gap-2 border-t border-white/[0.06] p-4 bg-[#161b28]">
        <input
          type="text"
          value={command}
          onChange={(e) => setCommand(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={!connected}
          placeholder="Type a command..."
          className="flex-1 px-3 py-2 border border-white/10 rounded-md font-mono text-sm bg-[#0f1419] text-slate-200
                     focus:outline-none focus:ring-2 focus:ring-[#dc2626] disabled:bg-white/[0.03] disabled:text-slate-500"
        />
        <button
          type="submit"
          disabled={!connected || !command.trim()}
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
