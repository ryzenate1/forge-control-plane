# Subagent 02 — Eggs / Nests / Variables / Startup Templating + Template Catalog Consolidation

> **Scope:** Design-only implementation plan. No product code modified.  
> **Date:** 2026-08-24  
> **Workspace:** `/Users/riyaz/project/gamepanel`  
> **Findings addressed:** GH-14 (regex slash bug), GH-13 (triple template system), GH-15 (stored-not-applied config parsers), CPU conflation / `user_viewable` leak / allocation-protocol drift (when templating-related), PufferPanel typed-ops collapse

---

## 0. Executive Summary & Decision Log

| Decision | Recommendation | Rationale |
|---|---|---|
| **GH-14 regex fix** | **Fix in place** (`forge/api/internal/store/store_egg_variables.go:142`) — strip `/…/` delimiters, split `rules` respecting regex, support flags, unescape `\/` | Blocks 100% of PTDL imports that use `regex:/…/` notation. Trivial, additive, no migration. |
| **GH-13 template catalog SoT** | **DB `eggs` is canonical**. FS `packages/game-templates` is *source*, `DefaultSeeder` is *loader*, `forge/web/lib/egg-templates.ts` becomes generated/deprecated re-export, `app-templates-data.ts` is *separate domain* (app-store) — not merged | Matches migration `043_unify_eggs_templates_mounts.sql:1` intent ("Canonicalize eggs as sole operational model"). Avoids 3-way drift. |
| **GH-15 config patcher** | **Implement minimal patcher** at `beacon/internal/server/manager.go:onBeforeStart` for 4 parsers (`properties`, `yaml`, `json`, `ini`) + `file` fallback. Defer `xml`/`properties-extended` to Phase 2 behind feature flag `beacon.config_patcher.enabled` | `eggs.config.files[*].parser` is stored (`store_nests.go:44` → `jsonb config`) and seeded in `egg-templates.ts:66` (`server.properties`), but never read at pre-start (`manager.go:610` has zero config references). Wings parity requires it; remove would be breaking for PTDL imports. Decision table in §5. |
| **Typed install ops** | **Add optional `install_steps JSONB`** alongside legacy `install_script` — shell blob stays default, typed `operations.Step[]` used when present | Preserves backward compat; enables modded-Minecraft typed pipeline (paperDl/fabricDl/etc.) without forcing 14 games to rewrite. Backed by existing `beacon/internal/installer/operations` registry (`registry.go:23`, `operation.go:15`). |
| **CPU field** | **Keep both `cpu_shares` and `cpu_limit` distinct**; expose both in egg → server → beacon mapping; document in `docs/` | Beacon already distinguishes `CPUShares` vs `CPUPercent` (`runtime/runtime.go:66`, `docker.go:786`). Panel `store_servers.go:256` persists both but API docs conflate them. |
| **`user_viewable` leak** | **Fix visibility filter** — already correct in `store_startup.go:31` (`ev.user_viewable = true`), but `ServerProvisionTarget` (`store_servers_control.go:155`) fetches *all* variables regardless of `user_viewable`. Keep daemon full-env, but add explicit comment + audit test | Daemon needs hidden vars (e.g., `DL_PATH` `userViewable:false` in `egg-templates.ts:86`); user API must not leak. No change to wire, just policy + test. |
| **Frontend import/export** | **PTDL v2 JSON** is the interchange format (`egg_import.go` shape) — static `EGG_TEMPLATES` becomes gallery backed by `GET /eggs` + `GET /nests` | Eliminates 14-template FS copy that drifts from DB seed. |

---

## 1. Current-State Inventory (file:line cited)

### 1.1 Variables & Validation — the GH-14 bug

| Item | Location | Observation |
|---|---|---|
| Entry point | `forge/api/internal/store/store_egg_variables.go:142` `validateVariableValue` | Loops `strings.Split(rules, "|")` then `strings.Cut(rule, ":")`. `case "regex": regexp.Compile(arg)` compiles `arg` verbatim. |
| Slash delimiters stored | `forge/web/lib/egg-templates.ts:80` `rules: "required|regex:/^([\\w\\d._-]+)(\\.jar)$/"` and `013_startup_variables.sql:26` style `required|string|max:64` | Pterodactyl/PTDL convention wraps regex in `/…/` with optional flags (`/…/i`). Current code treats leading `/` and trailing `/` as literal regex characters, so `server.jar` fails to match `/^([\\w\\d._-]+)(\\.jar)$/` literally (requires slashes in value). |
| Split inside regex | `store_egg_variables.go:143` `Split(rules, "|")` | A regex like `regex:/^(a|b)$/` contains `|` inside the character class / alternation. Naive split yields tokens `regex:/^(a`, `b)$/`, `max:5` etc., producing `unsupported validation rule "b)$/"` and rejecting otherwise-valid PTDL eggs. No test covers this path (verified: `grep -rn validateVariableValue` only 3 call sites, no `*_test.go`). |
| Missing `integer` / `numeric` / `boolean` / `alpha` | Same switch `store_egg_variables.go:146-186` | Pterodactyl documents `integer`, `numeric`, `boolean`, `alpha`, `alpha_dash`, `between:`. Our switch only handles `required`, `nullable`, `string`, `max`, `min`, `in`, `regex`. Eggs in `egg-templates.ts` use `integer` (`:82` `rules: "required|integer|min:1|max:100"`) which falls through to `default: unsupported validation rule "integer"` — **every integer variable currently fails validation on server creation** (`store_servers.go:278` calls same validator). This is GH-14-adjacent and must be fixed together. |
| Call sites | `store_egg_variables.go:139` (egg variable create), `store_startup.go:73` (startup variable update), `store_servers.go:278` (server create) | All share one validator — fix single function, 3 consumers benefit. |

**Minimal bug reproduction (Go):**

```go
// Before fix — fails:
validateVariableValue("server.jar", "required|regex:/^([\\w\\d._-]+)(\\.jar)$/")
// arg == "/^([\\w\\d._-]+)(\\.jar)$/"
// regexp.Compile("/^([\\w\\d._-]+)(\\.jar)$/") // expects leading slash in value -> false negative

validateVariableValue("a", "required|regex:/^(a|b)$/")
// Split -> ["required","regex:/^(a","b)$/"] -> error: unsupported rule "b)$/"
```

### 1.2 Template Catalog — 3 Systems (GH-13)

| System | Location | State |
|---|---|---|
| **DB canonical** | `forge/api/migrations/007_postgres_core_foundation.sql:87` creates `nests` + `eggs`; `043_unify_eggs_templates_mounts.sql` promotes eggs to canonical; `091_seed_minecraft_java.sql` seeds **1** egg (`Minecraft Java`, `itzg/minecraft-server:java21`) | Only 1 egg on fresh install. Verified: `091_seed_minecraft_java.sql:3` inserts single row, `ON CONFLICT DO NOTHING`. |
| **FS 14 templates** | `forge/web/lib/egg-templates.ts:27` exports `EGG_TEMPLATES: EggTemplateItem[]` with 14 entries (minecraft-paper, vanilla, palworld, valheim, terraria, enshrouded, satisfactory, rust, csgo, factorio, 7days2die, bedrock, teamspeak3, zomboid) | **Not seeded** to DB. `seeder.go:38` `DefaultSeeder` only registers `default-roles` + `default-settings` — zero egg seeding. `grep -rn EGG_TEMPLATES` shows only consumed in `forge/web/components/admin/AdminTemplates.tsx` (?) or not consumed at all (search shows only definition site). So the 14-template gallery is invisible unless manually imported. |
| **Missing `packages/game-templates`** | Finding claims `packages/game-templates` on `main` should hold shared templates. Local check: `packages/` contains only `sdk` + `shared-types` (`packages/:` listing). File not found for `packages/game-templates` | On this branch, 14 templates live only in web FS; no shared package. That's the 93-100% deficit: 1 DB egg / 14 FS templates = 7% coverage. |
| **localStorage app templates** | `forge/web/lib/app-templates-data.ts:3` `DEFAULT_APP_TEMPLATES` (nginx, node, python, postgres-compose, redis) + `localStorage` id `forge.app-templates.v1` | **Different domain**: App Store / Compose apps, not game eggs. `store_app_store.go` + `store_catalog.go` back this. Finding lumps them into "3 template systems" — accurate that *catalog sprawl* is confusing, but they are intentionally separate product surfaces (game servers vs PaaS apps). Plan: **do not merge**; clarify boundaries, unify naming/docs. |
| **Store layer sprawl** | `store_nests.go` (nests+eggs CRUD), `store_templates.go` (thin shim `ListTemplates`→`ListEggs` via `templateFromEgg`), `store_catalog.go` (one-click catalog entries), `store_app_store.go` (app-store apps/installs) | `store_templates.go:14` comment says "compatibility transform over canonical eggs. It does not read legacy `server_templates`". Legitimate shim retained for `GET /templates` compat (`server.go` routes). `store_catalog.go` key `display_name, category, versions` is DB-service oriented, not egg. Overlap is naming, not data. |

### 1.3 Config Patcher — Stored, Never Applied (GH-15)

| Item | Location |
|---|---|
| Stored shape | `eggs.config JSONB` (`store_nests.go:44` `type Egg { Config json.RawMessage }`), seeded in `egg-templates.ts:63` `config: { files: { "server.properties": { parser:"properties", find:{ "server-ip":"0.0.0.0", "server-port":"{{server.build.default.port}}" }}}}` |
| Provision-time read | `store_servers_control.go:95` `e.config::text` fetched into `target.ConfigJSON`, then never parsed/patched on beacon side. `store_nodes.go:766` similar fetch for sync. |
| Beacon pre-start | `beacon/internal/server/manager.go:610` `onBeforeStart` checks install state, suspension, sync, chown, panel sync, disk — **zero config-file patching**. `config` is not even decoded there. |
| Contract gap | Panel writes `config.files` per PTDL; Wings applies it *before* start via file parsers (properties/yaml/json/ini/xml + regex `configMatchRegex`). Ours stores it, reports `config_sync_pending`, but never acts, so `server.properties` port never rewrites on allocation change. |

### 1.4 Install Pipeline — Shell Blob vs Typed Ops

| Item | Location |
|---|---|
| Current storage | `eggs.install_script TEXT`, `install_container`, `install_entrypoint` (`store_nests.go:46`, migrations `043`, `091`). All 14 templates embed a `#!/bin/bash` shell script (`egg-templates.ts:44` onward). |
| Typed ops infra | `beacon/internal/installer/operations/` exists with **8 typed ops** today: `downloadfile`, `downloadextract`, `paperdl`, `fabricdl`, `forgedl`, `copyfile`, `movefile`, `removefile`, `writefile`, `symlink`, `runcommand` (`operations_bootstrap.go:4-14` registers each). `registry.go:23` `ExecuteSteps(ctx, serverDir, []Step)` implements transactional staging + atomic swap + rollback. |
| Collapse symptom | Pterodactyl/PufferPanel eggs define 24 typed ops with `if:`/`condition:` groups (paper version resolution, fabric installer, asset downloads). Our seeder collapses them to a single `installScript` shell string that does `curl | jq` imperatively. Loss of: (a) retry/rate-limit, (b) checksum verification (`paperdl` requires `expectedSha256`), (c) disk-space guard, (d) condition-aware skip. |
| Opportunity | Re-introduce `install_steps JSONB` (array of `operations.Step`) as *optional* typed pipeline. When empty, fall back to legacy shell script execution (existing behavior). Modded Minecraft is the proof case. |

