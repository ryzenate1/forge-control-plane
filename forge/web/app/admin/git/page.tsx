"use client";

import { useState, useMemo, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import {
  Key, Link, GitBranch, Trash2, Plus, RefreshCw, Webhook,
  Shield, Globe, Eye, EyeOff, AlertTriangle,
  Github, Gitlab, GitFork, Rocket, Users, Database, Search, ChevronDown,
  MoreHorizontal, Lock, User, ArrowRight, BookOpen, X,
} from "lucide-react";
import {
  listGitCredentials, listGitProviderTokens, listGitSources,
  createGitCredential, deleteGitCredential, generateGitDeployKey,
  connectGitProviderToken, disconnectGitProviderToken,
  createGitSource, deleteGitSource,
  listGitProviderRepos, listGitProviderBranches,
  testProviderInline, testGitProvider, updateGitProvider, getGitCredential,
} from "@/lib/api/git-admin";

interface GitCredential {
  id: string;
  userId: string;
  name: string;
  credentialType: "ssh_key" | "https_password" | "https_token";
  credential?: string;
  publicKey: string;
  description: string;
  createdAt: string;
  updatedAt: string;
}

interface GitProviderToken {
  id: string;
  userId: string;
  provider: "github" | "gitlab" | "bitbucket" | "gitea";
  providerName: string;
  accessToken?: string;
  tokenType: string;
  baseUrl: string;
  username: string;
  avatarUrl: string;
  createdAt: string;
  updatedAt: string;
}

interface GitSource {
  id: string;
  userId: string;
  credentialId?: string;
  providerTokenId?: string;
  provider: string;
  repositoryUrl: string;
  repositoryName: string;
  repositoryOwner: string;
  branch: string;
  autoDeploy: boolean;
  webhookId: string;
  webhookUrl: string;
  webhookSecret?: string;
  lastCommitSha: string;
  lastCommitMessage: string;
  lastCommitAuthor: string;
  lastDeployedAt?: string;
  webhookSetupError?: string;
  createdAt: string;
  updatedAt: string;
}

interface GitProviderRepo {
  name: string;
  fullName: string;
  cloneUrl: string;
  sshUrl: string;
  defaultBranch: string;
  private: boolean;
  description: string;
}

interface GitProviderBranch {
  name: string;
  sha: string;
  isMain: boolean;
}

type Tab = "credentials" | "providers" | "sources";

function ProviderIcon({ provider }: { provider: string }) {
  // Allowlist: provider ids are fixed union members, never raw user input —
  // an unknown id falls back to the local placeholder instead of building a
  // `https://<input>/favicon.ico` URL (which would let a crafted provider name
  // exfiltrate the admin's session via favicon fetch / onError probing).
  const icons: Record<string, string> = {
    github: "https://github.com/favicon.ico",
    gitlab: "https://gitlab.com/favicon.ico",
    bitbucket: "https://bitbucket.org/favicon.ico",
    gitea: "/gitea-favicon.ico",
  };
  const src = icons[provider];
  if (!src) return <GitBranch size={20} className="text-[var(--text-subtle)]" />;
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={src}
      alt={`${provider} icon`}
      className="w-5 h-5 rounded"
      onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }}
    />
  );
}

function ProviderGlyph({ provider, size = 26 }: { provider: string; size?: number }) {
  if (provider === "github") return <Github size={size} className="text-text" />;
  if (provider === "gitlab") return <Gitlab size={size} className="text-warn" />;
  return <ProviderIcon provider={provider} />;
}

const CRED_TYPE_META: Record<string, { label: string; icon: typeof Key }> = {
  ssh_key: { label: "SSH Key", icon: Key },
  https_token: { label: "Personal Token", icon: Lock },
  https_password: { label: "HTTPS Password", icon: User },
};

function credentialTypeLabel(t: string): string {
  return CRED_TYPE_META[t]?.label ?? t;
}

function formatDay(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
}

