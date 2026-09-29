# Scope 2 — canonical `*.sql` 043–097 (57 files) migration audit

WORK IN PROGRESS — sections filled incrementally per the BRIEF Restart Protocol.

## REPORT PATH NOTE (read first)
The dispatch message told me to write to `.audit-reports/briefs/scope-2.md`. That path is
**already occupied by a different, concurrent audit's dispatch brief** ("Scope 2 — Compose HTTP +
stack templates", owner of `handlers_compose.go` etc). BRIEF.md forbids writing into
`.audit-reports/` ("not yours", the app-code audit). Writing there would clobber that agent's brief.
Per BRIEF.md Output format and consistent with sibling migration agents (scope-1/4/5 already wrote
here), this report is at `.mig-audit/reports/scope-2.md`. No file was overwritten.

## SCOPE
- Canonical `forge/api/migrations/*.sql` with numeric prefix 043–097 inclusive (57 files on disk).
  Note the intentional gap 061–076 (retired), so the range is 043–060 then 077–097.
- Guard files I own: `044_cloud_node_links`, `054_activity_events`, `054_social_auth`,
  `057_job_queue`, `057_webauthn`, `057_backup_policies`, `080_recovery_execution_statuses`,
  `087_parity_schema`.
- EXCLUDED from editing (orchestrator-owned, currently being fixed): `060_region_slug_normalization.sql`,
  `090_allocation_transport.sql`.
- Read-only for cross-checks: `086_add_table_constraints.sql` (mine to audit), `123/125/126/127/128/133_b`
  (outside range — report only), `234_target_group_targets_fks.sql` (another agent's file — read only).
- Scratch DB: `mig_s2` (created). Reference: `mig_audit_schema` (read-only).

## KEY MACHINERY (verified in internal/store/migration.go)
- `migrationPrefix` (1365): `044_a_cloud_node_links` -> prefix `044_a`; `044_async_delivery_foundation`
  and `044_cloud_node_links` -> prefix `044`. Bare and letter-suffixed prefixes are counted separately.
- `validateNoDuplicatePrefixes` knownMax (1467): {015:2,018:2,020:2,044:2,054:2,057:3,080:2,082:3,083:2,087:2}.
  **VERIFIED against disk** bare-prefix counts in range: 044=2, 054=2, 057=3 (all three are the guards),
  080=2, 082=3, 083=2, 087=2. knownMax matches disk — the 057 3-file set and 082 3-file set are correct
  and no guard edit pushed a count over the max. No change needed.
- `migrationAliases` (1282): all 8 of my guards are registered guard->canonical pairs and the canonical
  files exist on disk. (Details in GUARD LEDGER below.)
- `fkFollowupMigrations` (1306): `target_group_targets` now maps to `234_target_group_targets_fks.sql`
  (previously a tracked gap `""`). Verified 234 exists and is analysed under FK FOLLOWUP below.
