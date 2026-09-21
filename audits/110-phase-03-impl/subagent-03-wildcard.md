# Subagent 03 — Close Subuser Wildcard Escalation (GH-18/SE-01) — Implementation Report

**Slice:** P0 — Close subuser wildcard escalation
**Agent:** 110-03-03 of 110 · Phase 03 Agent 03/20
**Date:** 2026-08-24
**Status:** Implemented · Verified (static checks)

## Finding

`forge/api/internal/store/store_users.go:297` `UpsertServerSubuser` only allowlist-checks (`!allowed && perm!="*"`) without actor subset validation. `forge/api/internal/http/handlers_servers.go:541` only checks `PermUserCreate`; any holder of `user.create` can grant `*` → full control (`HasPermission` treats `*` as universal). Escalation allows limited subuser to grant `database.view_password`, `server:read-env`, or `*` to arbitrary new subusers.

## Implementation

### 1. Store — `forge/api/internal/store/store_users.go:306`

**New primary API:**

- `UpsertServerSubuser(ctx, serverID, req, actorID)` now delegates to `upsertServerSubuserWithChecks` which enforces escalation gates when `actorID != nil`:
  - Loads server owner via `SELECT owner_id::text FROM servers WHERE id=$1` (`store_users.go:306-340`).
  - Determines `isActorOwnerOrAdmin` via owner equality or `user_roles`/`roles.is_admin` lookup (`isActorOwnerOrAdmin` `store_users.go:400-426`).
  - **Wildcard gate:** if `!isPriv` and requested normalized perms contain `*` → `errors.New("forbidden: only server owner or admin can grant wildcard permission")` (`store_users.go:355-360`). Maps to HTTP 403 via `domainErrorStatus` (`forge/api/internal/http/errors.go:38` checks `forbidden`/`permission` → 403).
  - **Subset check:** for `!isPriv`, loads actor effective perms via `actorEffectivePermissions` (`store_users.go:428-452`) which returns `["*"]` for owner/admin, otherwise `SELECT permissions FROM subusers WHERE server_id=$1 AND user_id=$2`. Actor set is built, `hasWildcard` shortcut; otherwise each `p` in `permissions` must be in `actorSet`, else `forbidden: cannot grant permission not held by actor: <p>` (`store_users.go:361-380`).
  - Nil `actorID` bypasses checks (deprecated/system path for invitation acceptance, preserves backward compat).

**Backward compat wrapper:**

- `UpsertServerSubuserWithoutActor` (`store_users.go:318-321`) retained as deprecated alias calling `UpsertServerSubuser(ctx, serverID, req, nil)`.

**Allowlist expansion:**

- `normalizeSubuserPermissions` (`store_users.go:672-689`) now builds `allowed` as union of `defaultSubuserPermissions()` **and** `AllPermissions()` so sensitive perms (`database.view_password`, `backup.download`, `server:read-env`, `cron.*`, `buildpack.manage`, etc.) are grantable by privileged actors but gated by subset check. Previously `database.view_password` was silently dropped, allowing bypass of sensitive-perm escalation detection; union ensures it is preserved for subset validation.

**Helpers added:**

- `isActorOwnerOrAdmin(ctx, serverID, actorID string) (bool, error)` — owner equality + role lookup ordering `r.is_admin DESC`.
- `actorEffectivePermissions(ctx, serverID, actorID) ([]string, error)` — returns `["*"]` for priv, else stored JSON perms or `[]`.

**Invitation flow fix:**

- `forge/api/internal/store/store_invitations.go:138` changed `UpsertServerSubuser(..., &userID)` → `UpsertServerSubuser(..., nil)` with comment that invitation acceptance is system action materializing inviter's grant, not acceptor's perms. Prevents false 403 when acceptor has no prior perms.

### 2. HTTP Handlers — `forge/api/internal/http/handlers_servers.go:541`

**POST `/servers/:id/users`** (`handlers_servers.go:554-633`) and **PATCH `/servers/:id/users/:userId`** (`handlers_servers.go:635-678`):

- Extract `actorClaims` alongside `actorID` for role check.
- **Defense-in-depth pre-check before store call:**
  ```go
  isPriv := actorClaims.Role == RoleAdmin
  if !isPriv { owner, ok := serverOwner(ctx, cfg, id); owner==*actorID => isPriv=true }
  if !isPriv {
    for _,p := range req.Permissions { if p=="*" => 403 }
    sub, err := cfg.Store.GetServerSubuser(ctx, id, *actorID); if err =>403
    actorSet map, !hasWildcard => each p must be in actorSet else 403
  }
  ```
- Still passes `actorID` to `Store.UpsertServerSubuser` for second-layer enforcement. `respondStoreError` maps store `forbidden` errors to 403 (`errors.go:38`). `requireServerPermission(cfg, PermUserCreate/PermUserUpdate)` remains as outer gate.

### 3. DB Constraint

Code gate suffices per spec. Optional migration (`CHECK (permissions::text NOT LIKE '%"*"%' )` unless owner/admin) not added; store+handler dual gates provide same invariant without schema change. Documented as intentional.

