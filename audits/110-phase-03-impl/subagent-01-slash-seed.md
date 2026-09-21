# Subagent 01 — Fix regex slash bug + template seeding (GH-14 + TMPL-01)

**Slice:** 110-03-01 / Phase 03 Implementation Agent 01/20  
**Date:** 2026-08-24  
**Branch:** `mvp-2` (HEAD `ca06f74` + working tree)  
**Status:** IMPLEMENTED & VERIFIED

---

## 1. Findings Addressed

| ID | Severity | Location | Defect |
|---|---|---|---|
| **GH-14 / REF-GAME-F-G-08** | P0 | `forge/api/internal/store/store_egg_variables.go:142-189` | `validateVariableValue` used `strings.Split(rules,"|")` which splits alternation and `|` inside char class; `regexp.Compile(arg)` compiled literal slashes `"/^...$/"` so `minecraft-paper.json:58` `SERVER_JARFILE` `server.jar` never matched. Blocks all PTDL v2 imports. |
| **TMPL-01 / GH-16** | P1 | `forge/api/migrations/091_seed_minecraft_java.sql` 1 egg vs `packages/game-templates/templates/*.json` 14 vs `forge/web/lib/egg-templates.ts:27` 14 vs Puffer 44 reference | 93% seeding deficit: fresh DB lists 1 egg, not 14. `DefaultSeeder` only seeded roles/settings; no game-template upsert. |
| **FS phantom** | - | `packages/game-templates` deleted on worktree (`git status --porcelain` showed `D` 14 files) | Restored via `git checkout HEAD -- packages/game-templates` |

---

## 2. Changes — File:Line

### 2.1 `forge/api/internal/store/store_egg_variables.go`

- **Line 142 `validateVariableValue`** — replaced `strings.Split(rules,"|")` with `splitValidationRules(rules)`; added early `nullable` empty check; added `integer` support; fixed `max`/`min` to handle numeric vs length; fixed `regex` to strip `/…/` delimiters with flags.

**Exact code (strip delimiters) — `forge/api/internal/store/store_egg_variables.go:200-226`:**
```go
case "regex":
    patternStr := arg
    if strings.HasPrefix(patternStr, "/") {
        lastSlash := strings.LastIndex(patternStr, "/")
        if lastSlash > 0 {
            inner := patternStr[1:lastSlash]
            flags := patternStr[lastSlash+1:]
            if flags != "" {
                prefix := ""
                if strings.Contains(flags, "i") { prefix += "i" }
                if strings.Contains(flags, "m") { prefix += "m" }
                if strings.Contains(flags, "s") { prefix += "s" }
                if prefix != "" {
                    inner = "(?" + prefix + ")" + inner
                } else {
                    return errors.New("invalid regex validation rule")
                }
            }
            patternStr = inner
        }
    }
    pattern, err := regexp.Compile(patternStr)
```

**Split fix — `forge/api/internal/store/store_egg_variables.go:240-326`:**
- Added `findRegexTokenEnd(s string, start int) (int,bool)` — scans `regex:/…/flags` respecting escaped `\/`, `[...]` char class, flags `[a-zA-Z]*`, and delimiter `|`/`EOF`.
- Added `splitValidationRules(rules string) []string` — collects `regex:` intervals atomically, replaces inner `|` with placeholder `\x1f`, splits on outer `|`, restores placeholder. Guarantees `regex:/^(foo|bar)$/` and `regex:/^[a|b]+$/` remain single tokens.
- Early `nullable` handling — `if value=="" && strings.Contains(rules,"nullable") && !hasRequired { return nil }` so empty `nullable|regex` passes.

**Other additive keeps:**
- Added `case "integer"` (validates `strconv.Atoi`) so `required|integer|min:1|max:100` no longer `unsupported`.
- `max`/`min` now branch on `strings.Contains(rules,"integer")` to compare numeric value vs length, preserving backward compatibility for string limits.

**Additive only:** No DROP, no signature change, dual-read not needed; existing eggs continue to validate.

### 2.2 `forge/api/internal/store/seed_game_templates.go` — NEW (391 lines)

- Created idempotent seeder that upserts **14** curated eggs matching `packages/game-templates/templates/*.json` (14 on-disk) and `forge/web/lib/egg-templates.ts:27`.
- Uses deterministic IDs `uuid.NewSHA1(NameSpaceURL, "gamepanel:template:"+tpl.ID)` and `ON CONFLICT (nest_id,name) DO NOTHING` for eggs, `ON CONFLICT (egg_id,env_variable) DO NOTHING` for variables.
- SQL (additive, idempotent):
```sql
INSERT INTO eggs (id, nest_id, name, description, docker_images, startup, config,
                  default_memory_mb, install_script, install_container, install_entrypoint,
                  file_denylist, author, features)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (nest_id, name) DO NOTHING;

INSERT INTO egg_variables (id, egg_id, name, description, env_variable, default_value, user_viewable, user_editable, rules, sort)
SELECT $1, e.id, $2,$3,$4,$5,$6,$7,$8,$9
FROM eggs e WHERE e.nest_id=$10 AND e.name=$11
ON CONFLICT (egg_id, env_variable) DO NOTHING;
```
- Also validates each variable at seed time via `validateVariableValue` — fails fast if regex bug regresses.
- Fallback hard-coded slice `fallbackGameTemplates` mirrors 14 templates (Paper, Vanilla, Bedrock, Palworld, Valheim, Terraria, Enshrouded, Satisfactory, Rust, CSGO, Factorio, 7Days2Die, TeamSpeak3, ProjectZomboid) so seeding works even if FS missing.
- Ensures `Games` nest exists (`SELECT … FROM nests WHERE name='Games'`, create if missing).

