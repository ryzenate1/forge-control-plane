"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import {
  Key, Link, GitBranch, Trash2, Plus, RefreshCw, Webhook,
  Shield, Globe, CheckCircle, XCircle
} from "lucide-react";

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? (process.env.NODE_ENV === "development" ? "http://localhost:8080/api/v1" : "/api/v1");

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
  const icons: Record<string, string> = {
    github: "https://github.com/favicon.ico",
    gitlab: "https://gitlab.com/favicon.ico",
    bitbucket: "https://bitbucket.org/favicon.ico",
    gitea: "/gitea-favicon.ico",
  };
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={icons[provider] || `https://${provider}.com/favicon.ico`}
      alt={provider}
      className="w-5 h-5 rounded"
      onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }}
    />
  );
}

export default function GitPage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [tab, setTab] = useState<Tab>("credentials");
  const [showCreateCredential, setShowCreateCredential] = useState(false);
  const [showConnectProvider, setShowConnectProvider] = useState(false);
  const [showCreateSource, setShowCreateSource] = useState(false);
  const [selectedProviderRepos, setSelectedProviderRepos] = useState<GitProviderRepo[]>([]);
  const [selectedProviderBranches, setSelectedProviderBranches] = useState<GitProviderBranch[]>([]);
  const [loadingRepos, setLoadingRepos] = useState("");
  const [loadingBranches, setLoadingBranches] = useState("");

  const { data: credentials = [] } = useQuery<GitCredential[]>({
    queryKey: ["git-credentials"],
    queryFn: async () => {
      const res = await fetch(`${API_BASE}/git/credentials`, { credentials: "include" });
      if (!res.ok) throw new Error("Failed to fetch credentials");
      return res.json();
    },
  });

  const { data: providerTokens = [] } = useQuery<GitProviderToken[]>({
    queryKey: ["git-providers"],
    queryFn: async () => {
      const res = await fetch(`${API_BASE}/git/providers`, { credentials: "include" });
      if (!res.ok) throw new Error("Failed to fetch providers");
      return res.json();
    },
  });

  const { data: sources = [] } = useQuery<GitSource[]>({
    queryKey: ["git-sources"],
    queryFn: async () => {
      const res = await fetch(`${API_BASE}/git/sources`, { credentials: "include" });
      if (!res.ok) throw new Error("Failed to fetch sources");
      return res.json();
    },
  });

  const createCredential = useMutation({
    mutationFn: async (data: { name: string; credentialType: string; credential: string; description: string }) => {
      const res = await fetch(`${API_BASE}/git/credentials`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(data),
      });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["git-credentials"] });
      setShowCreateCredential(false);
      toast({ tone: "success", title: "Credential created" });
    },
  });

  const deleteCredential = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`${API_BASE}/git/credentials/${id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!res.ok) throw new Error("Delete failed");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["git-credentials"] });
      toast({ tone: "success", title: "Credential deleted" });
    },
  });

  const generateKey = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`${API_BASE}/git/credentials/${id}/generate-key`, {
        method: "POST",
        credentials: "include",
      });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ["git-credentials"] });
      toast({ tone: "success", title: `Deploy key generated: ${data.type}` });
    },
  });

  const connectProvider = useMutation({
    mutationFn: async (data: {
      provider: string; providerName: string; accessToken: string;
      refreshToken: string; tokenType: string; baseUrl: string; username: string;
    }) => {
      const res = await fetch(`${API_BASE}/git/providers`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(data),
      });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["git-providers"] });
      setShowConnectProvider(false);
      toast({ tone: "success", title: "Provider connected" });
    },
  });

  const disconnectProvider = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`${API_BASE}/git/providers/${id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!res.ok) throw new Error("Disconnect failed");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["git-providers"] });
      toast({ tone: "success", title: "Provider disconnected" });
    },
  });

  const createSource = useMutation({
    mutationFn: async (data: {
      credentialId?: string; providerTokenId?: string; provider: string;
      repositoryUrl: string; repositoryName: string; repositoryOwner: string;
      branch: string; autoDeploy: boolean;
    }) => {
      const res = await fetch(`${API_BASE}/git/sources`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(data),
      });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["git-sources"] });
      setShowCreateSource(false);
      toast({ tone: "success", title: "Git source linked" });
    },
  });

  const deleteSource = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`${API_BASE}/git/sources/${id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!res.ok) throw new Error("Delete failed");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["git-sources"] });
      toast({ tone: "success", title: "Git source removed" });
    },
  });

  const loadProviderRepos = async (tokenId: string) => {
    setLoadingRepos(tokenId);
    try {
      const res = await fetch(`${API_BASE}/git/providers/${tokenId}/repos`, { credentials: "include" });
      if (res.ok) {
        setSelectedProviderRepos(await res.json());
      } else {
        toast({ tone: "error", title: "Failed to load repositories" });
      }
    } finally {
      setLoadingRepos("");
    }
  };

  const loadProviderBranches = async (tokenId: string, repoFullName: string) => {
    setLoadingBranches(tokenId);
    try {
      const res = await fetch(
        `${API_BASE}/git/providers/${tokenId}/branches?repo=${encodeURIComponent(repoFullName)}`,
        { credentials: "include" }
      );
      if (res.ok) {
        setSelectedProviderBranches(await res.json());
      } else {
        toast({ tone: "error", title: "Failed to load branches" });
      }
    } finally {
      setLoadingBranches("");
    }
  };

  return (
    <div className="space-y-6 p-6">
      <h1 className="text-2xl font-bold">Git Integration</h1>

      <div className="flex gap-2 border-b">
        {(["credentials", "providers", "sources"] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`px-4 py-2 text-sm font-medium rounded-t-lg ${
              tab === t
                ? "bg-primary text-primary-foreground border-b-2 border-primary"
                : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>

      {tab === "credentials" && (
        <div className="space-y-4">
          <div className="flex justify-between items-center">
            <h2 className="text-lg font-semibold flex items-center gap-2">
              <Key className="w-5 h-5" /> Credentials
            </h2>
            <button
              onClick={() => setShowCreateCredential(true)}
              className="inline-flex items-center gap-1 px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:opacity-90"
            >
              <Plus className="w-4 h-4" /> Add Credential
            </button>
          </div>

          {(!credentials || credentials.length === 0) && (
            <p className="text-muted-foreground text-sm">No credentials configured.</p>
          )}

          {credentials?.map((cred) => (
            <div key={cred.id} className="border rounded-lg p-4 space-y-2">
              <div className="flex justify-between items-start">
                <div>
                  <p className="font-medium">{cred.name}</p>
                  <p className="text-xs text-muted-foreground">
                    Type: {cred.credentialType}
                    {cred.description && ` — ${cred.description}`}
                  </p>
                </div>
                <div className="flex gap-1">
                  {cred.credentialType === "ssh_key" && (
                    <button
                      onClick={() => generateKey.mutate(cred.id)}
                      className="inline-flex items-center gap-1 px-2 py-1 text-xs border rounded hover:bg-muted"
                      disabled={generateKey.isPending}
                    >
                      <RefreshCw className="w-3 h-3" /> Generate Key
                    </button>
                  )}
                  <button
                    onClick={() => { if (confirm("Delete this credential?")) deleteCredential.mutate(cred.id); }}
                    className="inline-flex items-center gap-1 px-2 py-1 text-xs border rounded text-red-500 hover:bg-red-50"
                  >
                    <Trash2 className="w-3 h-3" /> Delete
                  </button>
                </div>
              </div>
              {cred.publicKey && (
                <details className="text-xs">
                  <summary className="cursor-pointer text-muted-foreground">Public Key</summary>
                  <pre className="mt-1 p-2 bg-muted rounded text-[10px] overflow-x-auto">{cred.publicKey}</pre>
                </details>
              )}
              {cred.credential && (
                <p className="text-xs text-muted-foreground">
                  Credential: <Shield className="w-3 h-3 inline" /> Stored encrypted
                </p>
              )}
            </div>
          ))}

          {showCreateCredential && (
            <CredentialForm
              onSubmit={(data) => createCredential.mutate(data)}
              onCancel={() => setShowCreateCredential(false)}
              loading={createCredential.isPending}
            />
          )}
        </div>
      )}

      {tab === "providers" && (
        <div className="space-y-4">
          <div className="flex justify-between items-center">
            <h2 className="text-lg font-semibold flex items-center gap-2">
              <Globe className="w-5 h-5" /> Git Providers
            </h2>
            <button
              onClick={() => setShowConnectProvider(true)}
              className="inline-flex items-center gap-1 px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:opacity-90"
            >
              <Link className="w-4 h-4" /> Connect Provider
            </button>
          </div>

          {(!providerTokens || providerTokens.length === 0) && (
            <p className="text-muted-foreground text-sm">No providers connected.</p>
          )}

          {providerTokens?.map((pt) => (
            <div key={pt.id} className="border rounded-lg p-4 space-y-2">
              <div className="flex justify-between items-start">
                <div className="flex items-center gap-2">
                  <ProviderIcon provider={pt.provider} />
                  <div>
                    <p className="font-medium capitalize">{pt.provider}{pt.providerName ? ` - ${pt.providerName}` : ""}</p>
                    <p className="text-xs text-muted-foreground">
                      {pt.username && `@${pt.username} — `}{pt.tokenType} token{pt.accessToken && <Shield className="w-3 h-3 inline ml-1" />}
                    </p>
                  </div>
                </div>
                <div className="flex gap-1">
                  <button
                    onClick={() => loadProviderRepos(pt.id)}
                    className="inline-flex items-center gap-1 px-2 py-1 text-xs border rounded hover:bg-muted"
                    disabled={loadingRepos === pt.id}
                  >
                    <RefreshCw className={`w-3 h-3 ${loadingRepos === pt.id ? "animate-spin" : ""}`} /> Repos
                  </button>
                  <button
                    onClick={() => { if (confirm("Disconnect this provider?")) disconnectProvider.mutate(pt.id); }}
                    className="inline-flex items-center gap-1 px-2 py-1 text-xs border rounded text-red-500 hover:bg-red-50"
                  >
                    <Trash2 className="w-3 h-3" /> Disconnect
                  </button>
                </div>
              </div>

              {selectedProviderRepos.length > 0 && (
                <div className="ml-6 mt-2 space-y-1 border-l-2 pl-3">
                  <p className="text-xs font-medium text-muted-foreground">Repositories:</p>
                  {selectedProviderRepos.map((repo) => (
                    <div key={repo.fullName} className="flex items-center justify-between text-xs">
                      <span>
                        {repo.name}
                        {repo.private && <span className="ml-1 text-muted-foreground">(private)</span>}
                      </span>
                      <button
                        onClick={() => loadProviderBranches(pt.id, repo.fullName)}
                        className="text-blue-500 hover:underline text-xs"
                        disabled={loadingBranches === pt.id}
                      >
                        branches
                      </button>
                    </div>
                  ))}
                </div>
              )}

              {selectedProviderBranches.length > 0 && (
                <div className="ml-10 mt-1 space-y-1 text-xs text-muted-foreground">
                  {selectedProviderBranches.map((b) => (
                    <span key={b.name} className="mr-2 inline-flex items-center gap-1 bg-muted px-1.5 py-0.5 rounded">
                      <GitBranch className="w-3 h-3" /> {b.name}
                    </span>
                  ))}
                </div>
              )}
            </div>
          ))}

          {showConnectProvider && (
            <ProviderForm
              onSubmit={(data) => connectProvider.mutate(data)}
              onCancel={() => setShowConnectProvider(false)}
              loading={connectProvider.isPending}
            />
          )}
        </div>
      )}

      {tab === "sources" && (
        <div className="space-y-4">
          <div className="flex justify-between items-center">
            <h2 className="text-lg font-semibold flex items-center gap-2">
              <GitBranch className="w-5 h-5" /> Git Sources
            </h2>
            <button
              onClick={() => setShowCreateSource(true)}
              className="inline-flex items-center gap-1 px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded-md hover:opacity-90"
            >
              <Plus className="w-4 h-4" /> Link Repository
            </button>
          </div>

          {(!sources || sources.length === 0) && (
            <p className="text-muted-foreground text-sm">No repositories linked.</p>
          )}

          {sources?.map((src) => (
            <div key={src.id} className="border rounded-lg p-4 space-y-2">
              <div className="flex justify-between items-start">
                <div>
                  <p className="font-medium">
                    {src.repositoryOwner}/{src.repositoryName}
                    {src.provider && <span className="ml-2 text-xs text-muted-foreground capitalize">({src.provider})</span>}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    Branch: {src.branch} — Auto-deploy: {src.autoDeploy ? <CheckCircle className="w-3 h-3 inline text-green-500" /> : <XCircle className="w-3 h-3 inline text-gray-400" />}
                  </p>
                  {src.lastCommitSha && (
                    <p className="text-xs text-muted-foreground">
                      Last: {src.lastCommitSha.slice(0, 7)} — {src.lastCommitMessage?.slice(0, 80)}
                    </p>
                  )}
                </div>
                <button
                  onClick={() => { if (confirm("Remove this git source?")) deleteSource.mutate(src.id); }}
                  className="inline-flex items-center gap-1 px-2 py-1 text-xs border rounded text-red-500 hover:bg-red-50"
                >
                  <Trash2 className="w-3 h-3" /> Remove
                </button>
              </div>
              {src.webhookUrl && (
                <details className="text-xs">
                  <summary className="cursor-pointer text-muted-foreground flex items-center gap-1">
                    <Webhook className="w-3 h-3" /> Webhook details
                  </summary>
                  <div className="mt-1 p-2 bg-muted rounded space-y-1">
                    <p>URL: <code className="text-[10px]">{src.webhookUrl}</code></p>
                    {src.webhookId && <p>ID: {src.webhookId}</p>}
                    {src.webhookSecret && <p>Secret: <Shield className="w-3 h-3 inline" /> verified</p>}
                  </div>
                </details>
              )}
            </div>
          ))}

          {showCreateSource && (
            <SourceForm
              credentials={credentials}
              providerTokens={providerTokens}
              onSubmit={(data) => createSource.mutate(data)}
              onCancel={() => setShowCreateSource(false)}
              loading={createSource.isPending}
            />
          )}
        </div>
      )}
    </div>
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
    <div className="border rounded-lg p-4 bg-card space-y-3">
      <h3 className="font-medium text-sm">New Credential</h3>
      <input
        placeholder="Name (e.g. GitHub Deploy Key)"
        value={name}
        onChange={(e) => setName(e.target.value)}
        className="w-full p-2 border rounded text-sm"
      />
      <select
        value={credType}
        onChange={(e) => setCredType(e.target.value)}
        className="w-full p-2 border rounded text-sm"
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
          className="w-full p-2 border rounded text-sm font-mono min-h-[100px]"
        />
      ) : credType === "https_token" ? (
        <input
          placeholder="Access token"
          value={credential}
          onChange={(e) => setCredential(e.target.value)}
          className="w-full p-2 border rounded text-sm"
        />
      ) : (
        <input
          placeholder="username:password"
          value={credential}
          onChange={(e) => setCredential(e.target.value)}
          className="w-full p-2 border rounded text-sm"
        />
      )}
      <input
        placeholder="Description (optional)"
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        className="w-full p-2 border rounded text-sm"
      />
      <div className="flex gap-2 justify-end">
        <button onClick={onCancel} className="px-3 py-1.5 text-sm border rounded hover:bg-muted">Cancel</button>
        <button
          onClick={() => onSubmit({ name, credentialType: credType, credential, description })}
          disabled={loading || !name || !credential}
          className="px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded hover:opacity-90 disabled:opacity-50"
        >
          Create
        </button>
      </div>
    </div>
  );
}