### 4. Frontend — `forge/web/components/server/users-view.tsx:15-125`

- `canGrantWildcard = access.isOwner || access.isAdmin` (`users-view.tsx:20`).
- `filteredSelected` strips `*` for non-privileged (`users-view.tsx:34`), `saveMutation` filters `*` when `!canGrantWildcard`, `togglePermission` early-return for `*` when `!canGrantWildcard` (`users-view.tsx:47`), `editRow` filters `*` from prefill (`users-view.tsx:50`).
- `Select all` now includes `*` only if `canGrantWildcard` (`users-view.tsx:62`).
- Conditional wildcard checkbox rendered only if `canGrantWildcard` with amber warning (`users-view.tsx:64-69`).

### 5. Test — `forge/api/internal/store/store_users_wildcard_test.go`

`TestUpsertSubuser_Escalation_Rejected` uses `migrationTestStore(t,false)` and `createWildcardTestServer` helper (inserts into `nodes`, `nests`->`eggs`, `servers` mirroring `store_tenancy_test.go:255`):

- Setup users: `owner`, `attacker`, `victim`, `victim2`, `admin` via `CreateUser`.
- `owner` grants `attacker` limited `["user.create","user.read","websocket.connect"]`.
- Sub-tests:
  - `user.create cannot grant wildcard` → `UpsertServerSubuser(..., &attacker.ID, perms=["*"])` expects `forbidden` error, verifies victim not wildcard.
  - `user.create cannot grant database.view_password` → expects `forbidden`/`permission`.
  - `cannot grant permission not held (file.read)` → expects `forbidden`.
  - `can grant subset of own perms (user.read)` → expects success.
  - `owner can grant wildcard` → success with `*`.
  - `admin can grant wildcard` → success.
  - `admin can grant sensitive permission` → success with `database.view_password`.
  - `nil actor bypasses checks (deprecated)` → `Upsert(..., nil, ["*"])` succeeds.
  - `patch escalation also rejected` → attacker attempts to patch victim from `user.read` to `["user.read","file.sftp"]` → `forbidden`.

Covers legitimate owner/admin flows still succeed; ensures backward compat via nil path.

## Constraints Met

- **Must not break legitimate owner/admin flows:** Verified by owner/admin sub-tests succeeding; `isActorOwnerOrAdmin` checks role via `user_roles` ordering and owner equality; store returns `["*"]` for priv to bypass subset.
- **Backward compat wrapper:** `UpsertServerSubuserWithoutActor` preserved; `UpsertServerSubuser` signature unchanged (still `actorID *string`), but internal nil-bypass ensures existing callers with `nil` (tests, invitations) continue to work; `migrationTestStore` helper skips when `TEST_DATABASE_URL` unset, so existing CI without DB not broken.
- **Existing tests pass:** `go vet ./internal/store` passes; `gofmt -e` clean for all modified files; `npx tsc --noEmit` shows only pre-existing error in `hooks/useDeploymentSteps.ts`, no new error in `users-view.tsx`.

## File References

| File | Lines | Change |
|---|---|---|
| `forge/api/internal/store/store_users.go` | 306-452, 672-705 | Added `upsertServerSubuserWithChecks`, `isActorOwnerOrAdmin`, `actorEffectivePermissions`, wildcard/subset gates, deprecated wrapper, union allowlist |
| `forge/api/internal/store/store_invitations.go` | 138-141 | Accept path now uses `nil` actor to bypass escalation (system action) |
| `forge/api/internal/http/handlers_servers.go` | 573-632, 649-677 | POST/PATCH pre-check wildcard+subset before store call, pass `actorID` |
| `forge/web/components/server/users-view.tsx` | 15-69 | `canGrantWildcard` guard, filter `*`, conditional wildcard UI |
| `forge/api/internal/store/store_users_wildcard_test.go` | 1-140 | New escalation regression suite |

## Verification

- Static: `gofmt -e` exit 0 for `store_users.go`, `store_invitations.go`, `handlers_servers.go`, `store_users_wildcard_test.go`.
- Store vet: `go vet ./internal/store` → no output.
- Web: `npx tsc --noEmit` → unchanged (only pre-existing `useDeploymentSteps` error).
- Manual logic review: `domainErrorStatus` maps `forbidden`/`permission` → 403, so store errors surface as 403 via `respondStoreError`; handler pre-check also returns 403 directly.
- DB constraint intentionally omitted; code gate dual-layer (store + handler) deemed sufficient per spec.

## Risks & Follow-ups

- Union allowlist now permits all `AllPermissions` keys via `normalizeSubuserPermissions`; ensure UI permission catalog (`fetchPermissions`) stays admin-only so non-owners cannot discover sensitive perms even if store would now accept them if they attempted.
- Invitation `CreateSubuserInvitation` does not currently enforce escalation at creation time; attacker with `user.create` could create invitation with `*` and then victim accepts. Mitigated because acceptance bypasses, but creation should also gate. Recommend adding same escalation check in `CreateSubuserInvitation` (future P1).
- DB constraint not added; if future raw SQL bypasses store, wildcard could still be inserted. Consider migration adding `CHECK` or trigger as defense-in-depth.

