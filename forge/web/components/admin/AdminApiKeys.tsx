"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Check, ChevronDown, ChevronUp, Copy, Eye, EyeOff, KeyRound, Plus, Shield, Trash2 } from "lucide-react";
import { createApiKey, deleteApiKey, fetchAdminScopes, fetchApiKeys, verifyBearerToken, type ApiKey } from "@/lib/api";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { copySecret } from "@/lib/clipboard";
import { AdminErrorState, AdminFormSection, AdminIconButton, AdminLoadingState, AdminPageLayout, Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader, cn } from "./admin-ui";
import { FreshnessBadge } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { formatDate } from "@/lib/utils";

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

function scopeGroups(scopes: Record<string, string>) {
  return Object.entries(scopes).reduce<Record<string, { scope: string; description: string }[]>>((groups, [scope, description]) => {
    const resource = scope.split(".")[0] ?? "other";
    const group = resource === "nests" ? "Nests & Eggs" : `${resource.charAt(0).toUpperCase()}${resource.slice(1)}`;
    (groups[group] ??= []).push({ scope, description });
    return groups;
  }, {});
}

/** `read`/`write`/`delete` alone told you nothing about *which* resource the
 * scope covered; the full `servers.delete` string is the identifier, so it is
 * what the chip shows. Colour still comes from a tone name, never a class. */
function ScopeLabel({ scope }: { scope: string }) {
  const action = scope.split(".")[1] ?? "";
  const tone: "ok" | "warn" | "danger" | "neutral" =
    action === "read" ? "ok" : action === "write" ? "warn" : action === "delete" ? "danger" : "neutral";
  return <Pill tone={tone}>{scope}</Pill>;
}

type Verification = { state: "pending" | "ok" | "failed"; message: string };

