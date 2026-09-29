# Scope 6 — Infrastructure B: host access + alternative runtimes

Read `.uiux-audit/briefs/00-shared-rubric.md` first and follow it exactly. READ-ONLY on source;
write only `.uiux-audit/reports/scope-6.md`, appending after each page.

All `secondary: true`.

| Registry label | href | files to open |
| --- | --- | --- |
| Host Inspector | `/admin/host` | `app/admin/host/**` |
| Host Files | `/admin/files` | `components/admin/host-files-view.tsx`, `app/admin/files/page.tsx` (uses `AdminPageHeader`) |
| Host Terminal | `/admin/terminal` | `app/admin/terminal/page.tsx` (uses `AdminPageHeader`) |
| SFTP | `/admin/sftp` | `components/admin/AdminSftp.tsx` |
| Image & Cache Cleanup | `/admin/docker-cleanup` | `components/admin/docker-cleanup-manager.tsx` |
| Kubernetes | `/admin/kubernetes` | `components/admin/AdminKubernetes.tsx` |
| Incus | `/admin/incus` | `app/admin/incus/**` |
| Nomad | `/admin/nomad` | `app/admin/nomad/**` |
| NetBird VPN | `/admin/netbird` | `app/admin/netbird/**` |

Focus areas — this slice has the highest risk of **fabricated UI**, so check every number against
a real source:
- **Four runtime consoles (Docker-adjacent, Kubernetes, Incus, Nomad) plus Containers (Scope 5) plus
  Firecracker/LXC/KVM/Postgres etc.** Do all runtime pages share one node-selector + resource-table
  language, or does each invent its own? The project states Docker is the only verified production
  path — so how do Kubernetes/Incus/Nomad present themselves? A runtime page that renders a full
  populated-looking console when the runtime is not installed on any node is **S1**.
- Per AGENTS.md, experimental runtime providers (LXC/KVM/Firecracker) require special node support.
  Verify unsupported runtimes are **disabled with a reason**, never silently absent and never
  enabled-and-empty-looking-like-zero.
- Host Inspector / Host Files / Host Terminal / SFTP are four "get at the machine" surfaces. Check:
  do they all demand an explicit node selection (ambiguous-target rule)? Does the terminal render
  a fake prompt when no session exists? Do file paths, permissions and sizes format consistently
  with Host Files? Is there a root/privileged-action confirmation?
- Terminal and Files are the two pages most likely to break accessibility and keyboard flow — check
  focus management, whether the xterm surface is reachable, and whether a user can tell they are
  on a real host.
- Image & Cache Cleanup: this is a **destructive disk operation**. Check preview-before-prune, an
  honest reclaim estimate (not a hardcoded number), a real confirmation, and whether "reclaimed
  space" is measured or invented.
- NetBird: VPN peers/keys — masking and connected-vs-enabled distinction.
- Look for pages whose data comes from an endpoint that returns `metadata-only` or unsupported, and
  render `0` / `—` / "Healthy" anyway. That is the core honesty failure mode of this slice.
