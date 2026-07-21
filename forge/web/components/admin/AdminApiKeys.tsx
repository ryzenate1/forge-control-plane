"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Check, ChevronDown, ChevronUp, Copy, KeyRound, Plus, Shield, Trash2 } from "lucide-react";
import { createApiKey, deleteApiKey, fetchAdminScopes, fetchApiKeys, verifyBearerToken } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { Btn, Card, CardHeader, EmptyState, Input, SectionHeader } from "./admin-ui";

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

function scopeGroups(scopes: Record<string, string>) {
  return Object.entries(scopes).reduce<Record<string, { scope: string; description: string }[]>>((groups, [scope, description]) => {
    const resource = scope.split(".")[0] ?? "other";
    const group = resource === "nests" ? "Nests & Eggs" : `${resource.charAt(0).toUpperCase()}${resource.slice(1)}`;
    (groups[group] ??= []).push({ scope, description });
    return groups;
  }, {});
}

function ScopeLabel({ scope }: { scope: string }) {
  const parts = scope.split(".");
  const action = parts[1] ?? scope;
  const colors: Record<string, string> = {
    read: "bg-emerald-700/30 text-emerald-300",
    write: "bg-amber-700/30 text-amber-300",
    delete: "bg-rose-700/30 text-rose-300",
  };
  return (
    <span className={`inline-block rounded px-1.5 py-0.5 text-[10px] font-mono ${colors[action] ?? "bg-slate-700/40 text-slate-300"}`}>
      {scope}
    </span>
  );
}