### 1.5 Frontend — `AdminNestsEggs` + `EGG_TEMPLATES` static array

| Item | Location |
|---|---|
| CRUD | `forge/web/app/admin/nests/[nestId]/eggs/page.tsx:86` (new `EggCard` route, full CRUD modal) + `forge/web/components/admin/AdminNestsEggs.tsx:27` (legacy sidebar table). Both use `fetchEggs(nestId)` → `GET /nests/:nestId/eggs` (`api.ts:1282`). Correct, DB-backed. |
| Variable editor | `forge/web/components/admin/AdminEggVariables.tsx:101` `AdminEggVariables` — drag-reorder, `userViewable`/`userEditable` toggles, rules freeform text. No reserved-name guard, no live regex validation, drag fires `reorderEggVariables` on *every* `dragover` event (`:192`). |
| Gallery / import | `AdminNestsEggs.tsx:133` export JSON, `155` import JSON (naive `dockerImageLines` guard). `AdminTemplates.tsx` (`forge/web/components/admin/AdminTemplates.tsx`) is templates marketplace but reads `fetchTemplates` (`api.ts:493` → `GET /eggs`) — DB only, so with 1 seeded egg the gallery is nearly empty. No PTDL v2 import (expects `meta.version` + `exported_at` envelope, see Pterodactyl export). |
| Static array unused | `egg-templates.ts:27` `EGG_TEMPLATES` 14 entries have correct shape (`EggTemplateItem`), but no component imports it — dead code after `043`. |

### 1.6 Cross-Cutting: CPU, `user_viewable`, Allocations

| Item | Location | Gap |
|---|---|---|
| CPU shares vs CPU% | `store_servers_control.go:97` selects `s.cpu_shares, s.cpu_limit`; `store_servers.go:478` updates both; `beacon/internal/runtime/runtime.go:66` `CPUPercent int64` vs `CPUShares int64`; `docker.go:786` `quota = period * CPUPercent /100`; `kubernetes.go:975` `cpu: "2500m"` | API/egg docs say `cpu` singular; panel persists two columns but Beacon only applies `CPUPercent` to cgroup quota, `CPUShares` to `cpu.shares`. Egg model has **no** `cpu_limit` field — can't template it. |
| `user_viewable` leak | `store_startup.go:31` filters `ev.user_viewable = true` for user-facing `GetServerStartup`; `store_servers_control.go:155` `SELECT ev.env_variable … FROM egg_variables … WHERE srv.id=$1 ORDER BY ev.env_variable` **no filter** for daemon provision (intentional). Risk is admin export leaking hidden vars to non-admin. | Keep daemon full, user-filtered; add API test to enforce. |
| Allocation protocol + containerPort | `migrations/090_allocation_transport.sql:12` adds `protocol tcp/udp` + `container_port`; `store_allocations.go:162` writes them; `store_servers_control.go:96` selects `host(a.ip), a.port` but **ignores** `container_port, protocol` for provision `ServerProvisionTarget` except additional allocations list (`:184`). Legacy `allocationPort` single-int still used in `manager.go:335` `UpdateRuntimeConfig` | Template `serverPort` vs `containerPort` mapping not egg-configurable; egg startup uses `{{SERVER_PORT}}` but container port is implicit `= host port`. |

---

## 2. Requirements & Non-Goals

### 2.1 Must

- GH-14: PTDL v2 imports with `regex:/…/` and `integer`/`numeric`/`in:` pass validation; existing eggs keep working; no DB migration.
- GH-13: Fresh install DB contains **14+ eggs** (the FS set) without manual import; `EGG_TEMPLATES` no longer drifts; single `packages/game-templates` source-of-truth is introduced *without* breaking existing egg IDs.
- GH-15: Either config parsers execute pre-start (4 parsers MVP) or schema docs explicitly mark `config.files` unsupported — with migration/feature-flag path. Decision: implement (table §5).
- Backward compat: `POST /eggs/:id/variables` with legacy `rules: "required|regex:/…/"` still validates; `PUT /eggs/:id` partial update (`store_nests.go:301` retains existing images/config when blank) unchanged.
- Idempotent seed: re-running `DefaultSeeder` or restarting API 10× yields same eggs, no duplicates, no overwrite of admin edits unless forced.
- Additive migrations only; no column drops, no `data loss` `ON CONFLICT` deletes.

### 2.2 Non-Goals (this slice)

- Full 24-op PufferPanel pipeline parity (only modded-Minecraft MVP, §6).
- Wings-level `configMatchRegex` engine (Phase 2).
- Per-allocation protocol UI redesign (track separately; only unblock templating here).
- Deprecating `GET /templates` (`store_templates.go`) — keep shim; remove in major version.

---

## 3. GH-14 — Regex Validation Fix (Detailed)

### 3.1 Root Cause Restated

`forge/api/internal/store/store_egg_variables.go:142-189` `validateVariableValue`:

```go
for _, rule := range strings.Split(rules, "|") {   // BUG 1: splits inside regex alternation
    rule = strings.TrimSpace(rule)
    name, arg, _ := strings.Cut(rule, ":")
    switch name {
    case "regex":
        pattern, err := regexp.Compile(arg)          // BUG 2: arg = "/^a$/", slashes treated as pattern
```

- **Bug 1 — Split:** `strings.Split(rules, "|")` is unaware of regex delimiters. `required|regex:/^(a|b)$/|max:10` → 4 tokens.
- **Bug 2 — Delimiters:** Pterodactyl stores `/^pattern$/` or `/^pattern$/i`. `regexp.Compile("/^a$/")` expects literal `/`.
- **Bug 3 — Missing verifiers:** `integer`, `numeric`, `boolean`, `alpha*`, `between:`, `exists` etc. absent → every integer egg variable fails (`egg-templates.ts:82` uses `integer`).

### 3.2 Target Behavior

| Input `rules` | `value` | Expected |
|---|---|---|
| `required|regex:/^([\\w\\d._-]+)(\\.jar)$/ ` | `server.jar` | pass (paper jar, `egg-templates.ts:80`) |
| `required|regex:/^([\\w\\d._-]+)(\\.jar)$/` | `server|jar` | fail |
| `required|regex:/^(a|b)$/` | `a` | pass |
| `required|regex:/^(a|b)$/` | `c` | fail |
| `required|alpha_num|max:10` | `abc123` | pass (new rule) |
| `required|integer|min:1|max:100` | `20` | pass (new rule, minecraft paper max players) |
| `required|regex:/^foo$/i` | `FOO` | pass (flag `i`) |
| `nullable|string|max:20` | `` | pass |
| `required|string` | `` | fail |
| `in:easy,normal,hard` | `hard` | pass |

### 3.3 Exact Go Implementation (proposed)

> File: `forge/api/internal/store/store_egg_variables.go` — replace `validateVariableValue` + add helpers. Keep signature.

```go
func validateVariableValue(value, rules string) error {
    for _, rule := range splitRules(rules) {
        rule = strings.TrimSpace(rule)
        if rule == "" {
            continue
        }
        // "regex:..." needs special handling because arg may contain colons and slashes
        if strings.HasPrefix(rule, "regex:") {
            raw := strings.TrimPrefix(rule, "regex:")
            pattern, flags, err := parseRegexPattern(raw)
            if err != nil {
                return errors.New("invalid regex validation rule")
            }
            re, err := compileRegexWithFlags(pattern, flags)
            if err != nil {
                return errors.New("invalid regex validation rule")
            }
            if !re.MatchString(value) {
                return errors.New("value does not match the required pattern")
            }
            continue
        }
        name, arg, _ := strings.Cut(rule, ":")
        name = strings.TrimSpace(name)
        arg = strings.TrimSpace(arg)
        switch name {
        case "", "nullable", "string":
            // no-op: nullable/string are type hints, not enforced beyond regex/max
        case "required":
            if value == "" {
                return errors.New("value is required")
            }
        case "max", "min":
            limit, err := strconv.Atoi(arg)
            if err != nil || limit < 0 {
                return fmt.Errorf("invalid %s validation rule", name)
            }
            length := len([]rune(value))
            if name == "max" && length > limit {
                return fmt.Errorf("value must be at most %d characters", limit)
            }
            if name == "min" && length < limit {
                return fmt.Errorf("value must be at least %d characters", limit)
            }
        case "between":
            loHi := strings.Split(arg, ",")
            if len(loHi) != 2 {
                return fmt.Errorf("invalid between validation rule")
            }
            lo, err1 := strconv.Atoi(strings.TrimSpace(loHi[0]))
            hi, err2 := strconv.Atoi(strings.TrimSpace(loHi[1]))
            if err1 != nil || err2 != nil || lo < 0 || hi < lo {
                return fmt.Errorf("invalid between validation rule")
            }
            l := len([]rune(value))
            if l < lo || l > hi {
                return fmt.Errorf("value must be between %d and %d characters", lo, hi)
            }
        case "in":
            allowed := strings.Split(arg, ",")
            found := false
            for _, c := range allowed {
                if value == strings.TrimSpace(c) {
                    found = true
                    break
                }
            }
            if !found {
                return errors.New("value is not in the allowed set")
            }
        case "not_in":
            disallowed := strings.Split(arg, ",")
            for _, c := range disallowed {
                if value == strings.TrimSpace(c) {
                    return errors.New("value is not allowed")
                }
            }
        case "integer":
            if value == "" {
                continue // let required handle emptiness; nullable skips
            }
            if _, err := strconv.Atoi(value); err != nil {
                return errors.New("value must be an integer")
            }
        case "numeric":
            if value == "" {
                continue
            }
            if _, err := strconv.ParseFloat(value, 64); err != nil {
                return errors.New("value must be numeric")
            }
        case "boolean":
            if value == "" {
                continue
            }
            switch strings.ToLower(value) {
            case "true", "false", "1", "0", "yes", "no", "on", "off":
            default:
                return errors.New("value must be a boolean")
            }
        case "alpha":
            if value == "" {
                continue
            }
            if !regexp.MustCompile(`^[A-Za-z]+$`).MatchString(value) {
                return errors.New("value must contain only alphabetic characters")
            }
        case "alpha_num", "alphanum":
            if value == "" {
                continue
            }
            if !regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(value) {
                return errors.New("value must contain only alphanumeric characters")
            }
        case "alpha_dash":
            if value == "" {
                continue
            }
            if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(value) {
                return errors.New("value must contain only alphanumeric, dash, and underscore characters")
            }
        case "regex":
            // already handled via prefix branch — here handles bare "regex" with no arg
            return errors.New("invalid regex validation rule")
        default:
            return fmt.Errorf("unsupported validation rule %q", name)
        }
    }
    return nil
}

// splitRules splits on '|' except when inside a regex:/.../ token.
// It scans left-to-right; when it encounters "regex:" it consumes
// up to the matching unescaped closing '/' (with optional flags).
func splitRules(rules string) []string {
    var out []string
    var cur strings.Builder
    i := 0
    for i < len(rules) {
        if strings.HasPrefix(rules[i:], "regex:") {
            // flush any pending before regex?
            // Actually cur already contains "regex:" prefix start —
            // detect if cur is empty or ends with '|' boundary:
            // Simpler: scan including the prefix.
            start := i
            i += len("regex:")
            // expect opening '/'
            if i < len(rules) && rules[i] == '/' {
                i++ // past opening slash
                escaped := false
                for i < len(rules) {
                    c := rules[i]
                    if escaped {
                        escaped = false
                        i++
                        continue
                    }
                    if c == '\\' {
                        escaped = true
                        i++
                        continue
                    }
                    if c == '/' {
                        i++ // past closing slash
                        // consume flags [a-z]*
                        for i < len(rules) && rules[i] >= 'a' && rules[i] <= 'z' { i++ }
                        break
                    }
                    i++
                }
            } else {
                // bare pattern without slashes: consume until '|' or end
                for i < len(rules) && rules[i] != '|' { i++ }
            }
            cur.WriteString(rules[start:i])
            // if next char is '|', that's the delimiter — emit token
            if i < len(rules) && rules[i] == '|' {
                out = append(out, cur.String())
                cur.Reset()
                i++ // skip '|'
            } else if i >= len(rules) {
                out = append(out, cur.String())
                cur.Reset()
            }
            continue
        }
        if rules[i] == '|' {
            out = append(out, cur.String())
            cur.Reset()
            i++
            continue
        }
        cur.WriteByte(rules[i])
        i++
    }
    if cur.Len() > 0 {
        out = append(out, cur.String())
    }
    return out
}

// parseRegexPattern strips optional /.../ delimiters, extracts flags,
// and unescapes \/ -> /. Returns pattern without delimiters.
func parseRegexPattern(raw string) (pattern string, flags string, err error) {
    raw = strings.TrimSpace(raw)
    if raw == "" {
        return "", "", errors.New("empty regex")
    }
    if raw[0] != '/' {
        // bare pattern without delimiters (Pterodactyl allows both)
        return raw, "", nil
    }
    // find closing unescaped '/'
    escaped := false
    closing := -1
    for idx := 1; idx < len(raw); idx++ {
        c := raw[idx]
        if escaped {
            escaped = false
            continue
        }
        if c == '\\' {
            escaped = true
            continue
        }
        if c == '/' {
            closing = idx
            break
        }
    }
    if closing == -1 {
        return "", "", errors.New("unterminated regex delimiter")
    }
    pattern = raw[1:closing]
    flags = raw[closing+1:]
    // validate flags: only a-z, known set i,m,s
    for _, f := range flags {
        if f != 'i' && f != 'm' && f != 's' {
            return "", "", fmt.Errorf("unsupported regex flag %q", string(f))
        }
    }
    // unescape \/ -> /  (keep other escapes intact)
    pattern = strings.ReplaceAll(pattern, `\/`, `/`)
    return pattern, flags, nil
}

func compileRegexWithFlags(pattern, flags string) (*regexp.Regexp, error) {
    if strings.Contains(flags, "i") {
        pattern = "(?i)" + pattern
    }
    if strings.Contains(flags, "m") {
        pattern = "(?m)" + pattern
    }
    if strings.Contains(flags, "s") {
        // Go RE2: s flag is (?s) dot matches newline
        pattern = "(?s)" + pattern
    }
    return regexp.Compile(pattern)
}
```

