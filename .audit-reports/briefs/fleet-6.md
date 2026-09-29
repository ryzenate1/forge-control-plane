# Scope 6 — Mounts, Host inspector, Host/container files: BACKEND (audit and FIX)

Read `.audit-reports/briefs/fleet-common.md` first. You are scope 6; report to `.audit-reports/reports/fleet-6.md`.

## You own (edit only these)

API:
- `forge/api/internal/http/handlers_mount_manage.go` (173 lines; registrar `("app-mounts", 210, …)` — the base
  `/mounts` CRUD inside `handlers_admin.go` belongs to scope 1, so stay in your file)
- `forge/api/internal/http/handlers_host.go` (156 lines, ~5 routes; `registerHostRoutes` at server.go:2675)
- `forge/api/internal/http/handlers_files.go` (522 lines, ~22 routes; `registerHostFileRoutes` at server.go:2650)
- `forge/api/internal/http/handlers_file_download.go` (212 lines)
- `forge/api/internal/http/handlers_container_files.go` (452 lines; registrar `("container-files", 220, …)`)
- `forge/api/internal/services/mounts/`
- `forge/api/internal/store/store_mounts_ext.go`, `store_app_mounts.go`
- `forge/api/internal/daemon/client.go` — ONLY `HostFiles*` (List/Read/Write/Upload/Download/Mkdir/Remove/Rename/
  Copy/Chmod), the generic file methods (`ListFiles`, `ReadFile`, `WriteFile`, `DeleteFile`, `DeleteFiles`,
  `RenameFile`, `MakeDir`, `ChmodFile`, `CopyFile`, `ArchiveFiles`, `DecompressFile`, `UploadFileChunk`), and
  `AdminContainerFiles*`. Surgical only.
Beacon:
- `beacon/internal/server/mounts.go`, `hostfiles.go`, `secure_files.go`, `container_files.go`, `handlers_host.go`,
  `diskcheck.go`, `sysinfo_linux.go`, `sysinfo_darwin.go`, `sysinfo_windows.go`

**Read first, as the contract:** `beacon/internal/server/hostfiles_confinement_test.go`, `hostfiles_test.go`,
`files_test.go`, `secure_files_test.go`. The implementation must be at least as strict as those tests — if a test
encodes a weaker guarantee than the code should have, say so rather than relaxing the code to match.

## Hunt for, with extreme depth

1. **Path traversal / jail escape (highest priority).** For every path-taking operation:
   - user segments joined with `filepath.Join` and then used without a post-resolution containment check;
   - `..`, absolute paths, empty segments, encoded separators, `%2e%2e`, NUL and newline injection;
   - containment tested with a prefix comparison in the wrong direction or against an uncleaned/unabsolutised path
     (`strings.HasPrefix(root, p)` is an inversion bug; compare cleaned absolute paths and require the separator
     boundary);
   - symlinks: root resolved once at the top but components followed afterwards; `EvalSymlinks`/`EvalLookPath`
     applied before rather than after resolution; **rename/copy/move/chmod re-targeting outside the jail** — these
     secondary ops are usually the ones the read tests do not cover;
   - per-tenant scoping: a caller reading another org's host path by supplying a node id plus an absolute path;
   - container-file paths resolved in the wrong mount namespace or escaping the container root onto the host.
2. **Bind mounts as a container-escape primitive**: the API accepting an arbitrary host path from a caller for a
   bind mount. Bind sources must be restricted to provisioned mount roots; validate on write, not just on read.
3. **Mount lifecycle**: delete or unmount while a workload still references the mount; a `CleanupMount` error
   ignored; unmount best-effort errors reported as success; a mount config persisted but never delivered to the
   Beacon, or delivered without checking the mount belongs to that node/server/egg; `readOnly` defaulting to
   writable; volume-vs-bind-vs-tmpfs type confusion.
4. **Host inspector honesty**: metrics read from a stale cache presented as live; a failed collection path returning
   a zero-value struct with `err == nil`; **values a platform cannot produce reported as 0 instead of unsupported**
   — this is a macOS dev box, so scrutinise `sysinfo_darwin.go` and `diskcheck.go`; process listings leaking secrets
   in command lines or environment; process/memory listing endpoints lacking admin scoping.
5. **Files**: chunked upload without size/quota enforcement, no reassembly validation, TOCTOU between finalize and
   move, unbounded temp growth, path traversal in the chunk id; downloads missing
   `Content-Disposition` escaping, no range support, path taken from a query param, or an error body streamed with
   status 200; `ChmodFile` accepting setuid/setgid bits; writes bypassing the configured size limit; delete of a
   path outside the jail via rename-then-delete.
6. **Ambiguous node targeting** in every file/mount/host operation — reject when the node is unnamed.
7. **Unknown-vs-zero** in file listings and disk usage: an unreadable directory must not render as empty.

## Report requirements

Add a `## Confinement audit` section: each path-consuming operation, the jail root it uses, where the containment
check happens relative to symlink resolution, and the payload you would send to escape it. State explicitly which
operations you proved confined and which you could not. End with `## Cannot verify`.
