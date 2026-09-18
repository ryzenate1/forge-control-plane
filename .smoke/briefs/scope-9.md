# Scope 9 — Beacon agent, panel↔Beacon control channel, cross-node, /api/remote, other runtimes

Read `.smoke/briefs/00-shared.md` first and obey it. You are agent 9.

You have one job nobody else can do: **make node B a second live node**, because cross-node features are
otherwise unverifiable and the orchestrator had to leave it offline.

## Bring node B online

Node B is `e43bb9aa-24c8-4900-a469-4656d45b55ad`, baseUrl `http://192.168.31.189:9091`, SFTP 2023,
allocation range 26566-27565. Rotate its token if needed (`POST /api/v1/nodes/:id/rotate-token` — that
route is scope 2's, use it, don't edit it). Run a **second Beacon process manually**, not via launchd:
build with `cd beacon && go build -o daemon-b ./cmd/daemon` (never overwrite `beacon/daemon` — that is the
running binary), then start it with the env/config overrides it needs to bind 9091/2023 and to authenticate
with node B's credentials, state dir `.dev-data/beacon-b/`, log `.dev-logs/beacon-b.log`. Read
`native.sh` (`start_beacon`, ~line 382) and `beacon/cmd/daemon` for the real variable names. If Colima's
Docker socket is needed, it is `~/.colima/default/docker.sock`. Kill it when you finish and say so in the
report. If you cannot bring B online, document precisely why in Not-verifiable — the orchestrator needs to
know whether cross-node is truly untestable or just untested.

## Owned files

Everything under `beacon/` **except** `beacon/internal/runtime/docker.go`, `registry.go`,
`containerd*.go`, `podman*.go` (scope 7) — you own `beacon/internal/remote/**`,
`beacon/internal/server/**`, `beacon/internal/sftpserver/**` is scope 6's, `beacon/internal/console*`,
`beacon/internal/host*`, `beacon/internal/config`, `beacon/cmd/**`, and the non-Docker runtimes:
`beacon/internal/runtime/kubernetes*`, `firecracker*`, `kvm*`, `lxc*`, plus `forge/api/internal/http/handlers_remote.go`,
`handlers_remote_extra.go`, `handlers_crossnode.go`, `handlers_host.go`, `handlers_kubernetes.go`,
`handlers_incus.go`, `handlers_nomad.go`, `handlers_netbird.go` is scope 5's, `handlers_clustermembership.go`,
`handlers_drain.go`, `handlers_failover.go`, `handlers_fencing.go`, `handlers_upgrade.go`,
`handlers_vault_provider.go`, `internal/services/crossnode/`, `internal/services/drain/`, `failover/`,
`fencing/`, `clustermembership/`, `evacuationplanner/`, `recovery/`, `incus/`, `nomad/`, `kubernetes/`,
`runtime/`, `upgrade/`, `vaultprovider/`, `internal/daemon/client` (the control channel itself — compose.go
is scope 7's), plus matching `_test.go`.

## Smoke checklist

Beacon health/reporting: `/health` shape, heartbeat cadence into the panel, node status transitions
(online → offline when you stop sending heartbeats, and back). Prove the panel notices a dead Beacon in
the documented window and that "not reporting" is not shown as "healthy".

Reconnect: kill node A's Beacon? **No — do not touch the launchd-managed Beacon or restart any service.**
Instead exercise reconnection against your own node-B process: stop it, watch the panel mark it offline,
restart it, and verify `beacon/internal/remote/reconnect.go` resumes cleanly (backoff honoured, no duplicate
registration, no orphaned streams, session/queue replay intact).

Control channel: commands go over HTTP via `internal/daemon.Client`, and WebSockets carry **only**
console/stats/log streams. Verify a command sent through the panel reaches the Beacon and its result is
reflected — and verify the reverse claim too: no command is being delivered over a WS path. Check auth of
the Beacon→panel `/api/remote` calls uses node credentials, not user sessions, and that a request with a
wrong/revoked node token is rejected.

Ambiguous-target policy (a project rule): any request that omits the node it refers to must be **rejected**,
not resolved to the first node that happens to have credentials. Now that two nodes exist, test this for
real on every control endpoint that takes an optional node — a silently-picked node is S1.

Cross-node (`internal/services/crossnode/`): endpoints, `/resolve`, `/health`, `/services`,
`POST /cache/clear`, `/cache/ttl`, `/describe/:host/:port`. Read the package's existing scenario docs/tests
(`SCENARIO7_REPORT.md`, `scenario7_test.go`) and verify the **implemented** behaviour matches: resolution
across nodes, health filtering of unhealthy targets, cache semantics. Then verify the operational flows with
node A + node B: server transfer/migration, evacuation/drain of a node, failover, fencing. For each, prove
the source actually stops and the destination actually starts — a transfer that marks both records active is
S1. Verify cross-node behaviour when B is offline: honest error, not a half-completed transfer.

Console/logs/stats streams: WS ticket → connect → real output for a running container; verify the stream
is scoped to the authorized server and that a ticket for one server cannot open another's console.

Other runtimes: kubernetes/incus/nomad/firecracker/lxc handlers are present. These require infrastructure
that is not on this machine. Do not fake them. Verify the **capability-gated honesty** of each: an
unconfigured runtime must report `Skipped`/unavailable with a reason, and must not be offered in the
placement/UI runtime selector as if usable. Report any that claims availability, and any phase registrar
that returns `nil` without mounting routes (that is a documented bug shape: use `ErrPhaseSkipped`).

Upgrade/host inspection: `/upgrade/*` (versions, system-status, plans), `/host/*` — verify readings are
live, not zeroed or stale, and that an upgrade plan against an unreachable node fails visibly.

## Report

`.smoke/reports/scope-9.md`. State clearly at the top whether node B came online and for how long, because
scopes 3, 4 and 7 may rely on it after you finish.