**Notes:**

- `splitRules` covers `|` inside alternation `|(a|b)` and character class `[a|b]` (the `|` is inside the `/…/` consumed span, not a delimiter). It also correctly handles `between:1,10` which contains comma but not pipe.
- Empty segments from `||` are skipped by caller (`case ""`).
- `parseRegexPattern` tolerates both `/^a$/` and bare `^a$` (some PTDL exports omit slashes). Both are real in the wild.
- Flags `i,m,s` mapped to RE2 inline flags; `g` ignored (Go has no global, match is full-string via `MatchString`).

### 3.4 Tests — `minecraft-paper.json:58`-style case

> Location: `forge/api/internal/store/store_egg_variables_test.go` (new file; keep `internal/store` package).

```go
package store

import "testing"

func TestValidateVariableValue_RegexSlashDelimiters(t *testing.T) {
    // Exact case from finding: GH-14 + egg-templates.ts:80
    rules := "required|regex:/^([\\w\\d._-]+)(\\.jar)$/"
    if err := validateVariableValue("server.jar", rules); err != nil {
        t.Fatalf("expected pass for server.jar, got %v", err)
    }
    if err := validateVariableValue("server|jar", rules); err == nil {
        t.Fatal("expected fail for server|jar")
    }
    // Without slash stripping, "server.jar" would require literal "/" and fail
}

func TestValidateVariableValue_RegexAlternationPipe(t *testing.T) {
    rules := "required|regex:/^(a|b)$/"
    if err := validateVariableValue("a", rules); err != nil {
        t.Fatalf("a should pass: %v", err)
    }
    if err := validateVariableValue("b", rules); err != nil {
        t.Fatalf("b should pass: %v", err)
    }
    if err := validateVariableValue("c", rules); err == nil {
        t.Fatal("c should fail")
    }
    // Combined with other rules — the original Split bug would produce 4 tokens
    rules2 := "required|regex:/^(easy|hard)$/|max:10"
    if err := validateVariableValue("easy", rules2); err != nil {
        t.Fatalf("easy should pass with max:10: %v", err)
    }
}

func TestValidateVariableValue_RegexFlags(t *testing.T) {
    if err := validateVariableValue("FOO", "required|regex:/^foo$/i"); err != nil {
        t.Fatalf("case-insensitive should pass: %v", err)
    }
    if err := validateVariableValue("FOO", "required|regex:/^foo$/"); err == nil {
        t.Fatal("case-sensitive should fail for FOO")
    }
}

func TestValidateVariableValue_IntegerAndIn(t *testing.T) {
    // egg-templates.ts:82 "required|integer|min:1|max:100"
    rules := "required|integer|min:1|max:100"
    if err := validateVariableValue("20", rules); err != nil {
        t.Fatalf("20 should pass: %v", err)
    }
    if err := validateVariableValue("abc", rules); err == nil {
        t.Fatal("abc should fail integer")
    }
    // string length vs numeric range: max:100 on integers currently checks rune length;
    // keep length semantics for max/min (Pterodactyl does string length), but integer validates numeric type
    // Separate numeric range guard is via min/max on runes; for real numeric bounds use between: or custom
}

func TestValidateVariableValue_Nullable(t *testing.T) {
    if err := validateVariableValue("", "nullable|string|max:20"); err != nil {
        t.Fatalf("empty nullable should pass: %v", err)
    }
    if err := validateVariableValue("toolongvalueexceeding", "nullable|string|max:5"); err == nil {
        t.Fatal("should fail max even when nullable")
    }
}

func TestValidateVariableValue_CharClassPipe(t *testing.T) {
    // | inside character class must not split
    rules := "required|regex:/^[a|b]+$/"
    if err := validateVariableValue("a|b", rules); err != nil {
        t.Fatalf("a|b should pass char-class pipe: %v", err)
    }
}

func TestSplitRules(t *testing.T) {
    cases := []struct{ in string; want int }{
        {"required|regex:/^(a|b)$/|max:10", 3},
        {"required|regex:/^a|b$/|string", 3}, // pipe inside regex, even if ambiguous
        {"nullable|string|max:20", 3},
        {"required|string", 2},
    }
    for _, c := range cases {
        got := splitRules(c.in)
        if len(got) != c.want {
            t.Fatalf("splitRules(%q) = %d parts %v, want %d", c.in, len(got), got, c.want)
        }
    }
}
```

### 3.5 Migration & Call-Site Impact

- No DB migration.
- `store_egg_variables.go:139`, `store_startup.go:73`, `store_servers.go:278` all benefit.
- Add `RESERVED_ENV_NAMES` guard in `validateEggVariableRequest` (`store_egg_variables.go:129`) — block `SERVER_MEMORY`, `SERVER_PORT`, `SERVER_IP`, `P_SERVER_UUID`, `P_SERVER_ALLOCATION_LIMIT` etc. (Wings reserved). See §7 frontend complement.

### 3.6 Rollback

Feature-flag via `rules` string — if new parser breaks, revert commit; old eggs still validate (strictly more permissive before? Actually old rejected valid PTDL — so rollback re-breaks PTDL imports only, safe).

---

## 4. GH-13 — Template Catalog Consolidation (Single Source of Truth)

### 4.1 Principle

> **DB `eggs` is canonical**. Everything else is a projection, seed, or separate product.

| Layer | Role after | Location | Status |
|---|---|---|---|
| `packages/game-templates` | **Source of truth in git** (JSON/TS per template) | `packages/game-templates/templates/*.json` (new) | **Create**. Versioned, reviewed, exported to DB via seeder. |
| `forge/api/internal/store/seeder.go` `DefaultSeeder` | **Idempotent loader** (upsert from FS package at boot) | `seeder.go:38` extended with `seedGameTemplates` | **Extend**. |
| `forge/api/migrations/09x_seed_*.sql` | **Deprecated** for game templates — keep `091_seed_minecraft_java.sql` but make seeder authoritative; migration path retained for transition | `migrations/091_…` stays, new migration only for `install_steps` column | **Freeze**. |
| `forge/web/lib/egg-templates.ts` | **Generated re-export** or **deprecated shim** (reads from DB, not static array) | `egg-templates.ts` | **Deprecate** static `EGG_TEMPLATES` array; replace with `fetchEggs()` gallery (see §7). Keep type `EggTemplateItem` for backwards compat, re-export `ApiEgg`. |
| `forge/web/lib/app-templates-data.ts` | **Not merged** — App Store domain (`app_store_apps`) | `app-templates-data.ts:5` | **Clarify** — rename file to `app-store-defaults.ts`, add header comment "Game eggs live in DB `eggs`; this file is App Store only". |
| `store_nests.go` | **Canonical CRUD** | `store_nests.go` | **Keep**, add `install_steps` JSONB. |
| `store_templates.go` | **Compat shim** | `store_templates.go:14` | **Keep**, add `Deprecated: use ListEggs` godoc, keep `templateFromEgg` but ensure it maps `install_steps` if present. |
| `store_catalog.go` | **Separate product** (managed DB/compose catalog) | `store_catalog.go` | **Keep**, no merge. Add package doc clarifying `catalog_entries != eggs`. |
| `store_app_store.go` | **Separate product** (OCI compose app store) | `store_app_store.go` | **Keep**, no merge. |

