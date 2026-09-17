# Scope 9 — Cloud instances + Kubernetes, both tiers and frontend (audit and FIX)

Read `.audit-reports/briefs/fleet-common.md` first. You are scope 9; report to `.audit-reports/reports/fleet-9.md`.

## You own (edit only these)

API:
- `forge/api/internal/http/handlers_cloud.go` (172 lines, ~7 routes; `registerCloudRoutes` at server.go:2693)
- `forge/api/internal/http/handlers_kubernetes.go` (172 lines, ~6 routes; `registerKubernetesRoutes` at 2571)
- `forge/api/internal/http/handlers_nodeautoscale.go` (135 lines, ~8 routes) if a cloud/scale path routes through it
- `forge/api/internal/store/store_cloud.go`, `store_kubernetes.go`, `store_autoscaler.go`, `store_phase6_autoscale.go`
- `forge/api/internal/services/clustermanager/`, `internal/services/clustermembership/`, `internal/services/autoscaler/`,
  `internal/services/nodeautoscale/` — only what your handlers actually reach; trace first.
- `forge/api/internal/daemon/client.go` — only `KubernetesPods`, `KubernetesDeployments`, `KubernetesServices`,
  `KubernetesScale`, `KubernetesEvents` and their helpers. Surgical.
Beacon:
- `beacon/internal/server/handlers_kubernetes.go`, `kubernetes_stub.go`
- `beacon/internal/runtime/kubernetes.go`, `containerd.go`, `podman.go`, `providers.go`, `factory.go`,
  `provider_containerd_enabled.go`, `provider_firecracker_enabled.go` — but ONLY as they concern kubernetes/cloud
  provider selection and honest capability reporting. `docker.go` and `lxc.go` are not yours.
Web:
- `forge/web/app/admin/cloud/page.tsx`, `kubernetes/page.tsx`
- `forge/web/components/admin/AdminKubernetes.tsx`
- `forge/web/lib/api/kubernetes.ts`, plus the cloud-provider client the cloud page actually imports (trace the
  import; only edit that module)

Read every owned file fully. `kubernetes_stub.go` sitting next to `handlers_kubernetes.go` is the prime suspect for a
stub pretending to be real — read it early.

## Hunt for, with extreme depth

1. **Stub-as-truth**: a Beacon kubernetes stub returning canned or hardcoded data, or returning an empty result with
   a nil error, so the panel renders a fabricated but authoritative-looking inventory. "No cluster configured" must
   be an explicit unavailable state; "cluster reports nothing running" is a different thing. Never fabricate.
2. **Scale/mutate honesty**: `KubernetesScale` reporting success without reading back observed replicas /
   generation; a mutate returning 200 after a partial failure; a cloud create/destroy marked complete while the
   provider operation is still in flight; autoscale decisions persisted as applied when the apply step errored.
3. **Destructive cloud ops**: instance destroy without confirming the instance belongs to the caller's org; no
   idempotency token so a retry provisions a second machine; quota not checked; a read endpoint returning provider
   credentials in plaintext.
4. **Secrets**: kubeconfig, bearer tokens and provider API keys stored unencrypted, logged, or returned by an API.
   Read how `forge/api/internal/store/store_secrets.go` encrypts and use the same path. Check error bodies and logs
   for leaked tokens.
5. **Tenancy/authz**: pod/deployment/service listings not scoped to the caller's org or cluster; a `:namespace` or
   `:node` param parsed and ignored; missing `requireAdminScope` or `adminIPAccess`; cloud write routes missing the
   mutation limiter.
6. **Ambiguous target**: a request that omits the cluster/provider/node and silently gets the first one with
   credentials (`SetDefaultNode`) — reject instead.
7. **Timeouts/cancellation**: kube and provider calls with no context deadline; a slow provider hanging the panel
   request; no backoff on transient 5xx; and equally, a blind retry of a non-idempotent create.
8. **Silent runtime fallback**: a kubernetes/cloud path falling back to docker when the adapter is unavailable or
   build-tag-disabled. Docker is the only verified production runtime — an unavailable provider must fail explicitly.
9. **Store**: swallowed `sql.ErrNoRows`, non-transactional multi-row writes, JSONB written as TEXT, unbounded lists.
10. **Frontend contract**: routes/verbs that do not exist server-side; snake_case/camelCase mismatch; `?? []` and
    `.catch(() => [])` erasing errors; missing empty/loading/error/not-configured/permission states; a control that
    silently picks a default cluster or node (must require an explicit selection); a bare `fetch` inside a component
    instead of `requestJSON`; raw hex and missing i18n keys on new strings.

## Report requirements

Add a `## Fabrication inventory`: every place in your slice that can present data the system never measured, and what
you changed it to. End with `## Cannot verify`.
