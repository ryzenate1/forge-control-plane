# Scope 7 — Mounts, App mounts, Host files, Host inspector: FRONTEND (audit and FIX)

Read `.audit-reports/briefs/fleet-common.md` first. You are scope 7; report to `.audit-reports/reports/fleet-7.md`.

## You own (edit only these)

- `forge/web/app/admin/files/page.tsx`, `host/page.tsx`, `mounts/page.tsx`, `app-mounts/page.tsx`
- `forge/web/components/admin/host-files-view.tsx`, `AdminMounts.tsx`, `app-mounts-manager.tsx`
- `forge/web/lib/api/files.ts`, `host-files.ts`, `mounts.ts`, `container-files.ts`, `host.ts`
- the files/host/mounts sections of `lib/api/query-keys.ts` and re-exports in `lib/api.ts` — your lines only.

Read every owned file fully. Check `forge/web/lib/api/mounts.test.ts` against `mounts.ts` — an existing test may
encode the intended contract. Then READ (never edit) scope 6's Go files: `handlers_mount_manage.go`,
`handlers_host.go`, `handlers_files.go`, `handlers_file_download.go`, `handlers_container_files.go`, plus the base
`/mounts` routes in `handlers_admin.go`, and verify path/verb/param/shape agreement.

## Hunt for, with extreme depth

1. **Path handling in a file browser** — the interesting bugs are here:
   - a path built by client-side string concatenation of directory + name with no per-component encoding, so `..`,
     `#`, `?`, `%` or a leading `/` changes the request target;
   - current-directory state drifting out of sync with what requests actually carry;
   - an allowed-root assumption baked into the client that the server does not enforce (the UI must not be the jail);
   - a file name used as a React key and also interpolated into an API path.
2. **Erased failures**: `?? []`, `.catch(() => [])`, `if (error) return null`, `?? 0` — a failed directory read must
   never render as "empty folder", and a failed file read must never render as a zero-byte or empty file. Distinct
   empty / loading / error / permission-denied / node-unreachable states everywhere.
3. **Editor safety**: save with no dirty-state guard and no conflict detection, silently clobbering a file changed
   since it was read; delete without confirmation; chmod UI able to set setuid bits; upload with no size limit
   surfaced; an unsaved-changes navigation escape.
4. **Implicit node selection**: file, mount and host-inspector actions must require an explicit node. Do not rely on
   `node-select.tsx` defaulting — the API rejects ambiguous targets, so a control that fires with no chosen node is
   dead on arrival and must be disabled with a reason.
5. **Host inspector honesty**: metrics rendered from a zero value when the probe never reported; values unsupported
   on the platform rendered as 0 rather than "not supported" (this is macOS, so darwin gaps are real); process lists
   truncated without saying so; an old snapshot labelled as live.
6. **Mount config that lies**: a bind source path typed freeform by the operator where the backend restricts to
   provisioned roots; `readOnly`, tmpfs and volume-vs-bind distinctions collapsed or defaulted to writable; a mount
   shown attached to a node the API did not accept it for.
7. **react-query**: `enabled` guards when node/path are unselected; directory listings invalidated after
   write/delete/rename/move; no aggressive polling left on the host inspector; mutations invalidating the right keys.
8. **Style**: raw hex, magic px, duplicated table/tree/modal markup instead of `components/ui/`, paths/sizes/
   permissions not in JetBrains Mono, new strings missing i18n keys.

## Report requirements

Add a `## Path safety` section: every place the client builds a path for a request, how it encodes, and what the
server-side jail is. End with `## Cannot verify`.