### 2.3 `forge/api/internal/store/seeder.go:38` — Modified

- Added third seeder entry:
```go
s.Register("game-templates", func(ctx context.Context, store *Store) error {
    return store.SeedGameTemplates(ctx)
})
```
Now `DefaultSeeder` contains `default-roles`, `default-settings`, `game-templates` (3 entries). Idempotent, additive.

### 2.4 `forge/api/internal/store/store_egg_variables_test.go` — NEW

- `TestValidateVariableValue_RegexSlash` — 28 sub-tests covering PTDL import, slash delimiters, flags (`/i`), alternation `|`, char-class `|`, palworld decimal regex, without-slash, `| plus max`, integer, string max/in.
- `TestSplitValidationRules_NoSplitInsideRegex` — 4 cases asserting `splitValidationRules` keeps `|` inside regex intact.
- `TestValidateVariableValue_PaperImport` — regression for `minecraft-paper.json:58`.
- `TestSeedGameTemplates_CountAndValidation` — asserts `loadGameTemplates()` returns 14, names unique, each variable default passes fixed validator, and specific GH-14 regressions (`server.jar`, `1.000000`) pass.
- `TestDefaultSeeder_IncludesGameTemplates` — asserts `DefaultSeeder(nil).entries` contains `game-templates` and length ≥3.

### 2.5 `packages/game-templates` — Restored

- Executed `git checkout HEAD -- packages/game-templates` (and `templates/`). Verified `ls packages/game-templates/templates/*.json | wc -l` == 14, `index.json` lists 14 registry entries.

---

## 3. Additive Migration

**None required.** Fix is code-only; seeding is runtime via `DefaultSeeder.SeedGameTemplates` using `ON CONFLICT … DO NOTHING` (no schema change). No DROP, no ALTER. Could be added as `092_seed_game_templates.sql` but task specifies runtime seeder is sufficient.

---

## 4. Test Results

```
go test ./forge/api/internal/store -run TestValidateVariableValue -count=1 -v
  TestValidateVariableValue_RegexSlash (28 sub-tests) — PASS
  TestValidateVariableValue_PaperImport — PASS
go test ./forge/api/internal/store -run TestSplitValidation -count=1 -v — PASS
go test ./forge/api/internal/store -run TestSeedGameTemplates -count=1 -v — PASS
go test ./forge/api/internal/store -run TestDefaultSeeder -count=1 -v — PASS
go vet ./forge/api/internal/store — PASS (no output)
```

Full store suite:
```
go test ./forge/api/internal/store -count=1
  FAIL TestComprehensiveMigrationValidation/sqlite/FreshInstallation — pre-existing duplicate 211 migration prefix (unrelated)
  No failures on new regex/seed tests.
```

Existing eggs still work: `required|string|max:5`, `in:easy,normal,hard,peaceful`, `integer|min:1|max:100` continue to validate; `required` empty still errors; `nullable` empty now correctly passes.

---

## 5. Verification

- **Before fix:** `validateVariableValue("server.jar","required|regex:/^([\\w\\d._-]+)(\\.jar)$/")` failed because `regexp.Compile("/^…$/")` treated slashes literally.
- **After fix:** passes; `go test -run TestValidateVariableValue_RegexSlash/paper_jar_valid` PASS.
- **Split bug:** `validateVariableValue("foo","required|regex:/^(foo|bar)$/")` previously split into `["required","regex:/^(foo","bar)$/"]` and failed `unsupported validation rule "bar"`; now `splitValidationRules` keeps `regex:/^(foo|bar)$/` atomic — PASS.
- **Seeding:** `DefaultSeeder` now seeds 14; `TestSeedGameTemplates_CountAndValidation` confirms 14 and validates palworld `regex:/^\d+\.\d+$/` with `1.000000` PASS; `git ls-files -- packages/game-templates` shows 14 tracked files restored on disk.

---

## 6. Constraints & Non-Breaking

- Additive only: no `DROP`, no column removal, no dual-read needed.
- `ON CONFLICT` upserts ensure reruns are safe on existing DBs with 1 egg or 14 eggs; custom user eggs preserved.
- `intel` flag handling maps `i`/`m`/`s` to Go `(?i)` etc.; unknown flags error as `invalid regex`.
- `integer` added as first-class rule but existing `string`/`nullable`/`in`/`max`/`min`/`regex` unchanged.

---

## 7. Paths Touched (additive slice 01/20 only)

- `forge/api/internal/store/store_egg_variables.go:142-326`
- `forge/api/internal/store/seed_game_templates.go` (new)
- `forge/api/internal/store/seeder.go:64-92`
- `forge/api/internal/store/store_egg_variables_test.go` (new, verifies GH-14)
- `packages/game-templates/templates/*` restored (git checkout, not code edit)

No other slices' files touched.

---

*Generated for 110-Phase-03-Impl Subagent 01. Verify with `go test ./forge/api/internal/store -run TestValidateVariableValue -v` and `ls packages/game-templates/templates | wc -l`.*