```
                    +---------------------------+
                    | packages/game-templates |  <-- git SoT, 14 JSON files, reviewed
                    |  templates/*.json       |
                    +------------+--------------+
                                 |  embedded FS or bind-mount
                                 v
                    +---------------------------+
                    |   DefaultSeeder         |  <-- idempotent UPSERT at API start
                    |   seedGameTemplates()   |      (seeder.go:38)
                    +------------+--------------+
                                 |
                                 v
                    +---------------------------+
                    |   DB: nests + eggs      |  <-- canonical runtime SoT
                    |   (store_nests.go)      |      eggs.config, eggs.install_script,
                    |                         |      eggs.install_steps (new)
                    +-(1)---+----(2)----+-----+
                     |      |           |
          (1) shims  |  (2) user  (3) app/catalog
                     v      v           v
              store_templates  API /eggs  store_catalog / store_app_store
              (compat)         (CRUD)     (separate products)
                                 |
                                 v
                       Web: DB-backed gallery
                       (no static EGG_TEMPLATES)
```

### 4.2 New Source Package — `packages/game-templates`

**Why a package, not just `forge/web/lib`?** `forge/web/lib/egg-templates.ts` is web-only (Next.js bundling). The API seeder (`forge/api`) cannot import `forge/web`. A shared `packages/game-templates` (or `forge/api/seeddata/game-templates/`) is importable by both API (Go embed) and Web (TS). Finding demands `packages/game-templates` on `main` — we align.

**Structure:**

```
packages/game-templates/
  package.json            # name: "@gamepanel/game-templates"
  README.md               # "Add a template: copy minecraft-paper.json → update → run pnpm gen"
  templates/
    minecraft-paper.json      # PTDL v2 envelope, see below
    minecraft-vanilla.json
    palworld.json
    valheim.json
    terraria.json
    enshrouded.json
    satisfactory.json
    rust.json
    csgo.json
    factorio.json
    seven-days-to-die.json
    minecraft-bedrock.json
    teamspeak3.json
    zomboid.json
  dist/
    index.json            # aggregated array, generated
    index.ts              # generated TS re-export for web (optional)
```

**Per-file shape (PTDL v2-compatible, subset we persist):**

```json
{
  "meta": { "version": "PTDL_v2", "update_url": "" },
  "exported_at": "2026-08-24T00:00:00Z",
  "name": "Minecraft (Paper)",
  "author": "GamePanel",
  "description": "High-performance PaperMC …",
  "nest": "Minecraft",
  "docker_images": { "Java 21": "ghcr.io/pterodactyl/yolks:java_21", "Java 17": "ghcr.io/pterodactyl/yolks:java_17" },
  "startup": "java -Xms128M -XX:MaxRAMPercentage=95.0 -Dterminal.jline=false -Dterminal.ansi=true -jar {{SERVER_JARFILE}}",
  "config": {
    "files": {
      "server.properties": {
        "parser": "properties",
        "find": { "server-ip": "0.0.0.0", "server-port": "{{server.build.default.port}}", "query.port": "{{server.build.default.port}}" }
      }
    },
    "startup": { "done": ")! For help, type \"" },
    "stop": "stop",
    "logs": {}
  },
  "scripts": {
    "installation": {
      "container": "ghcr.io/pterodactyl/installers:alpine",
      "entrypoint": "ash",
      "script": "#!/bin/ash\nPROJECT=paper\n…"
    }
  },
  "install_steps": [
    { "type": "paperDl", "args": { "project": "paper", "minecraftVersion": "{{MINECRAFT_VERSION}}", "build": "{{BUILD_NUMBER}}", "filename": "{{SERVER_JARFILE}}", "expectedSha256": "…" } }
  ],
  "variables": [
    { "name": "Minecraft Version", "env_variable": "MINECRAFT_VERSION", "description": "…", "default_value": "latest", "user_viewable": true, "user_editable": true, "rules": "nullable|string|max:20", "sort": 10 },
    { "name": "Server Jar File", "env_variable": "SERVER_JARFILE", "description": "…", "default_value": "server.jar", "user_viewable": true, "user_editable": true, "rules": "required|regex:/^([\\w\\d._-]+)(\\.jar)$/", "sort": 20 }
  ],
  "features": ["eula", "java_version", "pid_limit"],
  "file_denylist": [],
  "force_image": false
}
```

The 14 existing `egg-templates.ts:27` entries map 1:1 to these JSON files — `installScript` → `scripts.installation.script`, `env` → `variables`, `config` unchanged, `features` unchanged. A one-off script `scripts/migrate-egg-templates.ts` converts the TS array to JSON and is then deleted.

### 4.3 DB Migration — Additive Only

**Migration `095_add_egg_install_steps.sql` (next free sequence — verify `ls migrations/ | tail`):**

```sql
-- Add optional typed install pipeline. NULL/empty means "use legacy install_script".
ALTER TABLE eggs
    ADD COLUMN IF NOT EXISTS install_steps JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS script_container TEXT,
    ADD COLUMN IF NOT EXISTS script_entrypoint TEXT;

-- Backfill denormalized container/entrypoint into new columns for query ease (optional)
UPDATE eggs SET script_container = install_container WHERE script_container IS NULL;
UPDATE eggs SET script_entrypoint = install_entrypoint WHERE script_entrypoint IS NULL;

-- Ensure config can store parser metadata without schema change (already JSONB)
-- No change to nests.

-- Index for gallery filtering (optional, low cardinality)
CREATE INDEX IF NOT EXISTS eggs_nest_id_name_idx ON eggs (nest_id, name);
```

**No `DROP` of `install_script`** — both coexist; server provision prefers `install_steps` when `jsonb_array_length(install_steps) > 0`.

### 4.4 Seeder — `DefaultSeeder` Idempotent Upsert

> File: `forge/api/internal/store/seeder.go:38` — extend `DefaultSeeder`.

**Design:**

- Embed `packages/game-templates/templates/*.json` via `go:embed` (or read from `FORGE_TEMPLATES_DIR` env for operator overrides).
- For each JSON file: `nest` (create if missing), `egg` (upsert by `(nest_id, name)`), variables (upsert by `(egg_id, env_variable)`).
- **Idempotent rule:** `ON CONFLICT (nest_id, name) DO UPDATE SET description=EXCLUDED.description, docker_images=EXCLUDED.docker_images, startup=EXCLUDED.startup, config=EXCLUDED.config, default_memory_mb=EXCLUDED.default_memory_mb, install_script=EXCLUDED.install_script, install_container=EXCLUDED.install_container, install_entrypoint=EXCLUDED.install_entrypoint, install_steps=EXCLUDED.install_steps, author=EXCLUDED.author, features=EXCLUDED.features, file_denylist=EXCLUDED.file_denylist WHERE eggs.author = 'GamePanel' OR eggs.author IS NULL` — only overwrite GamePanel-owned eggs; user-custom eggs (different `author` or manually edited) are left alone unless `FORGE_TEMPLATES_FORCE=true`.
- Variables: `ON CONFLICT (egg_id, env_variable) DO UPDATE SET name=EXCLUDED.name, description=EXCLUDED.description, default_value=EXCLUDED.default_value, user_viewable=EXCLUDED.user_viewable, user_editable=EXCLUDED.user_editable, rules=EXCLUDED.rules, sort=EXCLUDED.sort` — but only when `FORGE_TEMPLATES_FORCE` or when `rules`/`default_value` changed upstream and egg is still GamePanel-owned. Otherwise `DO NOTHING` to preserve admin edits to defaults.
- Nests: `INSERT INTO nests (id, name, description) VALUES (...) ON CONFLICT (name) DO NOTHING` (matches `043_unify_eggs_templates_mounts.sql:21` pattern).

**Exact snippet:**

```go
//go:embed seeddata/game-templates/*.json
var gameTemplatesFS embed.FS

func (s *Seeder) seedGameTemplates(ctx context.Context) error {
    entries, err := fs.ReadDir(gameTemplatesFS, "seeddata/game-templates")
    if err != nil {
        // fallback to external dir
        dir := strings.TrimSpace(os.Getenv("FORGE_TEMPLATES_DIR"))
        if dir == "" { return nil }
        entries2, err2 := os.ReadDir(dir)
        if err2 != nil { return nil }
        // ... similar handling
    }
    for _, e := range entries {
        data, err := fs.ReadFile(gameTemplatesFS, "seeddata/game-templates/"+e.Name())
        if err != nil { return err }
        var tpl EggTemplateSeed
        if err := json.Unmarshal(data, &tpl); err != nil { return fmt.Errorf("template %s: %w", e.Name(), err) }

        // 1) ensure nest
        var nestID string
        err = s.store.db.QueryRow(ctx, `
            INSERT INTO nests (id, name, description)
            VALUES ($1, $2, $3)
            ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description
            RETURNING id::text
        `, uuid.NewString(), tpl.Nest, tpl.Nest+" servers").Scan(&nestID)
        if err != nil {
            // if conflict did not return, fetch
            _ = s.store.db.QueryRow(ctx, `SELECT id::text FROM nests WHERE name=$1`, tpl.Nest).Scan(&nestID)
        }

        // 2) upsert egg
        dockerImages, _ := json.Marshal(tpl.DockerImages)
        configJSON, _ := json.Marshal(tpl.Config)
        installSteps, _ := json.Marshal(tpl.InstallSteps)
        if len(installSteps) == 0 { installSteps = []byte("[]") }
        features, _ := json.Marshal(tpl.Features)
        denylist, _ := json.Marshal(tpl.FileDenylist)
        var eggID string
        err = s.store.db.QueryRow(ctx, `
            INSERT INTO eggs (id, nest_id, name, description, docker_images, startup, config,
                              default_memory_mb, install_script, install_container, install_entrypoint,
                              install_steps, file_denylist, author, features)
            VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
            ON CONFLICT (nest_id, name) DO UPDATE SET
                description = EXCLUDED.description,
                docker_images = EXCLUDED.docker_images,
                startup = EXCLUDED.startup,
                config = EXCLUDED.config,
                default_memory_mb = EXCLUDED.default_memory_mb,
                install_script = CASE WHEN eggs.author = 'GamePanel' OR eggs.author IS NULL
                                     THEN EXCLUDED.install_script ELSE eggs.install_script END,
                install_steps = CASE WHEN eggs.author = 'GamePanel' OR eggs.author IS NULL
                                    THEN EXCLUDED.install_steps ELSE eggs.install_steps END,
                file_denylist = EXCLUDED.file_denylist,
                author = EXCLUDED.author,
                features = EXCLUDED.features
            RETURNING id::text
        `, uuid.NewString(), nestID, tpl.Name, tpl.Description, dockerImages, tpl.Startup, configJSON,
            tpl.DefaultMemoryMB, tpl.Scripts.Installation.Script, tpl.Scripts.Installation.Container, tpl.Scripts.Installation.Entrypoint,
            installSteps, denylist, tpl.Author, features).Scan(&eggID)
        if err != nil {
            return fmt.Errorf("upsert egg %q: %w", tpl.Name, err)
        }

        // 3) upsert variables
        for idx, v := range tpl.Variables {
            sort := v.Sort
            if sort == 0 { sort = (idx+1)*10 }
            _, err := s.store.db.Exec(ctx, `
                INSERT INTO egg_variables (id, egg_id, name, description, env_variable, default_value,
                                           user_viewable, user_editable, rules, sort)
                VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
                ON CONFLICT (egg_id, env_variable) DO UPDATE SET
                    name = EXCLUDED.name,
                    description = EXCLUDED.description,
                    default_value = EXCLUDED.default_value,
                    user_viewable = EXCLUDED.user_viewable,
                    user_editable = EXCLUDED.user_editable,
                    rules = EXCLUDED.rules,
                    sort = EXCLUDED.sort
            `, uuid.NewString(), eggID, v.Name, v.Description, v.EnvVariable, v.DefaultValue,
                v.UserViewable, v.UserEditable, v.Rules, sort)
            if err != nil { return err }
        }

        // 4) prune variables removed upstream (only for GamePanel-owned eggs and when force)
        if os.Getenv("FORGE_TEMPLATES_PRUNE") == "true" {
            allowed := make([]string, 0, len(tpl.Variables))
            for _, v := range tpl.Variables { allowed = append(allowed, v.EnvVariable) }
            _, _ = s.store.db.Exec(ctx, `DELETE FROM egg_variables WHERE egg_id=$1 AND env_variable <> ALL($2)`, eggID, allowed)
        }
    }
    return nil
}

type EggTemplateSeed struct {
    Name         string            `json:"name"`
    Nest         string            `json:"nest"`
    Description  string            `json:"description"`
    Author       string            `json:"author"`
    DockerImages map[string]string `json:"docker_images"`
    Startup      string            `json:"startup"`
    Config       json.RawMessage   `json:"config"`
    DefaultMemoryMB int            `json:"-"` // derive or default 1024
    Scripts struct {
        Installation struct {
            Script     string `json:"script"`
            Container  string `json:"container"`
            Entrypoint string `json:"entrypoint"`
        } `json:"installation"`
    } `json:"scripts"`
    InstallSteps []json.RawMessage `json:"install_steps"`
    Variables   []struct{
        Name string `json:"name"`
        Description string `json:"description"`
        EnvVariable string `json:"env_variable"`
        DefaultValue string `json:"default_value"`
        UserViewable bool `json:"user_viewable"`
        UserEditable bool `json:"user_editable"`
        Rules string `json:"rules"`
        Sort int `json:"sort"`
    } `json:"variables"`
    Features []string `json:"features"`
    FileDenylist []string `json:"file_denylist"`
}
```

