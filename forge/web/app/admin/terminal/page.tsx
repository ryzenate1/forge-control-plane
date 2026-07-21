"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { Terminal as TerminalIcon, RefreshCw, Wifi, WifiOff } from "lucide-react";
import { API_BASE_URL } from "@/lib/api/http";
import { Card, CardHeader, SectionHeader, Btn } from "@/components/admin/admin-ui";
import { cn } from "@/lib/utils";
import "@xterm/xterm/css/xterm.css";

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

function useFit(terminal: XTerm | null, fitAddon: FitAddon | null) {
  useEffect(() => {
    if (!terminal || !fitAddon) return;
    const observer = new ResizeObserver(() => {
      try { fitAddon.fit(); } catch { /* layout not ready */ }
    });
    const el = terminal.element;
    if (el) observer.observe(el);
    return () => observer.disconnect();
  }, [terminal, fitAddon]);
}

export default function AdminTerminalPage() {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<XTerm | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectAttempt = useRef(0);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [connected, setConnected] = useState(false);
  const [nonce, setNonce] = useState(0);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!terminalRef.current || xtermRef.current) return;

    const terminal = new XTerm({
      theme: TERMINAL_THEME,
      fontFamily: '"JetBrains Mono", ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace',
      fontSize: 13,
      cursorBlink: true,
      cursorStyle: "block",
      allowTransparency: true,
      rows: 30,
      scrollback: 5000,
    });

    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon();

    terminal.loadAddon(fitAddon);
    terminal.loadAddon(webLinksAddon);

    terminal.open(terminalRef.current);

    xtermRef.current = terminal;
    fitAddonRef.current = fitAddon;

    terminal.attachCustomKeyEventHandler((e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "c") {
        const selection = terminal.getSelection();
        if (selection) navigator.clipboard.writeText(selection).catch(() => {});
        return false;
      }
      return true;
    });

    terminal.onData((data) => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(data);
      }
    });

    return () => {
      terminal.dispose();
    };
  }, []);

  useFit(xtermRef.current, fitAddonRef.current);

  useEffect(() => {
    if (!xtermRef.current) return;

    const terminal = xtermRef.current;
    setConnected(false);
    setError("");

    let aborted = false;

    const wsUrl = API_BASE_URL.replace(/^http/, "ws") + "/host/terminal/ws";
    const apiBase = API_BASE_URL;

    void (async () => {
      if (aborted) return;
      try {
        const res = await fetch(`${apiBase}/health`, { signal: AbortSignal.timeout(3000) });
        if (!res.ok && !aborted) {
          terminal.writeln("\x1b[1;33mAPI responded, connecting...\x1b[0m");
        }
      } catch {
        if (!aborted) {
          setError(`API unreachable at ${apiBase} — make sure the Go backend is running`);
          terminal.writeln(`\x1b[1;31mAPI unreachable\x1b[0m`);
          return;
        }
      }
    })();

    terminal.writeln("\x1b[1;33mConnecting to host terminal...\x1b[0m");

    const ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      if (aborted) { ws.close(); return; }
      wsRef.current = ws;
      setConnected(true);
      reconnectAttempt.current = 0;
      terminal.writeln("\x1b[1;32mConnected\x1b[0m");
      terminal.focus();
      try { fitAddonRef.current?.fit(); } catch { /* layout not ready */ }
    };

    ws.onmessage = (event) => {
      if (aborted) return;
      if (event.data instanceof Blob) {
        event.data.arrayBuffer().then((buf) => {
          terminal.write(new Uint8Array(buf));
        });
        return;
      }
      terminal.write(event.data);
    };

    ws.onerror = () => {
      if (aborted) return;
      setError(`WebSocket connection failed — ${apiBase}/host/terminal/ws not reachable`);
      terminal.writeln("\x1b[1;31mConnection failed\x1b[0m");
    };

    ws.onclose = () => {
      if (aborted) return;
      setConnected(false);
      terminal.writeln("\x1b[1;31mDisconnected\x1b[0m");
      const delay = Math.min(1000 * Math.pow(2, reconnectAttempt.current), 30000);
      reconnectAttempt.current += 1;
      reconnectTimer.current = setTimeout(() => {
        if (!aborted) setNonce((v) => v + 1);
      }, delay);
    };

    return () => {
      aborted = true;
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
      ws.close();
      if (wsRef.current === ws) wsRef.current = null;
    };
  }, [nonce]);

  const handleRetry = useCallback(() => {
    setNonce((v) => v + 1);
  }, []);

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Host Terminal"
        sub="Interactive shell on the host system"
      />
      <Card>
        <CardHeader
          title="Terminal"
          icon={TerminalIcon}
          action={
            <div className="flex items-center gap-3">
              <span
                className={cn(
                  "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[11px] font-bold uppercase tracking-wider border",
                  connected
                    ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300"
                    : "border-red-500/30 bg-red-500/10 text-red-300",
                )}
              >
                {connected ? <Wifi size={11} /> : <WifiOff size={11} />}
                {connected ? "Connected" : "Disconnected"}
              </span>
              <Btn onClick={handleRetry} size="sm" tone="subtle">
                <RefreshCw size={12} />
                Reconnect
              </Btn>
            </div>
          }
        />
        <div className="p-0">
          {error ? (
            <div className="mx-4 mt-4 rounded-lg border border-red-500/30 bg-red-500/10 p-3 text-sm text-red-200" role="alert">
              {error}
              <button className="ml-3 underline font-semibold" onClick={handleRetry} type="button">Retry</button>
            </div>
          ) : null}
          <div
            ref={terminalRef}
            className="h-[calc(100vh-20rem)] min-h-[300px] w-full bg-[#020617]"
          />
        </div>
      </Card>
    </div>
  );
}
