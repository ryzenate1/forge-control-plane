"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Terminal as XTerm } from "@xterm/xterm";
import type { FitAddon } from "@xterm/addon-fit";
import { Terminal as TerminalIcon, RefreshCw, Wifi, WifiOff } from "lucide-react";
import { getApiBaseUrl, buildWebSocketUrl, checkApiReachable } from "@/lib/api/http";
import { Card, CardHeader, Btn, AdminToolbar, AdminLoadingState, AdminPageLayout, SectionHeader } from "@/components/admin/admin-ui";
import { NodeSelect } from "@/components/admin/node-select";
import { cn } from "@/lib/utils";
import { terminalTheme } from "@/lib/design-tokens";
import "@xterm/xterm/css/xterm.css";

// xterm needs literal colors (not a CSS context) — shared with components/server/console
const TERMINAL_THEME = terminalTheme;

const TERMINAL_MAX_RETRIES = 15;

function useFit(terminal: XTerm | null, fitAddon: FitAddon | null, wrapper: HTMLDivElement | null) {
  useEffect(() => {
    if (!terminal || !fitAddon || !wrapper) return;
    const fit = () => {
      try {
        fitAddon.fit();
      } catch {
        /* layout not ready */
      }
    };
    // Initial fit after paint
    const raf = requestAnimationFrame(fit);
    const observer = new ResizeObserver(fit);
    observer.observe(wrapper);
    // Also refit on window resize (container queries)
    window.addEventListener("resize", fit);
    return () => {
      cancelAnimationFrame(raf);
      observer.disconnect();
      window.removeEventListener("resize", fit);
    };
  }, [terminal, fitAddon, wrapper]);
}