**Boot wiring:**

```go
func DefaultSeeder(store *Store) *Seeder {
    s := NewSeeder(store)
    s.Register("default-roles", …)
    s.Register("default-settings", …)
    s.Register("game-templates", func(ctx context.Context, store *Store) error {
        return s.seedGameTemplates(ctx)
    })
    return s
}
```

**FS sync for Go embed:** At build time, copy `packages/game-templates/templates/*.json` into `forge/api/internal/store/seeddata/game-templates/` via `make sync-templates` or `go generate`. Add `//go:generate cp -r ../../../../packages/game-templates/templates ./seeddata/game-templates`.

### 4.5 Unified Store Layer — What Changes, What Stays

| File | Action | Detail |
|---|---|---|
| `store_nests.go` | **Extend** | Add `InstallSteps json.RawMessage` to `Egg`, `CreateEggRequest`, `UpdateEggRequest`; extend `SELECT`/`INSERT`/`UPDATE` to include `install_steps`. Keep `normalizeJSONObject` for it (default `[]`). |
| `store_templates.go` | **Keep + annotate** | Add `// Deprecated: Use ListEggs` header. Extend `templateFromEgg` to surface `InstallSteps` if callers need it. No functional change. |
| `store_catalog.go` | **Keep, clarify** | Add package comment: `// catalog_entries are one-click DB/compose services, not game eggs. See store_nests.go for nests/eggs.` No code change. |
| `store_app_store.go` | **Keep, clarify** | Add comment distinguishing `app_store_apps` (OCI compose) vs `eggs`. No merge. |
| `store_egg_variables.go` | **Fix validator** (§3) + add `RESERVED_ENV_NAMES` set (§7). |
| `store_servers.go` / `store_servers_control.go` / `store_nodes.go` | **Extend provision** | When `egg.install_steps` non-empty, populate `ServerProvisionTarget.InstallSteps` for beacon to prefer typed ops. |

**Read path unification:**

```
ListEggs(ctx, nestID)           — canonical (store_nests.go:190)
ListTemplates(ctx)              — shim over ListEggs → Template (store_templates.go:14) — keep for GET /templates compat
ListCatalogEntries(ctx)         — separate (store_catalog.go:89) — NOT merged
ListAppStoreApps(ctx)           — separate (store_app_store.go:48) — NOT merged
```

**Write path unification:**

```
CreateEgg / UpdateEgg / DeleteEgg  — only via store_nests.go
CreateTemplate / UpdateTemplate    — delegates to CreateEgg/UpdateEgg under Games nest (store_templates.go:38)
```

### 4.6 Frontend — DB-Backed Gallery (§7 extended)

Already covered in §1.5; full plan in §7. Key API contract: `GET /nests` + `GET /nests/:id/eggs` already exists (`api.ts:1248`, `1282`). Gallery simply calls it; no new endpoint. Bulk import: `POST /eggs/import` (new) accepts PTDL v2 envelope + `variables` array, validates via `validateVariableValue`, returns created egg. See §7.

---

## 5. GH-15 — Config Files Patcher (Implement vs Remove — Decision Table)

### 5.1 Problem

`eggs.config` shape (Wings/PTDL):

```json
{
  "files": {
    "server.properties": {
      "parser": "properties",
      "find": {
        "server-ip": "0.0.0.0",
        "server-port": "{{server.build.default.port}}",
        "query.port": "{{server.build.default.port}}"
      }
    }
  },
  "startup": { "done": ")! For help, type \"" },
  "stop": "stop",
  "logs": {}
}
```

- `parser` in seed is one of `properties`, `yaml`, `json`, `ini`, `xml`, `file` (per Pterodactyl docs: `file`, `yaml`, `properties`, `ini`, `json`, `xml`).
- `find` maps config key → desired value, where values may contain `{{SERVER_PORT}}`, `{{server.build.default.port}}`, `{{env.VAR}}`.
- Our DB stores it (`eggs.config`) and even fetches it into `ServerProvisionTarget.ConfigJSON` (`store_servers_control.go:96`), but `beacon/internal/server/manager.go:610` `onBeforeStart` never decodes or applies it. Result: `server.properties` port never rewritten, startup done-string not configurable per egg, stop type not derived.

### 5.2 Decision Table

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| **A. Implement patcher in beacon `onBeforeStart` (MVP 4 parsers)** | Wings parity; fixes "stored-but-never-applied" gap; allocation changes auto-reflect in config files | New code in beacon (file I/O, parsers), must handle corruption, must be idempotent | **Chosen — MVP** |
| **B. Explicitly remove `config.files` from schema, document unsupported** | Zero beacon code, honest contract | Breaks PTDL imports that rely on it; `egg-templates.ts` configs become dead docs; diverges from Wings docs; requires migration to drop data | Rejected unless operator explicitly opts into `beacon.config_patcher.enabled=false` |
| **C. Panel-side dry-run only (no beacon write)** | No beacon change | Doesn't satisfy pre-start guarantee; race between panel write and beacon start | Rejected |

**Compromise:** Implement **A** behind feature flag `beacon.config_patcher.enabled` (default `true`), with allow-list of parsers. If operator sets `false`, `config.files` is ignored and documented as unsupported — satisfying "either implement or explicitly remove" phrasing.

### 5.3 Proposed Beacon Implementation

> File: `beacon/internal/server/manager.go:610` `onBeforeStart` — add `applyEggConfigs` step *after* disk check, *before* return.

```go
func (m *ServerManager) onBeforeStart(serverID string, state *ServerState) error {
    // ... existing checks: installing, suspended, synced, root, chown, panel sync, disk ...

    // NEW: apply egg config parsers (behind flag)
    if m.configPatcherEnabled && state.RootDir != "" && len(state.RawConfigJSON) > 0 {
        env := state.EnvVars // already populated by syncServerStateFromPanel
        allocs := map[string]string{
            "server.build.default.ip":   state.AllocationIP,
            "server.build.default.port": fmt.Sprintf("%d", state.AllocationPort),
        }
        if err := applyEggConfigs(state.RootDir, state.RawConfigJSON, env, allocs); err != nil {
            // Log but don't block startup on non-critical parser errors?
            // Decision: block on parser errors if strict, log+continue if permissive.
            // Use strict for now: return fmt.Errorf("apply egg configs: %w", err)
            return fmt.Errorf("apply egg configs: %w", err)
        }
    }
    return nil
}
```

**State augmentation:** `ServerState` needs `RawConfigJSON json.RawMessage` or `string` populated at `Reconcile` + `syncServerStateFromPanel`. Currently `manager.go:31` `ServerState` has no config field. Add two fields:

```go
type ServerState struct {
    // ... existing
    RawConfigJSON json.RawMessage     `json:"-"`
    InstallSteps  json.RawMessage     `json:"-"`
}
```

Hydrated from panel `Configuration` payload (`remote/types.go`): the `egg.config` → `settings.config` chain.

**`applyEggConfigs` package:**

> New package: `beacon/internal/server/configpatch/` (or `beacon/internal/configpatch/`)

```go
package configpatch

type EggConfig struct {
    Files   map[string]FileConfig `json:"files"`
    Startup struct { Done string `json:"done"` } `json:"startup"`
    Stop    string `json:"stop"`
}

type FileConfig struct {
    Parser string            `json:"parser"`
    Find   map[string]string `json:"find"`
}

func Apply(rootDir string, raw json.RawMessage, env, allocs map[string]string) error
```

**Parsers:**

| Parser | File | Implementation | Notes |
|---|---|---|---|
| `properties` | `server.properties` | Line-by-line `key=value` with `#`/`!` comments preserved; `find` keys matched case-sensitive; values rendered via `resolvePlaceholders(value, env, allocs)` | Most common (`egg-templates.ts:66` and `76`). Preserve ordering/comments by read-modify-write, don't rewrite whole file. |
| `yaml` | `config.yml`, `bukkit.yml` | `gopkg.in/yaml.v3` `yaml.Node` round-trip to preserve comments where possible; `find` keys are dot-paths (`server.port`) resolved via `yaml.Node` traversal | Add dependency if not present (check `beacon/go.mod`). |
| `json` | `server.json` | `encoding/json` map; `find` keys dot-path (`settings.port`) | Need dot-path resolver. |
| `ini` | `server.ini` | `gopkg.in/ini.v1` with `UnescapeValueDoubleQuotes=false` | |
| `file` | any | `find` is `filename -> content`? Pterodactyl's `file` parser writes raw values per key as whole file? Actually `file` parser replaces `{{variable}}` occurrences in file body via regex `configMatchRegex` | MVP: if `find` key is filename key, treat value as whole-file template and write after placeholder resolution. |
| `xml` | `server.xml` | Defer to Phase 2 (`encoding/xml` + XPath-lite `find` like `server.port`) | Guard: return `unsupported parser "xml"` when feature-flag `extended_parsers=false`. |