function ProviderForm({
  onSubmit, onCancel, loading,
}: {
  onSubmit: (data: { provider: string; providerName: string; accessToken: string; refreshToken: string; tokenType: string; baseUrl: string; username: string }) => void;
  onCancel: () => void;
  loading: boolean;
}) {
  const [provider, setProvider] = useState("github");
  const [providerName, setProviderName] = useState("");
  const [accessToken, setAccessToken] = useState("");
  const [refreshToken, setRefreshToken] = useState("");
  const [tokenType] = useState("bearer");
  const [baseUrl, setBaseUrl] = useState("");
  const [username, setUsername] = useState("");

  return (
    <div className="border rounded-lg p-4 bg-card space-y-3">
      <h3 className="font-medium text-sm">Connect Git Provider</h3>
      <select
        value={provider}
        onChange={(e) => setProvider(e.target.value)}
        className="w-full p-2 border rounded text-sm"
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
        className="w-full p-2 border rounded text-sm"
      />
      <input
        placeholder="Access token / Personal access token"
        value={accessToken}
        onChange={(e) => setAccessToken(e.target.value)}
        className="w-full p-2 border rounded text-sm"
        type="password"
      />
      {provider === "gitea" && (
        <input
          placeholder="Gitea base URL (e.g. https://git.example.com)"
          value={baseUrl}
          onChange={(e) => setBaseUrl(e.target.value)}
          className="w-full p-2 border rounded text-sm"
        />
      )}
      <input
        placeholder="Refresh token (optional)"
        value={refreshToken}
        onChange={(e) => setRefreshToken(e.target.value)}
        className="w-full p-2 border rounded text-sm"
        type="password"
      />
      <input
        placeholder="Username (optional)"
        value={username}
        onChange={(e) => setUsername(e.target.value)}
        className="w-full p-2 border rounded text-sm"
      />
      <div className="flex gap-2 justify-end">
        <button onClick={onCancel} className="px-3 py-1.5 text-sm border rounded hover:bg-muted">Cancel</button>
        <button
          onClick={() => onSubmit({ provider, providerName, accessToken, refreshToken, tokenType, baseUrl, username })}
          disabled={loading || !accessToken}
          className="px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded hover:opacity-90 disabled:opacity-50"
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

  return (
    <div className="border rounded-lg p-4 bg-card space-y-3">
      <h3 className="font-medium text-sm">Link Repository</h3>

      <div className="flex gap-2">
        {(["none", "credential", "provider"] as const).map((mode) => (
          <button
            key={mode}
            onClick={() => setAuthMode(mode)}
            className={`px-3 py-1 text-xs rounded ${
              authMode === mode ? "bg-primary text-primary-foreground" : "border hover:bg-muted"
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
          className="w-full p-2 border rounded text-sm"
        >
          <option value="">Select credential...</option>
          {credentials.map((c) => (
            <option key={c.id} value={c.id}>{c.name} ({c.credentialType})</option>
          ))}
        </select>
      )}

      {authMode === "provider" && (
        <select
          value={providerTokenId}
          onChange={(e) => {
            setProviderTokenId(e.target.value);
            const pt = providerTokens.find((t) => t.id === e.target.value);
            if (pt) setProvider(pt.provider);
          }}
          className="w-full p-2 border rounded text-sm"
        >
          <option value="">Select provider...</option>
          {providerTokens.map((pt) => (
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
        className="w-full p-2 border rounded text-sm"
      />
      <div className="grid grid-cols-2 gap-2">
        <input
          placeholder="Repository owner"
          value={repoOwner}
          onChange={(e) => setRepoOwner(e.target.value)}
          className="w-full p-2 border rounded text-sm"
        />
        <input
          placeholder="Repository name"
          value={repoName}
          onChange={(e) => setRepoName(e.target.value)}
          className="w-full p-2 border rounded text-sm"
        />
      </div>
      <div className="flex items-center gap-2">
        <input
          placeholder="Branch"
          value={branch}
          onChange={(e) => setBranch(e.target.value)}
          className="flex-1 p-2 border rounded text-sm"
        />
        <label className="flex items-center gap-1 text-sm">
          <input
            type="checkbox"
            checked={autoDeploy}
            onChange={(e) => setAutoDeploy(e.target.checked)}
          />
          Auto-deploy
        </label>
      </div>
      <div className="flex gap-2 justify-end">
        <button onClick={onCancel} className="px-3 py-1.5 text-sm border rounded hover:bg-muted">Cancel</button>
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
          className="px-3 py-1.5 text-sm bg-primary text-primary-foreground rounded hover:opacity-90 disabled:opacity-50"
        >
          Link
        </button>
      </div>
    </div>
  );
}
