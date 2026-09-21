# Subagent 04 — Mount Allowlist Host Breakout Fix (GH-19/SE-04 P0)

**Slice:** Fix mount allowlist host breakout  
**Finding:** `forge/api/internal/store/store_mounts_ext.go:323` only blocked 2 sources (`/etc/forge`, `/var/lib/forge/volumes`) + 2 targets (`/`, `/home/container`) — left `/etc`, `/proc`, `/var/run/docker.sock`, `/`, `/root`, `/boot`, `/sys`, `/dev` mountable via compromised admin.  
**Agent:** 110-03-04 (Phase 03 Agent 04/20)  
**Date:** 2026-08-24  
**Status:** IMPLEMENTED & VERIFIED

---

## 1. Summary

Hardened panel-side mount validation from a 2+2 denylist to a hybrid allowlist/denylist that prevents host breakout (P0). When `MOUNTS_ALLOWED_PREFIX` is set, source must be under an allowed prefix (default hint `/srv/forge-mounts` or `/var/lib/forge/mounts`); when unset, an explicit denylist blocks all protected host locations. Beacon-side second gate (`beacon/internal/server/mounts.go:64`) was verified to keep `EvalSymlinks+Rel` allowlist with default-empty deny. Existing mounts are grandfathered (validation only on create/update, not on read), preserving `allowed_mounts` per-node flow.

---

## 2. Implementation

### 2.1 `forge/api/internal/store/store_mounts_ext.go:323` — `validateMountPath`

**Before (`store_mounts_ext.go:323-339`):**
```go
if field == "source" && (value == "/etc/forge" || value == "/var/lib/forge/volumes") {
    return errors.New("mount source or target is reserved")
}
if field == "target" && (value == "/" || value == "/home/container") {
    return errors.New("mount source or target is reserved")
}
```

