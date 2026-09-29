"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Terminal as XTerm } from "@xterm/xterm";
import type { FitAddon } from "@xterm/addon-fit";
import { Cable, CircleSlash, Radio, RefreshCw, Server, ShieldAlert, ShieldCheck, Unplug } from "lucide-react";
import { buildWebSocketUrl, checkApiReachable, getApiBaseUrl } from "@/lib/api/http";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
  AdminPageLayout,
  AdminSelect,
  Btn,
  Card,
  EmptyState,
  Pill,
} from "@/components/admin/admin-ui";
import { useNodesQuery } from "@/lib/admin/telemetry";
import { terminalTheme } from "@/lib/design-tokens";
import { errorMessage } from "@/lib/utils";
import "@xterm/xterm/css/xterm.css";

// xterm needs literal colors (not a CSS context) — shared with components/server/console
const TERMINAL_THEME = terminalTheme;

const TERMINAL_MAX_RETRIES = 15;

/**
 * The session states this page can actually observe. `open-awaiting-host` exists
 * because the panel upgrades the browser socket before it has proved it could
 * dial Beacon — claiming "Connected" at that moment reported success for work
 * the system had not done. Only `attached` (host bytes, or an explicit status
 * frame, reached us) is allowed to read as a live shell.
 */
type Session =
  | { kind: "unattached" }
  | { kind: "checking"; node: string }
  | { kind: "opening"; node: string }
  | { kind: "open-awaiting-host"; node: string }
  | { kind: "attached"; node: string }
  | { kind: "retrying"; node: string; attempt: number }
  | { kind: "closed"; node: string }
  | { kind: "failed"; node?: string; reason: string };

