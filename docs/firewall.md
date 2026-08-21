# Firewall — Beacon (iptables) — CURRENT

> **Scope:** `beacon/internal/server/handlers_firewall.go` + `forge/web/components/admin/AdminFirewall.tsx` + `forge/api/internal/http/handlers_firewall.go` (proxy). **Status:** allow-only iptables backend, public-unicast source guard, atomic restore, persisted state.

## Model

- Backend: Linux `iptables` only (`runtime.GOOS != "linux"` → 503). Two Forge chains: `FORGE-BEACON` (filter/INPUT) and `FORGE-BEACON-FWD` (nat/PREROUTING). Hooks are idempotently inserted/removed on enable/disable (`ensureHook`/`removeHook`) and the rule set is bulk-applied via `iptables-restore --wait 10 --noflush` in `reconcile()` — atomic.
- State lives in `DAEMON_DATA_DIR/.beacon/firewall.json` (`firewallData.persistLocked()` atomic `CreateTemp+Rename+syncDirectory`). On restart `load()` validates persisted rows and `reconcile()` re-applies them.
- Every rule is tagged `-m comment --comment forge:<id>` so the Forge chain owns only its own rules.

## Validation (admission)

| Field | Rule | Error example | Notes |
|---|---|---|---|
| `port` | 1–65535 | `invalid port` | Inclusive |
| `protocol` | `tcp`\|`udp` (case-insensitive). `tcp/udp` rejected | `combined protocol tcp/udp is not supported; use separate rules` | |
| `action` | allow-only. Accepted: `allow`, `accept`, `open`, `` (defaults to `allow`). `deny`/`drop`/`reject`/`block` rejected | `unsupported action "deny": firewall is allow-only (omit the rule to deny; only allow/accept/open is supported)` | **Dead deny option fix:** the former UI “DENY” always returned 400 via `validateFirewallAction:132`. UI now exposes only `ALLOW` and docs clarify allow-only semantics. To deny traffic, remove/omit the rule (default deny); the chain is only hooked when `enabled`, and absence of a rule means deny. |
| `sourceIp` | Required, public unicast only. `net.ParseIP` or canonical CIDR. Rejects: empty, `0.0.0.0/0`, `0.0.0.0`, any private (`10/8`, `172.16/12`, `192.168/16`), loopback, link-local, multicast/unspecified, CGNAT `100.64/10` is via `IsGlobalUnicast`+`IsPrivate` guard, non-canonical CIDR (`192.0.2.1/24` where ip != network.IP), or `ones==0`. | `source CIDR "0.0.0.0/0" is unrestricted, non-canonical, or non-public` | **Placeholder fix:** previous UI placeholder `e.g. 0.0.0.0/0` guaranteed a 400. Now placeholders are `e.g. 203.0.113.42` or `e.g. 198.51.100.0/24`; empty field is also rejected. Valid examples: `198.51.100.0/24` (canonical public CIDR) or `203.0.113.42` (single public IP). |
| `toIp` (forwards) | Any `net.ParseIP` valid | `invalid forward IP address` | Not restricted to public — forwards can target internal address. |

Canonicalization: IPs via `ip.String()`, CIDRs via `network.String()` (`canonicalFirewallSource`).

## Endpoints (Beacon)

```
GET    /host/firewall/status
POST   /host/firewall/enable
POST   /host/firewall/disable
GET    /host/firewall/rules
POST   /host/firewall/rules        {port, protocol, sourceIp, action, description}
PUT    /host/firewall/rules/:id    (same body)
DELETE /host/firewall/rules/:id
POST   /host/firewall/port         (alias to add rule)
GET    /host/firewall/forward
POST   /host/firewall/forward      {fromPort, toPort, toIp, protocol, description}
DELETE /host/firewall/forward/:id
```

All require Linux + `iptables` present (`exec.LookPath("iptables")`). Responses are JSON `FirewallRule`/`PortForward` (`id: rule-<uuid>` / `fwd-<uuid>`, `createdAt` RFC3339). `ruleArgs()` always emits `-j ACCEPT` (allow-only).

## Forge Proxy

`forge/api/internal/http/handlers_firewall.go:registerFirewallRoutes` — `requireRole("admin")`, `mutationLimiter`, `resolveNodeHostTarget` → `cfg.Daemon.{GetFirewallStatus,Enable,Disable,List,Add,Update,Delete,OpenPort,ListForwards,AddForward,DeleteForward}`. Returns `502` on daemon error via `writeError`. No direct validation; beacon is source of truth.

## UI — `AdminFirewall.tsx`

- Node selector (`/admin/host` style) → `fetchFirewallStatus/Rules/Forwards` per `activeNodeId`.
- Two tabs: **Rules** (`Shield`) and **Forwards** (`Network`). Enable/Disable pill + mutation toast.
- Add/Edit Rule modal: Port (number), Protocol (TCP/UDP), **Source IP** placeholder `203.0.113.42 or 198.51.100.0/24`, **Action** only `ALLOW` (deny removed; helper text: “Firewall is allow-only. To deny, omit or remove the rule. See docs/firewall.md.”), Description. Forward modal: From/To port, To IP, Protocol.
- Validation mirrors beacon: empty source or `0.0.0.0/0` surfaces the beacon error (`400` → toast) instead of being suggested.
- Tables ordered by `ID` (`sort.Strings`) so reconcile/restore ordering is deterministic.

## Persistence & Reconciliation

```go
// handlers_firewall.go:reconcile
*filter
:FORGE-BEACON - [0:0]
-A FORGE-BEACON -p tcp --dport 80 -s 198.51.100.0/24 -m comment --comment forge:rule-... -j ACCEPT
COMMIT
*nat
:FORGE-BEACON-FWD - [0:0]
-A FORGE-BEACON-FWD -p tcp --dport 8080 -m comment --comment forge:fwd-... -j DNAT --to-destination 10.0.0.5:80
COMMIT
```

Atomic via single `iptables-restore`. New rules are applied with `iptables -w -A` before `persistLocked`; on persist failure they are rolled back (`-D`). Deletes and updates do the reverse (remove, then persist, roll back on failure).

## Security notes

- Broad CIDRs (`0.0.0.0/0`, `/0`) are rejected to avoid accidental world-exposed ports.
- Private source ranges are rejected for rules — use a forward to an internal `toIp` instead if you need NAT to a private destination.
- Audit log: `auditFirewall(action, objectID)` → `[security-audit] firewall action=add-rule object=rule-…` via `log.Printf`.
- Requires admin role; non-admin → 403 (`handlers_firewall_test.go:TestFirewallRoutes_NonAdmin`).

## Future / Frozen

- Firewall is **allow-only frozen** — no `DENY`/`REJECT` iptables target is planned (the allow-only model + chain hook/unhook gives default deny). If drop semantics are ever needed they should be proposed as a new `FORGE-BEACON-DENY` chain rather than overloading `action`.