export function AdminApiKeys() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const keysQuery = useQuery({ queryKey: ["api-keys"], queryFn: fetchApiKeys });
  const { isLoading, isError, error: keysError, refetch: refetchKeys, isSuccess } = keysQuery;
  const keys = useMemo(() => (Array.isArray(keysQuery.data) ? keysQuery.data : []), [keysQuery.data]);
  const scopesQuery = useQuery({ queryKey: ["admin-scopes"], queryFn: fetchAdminScopes });
  const groupedScopes = scopeGroups(scopesQuery.data ?? {});

  const [desc, setDesc] = useState("");
  const [selectedScopes, setSelectedScopes] = useState<string[]>([]);
  const [showScopes, setShowScopes] = useState(false);
  const [newToken, setNewToken] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [tokenMasked, setTokenMasked] = useState(false);
  const [verification, setVerification] = useState<Verification | null>(null);
  const [confirm, renderConfirm] = useConfirm();

  const toggleScope = (scope: string) => {
    setSelectedScopes((prev) => (prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope]));
  };

  const toggleGroup = (scopes: string[]) => {
    const allSelected = scopes.every((s) => selectedScopes.includes(s));
    if (allSelected) {
      setSelectedScopes((prev) => prev.filter((s) => !scopes.includes(s)));
    } else {
      setSelectedScopes((prev) => [...new Set([...prev, ...scopes])]);
    }
  };

  const selectAll = () => setSelectedScopes(["*"]);
  const clearAll = () => setSelectedScopes([]);
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
      setCopied(false);
      setTokenMasked(false);
      if (key.token) {
        setNewToken(key.token);
        // The verify call is a second network round-trip. It gets its own
        // state: pending is not success, and a failure must not render inside
        // the emerald "key created" panel the way it used to.
        setVerification({ state: "pending", message: "Verifying the token authenticates…" });
        verifyBearerToken(key.token)
          .then((user) => setVerification({ state: "ok", message: `Verified: the token authenticated as ${user.email}.` }))
          .catch((error) => setVerification({ state: "failed", message: errorMessage(error, "Token verification failed.") }));
      } else {
        setNewToken(null);
        setVerification({ state: "failed", message: "The key was created but the response carried no token to copy." });
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

  const copyToken = async () => {
    if (!newToken) return;
    if (await copySecret(newToken)) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const requestRevoke = (key: ApiKey) => {
    void (async () => {
      // `lastUsedAt` is `omitempty` on a pointer, so an absent value is the
      // server reporting "never used" — saying so here is what stops a revoke
      // from surprising an operator who has a live integration on the key.
      const usage = key.lastUsedAt
        ? `Last used ${formatDate(key.lastUsedAt, "an unknown time")}, so a client may still be calling with it.`
        : "This key has never been used, but a client that has not called yet still holds it.";
      if (await confirm({ title: "Revoke API key?", description: `"${key.description || "Unnamed key"}" (prefix ${key.tokenPrefix || "unknown"}) stops authenticating immediately for every client using it. ${usage} The secret is not recoverable and this cannot be undone.`, danger: true, confirmLabel: "Revoke" })) deleteMut.mutate(key.id);
    })();
  };

  useEffect(() => {
    const hide = () => setTokenMasked(true);
    const onVisibilityChange = () => { if (document.visibilityState === "hidden") hide(); };
    window.addEventListener("blur", hide);
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      window.removeEventListener("blur", hide);
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, []);

  const createBlockedReason = desc.trim() === ""
    ? "Give the key a description so you can recognise which client uses it."
    : selectedScopes.length === 0
      ? "Select at least one permission, or Full Access."
      : null;

  return (
    <AdminPageLayout>
      {/* Title, subtitle and glyph resolve from admin-registry.ts in the frame.
          The list is on-demand, so say how old the reading is instead of leaving
          revocation-sensitive rows undated. */}
      <SectionHeader status={<FreshnessBadge state={sourceState(keysQuery)} />} />

      <div className="grid gap-5 lg:grid-cols-2">
        {/* Create form */}
        <Card>
          <CardHeader title="Create new API key" icon={KeyRound} />
          <div className="space-y-4 p-4">
            <AdminFormSection title="Description">
              <Input label="Description" value={desc} onChange={setDesc} placeholder="My automation script" required />
            </AdminFormSection>

            <AdminFormSection title="Permissions">
              <div>
                <button
                  type="button"
                  aria-expanded={showScopes}
                  aria-controls="api-key-scope-selector"
                  onClick={() => setShowScopes((visible) => !visible)}
                  className="ui-input flex w-full items-center justify-between gap-2"
                >
                  <span>
                    {isFullAccess
                      ? "Full Access (*)"
                      : selectedScopes.length === 0
                        ? "Select permissions…"
                        : `${selectedScopes.length} scope${selectedScopes.length !== 1 ? "s" : ""} selected`}
                  </span>
                  {showScopes ? <ChevronUp size={14} aria-hidden="true" /> : <ChevronDown size={14} aria-hidden="true" />}
                </button>

                {showScopes ? (
                  <div id="api-key-scope-selector" className="mt-2 space-y-3 rounded-lg border border-line bg-surface p-3 max-h-80 overflow-y-auto">
                    <div className="flex gap-2 border-b border-line pb-2">
                      <button type="button" onClick={selectAll} aria-pressed={isFullAccess} className={cn("rounded px-2 py-1 text-xs transition", isFullAccess ? "ui-badge-brand ui-badge" : "ui-badge text-text-subtle hover:bg-overlay")}>
                        Full Access (*)
                      </button>
                      <button type="button" onClick={clearAll} aria-pressed={selectedScopes.length === 0} className="ui-badge text-text-subtle hover:bg-overlay" disabled={selectedScopes.length === 0}>
                        Clear All
                      </button>
                    </div>

                    {!isFullAccess && scopesQuery.isPending ? (
                      <AdminLoadingState label="Loading available permissions…" />
                    ) : !isFullAccess && scopesQuery.isError ? (
                      <AdminErrorState message={`Available permissions could not be loaded: ${errorMessage(scopesQuery.error, "unknown error")}`} retry={() => void scopesQuery.refetch()} />
                    ) : !isFullAccess && Object.entries(groupedScopes).map(([group, entries]) => {
                      const scopes = entries.map(({ scope }) => scope);
                      const groupChecked = scopes.every((scope) => selectedScopes.includes(scope));
                      return (
                        <div key={group}>
                          <div className="mb-1.5 flex items-center gap-2">
                            <button
                              type="button"
                              role="checkbox"
                              aria-checked={groupChecked}
                              onClick={() => toggleGroup(scopes)}
                              className="flex items-center gap-1.5 text-xs font-semibold text-text transition-colors hover:text-brand focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                            >
                              <span aria-hidden="true" className={cn("flex h-3.5 w-3.5 items-center justify-center rounded border", groupChecked ? "border-brand bg-brand text-white" : "border-line-strong")}>
                                {groupChecked ? <Check size={9} /> : null}
                              </span>
                              {group}
                              <span className="t-meta">
                                {entries.filter(({ scope }) => selectedScopes.includes(scope)).length}/{entries.length}
                              </span>
                            </button>
                          </div>
                          <div className="ml-5 flex flex-wrap gap-1.5">
                            {entries.map(({ scope, description }) => (
                              <button
                                key={scope}
                                type="button"
                                aria-pressed={selectedScopes.includes(scope)}
                                title={description}
                                onClick={() => toggleScope(scope)}
                                className={cn(
                                  "rounded border px-2 py-1 font-mono text-eyebrow transition",
                                  selectedScopes.includes(scope)
                                    ? "border-brand-line bg-brand-subtle text-brand"
                                    : "border-line bg-overlay text-text-subtle hover:border-line-strong hover:text-text",
                                )}
                              >
                                {scope}
                              </button>
                            ))}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                ) : null}
              </div>

              <div className={cn("ui-alert", isFullAccess ? "ui-alert-danger" : "ui-alert-warning")}>
                <AlertTriangle size={14} aria-hidden="true" className="mt-0.5 shrink-0" />
                <span>{isFullAccess ? "Full access keys can perform any action the owning account can. Use cautiously." : "Select only the permissions your application needs."}</span>
              </div>
            </AdminFormSection>
            <Btn onClick={() => createMut.mutate()} disabled={createBlockedReason !== null || createMut.isPending} loading={createMut.isPending}>
              <Plus size={14} /> Create Key
            </Btn>
            {createBlockedReason ? <p className="ui-hint">{createBlockedReason}</p> : null}
          </div>

          {newToken ? (
            <div className="mx-4 mb-4 space-y-2 rounded-lg border border-line bg-overlay-subtle p-3">
              <p className="flex items-center gap-1 font-semibold text-text">
                <Check size={12} aria-hidden="true" /> Key created — copy it now, it will not be shown again.
              </p>
              <div className="flex items-center gap-2">
                <code className="ui-code-inline flex-1 break-all">{tokenMasked ? "••••••••••••••••••••••••" : newToken}</code>
                <AdminIconButton label={tokenMasked ? "Reveal API key" : "Hide API key"} onClick={() => setTokenMasked((masked) => !masked)}>
                  {tokenMasked ? <Eye size={14} /> : <EyeOff size={14} />}
                </AdminIconButton>
                <AdminIconButton label="Copy API key" onClick={() => void copyToken()}>
                  {copied ? <Check size={14} /> : <Copy size={14} />}
                </AdminIconButton>
              </div>
              <p className="ui-hint">Copied keys are wiped from the clipboard 15s after copying, and whenever this window loses focus.</p>
              {verification ? (
                <div
                  aria-live="polite"
                  role={verification.state === "failed" ? "alert" : "status"}
                  className={cn("ui-alert", verification.state === "ok" ? "ui-alert-success" : verification.state === "failed" ? "ui-alert-danger" : "ui-alert-info")}
                >
                  <span>{verification.message}</span>
                </div>
              ) : null}
              <p className="ui-hint">Scopes are enforced by the API. Use this token as an external Bearer token; routes outside the selected scopes return 403.</p>
            </div>
          ) : null}
        </Card>

        {/* Keys list */}
        <Card>
          <CardHeader title={isSuccess ? `Existing keys (${keys.length})` : "Existing keys"} icon={Shield} />
          {isLoading ? (
            <div className="p-4"><AdminLoadingState label="Loading API keys…" /></div>
          ) : isError ? (
            <div className="p-4"><AdminErrorState message={errorMessage(keysError, "API keys could not be loaded.")} retry={() => void refetchKeys()} /></div>
          ) : keys.length === 0 ? (
            <EmptyState icon={KeyRound} title="No API keys" message="No API keys exist yet. Create one to enable programmatic access." />
          ) : (
            <ul className="divide-y divide-line">
              {keys.map((key) => (
                <li key={key.id} className="px-4 py-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0 flex-1">
                      <p className="text-sm font-medium text-text">{key.description || <span className="text-text-subtle">Unnamed key</span>}</p>
                      {/* Only the prefix survives creation (the server stores a hash), so
                          this is the one identifier that ties a row to a copied token. */}
                      <p className="t-meta mt-0.5">
                        Token{" "}
                        <code className="ui-code-inline">{key.tokenPrefix ? `${key.tokenPrefix}••••` : "prefix not reported"}</code>
                        {" · secret is not retrievable"}
                      </p>
                      <p className="t-meta mt-0.5">
                        Created {formatDate(key.createdAt, "Unknown")}
                        {key.lastUsedAt ? ` · Last used ${formatDate(key.lastUsedAt, "Unknown")}` : " · Never used"}
                      </p>
                      {key.scopes && key.scopes.length > 0 ? (
                        <div className="mt-1.5 flex flex-wrap gap-1">
                          {key.scopes.map((scope) => (
                            <ScopeLabel key={scope} scope={scope} />
                          ))}
                        </div>
                      ) : (
                        <p className="t-meta mt-0.5">No scopes recorded on this key</p>
                      )}
                    </div>
                    <AdminIconButton label={`Revoke API key ${key.description || key.id}`} tone="danger" disabled={deleteMut.isPending} onClick={() => requestRevoke(key)}>
                      <Trash2 size={14} />
                    </AdminIconButton>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>
      {renderConfirm()}
    </AdminPageLayout>
  );
}
