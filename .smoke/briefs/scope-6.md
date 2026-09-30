# Scope 6 — Storage: mounts, host/container files, SFTP, volumes, databases, backups

Read `.smoke/briefs/00-shared.md` first and obey it. You are agent 6.

Mounts were explicitly named by the user and the mounts UI component was the thing that took the whole
dashboard down at campaign start, so this scope is both API work and a careful read of
`forge/web/components/admin/AdminMounts.tsx` (edit that file only if you keep it syntactically valid —
the orchestrator fixed one unbalanced JSX tag there already; run a syntax check before you finish).

## Owned files (`forge/api/`)

`internal/http/handlers_mount_manage.go`, `handlers_files.go`, `handlers_file_download.go`,
`handlers_sftp.go`, `handlers_container_files.go` (container file browsing — the host-file counterpart
lives with scope 9's Beacon, coordinate through NEEDS-ORCHESTRATOR if the bug is agent-side),
`handlers_backups*.go` (`handlers_backup_extended.go`, `handlers_backup_engine.go`),
`handlers_database_services.go`, `handlers_db_containers.go`, `handlers_db_diagnostics.go`,
`handlers_database_hosts*.go`, `handlers_managed_databases.go`,
`internal/services/mounts/`, `internal/services/backup/`, `internal/services/backupengine/`,
`internal/services/dbbackup/`, `internal/services/dbprovisioner/`,
`internal/services/database_service_provisioner.go`, `internal/services/environments/`, `envgroups/`,
`envvars/`, `envmanifest/`, `envaffinity/` (storage-adjacent env plumbing), plus matching `_test.go`.
Beacon: `beacon/internal/sftpserver/**`. Frontend: `forge/web/lib/api/mounts*.ts`,
`forge/web/components/admin/AdminMounts.tsx`, `forge/web/app/admin/mounts/**`,
`forge/web/app/admin/app-mounts/**`, `forge/web/lib/api/backups*.ts`, `forge/web/lib/api/databases*.ts`.

## Smoke checklist

Mounts — the declarative model is `source → target` with `nodeIds`/`templateIds` eligibility and an
`isUserMount` flag. Verify the whole arc with real Docker:
1. `POST /api/v1/mounts` creates a mount whose source is a directory that actually exists on node A
   (create one under the Beacon's managed area, e.g. `.dev-data/beacon/…`), with a valid target.
2. It appears in `GET /mounts`, and `GET /mounts/:id` detail agrees with the list.
3. Attach it to a node and to an egg/template, and prove the attachment round-trips (read `nodeIds`,
   `templateIds` back — an earlier known trap here is `tags`/`nodeIds` stored as TEXT vs jsonb, so
   verify the array survives a reload rather than coming back empty or as a JSON string).
4. Start a `smoke6-*` workload that uses the mount and prove the bind actually exists in the created
   container (`docker inspect` the mounts) — a mount row that never reaches the container spec is S1.
5. Inheritance: a mount eligible via template must reach servers created from that template without being
   re-declared. Verify the inheritance path produces the same result as the direct path.
6. Editing a mount's source/target updates live workloads or clearly reports that a restart is required —
   verify which one it does, and that it does not claim success on the other.
7. Deleting a mount in use must be refused or must detach cleanly; verify no orphan bind survives on the
   host after the workload is recreated.
8. Volume vs bind vs tmpfs vs seed-file mount types: exercise each type the code claims to support and
   report any type that is accepted by validation and then silently ignored.

Host files / SFTP: list/read/write/mkdir/rename/delete through the panel's file endpoints and confirm the
bytes land on disk. SFTP on port 2022: actually connect (e.g. `sftp -P 2022 -i <key>` or the panel's
credentials) and do a real put/get. Verify path traversal is refused (`../../etc/passwd`), that
permissions/quota are enforced, and that a failed transfer is reported as failed.

Container files: `ls`/`read`/`download`/`upload`/`mkdir` on a running `smoke6-*` container. Prove upload
lands inside the container, download returns identical bytes, and reading a binary file is not corrupted
by encoding assumptions.

Databases: provision a managed/db-container service on node A, prove the container exists and the
credentials returned actually connect, then backup → restore → delete. Verify limits
(`databaseLimit`, `backupLimit`) are enforced rather than decorative. `GET /servers/:id/database-services`
and `db-diagnostics` must reflect reality for a stopped container (not report it running).

Backups: engines/repositories (`/admin/backup-engines/repositories`, `/snapshots`), create a backup for a
real server, verify the artifact exists on disk/MinIO/S3 as configured, restore it, verify encryption of
stored secrets, and verify a backup of a stopped workload behaves as documented. A backup that returns
200 and writes no artifact is S1. Check retention/pruning actually deletes.

## Report

`.smoke/reports/scope-6.md`. For each mount type and each file operation, give the command and the
observed evidence (`docker inspect` output fragment, file hash, byte count).