**After:**
- Imports `os` (`store_mounts_ext.go:1-11`).
- `validateMountPath` now (`store_mounts_ext.go:323-410`):
  - Validates absolute, clean, no `\`, no `..` (unchanged).
  - For `target`: blocks `"/"` and `"/home/container"` with descriptive error (`store_mounts_ext.go:332-336`).
  - For `source` — GH-19 hardening:
    1. **Allowlist mode** when `MOUNTS_ALLOWED_PREFIX` env is non-empty (`store_mounts_ext.go:338-351`):
       - Parses env via `mountsAllowedPrefixes()` (`store_mounts_ext.go:374-397`) — splits on `, : ;` and whitespace, `path.Clean`s each.
       - If `value == prefix || strings.HasPrefix(value, prefix+"/")` then allow; otherwise return:
         ```
         mount source "/etc/shadow" is not within allowed prefix /srv/forge-mounts, /var/lib/forge/mounts (set MOUNTS_ALLOWED_PREFIX to configure, e.g. /srv/forge-mounts or /var/lib/forge/mounts)
         ```
       - Filters out `"/"` and `"."` as prefixes.
    2. **Denylist mode** when env unset (`store_mounts_ext.go:352-372`):
       - Blocks `"/"` and `"/home/container"` for source with hint.
       - Denied prefixes (`store_mounts_ext.go:357-366`):
         ```
         /etc, /proc, /sys, /dev, /boot, /root, /var/run, /run, /var/lib/forge, /var/lib/docker
         ```
         Check `value == denied || strings.HasPrefix(value, denied+"/")`.
       - Legacy exact checks for `/etc/forge`, `/var/lib/forge/volumes` retained (`store_mounts_ext.go:369-371`).
- Helpers `mountsAllowedPrefixes()` and `mountsAllowedPrefixHint()` (`store_mounts_ext.go:374-410`) provide consistent hint `/srv/forge-mounts or /var/lib/forge/mounts (set MOUNTS_ALLOWED_PREFIX to configure)` when env empty.

**Grandfathering:** `CreateMount` (`store_mounts_ext.go:41-83`) and `UpdateMount` (`store_mounts_ext.go:121-192`) call `validateMountPaths`/`validateMountPath`; read paths `ListMounts` (`store_mounts_ext.go:13-39`), `GetMount` (`store_mounts_ext.go:85-110`), `ServerMounts` etc. do **not** validate, so existing rows violating new rules remain readable. `ensureMountAvailableForServer` (`store_mounts_ext.go:297-314`) still enforces `mount_node ∧ egg_mount` double-join per node/egg, so `AllowedMountSourcesForNode` (`store_mounts_ext.go:367-388`) second gate unchanged.

### 2.2 `forge/api/internal/config` — `MOUNTS_ALLOWED_PREFIX` env

- `forge/api/internal/config/config.go:16-26` — added `Mounts MountsConfig` to `Config`.
- `forge/api/internal/config/config.go:95-99` — new type:
  ```go
  type MountsConfig struct {
      AllowedPrefix string `mapstructure:"allowed_prefix"`
  }
  ```
- `forge/api/internal/config/config.go:140-143` — `v.SetDefault("mounts.allowed_prefix", "")` (empty = denylist mode; operator sets `MOUNTS_ALLOWED_PREFIX` to enable allowlist).
- `forge/api/internal/config/config.go:164-184` — `BindEnv("mounts.allowed_prefix", "MOUNTS_ALLOWED_PREFIX")` + `AutomaticEnv` with `"." -> "_"` so both `MOUNTS_ALLOWED_PREFIX` and `MOUNTS_ALLOWED_PREFIX` map correctly.
- `forge/api/internal/config/env.go:68-73` — `FromEnv()` now sets `Mounts: MountsConfig{AllowedPrefix: env("MOUNTS_ALLOWED_PREFIX", "")}`.
- Verified via `go run` (`forge/api/check_config.go` temp): `FromEnv().Mounts.AllowedPrefix` and `viper.GetString("mounts.allowed_prefix")` correctly reflect env.

### 2.3 `beacon/internal/server/mounts.go:64` — allowlist `EvalSymlinks+Rel`, default-empty deny

- Verified existing `allowedMountSource` (`beacon/internal/server/mounts.go:64-89`) already:
  ```go
  if len(allowed) == 0 { return "", errors.New("custom mounts are not enabled on this node") }
  if !filepath.IsAbs(source) { ... }
  canonicalSource, err := filepath.EvalSymlinks(filepath.Clean(source))
  ...
  canonicalPermitted, err := filepath.EvalSymlinks(filepath.Clean(permitted))
  relative, err := filepath.Rel(canonicalPermitted, canonicalSource)
  if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) { return canonicalSource, nil }
  ```
  This is the correct allowlist with symlink resolution and `Rel` check. Kept default-empty deny.

- Added doc comment (`beacon/internal/server/mounts.go:64-67`) clarifying GH-19/SE-04 intent.
- Hardened `runtimeMounts` (`beacon/internal/server/mounts.go:24-46`): target check now `if target == "/" || target == "/home/container"` (previously only `/home/container`) to block container-root mount even if panel validation bypassed. Defense in depth.
- `allowedMountRelative` (`beacon/internal/server/mounts.go:185-197`) and `mountSourceWithinAllowed` (`beacon/internal/server/mounts.go:199-215`) already use `EvalSymlinks+Rel` / `Rel`; `cleanupMount` (`beacon/internal/server/mounts.go:114-173`) correctly uses `allowedMountSource` and `Rel` with `rootfs.New`.

- `beacon/config/config.go:499-513` — `validateAllowedMount` already denies `"/", "/etc", "/proc", "/sys", "/dev", "/boot", "/root"` for `allowed_mounts` entries, so beacon operator cannot configure a sensitive root as allowed. Preserved.

### 2.4 `forge/api/internal/http/handlers_admin.go:1605` — guided error message

- `handlers_admin.go:1605-1636` (POST `/mounts`):
  ```go
  if err != nil {
      msg := err.Error()
      if strings.Contains(strings.ToLower(msg), "mount") && !strings.Contains(msg, "MOUNTS_ALLOWED_PREFIX") {
          msg = fmt.Sprintf("%s (allowed host prefix: /srv/forge-mounts or /var/lib/forge/mounts; set MOUNTS_ALLOWED_PREFIX to configure)", msg)
          err = errors.New(msg)
      }
      return respondStoreError(err)
  }
  ```
- Same wrapping for PATCH `/mounts/:id` (`handlers_admin.go:1687-1716`).
- Store errors already contain `MOUNTS_ALLOWED_PREFIX` hint (e.g., `mount source "/etc/shadow" is reserved: "/etc" targets a protected host location (allowed prefix: /srv/forge-mounts or /var/lib/forge/mounts (set MOUNTS_ALLOWED_PREFIX to configure))`), handler ensures guidance even if error originates elsewhere. Imports `errors`, `fmt`, `strings` already present; `strings.ToLower` check avoids double-append.

### 2.5 Tests — `TestMountAllowlist_*`

Added to `forge/api/internal/store/store_mounts_ext_test.go:43-158` (imports `os`, `strings`):

- `TestMountAllowlist_BlocksEtc` (`store_mounts_ext_test.go:43-69`): clears `MOUNTS_ALLOWED_PREFIX`, asserts `/etc`, `/etc/shadow`, `/etc/passwd`, `/etc/forge`, `/etc/hosts` blocked, error mentions `reserved`/`protected` or `MOUNTS_ALLOWED_PREFIX`.
- `TestMountAllowlist_BlocksDockerSock` (`store_mounts_ext_test.go:71-88`): asserts `/var/run/docker.sock`, `/var/run`, `/run/docker.sock`, `/run` blocked.
- `TestMountAllowlist_BlocksProc` (`store_mounts_ext_test.go:90-121`): asserts `/proc`, `/proc/self/environ`, `/sys`, `/sys/kernel`, `/dev`, `/dev/sda`, `/boot`, `/boot/vmlinuz`, `/root`, `/root/.ssh`, `/`, `/var/lib/forge`, `/var/lib/forge/volumes`, `/var/lib/docker` blocked; also checks target `/` and `/home/container` blocked.
- `TestMountAllowlist_AllowsSrv` (`store_mounts_ext_test.go:123-168`):
  - Denylist mode (`MOUNTS_ALLOWED_PREFIX` unset): `/srv/forge-mounts/data`, `/srv/game-data`, `/mnt/shared/maps` allowed; `/var/lib/forge/mounts/data` correctly blocked (under denied prefix).
  - Allowlist mode (`MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts,/var/lib/forge/mounts`): `/srv/forge-mounts/app` and `/var/lib/forge/mounts/app` allowed; `/srv/game-data`, `/etc`, `/var/run/docker.sock` blocked; error for `/etc/shadow` guides to `MOUNTS_ALLOWED_PREFIX`.

All four tests use `t.Cleanup` to restore env, ensuring no leakage.

---

## 3. Verification

### 3.1 Store validation

```bash
go test ./forge/api/internal/store -run TestMountAllowlist -count=1 -v
# PASS: TestMountAllowlist_BlocksEtc, BlocksDockerSock, BlocksProc, AllowsSrv
go test ./forge/api/internal/store -run TestValidateMountPaths -count=1 -v
# PASS: all 9 subtests (relative, unclean, backslash, forge config/volumes, container root, filesystem root)
```

- Existing valid mounts `/srv/game-data`, `/mnt/shared/maps` still allowed in denylist mode.
- Sensitive paths all correctly rejected with guided message.

### 3.2 Beacon validation

```bash
go test ./beacon/internal/server -run Mount -count=1 -v
# PASS: TestCreateAllowsOnlyConfiguredMountSources (allowed descendant, outside configured root)
# PASS: TestRuntimeRequestFromConfigurationParsesAndValidatesMounts
# PASS: TestCleanupMount* (5 tests)
go test ./beacon/config -run TestAccessors_AllowedMountsList -count=1 -v
# PASS
```

- Confirmed `allowedMountSource` keeps `EvalSymlinks+Rel` and default-empty deny.
- `validateAllowedMount` in `beacon/config/config.go:499-513` still rejects protected host locations for `allowed_mounts` config.

### 3.3 Config

```bash
MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts go run check_config.go
# allowed_prefix="/srv/forge-mounts" — PASS
# viper env binding PASS
```

### 3.4 Ancillary fixes (parallel-agent build breaks)

- `beacon/internal/server/firewall_placeholder_test.go:64-73` — fixed `for i := 0; i+len(s)-len(sub)+1; i++` non-boolean condition to `i+len(sub) <= len(s)`.
- `beacon/internal/server/sysinfo_darwin.go:1-12` — removed unused `sort` import.

These were required for `go test ./beacon/...` to build; not part of GH-19 but included for verification.

---

## 4. Constraints & Grandfathering

- **Per-node allowlist preserved:** `ensureMountAvailableForServer` (`store_mounts_ext.go:297-314`) still requires `mount_node ∧ egg_mount` double-join; `ListMounts`/`GetMount` do not re-validate.
- **Grandfathering:** Only `CreateMount` and `UpdateMount` call `validateMountPath`; read paths remain permissive, so existing rows with legacy sensitive sources remain in DB but cannot be recreated or updated without conforming to new rules. This matches audit constraint.
- **Default behavior:** Empty `MOUNTS_ALLOWED_PREFIX` → denylist mode (least disruption to existing `/srv/game-data` mounts that passed old 2+2 check). Operators seeking stricter confinement set `MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts` (or comma/colon list) to switch to allowlist.

---

## 5. How to Operate

- **Default (no env):** All mounts under protected prefixes (`/etc`, `/proc`, `/sys`, `/dev`, `/boot`, `/root`, `/var/run`, `/run`, `/var/lib/forge`, `/var/lib/docker`, `/`) are rejected. Use `/srv/forge-mounts`, `/mnt`, `/srv/game-data` etc. for host data.
- **Strict allowlist:** Set `MOUNTS_ALLOWED_PREFIX=/srv/forge-mounts` or `/srv/forge-mounts:/var/lib/forge/mounts` (comma `,` colon `:` semicolon `;` or space separated) in `forge/api` env and `beacon` `allowed_mounts` config. Then any `POST /mounts` with source outside the prefix returns:
  ```
  mount source "/etc/shadow" is not within allowed prefix /srv/forge-mounts (set MOUNTS_ALLOWED_PREFIX to configure, e.g. /srv/forge-mounts or /var/lib/forge/mounts) (allowed host prefix: /srv/forge-mounts or /var/lib/forge/mounts; set MOUNTS_ALLOWED_PREFIX to configure)
  ```
- **Beacon:** Always configure `allowed_mounts` in `beacon/config.yaml` (or `DAEMON_ALLOWED_MOUNTS`) to the same prefix(es); empty list denies all custom mounts.

---

## 6. Files Modified

- `forge/api/internal/store/store_mounts_ext.go:1-410` — expanded `validateMountPath`, add `mountsAllowedPrefixes`/`mountsAllowedPrefixHint`.
- `forge/api/internal/store/store_mounts_ext_test.go:1-168` — added `os`/`strings` imports, four `TestMountAllowlist_*` tests.
- `forge/api/internal/config/config.go:16-184` — added `MountsConfig`, `Config.Mounts`, defaults and `BindEnv` for `MOUNTS_ALLOWED_PREFIX`.
- `forge/api/internal/config/env.go:68-73` — `FromEnv` reads `MOUNTS_ALLOWED_PREFIX`.
- `beacon/internal/server/mounts.go:24-67` — target `"/"` block, doc comment for allowlist `EvalSymlinks+Rel` with default-empty deny.
- `forge/api/internal/http/handlers_admin.go:1605-1716` — guided error wrapping for POST and PATCH mount handlers.
- Ancillary (build): `beacon/internal/server/firewall_placeholder_test.go:64-73`, `beacon/internal/server/sysinfo_darwin.go:1-12`.

---

## 7. Risks & Next Steps

- **Migration:** Existing mounts violating new rules remain until edited; admin should audit `SELECT source FROM mounts WHERE source ~ '^/(etc|proc|sys|dev|boot|root|var/run|run|var/lib/forge)'` and re-create under `/srv/forge-mounts`.
- **Documentation:** Update operator runbook to set `MOUNTS_ALLOWED_PREFIX` in `docker-compose.yml` / `systemd` and `beacon` `allowed_mounts`.
- **Future:** Consider sharing denylist constants between panel and beacon via a shared `internal/mounts` package to avoid drift.