**Placeholder resolution:**

```go
func resolvePlaceholders(raw string, env, allocs map[string]string) string {
    // Pterodactyl placeholders:
    // {{SERVER_PORT}}, {{SERVER_MEMORY}}, {{server.build.default.port}}, {{server.build.default.ip}}, {{env.VAR}}
    // Also bare: {{SERVER_JARFILE}} is same as env[SERVER_JARFILE]
    // We also support Wings' {{server.build.default.port}} mapping.
    out := raw
    for k, v := range env {
        out = strings.ReplaceAll(out, "{{"+k+"}}", v)
        out = strings.ReplaceAll(out, "{{env."+k+"}}", v)
        out = strings.ReplaceAll(out, "{{"+strings.ToLower(k)+"}}", v)
    }
    for k, v := range allocs {
        out = strings.ReplaceAll(out, "{{"+k+"}}", v)
    }
    // also resolve server allocations via env fallback: SERVER_PORT -> allocs[server.build.default.port]
    if v, ok := allocs["server.build.default.port"]; ok {
        out = strings.ReplaceAll(out, "{{SERVER_PORT}}", v)
    }
    return out
}
```

**File I/O safety:**

- `filepath.Join(rootDir, filename)` via `operations.ResolvePath` style containment (`beacon/internal/installer/operations/util.go:11` `ResolvePath`) to prevent `../../etc/passwd`.
- Atomic write: write to `dest.tmp` + `fsync` + `rename` (same pattern as `manager.go:401` `persistPowerState`).
- If file missing, create with `find` entries as initial content (properties: `key=value\n` lines).
- Log per-file at `log.Printf("beacon: patched config %q (%s parser) for %s", filename, parser, serverID)`.

**Config for `startup.done` / `stop`:**

- `startup.done` is the log-match string that indicates server is ready (Wings uses it to mark `running`). Today `manager.go` doesn't consume it; log watcher does. Proposal: persist `startup.done` into `ServerState.StartupProbe` and use in log watcher (future). For now, patch `ServerState` but not act — additive, no behavior change.

### 5.4 Alternative: Explicitly Unsupported Path

If `beacon.config_patcher.enabled=false`, add OpenAPI field description update + `GET /eggs/:id` response adds `configUnsupported: true`, and `POST /eggs` accepts `config` but docs state "stored for reference, not applied at startup — configure `server.properties` manually or enable beacon config patcher". This satisfies audit finding's "document unsupported" escape hatch.

### 5.5 Tests

- `beacon/internal/server/configpatch/configpatch_test.go`: table-driven: properties file with `server-port={{server.build.default.port}}` → after `Apply` with `allocs{port:25566}` file contains `server-port=25566`.
- Missing file creates it.
- Unknown parser returns error.
- Path traversal `../../escape` rejected.
- Placeholder resolution matrix (env, allocs, case).

---

## 6. Install Pipeline — Typed Ops (Minimal, Modded-Minecraft Focus)

### 6.1 Current vs Desired

| Aspect | Current (shell blob) | Desired (typed pipeline + shell fallback) |
|---|---|---|
| Storage | `eggs.install_script TEXT` + `install_container`, `install_entrypoint` (`store_nests.go:46`) | Add `eggs.install_steps JSONB` (`[]operations.Step`, `registry.go:17`). Shell blob remains; `install_steps` takes precedence when non-empty. |
| Execution | Beacon runs `installScript` via `docker exec` in installer container (`runtime/docker.go` path) — unbounded `curl | bash` | Beacon `operations.ExecuteSteps(ctx, serverDir, steps)` (`registry.go:23`) does staged clone, per-step `Execute`, atomic swap. Each op enforces SSRF guard, disk-space guard, retry, timeout (`operations/http.go:20`). |
| PufferPanel parity | 24 ops with `if:` (version-specific branches) flattened to `if [ "$VER" = "latest" ]` shell | Represented as `Condition` (`operation.go:15` `FileExists`/`FileMissing` today; extend to `EnvEquals`, `EnvExists` for `if: { "MINECRAFT_VERSION": "latest" }` parity). |

### 6.2 Minimal Typed Pipeline for Modded Minecraft

**Keep shell blob for 13 simple games.** Add typed pipeline for **Paper + Fabric + Forge + Vanillia** (the ones that need version resolution).

**Egg JSON `install_steps` for `minecraft-paper` (example):**

```json
"install_steps": [
  { "type": "paperDl", "args": { "project": "paper", "minecraftVersion": "{{MINECRAFT_VERSION}}", "build": "{{BUILD_NUMBER}}", "filename": "{{SERVER_JARFILE}}", "expectedSha256": "{{PAPER_SHA256}}" } },
  { "type": "writeFile", "args": { "path": "eula.txt", "content": "eula=true\n", "mode": "0644" }, "condition": { "fileMissing": "eula.txt" } }
]
```

For operators not yet providing `PAPER_SHA256`, `paperDl` factory currently **requires** `expectedSha256` (`paperdl.go:56`). That is too strict for PTDL imports where checksum not exported. Plan: make it optional with warning + fetch-then-verify-api-checksum flow, or add `paperDlUnsafe` alias — decision in implementation PR. Short term: default paper steps *without* `paperDl`, keep shell script until checksum flow designed; typed pipeline MVP is ready infra but not seeded initially.

**Fabric example:**

```json
{ "type": "fabricDl", "args": { "minecraftVersion": "{{MINECRAFT_VERSION}}", "loaderVersion": "{{FABRIC_LOADER_VERSION}}", "filename": "fabric-installer.jar" } }
```

**DB → Beacon dispatch:**

```go
// In ServerProvisionTarget / install handler:
target.InstallSteps = egg.InstallSteps // json.RawMessage
target.InstallScript = egg.InstallScript
target.InstallContainer = egg.InstallContainer
target.InstallEntrypoint = egg.InstallEntrypoint
```

Beacon installer entrypoint:

```go
func (m *ServerManager) RunInstall(ctx context.Context, serverID, rootDir string, target ProvisionTarget) error {
    if len(target.InstallSteps) > 0 {
        steps, err := operations.StepsFromJSON(target.InstallSteps)
        if err != nil { return err }
        // interpolate env into step args before execution?
        // Option: steps already contain "{{VAR}}" placeholders; each op resolves at Execute via serverDir env map
        return operations.ExecuteSteps(ctx, rootDir, steps)
    }
    // legacy shell path
    return m.runShellInstall(ctx, rootDir, target)
}
```

**Condition extension (future, not MVP):**

```go
type Condition struct {
    FileExists  *string `json:"fileExists,omitempty"`
    FileMissing *string `json:"fileMissing,omitempty"`
    EnvEquals   *struct{ Var, Value string } `json:"envEquals,omitempty"`
    EnvExists   *string `json:"envExists,omitempty"`
}
```

### 6.3 Backwards Compatibility

- Existing eggs with `install_script` only: `install_steps = '[]'` → legacy path unchanged.
- New eggs with `install_steps`: beacon branch prefers typed ops; if beacon version predates typed ops, panel falls back to sending `install_script` rendered from steps (generate shell fallback via template expansion) — dual write.
- Downgrade safe: drop column `install_steps` if needed; `install_script` always present.

---

## 7. Frontend Plan

### 7.1 Admin Nests / Eggs CRUD — Current vs Plan

| Surface | Current | Plan |
|---|---|---|
| List | `forge/web/app/admin/nests/[nestId]/eggs/page.tsx:86` `NestEggsPage` + `AdminNestsEggs.tsx:27` sidebar/table — both DB-backed via `fetchEggs(nestId)` (`api.ts:1282`) | **Consolidate**: keep `app/admin/nests/[nestId]/eggs/page.tsx` as primary; deprecate `AdminNestsEggs.tsx` sidebar variant or keep for dashboard widget only. Ensure both share `EggCard` component. |
| Create/Edit | Modal with `Name`, `Description`, `Docker images` (textarea lines), `Startup`, `Stop`, `Install container/entrypoint`, `Features` (textarea), `Install script` (8 rows) (`page.tsx:267`) | **Add:** `Author`, `Update URL`, `File denylist` (JSON), `Config` (JSON editor), `Install steps` (typed ops editor — JSON or visual builder Phase 2). **Improve:** `DockerImages` as key-value `Label → Image` map (today one-per-line loses label). Show `config.files` parser preview. Link to variable editor inline. |
| Clone/Export | `cloneEggMut` (`page.tsx:162`), `exportEgg` Blob (`page.tsx:176`) exports `{name,description,dockerImages,startup,config,installScript,…}` | **Keep**, extend to include `variables` in export (opt-in checkbox). Add `import` parity (see below). |
| Delete | Confirm dialog (`page.tsx:257`) | Keep. Before delete, call `GET /nests/:id/eggs/:id/servers/count` (new or existing) to warn "N servers use this egg". |
| Nest CRUD | `AdminNestsEggs.tsx:72` `createNest/updateNest/deleteNest` (`api.ts`) | Keep; no change. |

**Component split (proposed):**

```
forge/web/components/admin/
  eggs/
    EggGallery.tsx         # DB-backed gallery, import/export, PTDL envelope
    EggEditor.tsx          # Create/Edit modal (shared by gallery + nest page)
    EggVariablesEditor.tsx # wrapper over AdminEggVariables with reserved guard
    EggConfigEditor.tsx    # JSON editor for eggs.config + parser preview
    EggInstallEditor.tsx   # Tabs: Shell Script | Typed Steps (JSON)
```

### 7.2 `EGG_TEMPLATES` Static Array → DB-Backed Gallery

**Step 1 — Deprecate array:**

```ts
// forge/web/lib/egg-templates.ts
/** @deprecated Use GET /nests/:id/eggs via fetchEggs(). This array is frozen for reference only.
 *  Source of truth is packages/game-templates and DB eggs. This file will be removed in v2. */
export const EGG_TEMPLATES: readonly EggTemplateItem[] = [ … ] as const;
```

**Step 2 — Gallery component:**

```tsx
export function TemplateGallery({ nestId }: { nestId?: string }) {
  const { data: nests } = useQuery({ queryKey: ["nests"], queryFn: fetchNests });
  const { data: eggs } = useQuery({ queryKey: ["eggs", nestId ?? "*"], queryFn: () => fetchEggs(nestId ?? "*") });
  // Grid of EggCard with "Install → Create Server" CTA
  // Replaces any hard-coded EGG_TEMPLATES mapping
}
```

**Step 3 — Operator seeding:**

- On `GET /templates` empty, UI shows "No eggs seeded. Run `Admin → Nests → Import → PTDL v2` or ensure API seeded via `packages/game-templates`". No silent empty state.

### 7.3 Import/Export — PTDL v2 Envelope

