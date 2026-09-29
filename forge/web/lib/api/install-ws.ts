// Install streaming — admin-only WebSocket for GET /servers/:id/install/ws
// Beacon enforces ScopeAdmin (beacon/internal/server/server.go:1600 installWS) so only admin-scoped
// beacon tokens are accepted; Forge mints them via daemon.MintAdminToken (forge/api/internal/daemon/wstoken.go).
// Panel exposes two ticketed WS routes that proxy to Beacon's /servers/:id/install/ws:
//   GET /servers/:id/ws/install  (normalized) and GET /servers/:id/install/ws (legacy beacon path)
// Both use realtimeProxy with stream=install (forge/api/internal/http/realtime.go, server.go:install alias).
// When INSTALLER_WORKFLOW_ENABLED=0, Forge still surfaces workflow rows (DB→UI) but defers execution;
// the WS manager below documents the flow and remains usable once the flag is enabled.

import { buildWebSocketUrl, postJSON } from "./http";
import { WebSocketManager } from "@/lib/api/ws";

export function installWebSocketURL(serverId: string): string {
  // Normalized panel route — Forge also aliases /servers/:id/install/ws for beacon compat
  return buildWebSocketUrl(`/servers/${encodeURIComponent(serverId)}/ws/install`);
}

export async function connectInstallWebSocket(serverId: string): Promise<WebSocket> {
  // Same ticket flow as every other server stream: POST /servers/:id/ws/ticket?stream=install
  // Issues a short-lived (60s) single-use ticket bound to the requesting admin user.
  // The subsequent WS upgrade at GET /servers/:id/ws/install (or /install/ws) validates
  // the ticket against wsTicketStore (handlers_ws_ticket.go) and then mints an
  // admin-scoped beacon token via MintAdminToken before dialing Beacon's installWS.
  // Built on the canonical primitive (./http) rather than the `@/lib/api` barrel
  // so this domain module never cycles back through the barrel that aggregates it.
  const ticket = await postJSON<{ token: string }>(
    `/servers/${encodeURIComponent(serverId)}/ws/ticket?stream=install`,
  );
  return new WebSocket(`${installWebSocketURL(serverId)}?token=${encodeURIComponent(ticket.token)}`);
}

export function createInstallWSManager(serverId: string, handlers: { onLog?: (line: string) => void; onStatus?: (status: string) => void; onComplete?: (success: boolean, exitCode?: number) => void; onError?: (err: string) => void }) {
  return new WebSocketManager({
    maxRetries: 5,
    baseDelay: 1000,
    maxDelay: 10000,
    factory: () => connectInstallWebSocket(serverId),
    onMessage: (raw) => {
      // Beacon installWS emits JSON frames: {type:"status"|"log"|"complete"|"error", data, success, exitCode}
      const msg = raw as { type?: string; data?: string; success?: boolean; exitCode?: number; error?: string };
      if (!msg || typeof msg.type !== "string") return;
      if (msg.type === "log" && msg.data) handlers.onLog?.(msg.data);
      else if (msg.type === "status" && msg.data) handlers.onStatus?.(msg.data);
      else if (msg.type === "complete") handlers.onComplete?.(Boolean(msg.success), msg.exitCode);
      else if (msg.type === "error") handlers.onError?.(msg.data ?? msg.error ?? "install error");
    },
  });
}

export async function fetchInstallWSTicket(
  serverId: string,
): Promise<{ token: string }> {
  return postJSON<{ token: string }>(
    `/servers/${encodeURIComponent(serverId)}/ws/ticket?stream=install`,
  );
}
