# Scope 7 — Docker runtime, containers, images/registries, compose, installer

Read `.smoke/briefs/00-shared.md` first and obey it. You are agent 7.

Docker is the only verified production runtime in this repo, so this scope should get the deepest real
execution of any: Colima is up, `docker` CLI works, node A's Beacon is online and can start containers.
Use it. Do not settle for "the endpoint returned 200".

## Owned files

`forge/api/internal/http/handlers_docker.go`, `handlers_docker_cleanup.go`, `handlers_docker_events.go`,
`handlers_containers*.go`, `handlers_compose.go`, `handlers_compose_templates.go`, `handlers_registries.go`,
`handlers_images*.go`, `handlers_installer.go`, `handlers_processes.go`, `handlers_portainer.go`,
`handlers_revisions.go`, `handlers_deployment.go`, `handlers_deployment_history.go`,
`handlers_deployment_rollback.go`, `forge/api/internal/services/docker/`, `dockerleanup/`, `compose/`,
`composeTemplates/`, `compsetemplates/` (whatever the real dir name is), `registries/`, `installer/`,
`process/`, `plugins/` (container-plugin paths only), `forge/api/internal/daemon/compose.go`,
`beacon/internal/runtime/docker.go`, `beacon/internal/runtime/registry.go`,
`beacon/internal/runtime/containerd*.go`, `beacon/internal/runtime/podman*.go`, plus matching `_test.go`.
Frontend: `forge/web/lib/api/docker*.ts`, `forge/web/lib/api/compose*.ts`,
`forge/web/components/admin/AdminDocker*.tsx` (coordinate with scope 10 — tell them in your report if you
touch a shared admin component).

Not yours: mounts/bind specs inside the compose service definition (scope 6 owns the mount model, but you
own the compose→container translation of `volumes:` — if a compose volume is dropped, fix the translation
and note it), git/build (scope 8), k8s/incus/nomad/firecracker (scope 9).

## Smoke checklist

Container lifecycle via panel → Beacon → Docker, with `smoke7-*` names:
create, start, stop, restart, kill, remove, pause/unpause if exposed. After each, verify with
`docker ps -a` **and** the panel's state endpoint that the two agree. Specifically hunt the known failure
mode: panel says running, container is exited (or the reverse). Also verify a container that dies on its
own is detected and reported, not left "running" forever.

Stats/logs/exec: stream logs (find the WS route), verify they contain the container's real output and that
`since`/`tail` work; stats must be fresh, not a cached or zeroed reading; verify a stopped container reports
no stats rather than stale ones.

Images and registries: pull a small image (`alpine`, `busybox`) through the panel, list, inspect, delete;
add a registry credential and prove an authenticated pull uses it (Docker Hub anonymous is fine for the
public case; a private registry with fake creds must fail visibly, not silently fall back to anonymous and
report success). Verify image cleanup policies (`/admin/docker-cleanup/policies`) actually remove unused
images and refuse to remove an image in use.

Docker events: `/admin/docker/events` must show real events you just generated (start/stop/die). Verify
ordering, that the list is not empty after activity, and that the source is the live event stream rather
than a re-derived guess.

Compose: write a 2-service `smoke7-*` compose file (web + sidecar with a named volume and a port mapping)
and deploy it. Verify: all services come up, networks are created, volumes exist, health checks gate
readiness, and **deploy failure detail is preserved end-to-end** — a known historical bug here wrapped
errors in a generic `ResponseError` and stripped the container's actual failure message, so deliberately
break one service (bad image tag) and check the API/UI reports *which* service failed and *why*.
Also verify compose down/stop removes everything it created (no orphan network/volume left behind), and
`/admin/compose-templates/:id/instantiate` produces a working stack.

Rollback/history: deploy twice, verify deployment history records both, then roll back and prove the
running container actually reverts (image tag check via `docker inspect`), and that rollback failure is
reported rather than half-applied.

Installer/egg-style workflows: `/servers/:id/install-workflows` and buildpack endpoints
(`/servers/:id/buildpacks`) — verify the list is honest for a node whose runtime cannot support them
(node B is offline; it must not offer installs it cannot perform).

## Report

`.smoke/reports/scope-7.md`. Every claim needs the docker command you used and its output fragment.
Clean up every `smoke7-*` container, network and volume; list anything you could not remove.
