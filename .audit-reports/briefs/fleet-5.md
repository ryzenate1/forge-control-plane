# Scope 5 — Docker, image/cache cleanup, docker events, registries: FRONTEND (audit and FIX)

Read `.audit-reports/briefs/fleet-common.md` first. You are scope 5; report to `.audit-reports/reports/fleet-5.md`.

## You own (edit only these)

- `forge/web/app/admin/docker/page.tsx`, `docker-cleanup/page.tsx`, `docker-events/page.tsx`,
  `containers/page.tsx`, `cleanup/page.tsx`, `registries/page.tsx`
- `forge/web/components/admin/docker-cleanup-manager.tsx`, `docker-events-feed.tsx`
- `forge/web/lib/api/docker.ts`, `docker-cleanup.ts`, `docker-events.ts`, `cleanup.ts`, `registries.ts`
- the docker/cleanup/registry sections of `lib/api/query-keys.ts` and re-exports in `lib/api.ts` — your lines only.

Read every owned file fully. Then READ (never edit) scope 4's Go files — `handlers_docker.go`,
`handlers_docker_cleanup.go`, `handlers_docker_events.go`, `handlers_cleanup.go` — and verify every call the UI
makes exists with the same path, verb, params and response shape.

## Hunt for, with extreme depth

1. **"0 bytes reclaimed" vs "no answer"** — the headline honesty bug. A cleanup result must render differently when
   the node never responded than when it responded with nothing to reclaim. Hunt `?? 0`, `|| 0`, `?? []`,
   `.catch(() => [])`, `Number(x) || 0`, and `toFixed` on a possibly-undefined metric.
2. **Destructive action UX**: image prune, volume prune, container delete, force-remove, registry credential delete.
   Each needs an explicit confirmation that states what will be deleted, whether anything in use is protected, and
   which node is targeted. Actions must be disabled with a reason while the node is unreachable rather than firing
   into the void.
3. **Implicit node selection**: any control that runs against a defaulted/first node when the operator has not
   chosen one. The API rejects ambiguous targets, so the UI must require an explicit selection and show which node a
   number came from.
4. **Retention controls that lie**: form fields for `keep_last` / `older_than` / `all` / `force` must map onto
   request fields the backend actually honours — cross-check names and types, including the snake_case/camelCase
   hazard.
5. **Event feed integrity**: a reconnecting stream must show the discontinuity, not pretend continuity. Timestamps,
   node names and action types render from reported values only — never inferred. A capped/truncated first page must
   say it is partial rather than reading as the whole history.
6. **react-query**: keys unique per node+filter; `enabled` guards when node is unselected; no tight `refetchInterval`
   on event lists; mutations invalidate the affected queries; unmount cleans up intervals and subscriptions.
7. **States**: distinct empty / loading / error / permission-denied / not-configured / node-unreachable rendering for
   every panel, including the images, networks and volumes sub-tabs.
8. **Style**: raw hex, magic px, duplicated table/badge markup that should come from `components/ui/`, byte counts
   and digests not in JetBrains Mono, new strings without i18n keys.

## Report requirements

Add a `## Honesty table`: each number or status the UI displays -> the field it comes from -> what renders when that
field is absent or the query failed. End with `## Cannot verify`.