**Export** (`page.tsx:176` extended):

```ts
function exportPTDLv2(egg: ApiEgg, variables: ApiEggVariable[]) {
  const envelope = {
    meta: { version: "PTDL_v2", update_url: egg.updateUrl ?? "" },
    exported_at: new Date().toISOString(),
    name: egg.name,
    author: egg.author ?? "GamePanel",
    description: egg.description,
    features: egg.features ?? [],
    docker_images: egg.dockerImages,
    file_denylist: egg.fileDenylist ?? [],
    startup: egg.startup,
    config: egg.config, // { files, startup, stop, logs }
    scripts: {
      installation: {
        script: egg.installScript,
        container: egg.installContainer,
        entrypoint: egg.installEntrypoint,
      },
    },
    install_steps: egg.installSteps ?? [],
    variables: variables.map(v => ({
      name: v.name, description: v.description, env_variable: v.envVariable,
      default_value: v.defaultValue, user_viewable: v.userViewable, user_editable: v.userEditable,
      rules: v.rules, sort: v.sort,
    })),
  };
  downloadJSON(`${slug(egg.name)}_ptdl_v2.json`, envelope);
}
```

**Import** (`AdminNestsEggs.tsx:155` `importEgg` hardened):

```ts
async function importPTDLv2(nestId: string, json: unknown) {
  if (!isRecord(json) || typeof json.name !== "string") throw new Error("PTDL v2 requires name");
  const payload: CreateEggInput = {
    nestId,
    name: String(json.name),
    description: String(json.description ?? ""),
    dockerImages: normalizeImages(json.docker_images ?? json.dockerImages),
    startup: String(json.startup ?? ""),
    config: isRecord(json.config) ? json.config : {},
    installScript: String(json.scripts?.installation?.script ?? json.installScript ?? ""),
    installContainer: String(json.scripts?.installation?.container ?? json.installContainer ?? "alpine:3.21"),
    installEntrypoint: String(json.scripts?.installation?.entrypoint ?? json.installEntrypoint ?? "sh"),
    installSteps: Array.isArray(json.install_steps) ? json.install_steps : undefined,
    author: String(json.author ?? ""),
    features: Array.isArray(json.features) ? json.features : [],
    fileDenylist: Array.isArray(json.file_denylist) ? json.file_denylist : [],
  };
  const egg = await createEgg(payload);
  // then POST variables sequentially
  const vars: any[] = Array.isArray(json.variables) ? json.variables : [];
  for (const v of vars) {
    await createEggVariable(egg.id, {
      name: String(v.name), description: String(v.description ?? ""),
      envVariable: String(v.env_variable ?? v.envVariable).toUpperCase(),
      defaultValue: String(v.default_value ?? v.defaultValue ?? ""),
      userViewable: Boolean(v.user_viewable ?? v.userViewable ?? true),
      userEditable: Boolean(v.user_editable ?? v.userEditable ?? true),
      rules: String(v.rules ?? "nullable|string"),
      sort: Number(v.sort ?? 0),
    });
  }
  return egg;
}
```

- Accepts **both** Pterodactyl native export (snake_case `docker_images`, `env_variable`, `scripts.installation`) and our legacy camelCase export.
- Validates `RESERVED_ENV_NAMES` client-side before POST (see below).
- On `rules` containing `regex:` show live preview (green/red badge) before import.

### 7.4 Variable Editor — Reserved Names & Validation Preview

> Current: `AdminEggVariables.tsx:101` `AdminEggVariables` — freeform `rules`, no guard.

**Reserved list (panel-enforced, single source):**

```ts
// forge/web/lib/egg-reserved.ts
export const RESERVED_ENV_NAMES = new Set([
  "SERVER_MEMORY", "SERVER_PORT", "SERVER_IP", "SERVER_UUID", "SERVER_UUID_SHORT",
  "P_SERVER_LOCATION", "P_SERVER_UUID", "P_SERVER_ALLOCATION_LIMIT",
  // Beacon-injected
  "TZ", "ALLOCATIONS_JSON",
]);
```

**UI changes:**

```tsx
// In AdminEggVariables.tsx modal:
const reserved = RESERVED_ENV_NAMES.has(varEnvVariable.toUpperCase());
{reserved && <p className="text-xs text-amber-400">This name is reserved by the panel and cannot be used for egg variables.</p>}

<Input label="Validation Rules" value={varRules} onChange={setVarRules} mono
       error={rulesError} hint={rulesHint} />

// Live preview:
const preview = useMemo(() => tryValidate(varDefaultValue, varRules), [varDefaultValue, varRules]);
{preview.ok ? <span className="text-emerald-400">Default passes</span> : <span className="text-red-400">{preview.error}</span>}
```

**Backend complement:** `store_egg_variables.go:129` `validateEggVariableRequest` adds:

```go
var reservedEnvNames = map[string]bool{ "SERVER_MEMORY":true, "SERVER_PORT":true, /* … */ }

if reservedEnvNames[strings.ToUpper(strings.TrimSpace(req.EnvVariable))] {
    return errors.New("envVariable is reserved and cannot be used")
}
```

**Drag reorder fix:** Current `AdminEggVariables.tsx:192` fires `reorderEggVariables` on *every* `dragover`. Debounce: collect final order on `dragEnd` and fire once.

```ts
const [pendingOrder, setPendingOrder] = useState<string[] | null>(null);
const handleDragOver = (e: React.DragEvent, index: number) => {
  e.preventDefault();
  if (dragIndex === null || dragIndex === index) return;
  const items = [...variables];
  const [moved] = items.splice(dragIndex, 1);
  items.splice(index, 0, moved);
  setDragIndex(index);
  setPendingOrder(items.map(i => i.id)); // don't mutate yet
};
const handleDragEnd = () => {
  if (pendingOrder) reorderMut.mutate(pendingOrder);
  setDragIndex(null); setPendingOrder(null);
};
```

### 7.5 Route Map

```
GET  /admin/nests                      → AdminNestsEggs (nests list)
GET  /admin/nests/:nestId/eggs         → NestEggsPage (eggs grid, CRUD, import/export PTDL v2)
GET  /admin/nests/:nestId/eggs/:eggId/variables → AdminEggVariables (variables CRUD, reserved guard)
GET  /admin/templates                  → AdminTemplates → now renders TemplateGallery (DB-backed)
GET  /admin/templates?nestId=:id       → filtered gallery
```

---

## 8. Migrations — Additive, Idempotent, Backward Compatible

### 8.1 Sequence

| Seq | File | Type | Purpose |
|---|---|---|---|
| 095 | `095_add_egg_install_steps.sql` | DDL | `eggs.install_steps JSONB DEFAULT '[]'::jsonb` (see §4.3) |
| — | `store_egg_variables.go` | Go fix | Regex + integer validators (no DDL) |
| — | `seeder.go` | Go seed | `seedGameTemplates` idempotent upsert (no DDL) — runs at API boot, not migration |

No migration for regex fix — Go-only. No migration for config patcher — beacon-only (no DB). Config patcher flag is env `BEACON_CONFIG_PATCHER_ENABLED` (default `true`).

### 8.2 Idempotency Matrix

| Operation | 1st run | 2nd run (no force) | 2nd run (`FORCE=true`) |
|---|---|---|---|
| Nest `Games` | INSERT | `ON CONFLICT DO NOTHING` | no-op |
| Egg `Minecraft (Paper)` with `author=GamePanel` | INSERT | `ON CONFLICT DO UPDATE` overwrites `description/docker_images/startup/config/install_*` if author still GamePanel | same |
| Egg `Custom Egg` with `author=Acme` | not touched | not overwritten (author guard) | **overwritten only if FORCE** |
| Variable `SERVER_JARFILE` on GamePanel egg | INSERT | `ON CONFLICT DO UPDATE` (keeps rules in sync) | same |
| Variable `CUSTOM_VAR` on game egg (user-added) | not in seed | **not deleted** (prune only with `PRUNE=true`) | deleted if prune |

### 8.3 Backward Compatibility Guarantees

- Existing `eggs` rows: `install_steps` defaults to `[]`, so provision falls through to shell path.
- Existing `egg_variables` with `rules: "required|regex:/…/"` now *pass* where they previously failed — strictly more permissive for PTDL imports, no break for existing servers.
- Existing `servers` rows: `egg_id`/`template_id` trigger (`043_unify_eggs_templates_mounts.sql:160`) keeps them synced; new seeder never changes an egg's `id`, so FKs stable.
- API `GET /nests/:id/eggs` returns `installSteps` as optional field — older web clients ignore unknown field.
- `POST /eggs` without `installSteps` still requires `dockerImages` (existing validation `normalizeDockerImages`).

---

## 9. Cross-Cutting: CPU, `user_viewable`, Allocation Protocol

### 9.1 CPU Shares vs CPU Limit

- Document in `docs/server-lifecycle.md:16` that `cpu_shares` is weight (relative, 10-10000) and `cpu_limit`/`CPUPercent` is quota (`period * percent /100`, Docker `cpu_quota`, K8s `cpu: "2500m"`). Egg template should expose **both** as optional build hints: `config.resources.cpu_shares` and `config.resources.cpu_limit` (new keys) that default to `1024` / `0` (unlimited). Panel `UpdateServer` (`store_servers.go:478`) already persists both; just surface in egg editor (advanced section behind toggle).

### 9.2 `user_viewable`

- No code change beyond test. Add integration test `store_eggs_integration_test.go:TestUserViewableFiltering` asserting:
  - `GetServerStartup` (`store_startup.go:31` filtered) returns only `user_viewable=true` vars.
  - `ServerProvisionTarget` (`store_servers_control.go:155` unfiltered) returns *all* vars (including hidden `DL_PATH`).
  - `ListEggVariables` (admin) returns all.
  - Non-admin `GET /servers/:id/startup` omits hidden.

### 9.3 Allocation Protocol / Container Port

- Already fixed at DB layer (`090_allocation_transport.sql:12`). Templating gap is doc only: document that `{{server.build.default.port}}` resolves to host `port`, and `{{SERVER_PORT}}` is alias. Egg `config.files["server.properties"].find["server-port"] = "{{server.build.default.port}}"` is correct. For games needing distinct container port, operator must create allocation with `container_port != port` and egg `startup` should use `{{SERVER_PORT}}` (host) while config uses container port — add comment in `egg-templates.ts` examples.

---

## 10. Rollout Plan (Phased)

### Phase 0 — Fixes with No Migration (Week 1)

1. `store_egg_variables.go:142` regex + `integer`/`numeric`/`boolean`/`alpha_*` fix (§3.3).
2. `store_egg_variables_test.go` (§3.4) + `go test ./forge/api/internal/store -run TestValidateVariableValue`.
3. `RESERVED_ENV_NAMES` guard in `validateEggVariableRequest`.

### Phase 1 — Source Package + Migration (Week 2)

4. Create `packages/game-templates/templates/*.json` (14 files) + `scripts/migrate-egg-templates.ts` (one-off converter from `egg-templates.ts:27`).
5. `095_add_egg_install_steps.sql` migration.
6. `store_nests.go` extend `Egg` with `InstallSteps`.
7. `seeder.go` `seedGameTemplates` + `go:embed` + `make sync-templates`.

