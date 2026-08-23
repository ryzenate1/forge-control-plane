# Releasing — Canonical Version Source

## Summary

**Canonical source: `VERSION` file (`0.1.0`) + git tag `vX.Y.Z`.**

Every artifact — API binary, Beacon binary, Docker images
(`ghcr.io/gamepanel/forge-api`, `ghcr.io/gamepanel/beacon`,
`ghcr.io/gamepanel/forge-web`), K8s manifests, and the UI — derives its
version from this single source. There is no separate `APP_VERSION`,
`package.json` version drift, or ad-hoc `latest` tag.

## Where the version lives

| File | Role |
|------|------|
| `VERSION` (repo root) | Human-readable canonical version. Bumped in PRs that change user-visible behavior. |
| `git tag vX.Y.Z` | Immutable release trigger. Must satisfy `^v?[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9._-]+)?$` and equal `VERSION` (without `v`) at tag point. |
| `forge/api/internal/version.Version` | Go var injected at build: `-ldflags "-X gamepanel/forge/internal/version.Version=$(cat VERSION)"`. Exposed via `GET /api/v1/health` (`version` field) and `GET /admin/system`. |
| `beacon/cmd/daemon.Version` | Injected: `-ldflags "-X main.Version=$(cat VERSION)"`. Exposed via heartbeat `remote.NodeHeartbeat.Version` and `/health`. |
| `forge/web` `NEXT_PUBLIC_APP_VERSION` | Build arg `VERSION` in `forge/web/Dockerfile` → `NEXT_PUBLIC_APP_VERSION`. Package.json `version` must equal `VERSION` (checked in CI). The web UI displays the API's `/health` version at runtime, so web and API stay coherent. |
| `infra/compose.yml` `TAG` | The only env var that pins images: `ghcr.io/gamepanel/<svc>:${TAG}`. In production `TAG` is the release tag (e.g. `0.1.0` or `v0.1.0` — publish publishes both). `APP_VERSION` is deprecated; compose now defaults it to `${TAG}`. |
| `infra/ship/kubernetes/*.yaml` | Must use `${TAG}` placeholder (`ghcr.io/gamepanel/<svc>:${TAG}`). Substitute before `kubectl apply` (e.g. `TAG=$(cat VERSION) envsubst < manifest | kubectl apply -f -`). Do NOT hard-code `v0.1.0` or `:latest`. |
| `infra/.env.example` / `.env.example` | Document that `TAG` is required; `APP_VERSION` mirrors `TAG`. |
| `CHANGELOG.md` | Keep-a-Changelog entries per release; links versions to git tags. |

## Cutting a release

```bash
# 1. Bump VERSION file and CHANGELOG.md in a PR
echo "0.2.0" > VERSION
# edit CHANGELOG.md – move Unreleased → [0.2.0]
# also update package.json sync (see below)

# 2. Verify all version injection points
./scripts/release/check-version.sh    # checks VERSION == package.json, no hard-coded latest/v0.1.0
./scripts/release/verify-artifacts.sh # optional: builds locally and checks image labels

# 3. Tag and push (triggers both workflows)
git tag -a v0.2.0 -m "Release v0.2.0" && git push origin v0.2.0

# 4. CI produces:
#   - .github/workflows/publish-images.yml → ghcr.io/gamepanel/{forge-api,beacon,forge-web}:0.2.0 and :v0.2.0
#   - .github/workflows/release-beacon.yml → beacon_linux_amd64, beacon_linux_arm64, checksums.txt, checksums.txt.sig as GitHub Release assets
# Verify in Actions → Publish Images + Release Beacon, then in Releases that
# the Docker images are pullable with the same TAG:
#   docker pull ghcr.io/gamepanel/forge-api:0.2.0
#   docker pull ghcr.io/gamepanel/forge-api:v0.2.0
```

## Local / dev builds

Local builds use `VERSION=dev` (no tag, no ldflags injection). `forge/api/internal/version.Version` defaults to `"dev"` and `beacon/cmd/daemon.Version` to `"beacon-dev"`. The beacon `update` command refuses to self-update from `dev` unless `--force` is given. `infra/gen-env.sh` sanitizes `git describe --tags --always` into `TAG` (dev fallback), so `docker compose build` still works.

## Package.json sync

`forge/web/package.json` `version` and root `package.json` `version` must stay equal to `VERSION` (without `v`). CI does not auto-mutate files; instead:

```bash
./scripts/release/sync-package-version.sh   # writes VERSION into both package.json files (preserves JSON formatting)
npm run build:packages && npm --workspace @forge/web run typecheck
```

Run this script as part of any PR that bumps `VERSION`.

## Docker image tags

- Release images are immutable and published with **both** `0.2.0` and `v0.2.0` tags so existing `TAG=v0.2.0` compose files keep working.
- **No `latest` is published.** Consumers must pin `TAG`. K8s manifests use `IfNotPresent` (not `Always`) to avoid non-reproducible pulls.
- Labels: every image carries `org.opencontainers.image.version`, `.revision`, `.created` from build args.

## K8s deployment

```bash
export TAG=$(cat VERSION)  # or explicit tag: v0.2.0
# Optionally use kustomize:
# kustomize build infra/ship/kubernetes | TAG=$TAG envsubst | kubectl apply -f -
envsubst < infra/ship/kubernetes/api-deployment.yaml | kubectl apply -f -
envsubst < infra/ship/kubernetes/beacon-daemonset.yaml | kubectl apply -f -
envsubst < infra/ship/kubernetes/web-deployment.yaml | kubectl apply -f -
```

## CHANGELOG policy

- Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) + [Semantic Versioning](https://semver.org/).
- Location: `CHANGELOG.md` at repo root (see file for template).
- Every user-facing change must add an entry under `## [Unreleased]`.
- The release PR moves `Unreleased` entries into a versioned section and adds the compare link.
- CI is not gating on CHANGELOG yet, but `check-version.sh` warns if `CHANGELOG.md` lacks an entry for the new `VERSION`.

## Stale snapshot tags

Old `freebuff-snapshot/*` tags are temporary development snapshots and not
releases. They must not be fetched as images. Clean them with:

```bash
./scripts/release/cleanup-snapshots.sh --list          # list local snapshot tags
./scripts/release/cleanup-snapshots.sh --delete-local  # delete local only
./scripts/release/cleanup-snapshots.sh --delete-remote # delete remote (requires confirmation)
```

Never run `--delete-remote` on CI without review — tags are immutable once referenced.

## Verification checklist (for PRs / releases)

- [ ] `VERSION` bumped and equals tag (minus `v`)
- [ ] `CHANGELOG.md` updated
- [ ] `./scripts/release/sync-package-version.sh` run
- [ ] `check-version.sh` passes (no hard-coded `latest`, `v0.1.0`, or stale `dev` claim in compose/K8s/Dockerfile)
- [ ] `publish-images.yml` produced pullable images for `${TAG}`
- [ ] `release-beacon.yml` attached signed checksums + both arch binaries
- [ ] K8s manifests still use `${TAG}` placeholder, not `latest`
- [ ] `APP_VERSION` not diverged from `TAG`