const SESSION_VIEW: Record<Session["kind"], { label: string; tone: "ok" | "pending" | "info" | "warn" | "danger" | "neutral" | "unknown" }> = {
  unattached: { label: "No shell", tone: "neutral" },
  checking: { label: "Checking API…", tone: "pending" },
  opening: { label: "Opening socket…", tone: "pending" },
  "open-awaiting-host": { label: "Socket open · host not confirmed", tone: "info" },
  attached: { label: "Attached", tone: "ok" },
  retrying: { label: "Reconnecting…", tone: "warn" },
  closed: { label: "Disconnected", tone: "warn" },
  failed: { label: "Failed", tone: "danger" },
};

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
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const attemptRef = useRef(0);

  const [terminalReady, setTerminalReady] = useState(false);
  const [runtimeError, setRuntimeError] = useState("");
  const [nonce, setNonce] = useState(0);
  const [nodeId, setNodeId] = useState("");
  /** The node a socket is bound to. Only an explicit Attach sets it. */
  const [attachTarget, setAttachTarget] = useState("");
  const [session, setSession] = useState<Session>({ kind: "unattached" });
  const [attempts, setAttempts] = useState(0);
  const [wrapperEl, setWrapperEl] = useState<HTMLDivElement | null>(null);

  const nodesQuery = useNodesQuery();
  const nodes = useMemo(() => nodesQuery.data ?? [], [nodesQuery.data]);
  const nodeOptions = useMemo(
    () => nodes.map((node) => ({ value: node.id, label: node.status === "active" ? node.name : `${node.name} · ${node.status}` })),
    [nodes],
  );

  // A `?nodeId=` in the URL names the machine explicitly (it is how Host Files
  // hands over), so it seeds the selector — it does not attach a shell by itself.
  useEffect(() => {
    if (typeof window === "undefined") return;
    const fromUrl = new URLSearchParams(window.location.search).get("nodeId");
    if (fromUrl) setNodeId(fromUrl);
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const url = new URL(window.location.href);
    if (nodeId) url.searchParams.set("nodeId", nodeId);
    else url.searchParams.delete("nodeId");
    window.history.replaceState(null, "", url.toString());
  }, [nodeId]);

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
      setRuntimeError("");
      setTerminalReady(true);
      // Fit after open
      try {
        fitAddon.fit();
      } catch {
        /* layout not ready */
      }

      terminal.attachCustomKeyEventHandler((event: KeyboardEvent) => {
        if ((event.ctrlKey || event.metaKey) && event.key === "c") {
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
        setRuntimeError("The terminal runtime (xterm) could not be loaded. Reload this page to try again.");
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

  const nodeName = nodes.find((node) => node.id === (attachTarget || nodeId))?.name
    ?? ((attachTarget || nodeId) ? `${(attachTarget || nodeId).slice(0, 12)}…` : "");

  // Names are read through a ref so a node-list refetch cannot re-run the socket
  // effect and drop a live shell.
  const nodeNameById = useRef<Map<string, string>>(new Map());
  useEffect(() => {
    nodeNameById.current = new Map(nodes.map((node) => [node.id, node.name]));
  }, [nodes]);

  useEffect(() => {
    // No socket is ever constructed without a named node: `/host/terminal/ws`
    // resolves the target before upgrading, so an untargeted connect is a 400
    // that used to surface here as a bogus "your session may have expired".
    if (!attachTarget || !terminalReady) return;
    const terminal = xtermRef.current;
    if (!terminal) return;

    const node = attachTarget;
    const targetName = nodeNameById.current.get(node) ?? `${node.slice(0, 12)}…`;
    let aborted = false;
    let socket: WebSocket | null = null;
    setSession({ kind: "checking", node });

    void (async () => {
      const reachable = await checkApiReachable();
      if (aborted) return;
      if (!reachable) {
        setSession({
          kind: "failed",
          node,
          reason: "The panel API is not reachable or your session has expired. Check that the API is running and that you are signed in — no shell was opened.",
        });
        terminal.writeln("\x1b[1;31mAPI unreachable — no shell opened\x1b[0m");
        return;
      }

      setSession({ kind: "opening", node });
      // /host/terminal/ws is a session-cookie-protected route with no ticket
      // flow, so it only works same-origin (cookies are not sent cross-origin).
      const wsUrl = buildWebSocketUrl("/host/terminal/ws") + `?nodeId=${encodeURIComponent(node)}`;
      const ws = new WebSocket(wsUrl);
      socket = ws;
      wsRef.current = ws;

      const markAttached = () => {
        attemptRef.current = 0;
        setAttempts(0);
        setSession((prev) => (prev.kind === "attached" ? prev : { kind: "attached", node }));
      };

      ws.onopen = () => {
        if (aborted) { ws.close(); return; }
        // The browser↔panel hop is up. Whether Beacon was dialled is not known
        // yet, so this is deliberately not called "Connected".
        setSession({ kind: "open-awaiting-host", node });
        try {
          fitAddonRef.current?.fit();
        } catch {
          /* layout not ready */
        }
        terminal.focus();
      };

      ws.onmessage = (event) => {
        if (aborted) return;
        if (typeof event.data === "string") {
          try {
            const parsed = JSON.parse(event.data) as { status?: string; error?: string; message?: string };
            if (parsed && typeof parsed === "object") {
              if (parsed.status === "connected") {
                markAttached();
                return;
              }
              if (parsed.status === "error" || parsed.status === "offline" || typeof parsed.error === "string") {
                const reason = parsed.error ?? parsed.message ?? `The panel could not open a shell on ${targetName}.`;
                terminal.writeln(`\x1b[1;31m${reason}\x1b[0m`);
                setSession({ kind: "failed", node, reason });
                return;
              }
            }
          } catch {
            // not JSON — terminal stream from the host
          }
          markAttached();
          terminal.write(event.data);
          return;
        }
        if (event.data instanceof Blob) {
          event.data.arrayBuffer().then((buf) => {
            if (aborted) return;
            markAttached();
            terminal.write(new Uint8Array(buf));
          }).catch((err) => console.error("[Terminal] arrayBuffer error:", err));
          return;
        }
        markAttached();
        terminal.write(event.data);
      };

      ws.onerror = () => {
        if (aborted) return;
        setSession({
          kind: "failed",
          node,
          reason: `No shell on ${targetName}: the WebSocket to ${getApiBaseUrl()}/host/terminal/ws failed. The node may be offline, or the panel may lack a credential for it.`,
        });
        terminal.writeln("\x1b[1;31mShell socket failed\x1b[0m");
      };

      ws.onclose = () => {
        if (aborted) return;
        if (wsRef.current === ws) wsRef.current = null;
        const next = attemptRef.current + 1;
        attemptRef.current = next;
        setAttempts(next);
        if (next > TERMINAL_MAX_RETRIES) {
          setSession({
            kind: "failed",
            node,
            reason: `The shell on ${targetName} dropped and was not re-established after ${TERMINAL_MAX_RETRIES} attempts. Use Reconnect to start a new one.`,
          });
          terminal.writeln("\x1b[1;31mAuto-reconnect exhausted; press Reconnect\x1b[0m");
          return;
        }
        setSession({ kind: "retrying", node, attempt: next });
        reconnectTimer.current = setTimeout(() => {
          if (!aborted) setNonce((value) => value + 1);
        }, Math.min(1000 * 2 ** (next - 1), 30_000));
      };
    })();

    return () => {
      aborted = true;
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
      socket?.close();
      if (wsRef.current === socket) wsRef.current = null;
    };
    // The node names used in messages come from `nodeNameById`, deliberately not
    // a dependency: re-running this effect would tear down a live shell.
  }, [nonce, attachTarget, terminalReady]);

  const attach = useCallback(() => {
    if (!nodeId) return;
    attemptRef.current = 0;
    setAttempts(0);
    setAttachTarget(nodeId);
    setNonce((value) => value + 1);
  }, [nodeId]);

  const detach = useCallback(() => {
    setAttachTarget("");
    attemptRef.current = 0;
    setAttempts(0);
    setSession({ kind: "unattached" });
  }, []);

  const handleNodeChange = useCallback((next: string) => {
    // A shell is node-local: changing the target detaches rather than silently
    // re-pointing a live root session at another machine.
    if (next !== attachTarget) {
      setAttachTarget("");
      attemptRef.current = 0;
      setAttempts(0);
      setSession({ kind: "unattached" });
    }
    setNodeId(next);
  }, [attachTarget]);

  const view = SESSION_VIEW[session.kind];

  return (
    <AdminPageLayout>
      <AdminPageHeader
        info={{
          description: "Opens an interactive shell on one named Beacon node over a session-authenticated WebSocket. Output streams as terminal bytes; structured status frames are consumed, never rendered.",
          eyebrow: "Architecture & Semantics",
          sections: [
            {
              content:
                "Nothing connects until you pick a node and press Attach, and changing the node detaches the shell. The route rejects a request that does not name a Beacon, so an untargeted socket would only ever fail.",
              icon: Server,
              title: "Explicit target",
            },
            {
              content:
                "The browser↔panel socket and the host session are two different things. The badge stays amber until bytes arrive from the host; only then does it read Attached. A failure frame from the panel is shown as the reason, never as a green light.",
              icon: Radio,
              title: "What the badge means",
            },
            {
              content:
                "The route requires a signed-in admin session on the same origin — cookies do not cross origins. Treat every keystroke as a privileged host action: this shell runs with whatever rights the Beacon daemon has, typically root.",
              icon: ShieldCheck,
              title: "Access and audit",
            },
          ],
          title: "Host terminal",
          triggerLabel: "About Host Terminal",
        }}
        status={<Pill tone={view.tone}>{view.label}</Pill>}
      />

      <Card>
        <div className="flex flex-wrap items-end justify-between gap-3 border-b border-line pb-4">
          <div className="min-w-56">
            <AdminSelect
              disabled={nodesQuery.isPending || nodeOptions.length === 0}
              label="Target node"
              onChange={handleNodeChange}
              options={nodeOptions}
              placeholder={nodesQuery.isPending ? "Loading nodes…" : nodeOptions.length === 0 ? "No nodes registered" : "Select a node…"}
              value={nodeId}
            />
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {attachTarget ? (
              <>
                <Btn disabled={session.kind === "checking" || session.kind === "opening"} onClick={() => { attemptRef.current = 0; setAttempts(0); setNonce((value) => value + 1); }} size="sm" tone="subtle">
                  <RefreshCw aria-hidden="true" size={14} /> Reconnect
                </Btn>
                <Btn onClick={detach} size="sm" tone="ghost">
                  <Unplug aria-hidden="true" size={14} /> Detach
                </Btn>
              </>
            ) : (
              <Btn
                disabled={!nodeId || nodesQuery.isPending}
                onClick={attach}
                size="sm"
                tone="primary"
                title={nodeId ? `Open a privileged shell on ${nodeName}` : "Select a node first"}
              >
                <Cable aria-hidden="true" size={14} /> Attach shell
              </Btn>
            )}
          </div>
        </div>

        <div className="space-y-3 pt-4">
          {nodesQuery.isPending ? (
            <AdminLoadingState label="Loading nodes…" />
          ) : nodesQuery.isError ? (
            <AdminErrorState message={errorMessage(nodesQuery.error, "The node list could not be loaded.")} retry={() => void nodesQuery.refetch()} />
          ) : nodesQuery.isSuccess && nodes.length === 0 ? (
            <EmptyState
              icon={CircleSlash}
              message="No Beacon node is registered, so there is no host to open a shell on. Add one under Infrastructure → Nodes."
              title="No nodes available"
            />
          ) : runtimeError ? (
            <AdminErrorState message={runtimeError} />
          ) : !terminalReady ? (
            <AdminLoadingState label="Loading terminal runtime…" />
          ) : !attachTarget ? (
            <EmptyState
              icon={Server}
              message="Pick a node and press Attach. A shell runs with the Beacon daemon's privileges on that one machine, so nothing connects until you name it."
              title="No shell attached"
            />
          ) : null}

          {session.kind === "failed" ? (
            <AdminErrorState
              message={session.reason}
              retry={() => { attemptRef.current = 0; setAttempts(0); setNonce((value) => value + 1); }}
            />
          ) : null}

          {attachTarget ? (
            <p className="flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-meta text-text-subtle">
              <ShieldAlert aria-hidden="true" className="text-warn" size={13} />
              <span>Shell target: {nodeName}</span>
              <span>· privileged host actions</span>
              <span>· reconnect attempt {attempts}/{TERMINAL_MAX_RETRIES}</span>
            </p>
          ) : null}

          <div
            aria-label={`Host terminal session${attachTarget ? ` on ${nodeName}` : ""}`}
            className="h-[60vh] min-h-[320px] w-full overflow-hidden rounded-lg border border-line bg-[var(--canvas)]"
            ref={terminalRef}
            role="region"
          />
        </div>
      </Card>
    </AdminPageLayout>
  );
}
