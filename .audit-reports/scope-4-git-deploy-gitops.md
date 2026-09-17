# Scope 4 — Git-triggered deploy + GitOps

Repo: `/Users/riyaz/forge-plane/forge-test`. Audit + fix pass over the git-triggered deploy and
GitOps path. Constraints honoured: no `go build` / `go vet` / `gofmt` / tests / lint / npm / make
were run; code-only changes.

## Wiring verdict (leads the report)

| Component | Constructor | Start method | Verdict |
| --- | --- | --- | --- |
| `compose.GitOpsController` (claim-based reconciler) | `cmd/api/main.go:859` | `main.go:871 gitOpsController.Start(appCtx)` | **LIVE** — the only wired gitops deployer |
| `compose.GitOpsService.StartPolling` | `handlers_compose.go:84` (lazy fallback; `Config.ComposeGitOpsService` is never assigned — `grep -n ComposeGitOpsService cmd/api/main.go` → no matches) | `StartPolling` | **DEAD** — `grep -rn StartPolling --include=*.go .` matches only its own definition at `gitops.go:1327`; no caller in `cmd/`, `internal/`, or tests |
| `compose.GitOpsService.PollForUpdates` / `checkAndPollStack` | — | called only from `StartPolling` | **DEAD** transitively |
| `git.GitDeploymentService` (deploy.go) | `NewGitDeploymentService` | `TriggerDeployment`, `HandleWebhookEvent` | **DEAD** — `grep -rn NewGitDeploymentService --include=*.go .` matches only the definition (`deploy.go:89`) |
| `compose.GitDeployAdapter` (only impl of `gitsvc.ComposeServiceInterface`) | `NewGitDeployAdapter` | — | **DEAD** — only match is its own definition (`git_deploy_adapter.go:31`); therefore the dead service's compose branch can never be given a real deployer |
| `git.DeploymentManagementService` (webhook → build) | `main.go:857` | via `handlers_git.go:1111` | **LIVE** — `POST /api/v1/git/webhook/deploy/:serverId` (`handlers_git.go:1160`, registered on `v1`, unauthenticated by design, secret-verified) |
| `git.DeployService` | `main.go:853` | `Config.GitDeployService` → `handlers_compose.go:84` (as the controller/service clone engine) and `handlers_git_deploy.go` | **LIVE** |
| `RegisterGitDeploymentRoutes` | — | `server.go:2741` | **MOUNTED** — all four handlers in `handlers_git_deploy.go` are registered (`:230` create, `:231` list, `:232` latest, `:233` cancel); no handler is left unmounted |
| `store.CreateGitDeploymentIfIdle` / `GetActiveGitDeployment` | — | — | **UNWIRED PRIMITIVES** — defined in `store_git_deployments.go`, zero non-test callers before this pass; the dedupe lock they provide was never used. Now wired into `InitiateDeployment`. |
| `reconciler.SetGitOpsService` | — | — | **DEAD** — `grep -rn SetGitOpsService` matches only its definition (`reconciler/service.go:149`); the reconciler's `gitOpsService` field is always nil, so its drift→redeploy branch can never run |

Net: the live gitops path is *webhook → `GitOpsService.HandleWebhook` → store `git_update_status='pending'` →
`GitOpsController.processPending` → claim → `CloneRepo` → verify SHA → `daemon.ComposeDeploy`*. Everything that
auto-updates by polling, and the whole `git.GitDeploymentService` "trigger service", is unwired.

## Half-applied repairs

## Fixed

## Dead / unwired code

## Security findings

## Needs elsewhere

## Checked clean