export function AdminApiKeys() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const { data: keys = [], isLoading } = useQuery({ queryKey: ["api-keys"], queryFn: fetchApiKeys });
  const scopesQuery = useQuery({ queryKey: ["admin-scopes"], queryFn: fetchAdminScopes });
  const groupedScopes = scopeGroups(scopesQuery.data ?? {});

  const [desc, setDesc] = useState("");
  const [selectedScopes, setSelectedScopes] = useState<string[]>([]);
  const [showScopes, setShowScopes] = useState(false);
  const [newToken, setNewToken] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [verification, setVerification] = useState<string>("");

  const toggleScope = (scope: string) => {
    setSelectedScopes((prev) =>
      prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope]
    );
  };

  const toggleGroup = (scopes: string[]) => {
    const allSelected = scopes.every((s) => selectedScopes.includes(s));
    if (allSelected) {
      setSelectedScopes((prev) => prev.filter((s) => !scopes.includes(s)));
    } else {
      setSelectedScopes((prev) => [...new Set([...prev, ...scopes])]);
    }
  };

  const selectAll = () => {
    setSelectedScopes(["*"]);
  };

  const clearAll = () => {
    setSelectedScopes([]);
  };

  const isFullAccess = selectedScopes.includes("*");

  const createMut = useMutation({
    mutationFn: () =>
      createApiKey({
        description: desc.trim(),
        scopes: isFullAccess ? ["*"] : selectedScopes,
      }),
    onSuccess: (key) => {
      qc.invalidateQueries({ queryKey: ["api-keys"] });
      toast({ tone: "success", title: "API key created" });
      setDesc("");
      setSelectedScopes([]);
      setShowScopes(false);
      setVerification("");
      if (key.token) {
        setNewToken(key.token);
        verifyBearerToken(key.token)
          .then((user) => setVerification(`Verified: token authenticated as ${user.email}.`))
          .catch((error) => setVerification(error instanceof Error ? error.message : "Token verification failed."));
      }
    },
    onError: (error) => toast({ tone: "error", title: "API key could not be created", message: errorMessage(error, "Please try again.") }),
  });

  const deleteMut = useMutation({
    mutationFn: deleteApiKey,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["api-keys"] });
      toast({ tone: "success", title: "API key revoked" });
    },
    onError: (error) => toast({ tone: "error", title: "API key could not be revoked", message: errorMessage(error, "Please try again.") }),
  });

  const copyToken = () => {
    if (!newToken) return;
    navigator.clipboard.writeText(newToken).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <div>
       <SectionHeader title="Application API Keys" sub="Scoped API tokens with granular permissions." />

      <div className="grid gap-6 lg:grid-cols-2">
        {/* Create form */}
        <Card>
          <CardHeader title="Create new API key" icon={KeyRound} />
          <div className="p-4 space-y-4">
            <Input label="Description" value={desc} onChange={setDesc} placeholder="My automation script" />

            {/* Scope selector */}
            <div>
              <button
                type="button"
                aria-expanded={showScopes}
                aria-controls="api-key-scope-selector"
                onClick={() => setShowScopes((visible) => !visible)}
                className="flex w-full items-center justify-between rounded-lg border border-white/10 bg-[#161b28] px-3 py-2 text-sm text-slate-300 hover:border-white/20 transition"
              >
                <span>
                  {isFullAccess
                    ? "Full Access (*)"
                    : selectedScopes.length === 0
                      ? "Select permissions..."
                      : `${selectedScopes.length} scope${selectedScopes.length !== 1 ? "s" : ""} selected`}
                </span>
                {showScopes ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
              </button>

              {showScopes && (
                <div id="api-key-scope-selector" className="mt-2 rounded-lg border border-white/10 bg-[#0f1319] p-3 space-y-3 max-h-80 overflow-y-auto">
                  {/* Quick actions */}
                  <div className="flex gap-2 pb-2 border-b border-white/[0.06]">
                    <button type="button" onClick={selectAll} className={`px-2 py-1 rounded text-xs transition ${isFullAccess ? "bg-[#dc2626]/20 text-[#dc2626]" : "bg-white/5 text-slate-400 hover:text-white"}`}>
                      Full Access (*)
                    </button>
                    <button type="button" onClick={clearAll} className="px-2 py-1 rounded text-xs bg-white/5 text-slate-400 hover:text-white transition">
                      Clear All
                    </button>
                  </div>

                  {!isFullAccess && scopesQuery.isPending ? (
                    <p className="text-xs text-slate-400">Loading available permissions...</p>
                  ) : !isFullAccess && scopesQuery.isError ? (
                    <div className="flex items-start justify-between gap-3 rounded-lg border border-red-500/20 bg-red-950/10 p-2 text-xs text-red-200"><span>Available permissions could not be loaded: {scopesQuery.error.message}</span><Btn size="sm" tone="ghost" type="button" onClick={() => void scopesQuery.refetch()}>Retry</Btn></div>
                  ) : !isFullAccess && Object.entries(groupedScopes).map(([group, entries]) => {
                    const scopes = entries.map(({ scope }) => scope);
                    return (
                      <div key={group}>
                        <div className="flex items-center gap-2 mb-1.5">
                          <button type="button" onClick={() => toggleGroup(scopes)} className="flex items-center gap-1.5 text-xs font-semibold text-slate-300 hover:text-white transition">
                            <span className={`flex h-3 w-3 items-center justify-center rounded border ${scopes.every((scope) => selectedScopes.includes(scope)) ? "bg-[#dc2626] border-[#dc2626]" : "border-white/20"}`}>
                              {scopes.every((scope) => selectedScopes.includes(scope)) && <Check size={8} className="text-white" />}
                            </span>
                            {group}
                          </button>
                        </div>
                        <div className="flex flex-wrap gap-1.5 ml-5">
                          {entries.map(({ scope, description }) => (
                            <button key={scope} title={description} type="button" onClick={() => toggleScope(scope)} className={`rounded px-2 py-1 text-[11px] font-mono transition ${selectedScopes.includes(scope) ? "bg-[#dc2626]/20 text-[#dc2626] border border-[#dc2626]/30" : "bg-white/5 text-slate-400 border border-white/[0.06] hover:border-white/20"}`}>
                              {scope.split(".")[1]}
                            </button>
                          ))}
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>

            <div className="rounded-lg border border-amber-700/30 bg-amber-900/10 px-3 py-2 flex gap-2 text-xs text-amber-300">
              <AlertTriangle size={14} className="mt-0.5 shrink-0" />
              <span>{isFullAccess ? "Full access keys can perform any action. Use cautiously." : "Select only the permissions your application needs."}</span>
            </div>
            <Btn onClick={() => createMut.mutate()} disabled={desc.trim() === "" || selectedScopes.length === 0 || createMut.isPending}>
              <Plus size={14} /> Create Key
            </Btn>
          </div>

          {newToken ? (
            <div className="mx-4 mb-4 rounded-lg border border-emerald-700/40 bg-emerald-900/20 p-3">
              <p className="text-xs text-emerald-300 font-semibold mb-2 flex items-center gap-1"><Check size={12} /> Key created - copy it now, it won&apos;t be shown again.</p>
              <div className="flex items-center gap-2">
                <code className="flex-1 rounded bg-[#0f1419] px-2 py-1.5 text-xs text-emerald-200 font-mono break-all">{newToken}</code>
                <button aria-label="Copy API key" onClick={copyToken} className="shrink-0 text-emerald-400 hover:text-emerald-200 transition" type="button">
                  {copied ? <Check size={16} /> : <Copy size={16} />}
                </button>
              </div>
              {verification ? <p className="mt-2 text-xs text-emerald-200">{verification}</p> : <p className="mt-2 text-xs text-emerald-200">Verifying token authentication...</p>}
              <p className="mt-2 text-xs text-slate-400">Scopes are enforced by the API. Use this token as an external Bearer token; routes outside the selected scopes return 403.</p>
            </div>
          ) : null}
        </Card>

        {/* Keys list */}
        <Card>
          <CardHeader title="Existing keys" icon={Shield} />
          {isLoading ? (
            <div className="py-10 text-center text-sm text-slate-500">Loading...</div>
          ) : !keys || keys.length === 0 ? (
            <EmptyState icon={KeyRound} message="No API keys yet." />
          ) : (
            <ul className="divide-y divide-white/[0.04]">
              {keys.map((key) => (
                <li key={key.id} className="px-4 py-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-slate-200">{key.description || <span className="text-slate-500">Unnamed key</span>}</p>
                      <p className="text-xs text-slate-500 mt-0.5">
                        Created {new Date(key.createdAt).toLocaleDateString()}
                        {key.lastUsedAt ? ` \u2022 Last used ${new Date(key.lastUsedAt).toLocaleDateString()}` : " \u2022 Never used"}
                      </p>
                      {key.scopes && key.scopes.length > 0 && (
                        <div className="flex flex-wrap gap-1 mt-1.5">
                          {key.scopes.map((scope) => (
                            <ScopeLabel key={scope} scope={scope} />
                          ))}
                        </div>
                      )}
                    </div>
                    <Btn size="sm" tone="danger" onClick={() => deleteMut.mutate(key.id)} disabled={deleteMut.isPending}><Trash2 size={12} /></Btn>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>
    </div>
  );
}
