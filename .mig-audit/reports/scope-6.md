# Scope 6 — migrations 219–235 (tail of the canonical stream)

## SCOPE

I own the canonical `*.sql` in `forge/api/migrations/` with numeric prefix **219–235
inclusive** — **24 files** (the brief's "~25" over-counts; the real set is below). I own
numbering integrity for the tail of the stream. Scratch DB: `mig_s6` (created with
`createdb -T mig_audit_schema`, so it starts from the fully-applied 263-table reference
schema and is writable; `mig_audit_schema` itself was only ever queried).

I did **not** touch `sqlite/`, `mysql/`, `rollbacks/`, or any Go file. Cross-dialect and
Go-side findings are reported in `NEEDS OTHER AGENTS' FILES`.

Verified apply order for my range (produced by re-implementing `migrationPrefix`,
`migrationSortKey` and `validateNoDuplicatePrefixes` over the real directory — see
VERIFICATION):

```
219_add_node_tunnel · 220_add_egg_install_steps · 221_node_heartbeat_metrics ·
221_a_preview_environments · 222_scheduler_policies · 223_notification_channels ·
224_resource_limits_health_checks · 225_tags_and_mounts · 225_a_scheduled_tasks ·
226_pipeline_runs_current_stage · 226_a_docker_cleanup_policies ·
227_deployment_rollbacks · 227_a_backup_restores_data · 228_upgrade_plans ·
228_a_backup_engine · 228_b_domain_redirects · 229_server_runtime_provider ·
229_a_docker_events · 230_a_docker_registries · 231_a_compose_templates ·
232_a_vault_connections · 233_a_git_push · 234_target_group_targets_fks ·
235_schedule_runs_recovered_index
```

## PREFIX-COLLISION REVIEW (owned: numbering integrity)

`migrationPrefix()` (`migration.go:1365-1374`; the brief's 911-920 is stale) makes `221`
and `221_a` **distinct** prefixes, and `migrationSortKey()` (`migration.go:1380-1394`)
orders a bare suffix `""` before `"_a"`/`"_b"`, so bare always applies first. Neither 221,
225, 226, 227 nor 228 is in `allowedHistoricalDuplicates`
(`migration.go:1431-1434` = `{015,018,020,044,054,057,080,082,083,087}`), so **two bare
files sharing any of these numbers would fail `validateNoDuplicatePrefixes` at startup.**

Confirmed for every colliding number: **exactly one bare file + lettered variants**, never
two bare. Full-stream validation over all 245 canonicals returns **no errors** (VERIFICATION
§V1). No fix needed; nothing to allocate.

| Number | Bare | Lettered | Verdict |
| --- | --- | --- | --- |
| 221 | `221_node_heartbeat_metrics` | `221_a_preview_environments` | 1 bare OK |
| 225 | `225_tags_and_mounts` | `225_a_scheduled_tasks` | 1 bare OK |
| 226 | `226_pipeline_runs_current_stage` | `226_a_docker_cleanup_policies` | 1 bare OK |
| 227 | `227_deployment_rollbacks` | `227_a_backup_restores_data` | 1 bare OK |
| 228 | `228_upgrade_plans` | `228_a_backup_engine`, `228_b_domain_redirects` | 1 bare OK |

Note `230`–`233` have **no bare file at all** (only `230_a`…`233_a`). Legal — a lettered
prefix alone is a distinct prefix — but it means `223_notification_channels.sql:16-18`'s
claim that 219+ "share[s] a numeric prefix across parallel files (see 211_a/211_b)"
describes a convention that does not exist. See ledger.

## PER-FILE LEDGER

219_add_node_tunnel.sql: CLEAN
220_add_egg_install_steps.sql: CLEAN
221_node_heartbeat_metrics.sql: CLEAN — metric columns are honest nullable, no `DEFAULT 0`
221_a_preview_environments.sql: `projects.preview_webhook_secret` HMAC key stored PLAINTEXT, breaking the `*_secret_encrypted` family -> FIXED (schema guard added; Go-side rewrite reported, not edited)
222_scheduler_policies.sql: `server_constraints.operator` enum not constrained + no index on two FK `node_id` columns -> FIXED
223_notification_channels.sql: header comment asserts a numbering convention that does not exist (misleading, it is the misconception that causes the duplicate-bare-prefix failure class) -> FIXED (comment only)
224_resource_limits_health_checks.sql: pending
225_tags_and_mounts.sql: pending
225_a_scheduled_tasks.sql: pending (lease/unique claim audit in progress)
226_pipeline_runs_current_stage.sql: pending
226_a_docker_cleanup_policies.sql: pending
227_deployment_rollbacks.sql: pending (119-vs-227 divergence audit)
227_a_backup_restores_data.sql: pending
228_upgrade_plans.sql: pending
228_a_backup_engine.sql: pending
228_b_domain_redirects.sql: pending
229_server_runtime_provider.sql: pending (provider CHECK vs beacon registry)
229_a_docker_events.sql: pending
230_a_docker_registries.sql: `docker_registries_name_idx` is **globally** unique while the table is `user_id`-scoped -> pending
231_a_compose_templates.sql: `created_by TEXT` user reference with no FK/tenant scoping; `values_json` verified SAFE (Go `sanitizeValues` drops secret params) -> pending
232_a_vault_connections.sql: CLEAN — `token_encrypted`/`secret_id_encrypted` present; Go writes `''` into both plaintext columns and hard-fails when encryption is unavailable, so the header's "encrypted column is authoritative" invariant actually holds
233_a_git_push.sql: `shared_secret` HMAC key stored PLAINTEXT (`store_gitpush.go:112-114` writes it raw, `gitpush/service.go:723` reads it raw) while `097:46` encrypts the same kind of secret -> pending
234_target_group_targets_fks.sql: pending (hard audit)
235_schedule_runs_recovered_index.sql: pending

## VERIFICATION (running)

- V1 numbering/apply-order replication over all 245 canonicals: PASS, 0 errors.
- V2 fresh apply of 219–235 into `mig_s6`: pending
- V3 double apply of 219–235 into `mig_s6`: pending

## STATUS

STATUS: partial — remaining: per-file audit of 224, 225, 225_a, 226, 226_a, 227, 227_a,
228, 228_a, 228_b, 229, 229_a, 230_a, 231_a, 233_a, 234, 235; the 119-vs-227 divergence
determination; the 229 provider-CHECK vs `beacon/internal/runtime` registry comparison;
the 234 hard audit; and the double-apply proofs (V2/V3).