function timeAgo(iso?: string): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "—";
  const s = Math.max(0, Math.floor((Date.now() - t) / 1000));
  if (s < 60) return "just now";
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} minute${m === 1 ? "" : "s"} ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} hour${h === 1 ? "" : "s"} ago`;
  const d = Math.floor(h / 24);
  if (d < 30) return `${d} day${d === 1 ? "" : "s"} ago`;
  const w = Math.floor(d / 7);
  if (w < 8) return `${w} week${w === 1 ? "" : "s"} ago`;
  const mo = Math.floor(d / 30);
  if (mo < 12) return `${mo} month${mo === 1 ? "" : "s"} ago`;
  return `${Math.floor(mo / 12)} year(s) ago`;
}

const PROVIDER_CARDS = [
  { id: "github", name: "GitHub", blurb: "Connect your GitHub account to access repositories and setup webhooks." },
  { id: "gitlab", name: "GitLab", blurb: "Connect your GitLab instance to access projects and configure webhooks." },
  { id: "bitbucket", name: "Bitbucket", blurb: "Connect your Bitbucket account to access repositories and setup webhooks." },
  { id: "gitea", name: "Gitea", blurb: "Connect your Gitea instance to access repositories and setup webhooks." },
];

import { AdminPageLayout, SectionHeader, AdminTabs, AdminLoadingState, AdminErrorState, EmptyState } from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";

export default function GitPage() {
  const [confirm, renderConfirm] = useConfirm();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [tab, setTab] = useState<Tab>(() => {
    if (typeof window === "undefined") return "credentials";
    const initial = new URLSearchParams(window.location.search).get("tab");
    return initial === "providers" || initial === "sources" ? initial : "credentials";
  });
  const changeTab = (next: Tab) => {
    setTab(next);
    const url = next === "credentials" ? window.location.pathname : `${window.location.pathname}?tab=${next}`;
    window.history.replaceState(null, "", url);
  };
  const [showCreateCredential, setShowCreateCredential] = useState(false);
  const [showConnectProvider, setShowConnectProvider] = useState(false);
  const [connectPreset, setConnectPreset] = useState("github");
  const [showCreateSource, setShowCreateSource] = useState(false);
  const [selectedProviderRepos, setSelectedProviderRepos] = useState<GitProviderRepo[]>([]);
  const [selectedProviderBranches, setSelectedProviderBranches] = useState<GitProviderBranch[]>([]);
  const [loadingRepos, setLoadingRepos] = useState("");
  const [loadingBranches, setLoadingBranches] = useState("");
  const [revealSecrets, setRevealSecrets] = useState<Record<string, boolean>>({});
  const [editingProviderId, setEditingProviderId] = useState<string | null>(null);
  const [editProviderName, setEditProviderName] = useState("");
  const [inlineTestResult, setInlineTestResult] = useState<Record<string, string>>({});
  const [credSearch, setCredSearch] = useState("");
  const [credProviderFilter, setCredProviderFilter] = useState("all");
  const [credMenuId, setCredMenuId] = useState<string | null>(null);
  const [showDocs, setShowDocs] = useState(false);
  const [repoCounts, setRepoCounts] = useState<Record<string, { count: number } | { error: string }>>({});

  const credentialsQuery = useQuery<GitCredential[]>({
    queryKey: ["git-credentials"],
    queryFn: () => listGitCredentials(),
  });
  const credentialsRaw = credentialsQuery.data;

  const providerTokensQuery = useQuery<GitProviderToken[]>({
    queryKey: ["git-providers"],
    queryFn: () => listGitProviderTokens(),
  });
  const providerTokensRaw = providerTokensQuery.data;

  const sourcesQuery = useQuery<GitSource[]>({
    queryKey: ["git-sources"],
    queryFn: () => listGitSources(),
  });
  const sourcesRaw = sourcesQuery.data;

  const credentials = useMemo(() => credentialsRaw ?? [], [credentialsRaw]);
  const providerTokens = useMemo(() => providerTokensRaw ?? [], [providerTokensRaw]);
  const sources = useMemo(() => sourcesRaw ?? [], [sourcesRaw]);

  // Per-credential stats derived from linked repository sources (a source
  // uses a credential via credentialId). Provider = distinct providers of
  // linked sources; last used = most recent deployment through them; a
  // credential linked to at least one source counts as active.
  const credStatsById = useMemo(() => {
    const map = new Map<string, { providers: string[]; lastUsed?: string; linked: number }>();
    for (const c of credentials) {
      const linked = sources.filter((s) => s.credentialId === c.id);
      const providers = [...new Set(linked.map((s) => (s.provider || "").toLowerCase()).filter(Boolean))];
      const stamps = linked.map((s) => s.lastDeployedAt).filter(Boolean) as string[];
      stamps.sort();
      map.set(c.id, {
        providers,
        lastUsed: stamps.length > 0 ? stamps[stamps.length - 1] : undefined,
        linked: linked.length,
      });
    }
    return map;
  }, [credentials, sources]);

  const filteredCredentials = useMemo(() => {
    const s = credSearch.trim().toLowerCase();
    return credentials.filter((c) => {
      const stat = credStatsById.get(c.id);
      if (credProviderFilter !== "all" && !stat?.providers.includes(credProviderFilter)) return false;
      if (!s) return true;
      return (
        c.name.toLowerCase().includes(s) ||
        (c.description || "").toLowerCase().includes(s) ||
        credentialTypeLabel(c.credentialType).toLowerCase().includes(s) ||
        (stat?.providers.some((p) => p.toLowerCase().includes(s)) ?? false)
      );
    });
  }, [credentials, credSearch, credProviderFilter, credStatsById]);

  // Repository counts for connected providers (best-effort; failures render as —).
  useEffect(() => {
    let cancelled = false;
    if (providerTokens.length === 0) {
      setRepoCounts({});
      return;
    }
    void (async () => {
      const entries = await Promise.all(
        providerTokens.map(async (t) => {
          try {
            const repos = await listGitProviderRepos(t.id);
            return [t.id, { count: repos.length }] as const;
          } catch {
            return [t.id, { error: "unavailable" }] as const;
          }
        }),
      );
      if (!cancelled) setRepoCounts(Object.fromEntries(entries));
    })();
    return () => {
      cancelled = true;
    };
  }, [providerTokens]);

  const webhooksForProvider = (tokenId: string) =>
    sources.filter((s) => s.providerTokenId === tokenId && s.webhookId).length;

  const createCredential = useMutation({
    mutationFn: (data: { name: string; credentialType: string; credential: string; description: string }) =>
      createGitCredential(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["git-credentials"] });
      setShowCreateCredential(false);
      toast({ tone: "success", title: "Credential created" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Create failed", message: e.message }),
  });

  const deleteCredential = useMutation({
    mutationFn: (id: string) => deleteGitCredential(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["git-credentials"] });
      setCredMenuId(null);
      toast({ tone: "success", title: "Credential deleted" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  const generateKey = useMutation({
    mutationFn: (id: string) => generateGitDeployKey(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["git-credentials"] });
      setCredMenuId(null);
      toast({ tone: "success", title: "Deploy key generated" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Generate failed", message: e.message }),
  });

  const viewCredential = useMutation({
    mutationFn: (id: string) => getGitCredential(id),
    onSuccess: (cred) => {
      setCredMenuId(null);
      toast({ tone: "success", title: `Credential ${cred.name}`, message: cred.credentialType });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Fetch failed", message: e.message }),
  });

  const connectProvider = useMutation({
    mutationFn: (data: {
      provider: string; providerName: string; accessToken: string;
      refreshToken: string; tokenType: string; baseUrl: string; username: string;
    }) => connectGitProviderToken(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["git-providers"] });
      setShowConnectProvider(false);
      toast({ tone: "success", title: "Provider connected" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Connect failed", message: e.message }),
  });

  const disconnectProvider = useMutation({
    mutationFn: (id: string) => disconnectGitProviderToken(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["git-providers"] });
      toast({ tone: "success", title: "Provider disconnected" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Disconnect failed", message: e.message }),
  });

  const testProviderInlineMut = useMutation({
    mutationFn: (body: { provider: string; accessToken: string; baseUrl?: string }) => testProviderInline(body),
    onSuccess: (res) => toast({ tone: res.ok ? "success" : "error", title: res.ok ? "Provider test succeeded" : "Provider test failed", message: res.message }),
    onError: (e: Error) => toast({ tone: "error", title: "Test failed", message: e.message }),
  });

  const testProviderMut = useMutation({
    mutationFn: (id: string) => testGitProvider(id),
    onSuccess: (res, id) => {
      setInlineTestResult((p) => ({ ...p, [id]: res.ok ? "ok" : res.message || "failed" }));
      toast({ tone: res.ok ? "success" : "error", title: res.ok ? "Provider test succeeded — POST /git/providers/:id/test" : "Provider test failed", message: res.message });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Test failed", message: e.message }),
  });

  const updateProviderMut = useMutation({
    mutationFn: ({ id, body }: { id: string; body: Partial<{ providerName: string; baseUrl: string; username: string }> }) => updateGitProvider(id, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["git-providers"] });
      setEditingProviderId(null);
      toast({ tone: "success", title: "Provider updated — PATCH /git/providers/:id" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Update failed", message: e.message }),
  });

  const createSource = useMutation({
    mutationFn: (data: {
      credentialId?: string; providerTokenId?: string; provider: string;
      repositoryUrl: string; repositoryName: string; repositoryOwner: string;
      branch: string; autoDeploy: boolean;
    }) => createGitSource(data),
    onSuccess: (src: GitSource) => {
      queryClient.invalidateQueries({ queryKey: ["git-sources"] });
      setShowCreateSource(false);
      if (src.autoDeploy && !src.webhookId && !src.webhookUrl) {
        toast({ tone: "error", title: "Auto-deploy webhook not configured", message: (src as GitSource).webhookSetupError || "Webhook setup failed — check provider token permissions and PANEL_URL" });
      } else if (src.autoDeploy && (src as GitSource).webhookSetupError) {
        toast({ tone: "error", title: "Auto-deploy webhook error", message: (src as GitSource).webhookSetupError });
      } else {
        toast({ tone: "success", title: "Git source linked" });
      }
    },
    onError: (e: Error) => toast({ tone: "error", title: "Link failed", message: e.message }),
  });

  const deleteSource = useMutation({
    mutationFn: (id: string) => deleteGitSource(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["git-sources"] });
      toast({ tone: "success", title: "Git source removed" });
    },
    onError: (e: Error) => toast({ tone: "error", title: "Delete failed", message: e.message }),
  });

  const loadProviderRepos = async (tokenId: string) => {
    setLoadingRepos(tokenId);
    try {
      const repos = await listGitProviderRepos(tokenId);
      setSelectedProviderRepos(repos);
    } catch {
      toast({ tone: "error", title: "Failed to load repositories" });
    } finally {
      setLoadingRepos("");
    }
  };

  const loadProviderBranches = async (tokenId: string, repoFullName: string) => {
    setLoadingBranches(tokenId);
    try {
      const branches = await listGitProviderBranches(tokenId, repoFullName);
      setSelectedProviderBranches(branches);
    } catch {
      toast({ tone: "error", title: "Failed to load branches" });
    } finally {
      setLoadingBranches("");
    }
  };

  const openConnect = (provider: string) => {
    setConnectPreset(provider);
    changeTab("providers");
    setShowConnectProvider(true);
  };

  const tokenForProvider = (provider: string) => providerTokens.find((t) => t.provider === provider);

  return (
    <AdminPageLayout>
      <SectionHeader
        title={
          <span className="flex items-center gap-2.5">
            Git Integrations
            <span className="grid h-5 w-5 place-items-center rounded-md border border-line bg-overlay text-text-muted">
              <Link size={12} />
            </span>
          </span>
        }
        sub="Git providers, credentials and connected sources."
        action={
          <button
            type="button"
            onClick={() => setShowDocs(true)}
            className="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg border border-line bg-overlay-subtle px-3.5 text-xs font-medium text-text transition hover:bg-overlay-strong hover:text-text"
          >
            <BookOpen size={14} /> View Documentation
          </button>
        }
      />

      {/* How it works */}
      <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-4 sm:p-5">
        <h2 className="flex items-center gap-2 text-sm font-bold text-text">
          <span className="grid h-5 w-5 place-items-center rounded-full bg-info-subtle text-[11px] font-bold text-info">i</span>
          How it works
        </h2>
        <ol className="mt-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-[1fr_auto_1fr_auto_1fr_auto_1fr] sm:items-center">
          <HowStep
            icon={<Key size={20} className="text-warn" />}
            tile="border-warn-line bg-warn-subtle"
            title="Add credentials"
            text="Store access tokens for your Git provider"
          />
          <HowArrow />
          <HowStep
            icon={<Github size={20} className="text-text" />}
            tile="border-line bg-overlay"
            title="Connect provider"
            text="Link a GitHub, GitLab, Bitbucket or Gitea account"
          />
          <HowArrow />
          <HowStep
            icon={<GitFork size={20} className="text-info" />}
            tile="border-info-line bg-info-subtle"
            title="Add repository sources"
            text="Select repositories to use in deployments or pipelines"
          />
          <HowArrow />
          <HowStep
            icon={<Rocket size={20} className="text-danger" />}
            tile="border-danger-line bg-danger/[0.08]"
            title="Auto-deploy"
            text="Webhooks trigger builds and deployments"
          />
        </ol>
      </section>

      <AdminTabs tabs={[
        { id: "credentials", label: "Credentials", icon: Key },
        { id: "providers", label: "Providers", icon: Users },
        { id: "sources", label: "Repository Sources", icon: Database },
      ]} active={tab} onChange={(id) => changeTab(id as Tab)} />

      {tab === "credentials" && (
        <div className="space-y-4">
          {/* Credentials table card */}
          <section className="overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface)]">
            <div className="flex flex-col gap-3 px-4 py-4 sm:flex-row sm:items-start sm:justify-between sm:px-5">
              <div className="flex items-start gap-2.5">
                <Key size={17} className="mt-0.5 shrink-0 text-text" />
                <div>
                  <h2 className="text-sm font-bold text-text">Git Credentials</h2>
                  <p className="mt-0.5 max-w-3xl text-xs leading-5 text-text-muted">
                    Securely store tokens and credentials used to access your Git providers. These are encrypted and used for cloning repositories and webhook setup.
                  </p>
                </div>
              </div>
              <button
                onClick={() => setShowCreateCredential(true)}
                className="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg border border-transparent bg-[var(--brand)] px-3.5 text-xs font-semibold text-white transition hover:bg-[var(--brand-hover)]"
              >
                <Plus size={14} strokeWidth={2.5} /> Add Credential
              </button>
            </div>

            <div className="flex flex-col gap-2 px-4 pb-3 sm:flex-row sm:px-5">
              <div className="relative min-w-0 flex-1">
                <Search size={15} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-text-muted" />
                <input
                  value={credSearch}
                  onChange={(e) => setCredSearch(e.target.value)}
                  placeholder="Search credentials by name or provider…"
                  aria-label="Search credentials by name or provider"
                  className="h-10 w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] pl-9 pr-3 text-xs text-text outline-none transition placeholder:text-text-muted hover:border-[var(--line-strong)] focus:border-[var(--brand)]"
                />
              </div>
              <label className="relative block sm:w-44">
                <span className="sr-only">Filter by provider</span>
                <select
                  aria-label="Filter by provider"
                  value={credProviderFilter}
                  onChange={(e) => setCredProviderFilter(e.target.value)}
                  className="h-10 w-full appearance-none rounded-lg border border-[var(--line)] bg-[var(--surface-input)] pl-3 pr-8 text-xs text-text outline-none transition hover:border-[var(--line-strong)] focus:border-[var(--brand)]"
                >
                  <option value="all">All providers</option>
                  <option value="github">GitHub</option>
                  <option value="gitlab">GitLab</option>
                  <option value="bitbucket">Bitbucket</option>
                  <option value="gitea">Gitea</option>
                </select>
                <ChevronDown size={14} className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-text-muted" />
              </label>
            </div>

            <div className="overflow-x-auto">
              {credentialsQuery.isLoading ? (
                <AdminLoadingState label="Loading git credentials…" />
              ) : credentialsQuery.isError ? (
                <div className="p-4">
                  <AdminErrorState
                    message={credentialsQuery.error instanceof Error ? credentialsQuery.error.message : "Failed to load git credentials"}
                    retry={() => void credentialsQuery.refetch()}
                  />
                </div>
              ) : (
              <table className="w-full min-w-[900px] text-sm">
                <thead>
                  <tr className="border-y border-[var(--line)] text-left text-[10px] uppercase tracking-[0.12em] text-text-muted">
                    <th className="px-4 py-2.5 font-medium sm:px-5">Name</th>
                    <th className="px-4 py-2.5 font-medium">Provider</th>
                    <th className="px-4 py-2.5 font-medium">Type</th>
                    <th className="px-4 py-2.5 font-medium">Created</th>
                    <th className="px-4 py-2.5 font-medium">Last used</th>
                    <th className="px-4 py-2.5 font-medium">Status</th>
                    <th className="px-4 py-2.5 text-right font-medium sm:pr-5">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {filteredCredentials.map((cred) => {
                    const meta = CRED_TYPE_META[cred.credentialType];
                    const TypeIcon = meta?.icon ?? Key;
                    const stat = credStatsById.get(cred.id) ?? { providers: [], lastUsed: undefined, linked: 0 };
                    const provider = stat.providers.length === 1 ? stat.providers[0] : undefined;
                    const active = stat.linked > 0;
                    return (
                      <tr key={cred.id} className="transition hover:bg-overlay-subtle">
                        <td className="px-4 py-3 sm:pl-5">
                          <div className="flex items-center gap-3">
                            <span className="grid h-9 w-9 shrink-0 place-items-center rounded-lg border border-line bg-overlay text-text">
                              {provider ? <ProviderGlyph provider={provider} size={18} /> : <TypeIcon size={16} />}
                            </span>
                            <span className="min-w-0">
                              <span className="block truncate text-[13px] font-semibold text-text">{cred.name}</span>
                              <span className="block truncate text-[11px] text-text-muted">{cred.description || "—"}</span>
                            </span>
                          </div>
                        </td>
                        <td className="px-4 py-3">
                          {provider ? (
                            <span className="inline-flex items-center gap-1.5 text-xs text-text">
                              <ProviderGlyph provider={provider} size={15} />
                              <span className="capitalize">{provider}</span>
                            </span>
                          ) : (
                            <span className="text-xs text-text-muted" title="No linked repository source names a provider for this credential">—</span>
                          )}
                        </td>
                        <td className="px-4 py-3">
                          <span className="inline-flex items-center rounded-md border border-line bg-overlay px-2 py-1 text-[11px] text-text">
                            {credentialTypeLabel(cred.credentialType)}
                          </span>
                        </td>
                        <td className="px-4 py-3">
                          <span className="block text-xs text-text">{formatDay(cred.createdAt)}</span>
                          <span className="block text-[11px] text-text-muted">{timeAgo(cred.createdAt)}</span>
                        </td>
                        <td className="px-4 py-3">
                          {stat.lastUsed ? (
                            <>
                              <span className="block text-xs text-text">{formatDay(stat.lastUsed)}</span>
                              <span className="block text-[11px] text-text-muted">{timeAgo(stat.lastUsed)}</span>
                            </>
                          ) : (
                            <>
                              <span className="block text-xs text-text">Never</span>
                              <span className="block text-[11px] text-text-muted">—</span>
                            </>
                          )}
                        </td>
                        <td className="px-4 py-3">
                          {active ? (
                            <span
                              className="inline-flex items-center gap-1.5 rounded-full border border-ok-line bg-ok-subtle px-2.5 py-1 text-[11px] font-medium text-ok"
                              title={`Linked to ${stat.linked} repository source${stat.linked === 1 ? "" : "s"}`}
                            >
                              <span className="h-1.5 w-1.5 rounded-full bg-ok" /> Active
                            </span>
                          ) : (
                            <span
                              className="inline-flex items-center gap-1.5 rounded-full border border-line bg-overlay px-2.5 py-1 text-[11px] font-medium text-text-subtle"
                              title="Not linked to any repository source"
                            >
                              <span className="h-1.5 w-1.5 rounded-full bg-text-muted" /> Inactive
                            </span>
                          )}
                        </td>
                        <td className="px-4 py-3 text-right sm:pr-5">
                          <div className="relative inline-block">
                            <button
                              type="button"
                              aria-label={`Actions for ${cred.name}`}
                              aria-expanded={credMenuId === cred.id}
                              onClick={() => setCredMenuId(credMenuId === cred.id ? null : cred.id)}
                              className="grid h-8 w-8 place-items-center rounded-lg border border-line bg-overlay-subtle text-text-subtle transition hover:bg-overlay-strong hover:text-text"
                            >
                              <MoreHorizontal size={15} />
                            </button>
                            {credMenuId === cred.id && (
                              <>
                                <button
                                  type="button"
                                  aria-label="Close actions"
                                  className="fixed inset-0 z-10 cursor-default"
                                  onClick={() => setCredMenuId(null)}
                                />
                                <div className="absolute right-0 z-20 mt-1.5 w-44 overflow-hidden rounded-lg border border-line bg-[var(--surface-raised)] py-1 shadow-2xl">
                                  <button
                                    type="button"
                                    onClick={() => viewCredential.mutate(cred.id)}
                                    disabled={viewCredential.isPending}
                                    className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs text-text transition hover:bg-overlay-strong"
                                  >
                                    <Eye size={13} /> View details
                                  </button>
                                  {cred.credentialType === "ssh_key" && (
                                    <button
                                      type="button"
                                      onClick={() => generateKey.mutate(cred.id)}
                                      disabled={generateKey.isPending}
                                      className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs text-text transition hover:bg-overlay-strong"
                                    >
                                      <RefreshCw size={13} /> Generate key
                                    </button>
                                  )}
                                  <button
                                    type="button"
                                    onClick={() => { void (async () => { if (await confirm({ title: "Delete this git credential?", description: "The stored credential will be removed. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteCredential.mutate(cred.id); })(); }}
                                    className="flex w-full items-center gap-2 px-3 py-2 text-left text-xs text-danger transition hover:bg-danger-subtle"
                                  >
                                    <Trash2 size={13} /> Delete
                                  </button>
                                </div>
                              </>
                            )}
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
              )}
              {credentialsQuery.isSuccess && filteredCredentials.length === 0 && (
                <EmptyState
                  icon={Key}
                  title={credentials.length === 0 ? "No credentials" : "No matches"}
                  message={credentials.length === 0 ? "No credentials configured yet — add one to get started." : "No credentials match the current search."}
                />
              )}
            </div>

            {showCreateCredential && (
              <div className="border-t border-[var(--line)] p-4 sm:p-5">
                <CredentialForm
                  onSubmit={(data) => createCredential.mutate(data)}
                  onCancel={() => setShowCreateCredential(false)}
                  loading={createCredential.isPending}
                />
              </div>
            )}
          </section>

          {/* Providers overview cards */}
          <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-4 sm:p-5">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div className="flex items-start gap-2.5">
                <Link size={17} className="mt-0.5 shrink-0 text-text" />
                <div>
                  <h2 className="text-sm font-bold text-text">Git Providers</h2>
                  <p className="mt-0.5 text-xs text-text-muted">Connect your Git provider accounts to fetch repositories and configure webhooks.</p>
                </div>
              </div>
              <button
                onClick={() => openConnect("github")}
                className="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg border border-line bg-overlay px-3.5 text-xs font-medium text-text transition hover:bg-overlay-strong"
              >
                <Plus size={14} /> Add Provider
              </button>
            </div>

            <div className="mt-4 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
              {PROVIDER_CARDS.map((card) => {
                const token = tokenForProvider(card.id);
                const connected = Boolean(token);
                const repoCount = token ? repoCounts[token.id] : undefined;
                const webhooks = token ? webhooksForProvider(token.id) : 0;
                return (
                  <article key={card.id} className="flex flex-col rounded-xl border border-[var(--line)] bg-overlay-subtle p-4 transition hover:border-line-strong">
                    <div className="flex items-start justify-between gap-2">
                      <div className="flex min-w-0 items-center gap-3">
                        <span className="grid h-11 w-11 shrink-0 place-items-center rounded-xl border border-line bg-overlay-subtle">
                          <ProviderGlyph provider={card.id} />
                        </span>
                        <h3 className="truncate text-[13px] font-bold text-text">{card.name}</h3>
                      </div>
                      {connected ? (
                        <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-ok-line bg-ok-subtle px-2.5 py-1 text-[11px] font-medium text-ok">
                          <span className="h-1.5 w-1.5 rounded-full bg-ok" /> Connected
                        </span>
                      ) : (
                        <span className="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-line bg-overlay px-2.5 py-1 text-[11px] font-medium text-text-subtle">
                          <span className="h-1.5 w-1.5 rounded-full bg-text-muted" /> Not connected
                        </span>
                      )}
                    </div>
                    <p className="mt-2.5 min-h-10 text-xs leading-5 text-text-subtle">{card.blurb}</p>
                    <div className="mt-3">
                      {connected && token ? (
                        <div className="flex items-center gap-2">
                          <div className="flex h-10 flex-1 items-center gap-4 rounded-lg border border-line bg-overlay-subtle px-3 text-[11px] text-text-subtle">
                            <span className="inline-flex items-center gap-1.5" title="Repositories visible to this provider token">
                              <GitBranch size={13} />
                              {repoCount && "count" in repoCount ? `${repoCount.count} repositor${repoCount.count === 1 ? "y" : "ies"}` : "— repositories"}
                            </span>
                            <span className="inline-flex items-center gap-1.5" title="Auto-deploy webhooks installed via this provider">
                              <Webhook size={13} />
                              {webhooks} webhook{webhooks === 1 ? "" : "s"}
                            </span>
                          </div>
                          <button
                            type="button"
                            aria-label={`Manage ${card.name}`}
                            onClick={() => changeTab("providers")}
                            className="grid h-10 w-10 shrink-0 place-items-center rounded-lg border border-line bg-overlay-subtle text-text transition hover:bg-overlay-strong hover:text-text"
                          >
                            <ArrowRight size={15} />
                          </button>
                        </div>
                      ) : (
                        <button
                          type="button"
                          onClick={() => openConnect(card.id)}
                          className="h-10 w-full rounded-lg border border-line bg-overlay text-xs font-medium text-text transition hover:bg-overlay-strong hover:text-text"
                        >
                          Connect
                        </button>
                      )}
                    </div>
                  </article>
                );
              })}
            </div>
          </section>
        </div>
      )}

      {tab === "providers" && (
        <div className="space-y-4">
          <section className="rounded-xl border border-[var(--line)] bg-[var(--surface)] p-4 sm:p-5">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div className="flex items-start gap-2.5">
                <Globe size={17} className="mt-0.5 shrink-0 text-text" />
                <div>
                  <h2 className="text-sm font-bold text-text">Connected Providers</h2>
                  <p className="mt-0.5 text-xs text-text-muted">Test saved tokens, browse repositories, rename, or disconnect. Tokens stay server-side.</p>
                </div>
              </div>
              <button
                onClick={() => { setConnectPreset("github"); setShowConnectProvider(true); }}
                className="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg border border-line bg-overlay px-3.5 text-xs font-medium text-text transition hover:bg-overlay-strong"
              >
                <Plus size={14} /> Add Provider
              </button>
            </div>

            <div className="mt-4 space-y-3">
              {providerTokensQuery.isLoading ? (
                <AdminLoadingState label="Loading git providers…" />
              ) : providerTokensQuery.isError ? (
                <AdminErrorState
                  message={providerTokensQuery.error instanceof Error ? providerTokensQuery.error.message : "Failed to load git providers"}
                  retry={() => void providerTokensQuery.refetch()}
                />
              ) : providerTokens.length === 0 ? (
                <EmptyState
                  icon={Users}
                  title="No providers"
                  message="No providers connected yet — connect one to deploy from repositories."
                />
              ) : null}

              {providerTokens.map((pt) => (
                <article key={pt.id} className="rounded-xl border border-[var(--line)] bg-overlay-subtle p-4">
                  <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                    <div className="flex min-w-0 items-start gap-3">
                      <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-line bg-overlay-subtle">
                        <ProviderGlyph provider={pt.provider} size={22} />
                      </span>
                      <div className="min-w-0">
                        {editingProviderId === pt.id ? (
                          <div className="flex flex-wrap items-center gap-2">
                            <input value={editProviderName} onChange={(e) => setEditProviderName(e.target.value)} placeholder="Display name" aria-label="Display name" className="h-8 rounded-lg border border-line bg-[var(--surface-input)] px-2.5 text-xs text-text outline-none focus:border-[var(--brand)]" />
                            <button onClick={() => updateProviderMut.mutate({ id: pt.id, body: { providerName: editProviderName } })} className="h-8 rounded-lg bg-[var(--brand)] px-3 text-xs font-semibold text-white hover:opacity-90">Save</button>
                            <button onClick={() => setEditingProviderId(null)} className="h-8 rounded-lg border border-line px-3 text-xs text-text hover:bg-overlay-strong">Cancel</button>
                          </div>
                        ) : (
                          <p className="text-[13px] font-semibold capitalize text-text">
                            {pt.provider}{pt.providerName ? <span className="font-normal text-text-subtle"> — {pt.providerName}</span> : ""}
                            <button onClick={() => { setEditingProviderId(pt.id); setEditProviderName(pt.providerName); }} className="ml-2 align-middle text-[11px] font-normal text-text-muted underline hover:text-text">Rename</button>
                          </p>
                        )}
                        <p className="mt-0.5 truncate text-xs text-text-muted">
                          {pt.username && `@${pt.username} · `}{pt.tokenType} token
                          {pt.baseUrl && <span className="ml-1 font-mono text-[11px]">{pt.baseUrl}</span>}
                        </p>
                        {inlineTestResult[pt.id] && (
                          <p className={`mt-1 text-xs ${inlineTestResult[pt.id] === "ok" ? "text-ok" : "text-warn"}`}>Test: {inlineTestResult[pt.id]}</p>
                        )}
                      </div>
                    </div>
                    <div className="flex shrink-0 flex-wrap gap-1.5">
                      <button
                        onClick={() => testProviderMut.mutate(pt.id)}
                        className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-danger-line bg-danger-subtle px-2.5 text-xs font-medium text-danger transition hover:bg-danger-subtle"
                        disabled={testProviderMut.isPending}
                        title="POST /git/providers/:id/test"
                      >
                        <Shield size={13} /> Test
                      </button>
                      <button
                        onClick={() => loadProviderRepos(pt.id)}
                        className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-line bg-overlay-subtle px-2.5 text-xs text-text transition hover:bg-overlay-strong"
                        disabled={loadingRepos === pt.id}
                      >
                        <RefreshCw size={13} className={loadingRepos === pt.id ? "animate-spin" : ""} /> Repos
                      </button>
                      <button
                        onClick={() => { void (async () => { if (await confirm({ title: "Disconnect this git provider?", description: "The provider connection will be removed from the panel. This cannot be undone.", danger: true, confirmLabel: "Disconnect" })) disconnectProvider.mutate(pt.id); })(); }}
                        className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-line bg-overlay-subtle px-2.5 text-xs text-danger transition hover:bg-danger-subtle"
                      >
                        <Trash2 size={13} /> Disconnect
                      </button>
                    </div>
                  </div>

                  <details className="mt-3 text-xs">
                    <summary className="flex cursor-pointer items-center gap-1.5 text-text-muted hover:text-text">
                      <Webhook size={13} /> Webhook secrets — {sources.filter((s) => s.providerTokenId === pt.id).length} linked source(s)
                    </summary>
                    <div className="mt-2 space-y-2">
                      {sources.filter((s) => s.providerTokenId === pt.id).length === 0 ? (
                        <p className="text-xs text-text-muted">No sources linked to this provider. Link a repository in Repository Sources to see webhook secrets.</p>
                      ) : sources.filter((s) => s.providerTokenId === pt.id).map((src) => (
                        <div key={src.id} className="space-y-1 rounded-lg bg-overlay p-2.5">
                          <p className="font-mono text-[11px] text-text">{src.repositoryOwner}/{src.repositoryName} — {src.branch}</p>
                          <p className="text-[11px] text-text-muted">Webhook URL: <code className="break-all text-[10px]">{src.webhookUrl || "—"}</code></p>
                          <div className="flex items-center gap-2">
                            <span className="text-[11px] text-text-muted">Secret:</span>
                            {src.webhookSecret ? (
                              <>
                                <code className="rounded bg-well px-1.5 py-0.5 font-mono text-[11px]">
                                  {revealSecrets[src.id] ? src.webhookSecret : "•".repeat(16)}
                                </code>
                                <button onClick={() => setRevealSecrets((p) => ({ ...p, [src.id]: !p[src.id] }))} className="rounded p-1 hover:bg-overlay-strong" aria-label="Toggle secret visibility">
                                  {revealSecrets[src.id] ? <EyeOff size={13} /> : <Eye size={13} />}
                                </button>
                                <button onClick={() => { navigator.clipboard.writeText(src.webhookSecret!); toast({ tone: "success", title: "Secret copied" }); }} className="text-[11px] text-text-subtle underline">Copy</button>
                              </>
                            ) : (
                              <span className="text-[11px] text-warn">No secret — autoDeploy webhook may have failed</span>
                            )}
                          </div>
                          {src.webhookId && <p className="text-[11px] text-text-muted">Webhook ID: {src.webhookId}</p>}
                        </div>
                      ))}
                    </div>
                  </details>

                  {selectedProviderRepos.length > 0 && (
                    <div className="ml-4 mt-2 space-y-1 border-l-2 border-line pl-3">
                      <p className="text-xs font-medium text-text-subtle">Repositories:</p>
                      {selectedProviderRepos.map((repo) => (
                        <div key={repo.fullName} className="flex items-center justify-between text-xs">
                          <span className="text-text">
                            {repo.name}
                            {repo.private && <span className="ml-1 text-text-muted">(private)</span>}
                          </span>
                          <button
                            onClick={() => loadProviderBranches(pt.id, repo.fullName)}
                            className="text-xs text-text-subtle underline hover:text-text"
                            disabled={loadingBranches === pt.id}
                          >
                            branches
                          </button>
                        </div>
                      ))}
                    </div>
                  )}

                  {selectedProviderBranches.length > 0 && (
                    <div className="ml-8 mt-1 space-y-1 text-xs text-text-subtle">
                      {selectedProviderBranches.map((b) => (
                        <span key={b.name} className="mr-2 inline-flex items-center gap-1 rounded bg-overlay-strong px-1.5 py-0.5">
                          <GitBranch size={12} /> {b.name}
                        </span>
                      ))}
                    </div>
                  )}
                </article>
              ))}
            </div>

            {showConnectProvider && (
              <div className="mt-4 border-t border-[var(--line)] pt-4">
                <ProviderForm
                  key={connectPreset}
                  initialProvider={connectPreset}
                  onInlineTest={(body) => testProviderInlineMut.mutate(body)}
                  onSubmit={(data) => connectProvider.mutate(data)}
                  onCancel={() => setShowConnectProvider(false)}
                  loading={connectProvider.isPending}
                />
              </div>
            )}
          </section>
        </div>
      )}

      {tab === "sources" && (
        <div className="space-y-4">
          <section className="overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--surface)]">
            <div className="flex flex-col gap-3 px-4 py-4 sm:flex-row sm:items-start sm:justify-between sm:px-5">
              <div className="flex items-start gap-2.5">
                <Database size={17} className="mt-0.5 shrink-0 text-text" />
                <div>
                  <h2 className="text-sm font-bold text-text">Repository Sources</h2>
                  <p className="mt-0.5 text-xs text-text-muted">Linked repositories that drive auto-deploy webhooks and pipeline triggers.</p>
                </div>
              </div>
              <button
                onClick={() => setShowCreateSource(true)}
                className="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg border border-transparent bg-[var(--brand)] px-3.5 text-xs font-semibold text-white transition hover:bg-[var(--brand-hover)]"
              >
                <Plus size={14} strokeWidth={2.5} /> Link Repository
              </button>
            </div>

            <div className="overflow-x-auto">
              {sourcesQuery.isLoading ? (
                <AdminLoadingState label="Loading repository sources…" />
              ) : sourcesQuery.isError ? (
                <div className="p-4">
                  <AdminErrorState
                    message={sourcesQuery.error instanceof Error ? sourcesQuery.error.message : "Failed to load repository sources"}
                    retry={() => void sourcesQuery.refetch()}
                  />
                </div>
              ) : (
              <table className="w-full min-w-[760px] text-sm">
                <thead>
                  <tr className="border-y border-[var(--line)] text-left text-[10px] uppercase tracking-[0.12em] text-text-muted">
                    <th className="px-4 py-2.5 font-medium sm:px-5">Repository</th>
                    <th className="px-4 py-2.5 font-medium">Branch</th>
                    <th className="px-4 py-2.5 font-medium">Auto-deploy</th>
                    <th className="px-4 py-2.5 font-medium">Last commit</th>
                    <th className="px-4 py-2.5 text-right font-medium sm:pr-5">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {sources.map((src) => (
                    <tr key={src.id} className="transition hover:bg-overlay-subtle">
                      <td className="px-4 py-3 sm:pl-5">
                        <span className="block truncate font-mono text-xs font-medium text-text">
                          {src.repositoryOwner}/{src.repositoryName}
                        </span>
                        <span className="mt-0.5 block text-[11px] capitalize text-text-muted">
                          {src.provider || "—"}
                          {src.webhookId ? "" : src.autoDeploy ? " · webhook missing" : ""}
                        </span>
                      </td>
                      <td className="px-4 py-3 font-mono text-xs text-text">{src.branch}</td>
                      <td className="px-4 py-3">
                        {src.autoDeploy ? (
                          <span className="inline-flex items-center gap-1.5 rounded-full border border-ok-line bg-ok-subtle px-2.5 py-0.5 text-[11px] font-medium text-ok">
                            <span className="h-1.5 w-1.5 rounded-full bg-ok" /> On
                          </span>
                        ) : (
                          <span className="inline-flex items-center gap-1.5 rounded-full border border-line bg-overlay px-2.5 py-0.5 text-[11px] font-medium text-text-subtle">
                            <span className="h-1.5 w-1.5 rounded-full bg-text-muted" /> Off
                          </span>
                        )}
                        {src.autoDeploy && !src.webhookId && (
                          <span className="mt-1 flex items-center gap-1 text-[11px] text-warn" title={(src as GitSource).webhookSetupError || "provider webhook setup failed or token lacks webhook permission"}>
                            <AlertTriangle size={12} /> No webhook
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-3 text-xs text-text-subtle">
                        {src.lastCommitSha ? (
                          <>
                            <span className="block font-mono text-[11px] text-text">{src.lastCommitSha.slice(0, 7)}</span>
                            <span className="block max-w-56 truncate text-[11px] text-text-muted">{src.lastCommitMessage}</span>
                          </>
                        ) : "—"}
                      </td>
                      <td className="px-4 py-3 text-right sm:pr-5">
                        <div className="flex justify-end gap-1.5">
                          {src.webhookUrl && (
                            <button
                              type="button"
                              onClick={() => { navigator.clipboard.writeText(src.webhookUrl); toast({ tone: "success", title: "Webhook URL copied" }); }}
                              className="grid h-8 w-8 place-items-center rounded-lg border border-line bg-overlay-subtle text-text-subtle transition hover:bg-overlay-strong hover:text-text"
                              title="Copy webhook URL"
                              aria-label="Copy webhook URL"
                            >
                              <Link size={14} />
                            </button>
                          )}
                          <button
                            type="button"
                            onClick={() => { void (async () => { if (await confirm({ title: "Remove this git source?", description: "The source deployment and its history will be removed. This cannot be undone.", danger: true, confirmLabel: "Remove" })) deleteSource.mutate(src.id); })(); }}
                            className="grid h-8 w-8 place-items-center rounded-lg border border-line bg-overlay-subtle text-danger transition hover:bg-danger-subtle"
                            title="Remove source"
                            aria-label={`Remove ${src.repositoryOwner}/${src.repositoryName}`}
                          >
                            <Trash2 size={14} />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              )}
              {sourcesQuery.isSuccess && sources.length === 0 && (
                <EmptyState
                  icon={Database}
                  title="No repository sources"
                  message="No repositories linked yet — link one to enable auto-deploy."
                />
              )}
            </div>

            {showCreateSource && (
              <div className="border-t border-[var(--line)] p-4 sm:p-5">
                <SourceForm
                  credentials={credentials}
                  providerTokens={providerTokens}
                  onSubmit={(data) => createSource.mutate(data)}
                  onCancel={() => setShowCreateSource(false)}
                  loading={createSource.isPending}
                />
              </div>
            )}
          </section>
        </div>
      )}
      {showDocs && <GitDocsModal onClose={() => setShowDocs(false)} />}
      {renderConfirm()}
    </AdminPageLayout>
  );
}

function GitDocsModal({ onClose }: { onClose: () => void }) {
  return (
    <div
      className="fixed inset-0 z-50 overflow-y-auto bg-black/70 p-4 backdrop-blur-sm sm:p-6"
      role="presentation"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Git Integrations documentation"
        className="mx-auto w-full max-w-2xl overflow-hidden rounded-2xl border border-line bg-[var(--surface)] shadow-2xl"
      >
        <div className="flex items-start justify-between gap-4 border-b border-line px-5 pb-4 pt-5 sm:px-6">
          <div className="flex items-start gap-3">
            <span className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-line bg-overlay text-text">
              <BookOpen size={18} />
            </span>
            <div>
              <h2 className="text-base font-bold text-text">Git Integrations</h2>
              <p className="mt-0.5 text-xs text-text-subtle">Credentials, providers, sources, and auto-deploy webhooks.</p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close documentation"
            className="grid h-8 w-8 shrink-0 place-items-center rounded-lg text-text-subtle transition hover:bg-overlay-strong hover:text-text"
          >
            <X size={16} />
          </button>
        </div>
        <div className="max-h-[70vh] space-y-5 overflow-y-auto px-5 py-5 text-sm leading-6 text-text sm:px-6">
          <section>
            <h3 className="text-[13px] font-bold text-text">Workflow</h3>
            <ol className="mt-2 list-decimal space-y-1.5 pl-5 text-xs text-text-subtle">
              <li><span className="font-semibold text-text">Add credentials</span> — store deploy keys (SSH) or HTTPS tokens. Secrets stay server-side, encrypted.</li>
              <li><span className="font-semibold text-text">Connect provider</span> — link a GitHub, GitLab, Bitbucket, or Gitea account with a personal access token. Test it inline before saving.</li>
              <li><span className="font-semibold text-text">Add repository sources</span> — link repositories, choosing public access, a deploy credential, or a provider token.</li>
              <li><span className="font-semibold text-text">Auto-deploy</span> — with a provider token linked, Forge installs a webhook so pushes trigger builds and deployments.</li>
            </ol>
          </section>
          <section>
            <h3 className="text-[13px] font-bold text-text">Rules that matter</h3>
            <ul className="mt-2 list-disc space-y-1.5 pl-5 text-xs text-text-subtle">
              <li>Auto-deploy webhooks install only when the source is linked via a <span className="font-semibold text-text">provider token</span>. Credential-linked or public sources cannot install webhooks.</li>
              <li>A credential counts as <span className="font-semibold text-text">active</span> while at least one repository source uses it; unused credentials show as inactive.</li>
              <li>Provider tokens are validated live against the provider API — a failed test means the token is rejected or lacks permission.</li>
            </ul>
          </section>
          <section>
            <h3 className="text-[13px] font-bold text-text">API reference</h3>
            <div className="mt-2 space-y-1 font-mono text-[11px]">
              {[
                "GET /git/credentials · POST /git/credentials · DELETE /git/credentials/:id",
                "POST /git/credentials/:id/generate-key",
                "GET /git/providers · POST /git/providers · PATCH /git/providers/:id · DELETE /git/providers/:id",
                "POST /git/providers/test · POST /git/providers/:id/test",
                "GET /git/providers/:id/repos · GET /git/providers/:id/branches?repo=",
                "GET /git/sources · POST /git/sources · DELETE /git/sources/:id",
              ].map((line) => (
                <p key={line} className="rounded-md border border-line bg-overlay-subtle px-2.5 py-1.5 text-text-subtle">{line}</p>
              ))}
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}

function HowStep({ icon, tile, title, text }: { icon: React.ReactNode; tile: string; title: string; text: string }) {
  return (
    <li className="flex min-w-0 items-start gap-3">
      <span className={`grid h-11 w-11 shrink-0 place-items-center rounded-xl border ${tile}`}>
        {icon}
      </span>
      <span className="min-w-0">
        <span className="block text-[13px] font-semibold text-text">{title}</span>
        <span className="mt-0.5 block text-[11px] leading-4 text-text-muted">{text}</span>
      </span>
    </li>
  );
}

function HowArrow() {
  return (
    <li aria-hidden="true" className="hidden justify-center sm:flex xl:table-cell">
      <ArrowRight size={16} className="text-text-muted" />
    </li>
  );
}

function CredentialForm({
  onSubmit, onCancel, loading,
}: {
  onSubmit: (data: { name: string; credentialType: string; credential: string; description: string }) => void;
  onCancel: () => void;
  loading: boolean;
}) {
  const [name, setName] = useState("");
  const [credType, setCredType] = useState("ssh_key");
  const [credential, setCredential] = useState("");
  const [description, setDescription] = useState("");

  return (
    <div className="rounded-xl border border-[var(--line)] bg-overlay-subtle p-4 space-y-3">
      <h3 className="font-semibold text-sm text-text">New Credential</h3>
      <input
        placeholder="Name (e.g. GitHub Deploy Key)"
        value={name}
        onChange={(e) => setName(e.target.value)}
        className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
      />
      <select
        value={credType}
        onChange={(e) => setCredType(e.target.value)}
        className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text outline-none focus:border-[var(--brand)]"
      >
        <option value="ssh_key">SSH Key</option>
        <option value="https_token">HTTPS Token</option>
        <option value="https_password">HTTPS Username/Password</option>
      </select>
      {credType === "ssh_key" ? (
        <textarea
          placeholder="Paste SSH private key..."
          value={credential}
          onChange={(e) => setCredential(e.target.value)}
          className="w-full p-2 border border-line rounded-lg text-sm font-mono min-h-[100px] bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
        />
      ) : credType === "https_token" ? (
        <input
          placeholder="Access token"
          value={credential}
          onChange={(e) => setCredential(e.target.value)}
          className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
        />
      ) : (
        <input
          placeholder="username:password"
          value={credential}
          onChange={(e) => setCredential(e.target.value)}
          className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
        />
      )}
      <input
        placeholder="Description (optional)"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
      />
      <div className="flex gap-2 justify-end">
        <button onClick={onCancel} className="px-3 py-1.5 text-sm border border-line rounded-lg text-text hover:bg-overlay-strong">Cancel</button>
        <button
          onClick={() => onSubmit({ name, credentialType: credType, credential, description })}
          disabled={loading || !name || !credential}
          className="px-3 py-1.5 text-sm bg-[var(--brand)] text-white rounded-lg hover:opacity-90 disabled:opacity-50"
        >
          Create
        </button>
      </div>
    </div>
  );
}

function ProviderForm({
  onSubmit, onCancel, loading, onInlineTest, initialProvider,
}: {
  onSubmit: (data: { provider: string; providerName: string; accessToken: string; refreshToken: string; tokenType: string; baseUrl: string; username: string }) => void;
  onCancel: () => void;
  loading: boolean;
  onInlineTest?: (body: { provider: string; accessToken: string; baseUrl?: string }) => void;
  initialProvider?: string;
}) {
  const [provider, setProvider] = useState(initialProvider ?? "github");
  const [providerName, setProviderName] = useState("");
  const [accessToken, setAccessToken] = useState("");
  const [refreshToken, setRefreshToken] = useState("");
  const [tokenType] = useState("bearer");
  const [baseUrl, setBaseUrl] = useState("");
  const [username, setUsername] = useState("");

  return (
    <div className="rounded-xl border border-[var(--line)] bg-overlay-subtle p-4 space-y-3">
      <h3 className="font-semibold text-sm text-text">Connect Git Provider</h3>
      <select
        value={provider}
        onChange={(e) => setProvider(e.target.value)}
        className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text outline-none focus:border-[var(--brand)]"
      >
        <option value="github">GitHub</option>
        <option value="gitlab">GitLab</option>
        <option value="bitbucket">Bitbucket</option>
        <option value="gitea">Gitea</option>
      </select>
      <input
        placeholder="Display name (e.g. Personal GitHub)"
        value={providerName}
        onChange={(e) => setProviderName(e.target.value)}
        className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
      />
      <input
        placeholder="Access token / Personal access token"
        value={accessToken}
        onChange={(e) => setAccessToken(e.target.value)}
        className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
        type="password"
        autoComplete="off"
      />
      {provider === "gitea" && (
        <input
          placeholder="Gitea base URL (e.g. https://git.example.com)"
          value={baseUrl}
          onChange={(e) => setBaseUrl(e.target.value)}
          className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
        />
      )}
      <input
        placeholder="Refresh token (optional)"
        value={refreshToken}
        onChange={(e) => setRefreshToken(e.target.value)}
        className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
        type="password"
        autoComplete="off"
      />
      <input
        placeholder="Username (optional)"
        value={username}
        onChange={(e) => setUsername(e.target.value)}
        className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
      />
      <p className="text-[11px] text-text-muted">Inline test validates the token without persisting it.</p>
      <div className="flex gap-2 justify-end">
        <button
          type="button"
          onClick={() => onInlineTest?.({ provider, accessToken, baseUrl: baseUrl || undefined })}
          disabled={!accessToken}
          className="px-3 py-1.5 text-sm border border-line rounded-lg text-text hover:bg-overlay-strong disabled:opacity-40"
        >
          Test (inline)
        </button>
        <button onClick={onCancel} className="px-3 py-1.5 text-sm border border-line rounded-lg text-text hover:bg-overlay-strong">Cancel</button>
        <button
          onClick={() => onSubmit({ provider, providerName, accessToken, refreshToken, tokenType, baseUrl, username })}
          disabled={loading || !accessToken}
          className="px-3 py-1.5 text-sm bg-[var(--brand)] text-white rounded-lg hover:opacity-90 disabled:opacity-50"
        >
          Connect
        </button>
      </div>
    </div>
  );
}

function SourceForm({
  credentials, providerTokens, onSubmit, onCancel, loading,
}: {
  credentials: GitCredential[];
  providerTokens: GitProviderToken[];
  onSubmit: (data: {
    credentialId?: string; providerTokenId?: string; provider: string;
    repositoryUrl: string; repositoryName: string; repositoryOwner: string;
    branch: string; autoDeploy: boolean;
  }) => void;
  onCancel: () => void;
  loading: boolean;
}) {
  const [repoUrl, setRepoUrl] = useState("");
  const [repoName, setRepoName] = useState("");
  const [repoOwner, setRepoOwner] = useState("");
  const [branch, setBranch] = useState("main");
  const [autoDeploy, setAutoDeploy] = useState(true);
  const [credentialId, setCredentialId] = useState("");
  const [providerTokenId, setProviderTokenId] = useState("");
  const [provider, setProvider] = useState("");
  const [authMode, setAuthMode] = useState<"credential" | "provider" | "none">("none");
  const showAutoDeployWarning = autoDeploy && authMode !== "provider";

  return (
    <div className="rounded-xl border border-[var(--line)] bg-overlay-subtle p-4 space-y-3">
      <h3 className="font-semibold text-sm text-text">Link Repository</h3>
      {showAutoDeployWarning && (
        <div className="rounded-lg border border-warn-line bg-warn-subtle p-2.5 text-xs text-warn flex items-center gap-2">
          <AlertTriangle size={14} /> Auto-deploy requires a Provider Token — generic/credential mode cannot install webhooks
        </div>
      )}

      <div className="flex gap-2">
        {(["none", "credential", "provider"] as const).map((mode) => (
          <button
            key={mode}
            onClick={() => setAuthMode(mode)}
            className={`px-3 py-1.5 text-xs rounded-lg ${
              authMode === mode ? "bg-[var(--brand)] text-white" : "border border-line text-text hover:bg-overlay-strong"
            }`}
          >
            {mode === "none" ? "Public" : mode === "credential" ? "Deploy Key/Credential" : "Provider Token"}
          </button>
        ))}
      </div>

      {authMode === "credential" && (
        <select
          value={credentialId}
          onChange={(e) => setCredentialId(e.target.value)}
          className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text outline-none focus:border-[var(--brand)]"
        >
          <option value="">Select credential...</option>
          {Array.isArray(credentials) && credentials.map((c) => (
            <option key={c.id} value={c.id}>{c.name} ({c.credentialType})</option>
          ))}
        </select>
      )}

      {authMode === "provider" && (
        <select
          value={providerTokenId}
          onChange={(e) => {
            setProviderTokenId(e.target.value);
            const pt = Array.isArray(providerTokens) ? providerTokens.find((t) => t.id === e.target.value) : undefined;
            if (pt) setProvider(pt.provider);
          }}
          className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text outline-none focus:border-[var(--brand)]"
        >
          <option value="">Select provider...</option>
          {Array.isArray(providerTokens) && providerTokens.map((pt) => (
            <option key={pt.id} value={pt.id}>
              {pt.provider}{pt.providerName ? ` (${pt.providerName})` : ""}
            </option>
          ))}
        </select>
      )}

      <input
        placeholder="Repository URL (e.g. https://github.com/user/repo.git)"
        value={repoUrl}
        onChange={(e) => setRepoUrl(e.target.value)}
        className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
      />
      <div className="grid grid-cols-2 gap-2">
        <input
          placeholder="Repository owner"
          value={repoOwner}
          onChange={(e) => setRepoOwner(e.target.value)}
          className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
        />
        <input
          placeholder="Repository name"
          value={repoName}
          onChange={(e) => setRepoName(e.target.value)}
          className="w-full p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
        />
      </div>
      <div className="flex items-center gap-2">
        <input
          placeholder="Branch"
          value={branch}
          onChange={(e) => setBranch(e.target.value)}
          className="flex-1 p-2 border border-line rounded-lg text-sm bg-[var(--surface-input)] text-text placeholder:text-text-muted outline-none focus:border-[var(--brand)]"
        />
        <label className="flex items-center gap-1.5 text-sm text-text">
          <input
            type="checkbox"
            checked={autoDeploy}
            onChange={(e) => setAutoDeploy(e.target.checked)}
          />
          Auto-deploy
        </label>
      </div>
      {autoDeploy && !providerTokenId && (
        <p className="text-xs text-warn">Auto-deploy will be saved but webhook will not be installed without a provider token — you can link provider later and re-enable</p>
      )}
      <div className="flex gap-2 justify-end">
        <button onClick={onCancel} className="px-3 py-1.5 text-sm border border-line rounded-lg text-text hover:bg-overlay-strong">Cancel</button>
        <button
          onClick={() => onSubmit({
            credentialId: authMode === "credential" && credentialId ? credentialId : undefined,
            providerTokenId: authMode === "provider" && providerTokenId ? providerTokenId : undefined,
            provider,
            repositoryUrl: repoUrl,
            repositoryName: repoName,
            repositoryOwner: repoOwner,
            branch,
            autoDeploy,
          })}
          disabled={loading || !repoUrl || !repoName}
          className="px-3 py-1.5 text-sm bg-[var(--brand)] text-white rounded-lg hover:opacity-90 disabled:opacity-50"
        >
          Link
        </button>
      </div>
    </div>
  );
}