### Phase 2 — Beacon Config Patcher MVP (Week 3)

8. `beacon/internal/server/configpatch/` package (properties/yaml/json/ini) + `manager.go:610` `onBeforeStart` hook + `ServerState.RawConfigJSON` hydration.
9. Feature flag `BEACON_CONFIG_PATCHER_ENABLED` (default true), parser allow-list.
10. `configpatch_test.go`.

### Phase 3 — Typed Install Pipeline (Week 4, optional behind flag)

11. Extend `operations.Condition` with `EnvEquals`/`EnvExists`.
12. Seed `install_steps` for Paper/Fabric/Forge eggs (keep shell fallback).
13. Beacon `RunInstall` typed-path + `provisionTarget.InstallSteps` wiring.

### Phase 4 — Frontend Cutover (Week 4-5)

14. `egg-reserved.ts` + `AdminEggVariables.tsx` reserved guard + live preview + debounced drag.
15. `EggEditor.tsx` / `EggConfigEditor.tsx` / `EggInstallEditor.tsx` + PTDL v2 import/export envelope.
16. Deprecate `EGG_TEMPLATES` array (keep type, add `@deprecated`), replace gallery with `TemplateGallery` (DB-backed). Rename `app-templates-data.ts` → `app-store-defaults.ts` with clarifying header.
17. Verify `forge/web/lib/api.ts:1282` `fetchEggs` + `fetchNests` gallery, import toast coverage.

---

## 11. Verification Checklist

### GH-14

- [ ] `go test ./forge/api/internal/store -run TestValidateVariableValue` passes (slash delimiters, alternation pipe, flags, integer).
- [ ] Import `minecraft-paper.json` with `rules: "required|regex:/^([\\w\\d._-]+)(\\.jar)$/"` via `POST /eggs/:id/variables` succeeds; `POST /servers` with `SERVER_JARFILE=server.jar` succeeds; `SERVER_JARFILE=bad|name` fails.
- [ ] `grep -rn validateVariableValue` still 3 call sites, single fix.
- [ ] Backwards: existing egg with `rules: "required|string|max:64"` (013 seed) still validates.

### GH-13

- [ ] Fresh `TEST_DATABASE_URL` migration run → `SELECT count(*) FROM eggs` ≥ 14 after `DefaultSeeder`.
- [ ] `SELECT * FROM eggs WHERE name='Minecraft (Paper)'` has `docker_images` with both `Java 21`/`Java 17`, `config` has `files.server.properties` parser=properties.
- [ ] Second `DefaultSeeder` run → no duplicate `nests`/`eggs`/`egg_variables`.
- [ ] `GET /nests` + `GET /nests/:id/eggs` returns seeded eggs; `GET /templates` shim returns same count.
- [ ] `packages/game-templates/templates/*.json` count == DB egg count (minus legacy).
- [ ] `localStorage` id `forge.app-templates.v1` no longer confused with eggs — `app-templates-data.ts` renamed/doc'd, tests pass.

### GH-15

- [ ] `beacon/internal/server/configpatch` `go test` passes (properties apply, idempotent, traversal rejected).
- [ ] `manager.go:onBeforeStart` with `RawConfigJSON` containing `server.properties: server-port={{server.build.default.port}}` + `AllocationPort=25566` → file contains `server-port=25566` after `HandlePower("start")` (mock FS test).
- [ ] Flag `BEACON_CONFIG_PATCHER_ENABLED=false` → allocation change does not rewrite file (and is documented).

### Typed Ops

- [ ] `eggs.install_steps` column exists; legacy egg (empty array) still installs via shell script.
- [ ] Paper egg with `paperDl` step (mocked API) writes `server.jar` via `ExecuteSteps` (installer tests).

### Frontend

- [ ] `AdminEggVariables` rejects `SERVER_PORT` as reserved; live preview shows green for `server.jar` / red for empty when `required`.
- [ ] Export PTDL v2 → re-import into fresh nest → egg + 8 variables recreated, `config.files` preserved.
- [ ] Drag reorder fires single `PUT /eggs/:id/variables/reorder` on `dragEnd`, not on every `dragover`.

---

## 12. Risks & Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| `go:embed` bloats API binary with 14 JSON files | +~50KB, negligible | Accept; alternative is runtime `FORGE_TEMPLATES_DIR` mount. |
| Admin edited `Minecraft (Paper)` and re-seed overwrites | Lost customization | Guard `WHERE eggs.author='GamePanel'`; add `FORGE_TEMPLATES_FORCE=false` default; log `templates: skipping custom egg`. |
| Beacon config patcher corrupts `server.properties` | Server fails to start | Atomic write + rollback to `.bak`; on error return `apply egg configs: …` which blocks start — visible in `config_sync_error` column. |
| `xml` parser deferred | PTDL with `xml` parser silently ignored | Log `unsupported parser "xml" for eggs.config.files[server.xml] — skipped (enable extended_parsers)`; document. |
| Web `EGG_TEMPLATES` still imported somewhere | Build fails after deprecation | Keep re-export alias `export const EGG_TEMPLATES = []` deprecated; grep CI `grep -R EGG_TEMPLATES forge/web --include="*.ts" --include="*.tsx"` must be 0 outside `egg-templates.ts`. |
| `cpu_shares` vs `cpu_limit` UI confusion | Operator sets wrong field | Hide `cpu_limit` behind Advanced toggle with tooltip; docs link. |

---

## 13. File-Change Impact Map (no code executed, plan only)

| File | Change | Lines | Reason |
|---|---|---|---|
| `forge/api/internal/store/store_egg_variables.go` | **Edit** | `142-189` | GH-14 fix + reserved guard |
| `forge/api/internal/store/store_egg_variables_test.go` | **New** | — | GH-14 tests (§3.4) |
| `forge/api/internal/store/store_nests.go` | **Edit** | `36`, `59`, `190`, `274`, `292`, `341` | Add `InstallSteps` to `Egg` struct + CRUD |
| `forge/api/internal/store/seeder.go` | **Edit** | `38-92` | GH-13 seeder |
| `forge/api/internal/store/seeddata/game-templates/*.json` | **New** | 14 files | GH-13 source copies |
| `forge/api/migrations/095_add_egg_install_steps.sql` | **New** | 1 file | GH-13/§6 DDL |
| `beacon/internal/server/manager.go` | **Edit** | `31-65`, `610-673` | GH-15 `RawConfigJSON` + `onBeforeStart` hook |
| `beacon/internal/server/configpatch/*.go` | **New** | `configpatch.go`, `properties.go`, `yaml.go`, `json.go`, `ini.go`, `file.go` | GH-15 parsers |
| `beacon/internal/installer/operations/operation.go` | **Edit** | `15-39` | GH typed-ops `Condition` extension |
| `forge/web/lib/egg-templates.ts` | **Edit** | `27` | Deprecate array |
| `forge/web/lib/app-templates-data.ts` | **Rename+Edit** | `1` | GH-13 clarify |
| `forge/web/lib/egg-reserved.ts` | **New** | — | GH-14 frontend guard |
| `forge/web/components/admin/AdminEggVariables.tsx` | **Edit** | `114`, `182`, `322` | GH-14 guard + debounce |
| `forge/web/components/admin/eggs/*` | **New** | 4 files | GH-13 gallery/editor |
| `packages/game-templates/*` | **New** | 14 JSON + `package.json` | GH-13 SoT |

---

## 14. Open Questions for Review

1. Should `paperDl` require `expectedSha256` on first seed, or should MVP seed typed steps *without* checksum and keep shell script as fallback until checksum flow is independently supplied? (Proposed: fallback.)
2. Should `seeder.go` prune removed variables by default, or require explicit `FORGE_TEMPLATES_PRUNE=true`? (Proposed: explicit.)
3. Should beacon `applyEggConfigs` block startup on parser error (strict) or log-and-continue (permissive)? (Proposed: strict for correctness; add env `BEACON_CONFIG_PATCHER_STRICT=false` for permissive.)
4. Should `egg_templates.ts` remain as generated re-export for tooling that `import { EGG_TEMPLATES }`, or fully delete after gallery cutover?
5. Confirm next migration sequence number — `ls forge/api/migrations/*.sql | sort -V | tail -5` on implementer's machine (assumed 095; adjust if 094/095 taken).

---

## 15. References (file:line cited, no URL fabrication)

- `forge/api/internal/store/store_egg_variables.go:15` `eggVariableNamePattern`, `142` `validateVariableValue`, `143` `strings.Split(rules,"|")`, `176` `regexp.Compile(arg)` — GH-14 core.
- `forge/api/internal/store/store_startup.go:31` `ev.user_viewable = true` filter (correct) vs `73` same validator, `store_servers.go:278` integer eggs currently fail.
- `forge/web/lib/egg-templates.ts:27` `EGG_TEMPLATES: EggTemplateItem[]` 14 entries, `80` `regex:/^([\\w\\d._-]+)(\\.jar)$/`, `66` `parser:"properties"` seeded but not consumed, `44` `installScript: #!/bin/ash`.
- `forge/web/lib/app-templates-data.ts:3` `DEFAULT_APP_TEMPLATES` + `58` `localStorage` — app-store domain, not eggs.
- `forge/api/internal/store/seeder.go:38` `DefaultSeeder` only 2 entries, no eggs.
- `forge/api/migrations/091_seed_minecraft_java.sql:3` single egg, `043_unify_eggs_templates_mounts.sql:1` canonical eggs, `007_postgres_core_foundation.sql:87` nests/eggs creation, `013_startup_variables.sql:1` `egg_variables` with `rules`.
- `forge/api/internal/store/store_nests.go:16` `Nest`/`36` `Egg`/`190` `ListEggs`/`248` `CreateEgg`, `store_templates.go:14` shim, `store_catalog.go:89` catalog, `store_app_store.go:48` app store.
- `forge/api/internal/store/store_servers_control.go:95` `e.config::text` fetched, `155` `SELECT ev.env_variable` unfiltered for daemon.
- `beacon/internal/server/manager.go:610` `onBeforeStart` no config logic, `335` `UpdateRuntimeConfig`, `31` `ServerState`.
- `beacon/internal/installer/operations/registry.go:23` `ExecuteSteps`, `operation.go:15` `Condition`, `paperdl/paperdl.go:1` typed op example.
- `forge/web/app/admin/nests/[nestId]/eggs/page.tsx:86` `NestEggsPage`, `forge/web/components/admin/AdminNestsEggs.tsx:27`, `AdminEggVariables.tsx:101`, `AdminTemplates.tsx`, `forge/web/lib/api.ts:1282` `fetchEggs`, `1248` `fetchNests`.
- `migrations/090_allocation_transport.sql:12` protocol/container_port, `011_wings_config_install.sql:1` install columns.

---

*End of plan — ready for prioritized issue creation (GH-14 → GH-13 → GH-15 → typed-ops → frontend).*
