"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { listGitProviders, connectGitProvider, disconnectGitProvider, listProviderRepos, listProviderBranches, type GitProvider, type GitProviderRepo, type GitProviderBranch } from "@/lib/api/source-deployments";
import { Plus, Trash2, RefreshCw, Globe, Key, GitBranch, CheckCircle, XCircle } from "lucide-react";

const providerIcons: Record<string, string> = {
  github: "https://github.com/favicon.ico",
  gitlab: "https://gitlab.com/favicon.ico",
  bitbucket: "https://bitbucket.org/favicon.ico",
  gitea: "/gitea-favicon.ico",
  generic: "",
};

function ProviderIcon({ provider }: { provider: string }) {
  if (provider === "generic") return <Globe className="w-5 h-5" />;
  return (
    <img
      src={providerIcons[provider] || `https://${provider}.com/favicon.ico`}
      alt={provider}
      className="w-5 h-5 rounded"
      onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }}
    />
  );
}

export default function GitProvidersPage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [showConnect, setShowConnect] = useState(false);
  const [connectForm, setConnectForm] = useState({ provider: "github", accessToken: "", baseUrl: "", name: "" });

  const { data: providers, isLoading } = useQuery({
    queryKey: ["gitProviders"],
    queryFn: listGitProviders,
  });

  const connectMutation = useMutation({
    mutationFn: () => connectGitProvider({
      provider: connectForm.provider,
      accessToken: connectForm.accessToken,
      baseUrl: connectForm.baseUrl || undefined,
      providerName: connectForm.name || undefined,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gitProviders"] });
      setShowConnect(false);
      setConnectForm({ provider: "github", accessToken: "", baseUrl: "", name: "" });
      toast({ title: "Provider connected", message: "Git provider has been connected successfully.", tone: "success" });
    },
    onError: () => {
      toast({ title: "Connection failed", message: "Failed to connect git provider. Check your token.", tone: "error" });
    },
  });

  const disconnectMutation = useMutation({
    mutationFn: (id: string) => disconnectGitProvider(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gitProviders"] });
      toast({ title: "Provider disconnected", tone: "success" });
    },
    onError: () => {
      toast({ title: "Disconnect failed", tone: "error" });
    },
  });

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold">Git Providers</h1>
          <p className="text-sm opacity-70">Connect your git hosting providers</p>
        </div>
        <button
          onClick={() => setShowConnect(!showConnect)}
          className="btn btn-primary flex items-center gap-2"
        >
          <Plus className="w-4 h-4" /> Connect Provider
        </button>
      </div>

      {showConnect && (
        <div className="card p-4 mb-6">
          <h2 className="text-lg font-semibold mb-4">Connect a Git Provider</h2>
          <div className="grid gap-4">
            <div>
              <label className="block text-sm font-medium mb-1">Provider</label>
              <select
                className="input w-full"
                value={connectForm.provider}
                onChange={(e) => setConnectForm({ ...connectForm, provider: e.target.value })}
              >
                <option value="github">GitHub</option>
                <option value="gitlab">GitLab</option>
                <option value="bitbucket">Bitbucket</option>
                <option value="gitea">Gitea</option>
                <option value="generic">Generic</option>
              </select>
            </div>
            {connectForm.provider === "generic" && (
              <div>
                <label className="block text-sm font-medium mb-1">Provider Name</label>
                <input
                  className="input w-full"
                  placeholder="My Git Server"
                  value={connectForm.name}
                  onChange={(e) => setConnectForm({ ...connectForm, name: e.target.value })}
                />
              </div>
            )}
            {connectForm.provider === "gitea" && (
              <div>
                <label className="block text-sm font-medium mb-1">Base URL</label>
                <input
                  className="input w-full"
                  placeholder="https://gitea.example.com"
                  value={connectForm.baseUrl}
                  onChange={(e) => setConnectForm({ ...connectForm, baseUrl: e.target.value })}
                />
              </div>
            )}
            <div>
              <label className="block text-sm font-medium mb-1">Access Token</label>
              <input
                className="input w-full"
                type="password"
                placeholder="Personal access token"
                value={connectForm.accessToken}
                onChange={(e) => setConnectForm({ ...connectForm, accessToken: e.target.value })}
              />
            </div>
            <div className="flex gap-2 justify-end">
              <button
                onClick={() => setShowConnect(false)}
                className="btn btn-ghost"
              >
                Cancel
              </button>
              <button
                onClick={() => connectMutation.mutate()}
                className="btn btn-primary"
                disabled={!connectForm.accessToken.trim() || connectMutation.isPending}
              >
                {connectMutation.isPending ? "Connecting..." : "Connect"}
              </button>
            </div>
          </div>
        </div>
      )}

      {isLoading ? (
        <div className="text-center py-8 opacity-50">Loading providers...</div>
      ) : providers && providers.length > 0 ? (
        <div className="grid gap-4">
          {providers.map((p) => (
            <div key={p.id} className="card p-4 flex items-center justify-between">
              <div className="flex items-center gap-3">
                <ProviderIcon provider={p.type} />
                <div>
                  <div className="font-medium">{p.username || p.name}</div>
                  <div className="text-sm opacity-60 flex items-center gap-2">
                    <span className="capitalize">{p.type}</span>
                    {p.type !== "generic" && p.baseUrl && (
                      <span className="text-xs">{p.baseUrl}</span>
                    )}
                    {p.accessToken && <Key className="w-3 h-3" />}
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <span className={`text-xs px-2 py-1 rounded-full ${p.accessToken ? 'bg-green-500/20 text-green-400' : 'bg-yellow-500/20 text-yellow-400'}`}>
                  {p.accessToken ? 'Connected' : 'No Token'}
                </span>
                <button
                  onClick={() => disconnectMutation.mutate(p.id)}
                  className="btn btn-ghost btn-sm text-red-400"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="text-center py-12 card">
          <Globe className="w-12 h-12 mx-auto mb-3 opacity-30" />
          <p className="opacity-60">No git providers connected yet.</p>
          <p className="text-sm opacity-40">Connect a provider to deploy from repositories.</p>
        </div>
      )}
    </div>
  );
}