export default function AdminTerminalPage() {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<XTerm | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectAttempt = useRef(0);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const [terminalReady, setTerminalReady] = useState(false);
  const [connected, setConnected] = useState(false);
  const [nonce, setNonce] = useState(0);
  const [error, setError] = useState("");
  const [nodeId, setNodeId] = useState("");
  const [wrapperEl, setWrapperEl] = useState<HTMLDivElement | null>(null);

  // Hydrate initial nodeId from ?nodeId= query (e.g. host-files-view Terminal button)
  useEffect(() => {
    if (typeof window === "undefined") return;
    const q = new URLSearchParams(window.location.search).get("nodeId");
    if (q) setNodeId(q);
  }, []);

  useEffect(() => {
    setWrapperEl(terminalRef.current);
  }, [terminalReady]);

  useEffect(() => {
    if (!terminalRef.current || xtermRef.current) return;

    let disposed = false;
    void Promise.all([
      import("@xterm/xterm"),
      import("@xterm/addon-fit"),
      import("@xterm/addon-web-links"),
    ]).then(([xtermModule, fitModule, linksModule]) => {
      if (disposed || !terminalRef.current) return;

      const terminal = new xtermModule.Terminal({
        theme: TERMINAL_THEME,
        fontFamily: "var(--font-mono), ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
        fontSize: 13,
        cursorBlink: true,
        cursorStyle: "block",
        allowTransparency: true,
        rows: 30,
        scrollback: 5000,
      });
      const fitAddon = new fitModule.FitAddon();

      terminal.loadAddon(fitAddon);
      terminal.loadAddon(new linksModule.WebLinksAddon());
      terminal.open(terminalRef.current);

      xtermRef.current = terminal;
      fitAddonRef.current = fitAddon;
      setTerminalReady(true);
      // Fit after open
      try {
        fitAddon.fit();
      } catch {
        /* layout not ready */
      }

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
    }).catch(() => {
      if (!disposed) {
        setError("Terminal runtime failed to load");
      }
    });

    return () => {
      disposed = true;
      xtermRef.current?.dispose();
      xtermRef.current = null;
      fitAddonRef.current = null;
      setTerminalReady(false);
    };
  }, []);

  useFit(xtermRef.current, fitAddonRef.current, wrapperEl);

  useEffect(() => {
    if (!xtermRef.current) return;

    const terminal = xtermRef.current;
    setConnected(false);
    setError("");

    let aborted = false;

    // /host/terminal/ws is a session-cookie-protected route with no ticket
    // flow, so it only works same-origin (cookies are not sent cross-origin).
    const wsUrl = buildWebSocketUrl("/host/terminal/ws")
      + (nodeId ? `?nodeId=${encodeURIComponent(nodeId)}` : "");

    void (async () => {
      if (aborted) return;
      const reachable = await checkApiReachable();
      if (!reachable && !aborted) {
        setError("API unreachable or session expired — make sure the Go backend is running and you are signed in");
        terminal.writeln("\x1b[1;31mAPI unreachable\x1b[0m");
        return;
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
      // Status JSON: ignore structured ping, handle blob/text
      if (typeof event.data === "string") {
        // Try parse status JSON — e.g. {"status":"connected"} — don't render as terminal noise
        try {
          const parsed = JSON.parse(event.data);
          if (parsed && typeof parsed === "object" && "status" in parsed) {
            const s = (parsed as { status?: string }).status;
            if (s === "connected") {
              // already handled via onopen, no need to write
              return;
            }
            if (s === "error" || s === "offline") {
              const msg = (parsed as { error?: string; message?: string }).error ?? (parsed as { message?: string }).message ?? "host error";
              terminal.writeln(`\x1b[1;31m${msg}\x1b[0m`);
              setError(String(msg));
              return;
            }
          }
        } catch {
          // not JSON, treat as terminal stream
        }
        terminal.write(event.data);
        return;
      }
      if (event.data instanceof Blob) {
        event.data.arrayBuffer().then((buf) => {
          if (aborted) return;
          terminal.write(new Uint8Array(buf));
        }).catch((err) => console.error("[Terminal] arrayBuffer error:", err));
        return;
      }
      terminal.write(event.data);
    };

    ws.onerror = () => {
      if (aborted) return;
      setError(`WebSocket connection failed — ${getApiBaseUrl()}/host/terminal/ws not reachable. If the API is up, your session may have expired — sign in again.`);
      terminal.writeln("\x1b[1;31mConnection failed\x1b[0m");
    };

    ws.onclose = () => {
      if (aborted) return;
      setConnected(false);
      terminal.writeln("\x1b[1;31mDisconnected\x1b[0m");
      if (reconnectAttempt.current >= TERMINAL_MAX_RETRIES) {
        setError(`Connection dropped after ${TERMINAL_MAX_RETRIES} retries — use Reconnect to try again`);
        terminal.writeln("\x1b[1;31mAuto-reconnect exhausted; press Reconnect to retry\x1b[0m");
        return;
      }
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
  }, [nonce, nodeId, terminalReady]);

  const handleRetry = useCallback(() => {
    reconnectAttempt.current = 0;
    setError("");
    setNonce((v) => v + 1);
  }, []);

  // Switching nodes resets the reconnect backoff and re-runs the WS effect.
  const handleNodeChange = useCallback((next: string) => {
    reconnectAttempt.current = 0;
    setNodeId(next);
  }, []);

  const statusJson = useMemo(() => {
    return JSON.stringify(
      {
        connected,
        nodeId: nodeId || null,
        apiBase: getApiBaseUrl(),
        retries: reconnectAttempt.current,
        maxRetries: TERMINAL_MAX_RETRIES,
        ready: terminalReady,
      },
      null,
      2,
    );
  }, [connected, nodeId, terminalReady]);

  return (
    <AdminPageLayout>
      <SectionHeader title="Host Terminal" sub="Secure host shell and console access" />
      <Card>
        <CardHeader
          title="Terminal"
          icon={TerminalIcon}
          action={
            <AdminToolbar className="border-0 bg-transparent p-0">
              <NodeSelect value={nodeId} onChange={handleNodeChange} />
              <span
                className={cn(
                  "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[11px] font-bold uppercase tracking-wider border",
                  connected
                    ? "border-ok-line bg-ok-subtle text-ok"
                    : "border-danger-line bg-danger-subtle text-danger",
                )}
              >
                {connected ? <Wifi size={11} /> : <WifiOff size={11} />}
                {connected ? "Connected" : "Disconnected"}
              </span>
              <Btn onClick={handleRetry} size="sm" tone="subtle">
                <RefreshCw size={12} />
                Reconnect
              </Btn>
            </AdminToolbar>
          }
        />
        <div className="p-0">
          {error ? (
            <div className="mx-4 mt-4 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger" role="alert">
              <span className="flex-1">{error}</span>
              <button
                className="inline-flex h-8 items-center justify-center rounded-lg bg-[var(--brand)] px-3 text-xs font-bold text-text hover:bg-[var(--brand-hover)] disabled:opacity-40 transition-colors"
                onClick={handleRetry}
                type="button"
              >
                Retry
              </button>
            </div>
          ) : null}
          {!terminalReady && !error ? (
            <div className="mx-4 mt-4">
              <AdminLoadingState label="Initializing terminal..." />
            </div>
          ) : null}
          <div
            ref={terminalRef}
            className={cn("h-[calc(100vh-20rem)] min-h-[300px] w-full bg-[var(--canvas)] /* intentional terminal chrome, not surface */", !terminalReady && "hidden")}
          />
          {/* Status JSON — tokenized */}
          <div className="mx-4 mb-4 mt-3 rounded-lg border border-[var(--line)] bg-[var(--surface-input)] p-3">
            <div className="mb-1 flex items-center justify-between">
              <span className="text-[11px] font-bold uppercase tracking-widest text-[var(--text-subtle)]">Terminal status</span>
              <span className={cn("inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider border", connected ? "border-ok-line bg-ok-subtle text-ok" : "border-warn-line bg-warn-subtle text-warn")}>
                {connected ? "live" : "idle"} · {reconnectAttempt.current}/{TERMINAL_MAX_RETRIES}
              </span>
            </div>
            <pre className="overflow-auto rounded bg-well p-2 font-mono text-[11px] leading-5 text-[var(--text-subtle)]">{statusJson}</pre>
            <p className="mt-1.5 text-xs text-[var(--text-subtle)]">FitAddon auto-fits on resize · {connected ? "WebSocket live" : "disconnected"} · use Retry to reset backoff.</p>
          </div>
        </div>
      </Card>
    </AdminPageLayout>
  );
}
