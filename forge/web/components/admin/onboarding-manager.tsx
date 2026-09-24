"use client";

import { useEffect, useState } from "react";
import { AdminCard, AdminPageLayout } from "@/components/admin/admin-layout";
import { OfflineBanner } from "@/components/shared/states-offline";
import * as api from "@/lib/api/onboarding";
import { sanitizeError } from "@/lib/sanitize";

export function OnboardingManager() {
  const [status, setStatus] = useState<api.StatusView | null>(null);
  const [repos, setRepos] = useState<api.GitProviderRepo[]>([]);
  const [branches, setBranches] = useState<api.GitProviderBranch[]>([]);
  const [loadedSources, setLoadedSources] = useState<api.LoadedSource[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [providerId, setProviderId] = useState("");
  const [repoName, setRepoName] = useState("");
  const [deployPayload, setDeployPayload] = useState({ repo: "", branch: "main", buildType: "nixpacks", name: "", port: 8080 });
  const [deployResult, setDeployResult] = useState<api.DeployResult | null>(null);

  const [connectForm, setConnectForm] = useState({ provider: "github", providerName: "", accessToken: "", username: "", baseUrl: "" });

  async function loadStatus() {
    try {
      const s = await api.fetchOnboardingStatus();
      setStatus(s);
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Status failed"));
    }
  }

  async function loadRepos() {
    try {
      const r = await api.listRepos(providerId || undefined);
      setRepos(r);
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "List repos failed"));
    }
  }

  async function loadBranches() {
    if (!repoName.trim()) {
      setError("repo required");
      return;
    }
    try {
      const b = await api.listBranches(repoName.trim(), providerId);
      setBranches(b);
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "List branches failed"));
    }
  }

  async function loadSources() {
    try {
      const s = await api.listLoadedSources();
      setLoadedSources(s);
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Load sources failed"));
    }
  }

  useEffect(() => {
    void loadStatus();
    void loadSources();
  }, []);

  async function handleConnect() {
    setError(null);
    try {
      await api.connect(connectForm);
      setSuccess("Provider connected");
      await loadStatus();
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Connect failed"));
    }
  }

  async function handleDeploy() {
    setError(null);
    try {
      const res = await api.deploy({ repo: deployPayload.repo, branch: deployPayload.branch, buildType: deployPayload.buildType, name: deployPayload.name || undefined, port: deployPayload.port });
      setDeployResult(res);
      setSuccess(`Deployed ${res.appName} → ${res.url}`);
      await loadStatus();
    } catch (e) {
      setError(sanitizeError(e instanceof Error ? e.message : "Deploy failed"));
    }
  }

  return (
    <AdminPageLayout
      title="Onboarding Wizard"
      description="Connect repo → get URL. Phase 8 scaffold: no remote registry push; returns queued deployment + demo URL. Endpoints: GET /onboarding/status, GET /onboarding/repos, GET /onboarding/repos/:repo/branches, POST /onboarding/connect, POST /onboarding/deploy, GET /ide/files."
      breadcrumbs={[{ label: "Admin", href: "/admin/onboarding" }, { label: "Onboarding" }]}
    >
      <OfflineBanner onRetry={() => { void loadStatus(); void loadSources(); }} />
      <div className="flex items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-3 py-2 font-mono text-[11px] text-[var(--text-subtle)]">
        <span className="h-2 w-2 rounded-full bg-[var(--brand)]" />
        <span>onboarding</span>
        <span className="text-[var(--text-subtle)]">::</span>
        <span className="text-[var(--brand)]">wizard</span>
        <span className="ml-auto hidden sm:inline uppercase tracking-widest text-[var(--text-subtle)]">var(--brand) var(--canvas) var(--surface) var(--line)</span>
      </div>
      {error && (
        <div role="alert" className="flex items-center justify-between gap-3 rounded-xl border border-red-500/25 bg-red-500/[0.09] p-4 text-sm text-red-200 motion-safe:transition-colors motion-reduce:transition-none">
          <span>{error}</span> <button onClick={() => setError(null)} className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
        </div>
      )}
      {success && (
        <div role="status" className="flex items-center justify-between gap-3 rounded-xl border border-emerald-500/25 bg-emerald-500/[0.09] p-4 text-sm text-emerald-200 motion-safe:transition-colors motion-reduce:transition-none">
          <span>{success}</span> <button onClick={() => setSuccess(null)} className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <AdminCard title="Status" description="GET /onboarding/status — connected providers, source count, template keys, OAuth link.">
          {status ? (
            <div className="space-y-2 text-sm text-[var(--text)]">
              <p>Connected: {String(status.connected)} · Sources: {status.sourceCount} · Has apps: {String(status.hasApps)}</p>
              <p className="text-xs text-[var(--text-subtle)]">Providers: {(status.providers ?? []).join(", ") || "none"} · Templates: {(status.templateKeys ?? []).join(", ")}</p>
              {status.oauthUrl && <a href={status.oauthUrl} target="_blank" rel="noreferrer" className="text-xs text-[var(--brand)] underline hover:text-[var(--brand-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] rounded">GitHub OAuth →</a>}
            </div>
          ) : (
            <p className="text-sm text-[var(--text-subtle)]">Loading…</p>
          )}
          <button onClick={() => void loadStatus()} aria-label="Refresh status" className="mt-3 rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-3 py-1.5 text-xs text-[var(--text)] hover:bg-[var(--surface-hover)] motion-safe:transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface)]">Refresh Status</button>

          <div className="mt-6 space-y-3 rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3">
            <p className="text-xs font-bold uppercase tracking-[0.12em] text-[var(--text-subtle)]">Connect Provider (paste token)</p>
            <input value={connectForm.provider} onChange={(e) => setConnectForm({ ...connectForm, provider: e.target.value })} placeholder="provider (github)" className="w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
            <input value={connectForm.accessToken} onChange={(e) => setConnectForm({ ...connectForm, accessToken: e.target.value })} placeholder="accessToken" className="w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
            <div className="grid grid-cols-2 gap-2">
              <input value={connectForm.username} onChange={(e) => setConnectForm({ ...connectForm, username: e.target.value })} placeholder="username" className="rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
              <input value={connectForm.baseUrl} onChange={(e) => setConnectForm({ ...connectForm, baseUrl: e.target.value })} placeholder="baseUrl (optional)" className="rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
            </div>
            <button onClick={() => void handleConnect()} aria-label="Connect provider" className="rounded-lg bg-[var(--brand)] px-4 py-2 text-xs font-bold text-white hover:bg-[var(--brand-hover)] motion-safe:transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface)]">Connect</button>
          </div>
        </AdminCard>

        <AdminCard title="Deploy" description="POST /onboarding/deploy — template pick → app + demo URL. Builder: static|node|next|nixpacks|dockerfile|heroku → nixpacks|dockerfile|static etc.">
          <div className="space-y-3">
            <input value={deployPayload.repo} onChange={(e) => setDeployPayload({ ...deployPayload, repo: e.target.value })} placeholder="repo (owner/name)" className="w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
            <div className="grid grid-cols-2 gap-2">
              <input value={deployPayload.branch} onChange={(e) => setDeployPayload({ ...deployPayload, branch: e.target.value })} placeholder="branch" className="rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
              <input value={deployPayload.name} onChange={(e) => setDeployPayload({ ...deployPayload, name: e.target.value })} placeholder="app name (auto from repo)" className="rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
            </div>
            <div className="grid grid-cols-2 gap-2">
              <select value={deployPayload.buildType} onChange={(e) => setDeployPayload({ ...deployPayload, buildType: e.target.value })} className="rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">
                <option value="static">static</option>
                <option value="node">node</option>
                <option value="next">next</option>
                <option value="nixpacks">nixpacks</option>
                <option value="dockerfile">dockerfile</option>
                <option value="heroku">heroku</option>
              </select>
              <input type="number" value={deployPayload.port} onChange={(e) => setDeployPayload({ ...deployPayload, port: Number(e.target.value) })} placeholder="port" className="rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
            </div>
            <button onClick={() => void handleDeploy()} aria-label="Deploy" className="rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-bold text-white hover:bg-[var(--brand-hover)] motion-safe:transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface)]">Deploy</button>
            {deployResult && (
              <div className="rounded-lg border border-emerald-500/25 bg-emerald-500/[0.09] p-3 text-xs text-emerald-200">
                <p className="font-bold text-[var(--text)]">{deployResult.appName} · {deployResult.builder} · {deployResult.status}</p>
                <a href={deployResult.url} target="_blank" rel="noreferrer" className="text-[var(--brand)] underline hover:text-[var(--brand-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] rounded">{deployResult.url}</a>
                <p className="text-[var(--text-subtle)]">Internal: {deployResult.internalUrl} · Deployment {deployResult.deploymentId.slice(0, 8)}</p>
              </div>
            )}
          </div>
        </AdminCard>
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <AdminCard title="Repositories" description="GET /onboarding/repos?providerId= — autocomplete listing via git service.">
          <div className="flex gap-2">
            <input value={providerId} onChange={(e) => setProviderId(e.target.value)} placeholder="providerId (optional auto-pick)" className="flex-1 rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
            <button onClick={() => void loadRepos()} aria-label="List repos" className="rounded-lg bg-[var(--surface-raised)] border border-[var(--line)] px-4 py-2 text-xs font-bold text-[var(--text)] hover:bg-[var(--surface-hover)] motion-safe:transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">List Repos</button>
          </div>
          {repos.length > 0 && (
            <div className="mt-3 max-h-64 overflow-auto space-y-1">
              {repos.map((r, i) => (
                <div key={i} className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-3 py-2 text-xs motion-safe:transition-colors hover:bg-[var(--surface-hover)]">
                  <p className="font-bold text-[var(--text)]">{(r as unknown as { fullName?: string; full_name?: string }).fullName ?? (r as unknown as { full_name?: string }).full_name ?? r.name}</p>
                  <p className="text-[var(--text-subtle)]">{r.cloneUrl} · default {r.defaultBranch}</p>
                </div>
              ))}
            </div>
          )}

          <div className="mt-4 flex gap-2">
            <input value={repoName} onChange={(e) => setRepoName(e.target.value)} placeholder="repo (owner/name) for branches" className="flex-1 rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
            <button onClick={() => void loadBranches()} aria-label="List branches" className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-4 py-2 text-xs text-[var(--text)] hover:bg-[var(--surface-hover)] motion-safe:transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Branches</button>
          </div>
          {branches.length > 0 && (
            <div className="mt-2 space-y-1">
              {branches.map((b, i) => (
                <div key={i} className="rounded border border-[var(--line)] bg-[var(--surface-raised)] px-3 py-1.5 text-xs text-[var(--text)]">
                  {b.name} · {b.commit?.slice(0, 8)}
                </div>
              ))}
            </div>
          )}
        </AdminCard>

        <AdminCard title="IDE Files" description="GET /ide/files — mock tree: loaded git sources + conventional entrypoints.">
          <button onClick={() => void loadSources()} aria-label="Reload sources" className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-3 py-1.5 text-xs text-[var(--text)] hover:bg-[var(--surface-hover)] motion-safe:transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Reload</button>
          {loadedSources.length === 0 ? (
            <p className="mt-3 text-sm text-[var(--text-subtle)]">No loaded sources. Connect a provider and load a repo.</p>
          ) : (
            <div className="mt-3 space-y-2">
              {loadedSources.map((s) => (
                <div key={s.id} className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] p-3 text-xs">
                  <p className="font-bold text-[var(--text)]">{s.owner}/{s.name} · {s.branch}</p>
                  <p className="text-[var(--text-subtle)]">{s.cloneUrl}</p>
                  <p className="mt-1 font-mono text-[11px] text-[var(--text-subtle)]">{(s.files ?? []).join(" · ")}</p>
                </div>
              ))}
            </div>
          )}
        </AdminCard>
      </div>
    </AdminPageLayout>
  );
}
