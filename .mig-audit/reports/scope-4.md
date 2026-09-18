# scope-4 — canonical migrations 115–138

## SCOPE

Canonical `*.sql` in `forge/api/migrations/` with numeric prefix **115–138 inclusive**
(28 files). Scratch DB `mig_s4`. Owns: content edits to these 28 files only.
Does NOT own: `rollbacks/138_consolidate_legacy_batch2.down.sql` (mysql/rollbacks agent),
`mysql/`+`sqlite/` override dirs (dialect agents), the four orchestrator-owned files
(`060`, `090`, `104_a`, `114_a`), and all Go application code (other audit owns it —
grep read-only).

## Assessment of pre-existing working-tree state (Restart Protocol step 1)

- `git status --short -- forge/api/migrations/` and `git diff HEAD -- forge/api/migrations/115*.sql..138*.sql`:
  **no uncommitted changes in the canonical 115–138 range.** The `128_autoscaler.sql`
  "~34 added lines in-flight diff" described in the dispatch is already **committed**
  (`7389900 chore: commit working tree on mvp-4`); `git diff HEAD -- forge/api/migrations/128_autoscaler.sql`
  is empty. It is therefore read as it stands, no revert needed.
- `find forge/api/migrations -maxdepth 1 -name '*.sql' -mmin -600` → only
  `234_target_group_targets_fks.sql` and `235_schedule_runs_recovered_index.sql` (outside my range).
- Concurrent changes in the migrations dir that are NOT mine and I did not touch:
  `mysql/*` (several), `rollbacks/127_deployments.down.sql`,
  `rollbacks/138_consolidate_legacy_batch2.down.sql`, `rollbacks/104_a_backup_system.down.sql`,
  `sqlite/088_server_parity_fields.sql`.

## PER-FILE LEDGER

(pending — written incrementally)

## FIXES APPLIED

(pending)

## NEEDS NEW MIGRATION

(pending)

## NEEDS OTHER AGENTS' FILES

(pending)

## VERIFICATION

(pending)

STATUS: partial — remaining: full per-file read, double-apply proof on all 28 files, Go read-side greps
