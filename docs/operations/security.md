# Security

## Container Isolation

- No privileged game containers.
- Run containers as non-root where images support it.
- Apply memory and CPU limits.
- Use isolated Docker networks.
- Drop unnecessary Linux capabilities.
- Prefer seccomp and AppArmor defaults.
- Never mount the Docker socket into game containers.

## File Safety

- The frontend never accesses the host filesystem.
- The API delegates file operations to the daemon.
- The daemon sanitizes paths and restricts every path to the server data directory.
- Reject `..`, absolute paths, symlink escapes, and null bytes.
- Reject root directory deletion through the file API.
- Cap initial file reads at 1 MiB and single-shot file writes at 16 MiB.
- Chunked uploads stream through API and daemon with an 8 MiB per-chunk cap and ordered offsets.

## Auth And Node Trust

- Users authenticate with email/password.
- API routes require signed bearer tokens when `API_AUTH_SECRET` is configured.
- Admin/user permissions are enforced with RBAC.
- Server-scoped routes enforce granular subuser permissions with admin and owner bypass.
- Server WebSockets require `websocket.connect` for subusers.
- Panel-to-daemon requests use signed node tokens.
- Signed daemon requests include a timestamp and are rejected outside a five-minute clock-skew window.
- Security-sensitive actions are written to audit logs.
- Rate limits use Redis-backed counters.

## Dependency Audit Notes

- `npm audit --omit=dev` currently reports moderate advisories in upstream `monaco-editor`/`dompurify` and Next's bundled `postcss` with no available fix from npm audit after dependency refresh.
- Monaco is used as a plain code editor for server files, not as an HTML sanitizer or renderer. Do not render edited server file contents as trusted HTML.

## Transfer Safety

- Native daemon SFTP authenticates every password login against the panel remote API.
- SFTP sessions are jailed to the authenticated server data directory.
- SFTP file operations enforce panel-granted file permissions returned by `/api/remote/sftp/auth`.
- The native SFTP server rejects absolute paths, traversal attempts, and null bytes before touching disk.
- rsync should run over SSH/SFTP access to the server data volume, never against raw host paths.
