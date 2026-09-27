"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Eye, EyeOff, Plus, Trash2, History } from "lucide-react";
import { Btn, Card, CardHeader, EmptyState, Pill } from "@/components/admin/admin-ui";
import { Dialog } from "@/components/ui/primitives";
import { fetchEnvVars, createEnvVar, deleteEnvVar, type EnvVarResponse } from "@/lib/api/env-vars";
import { fetchEnvVarRevisions } from "@/lib/api/tenancy";

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

const VARS_KEY = ["env-vars"] as const;
const REVISIONS_KEY = ["env-var-revisions"] as const;

/**
 * Scoped to a project/environment record (scopeType + scopeId) — this is not the
 * app-config editor in components/admin/AdminAppsShared.tsx, which edits an
 * in-memory Record<string, string>.
 */
export function EnvVarEditor({
  scopeType,
  scopeId,
  title = "Environment Variables",
}: {
  scopeType: "project" | "environment";
  scopeId: string;
  title?: string;
}) {
  const queryClient = useQueryClient();
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});
  const [historyVar, setHistoryVar] = useState<EnvVarResponse | null>(null);

  const varsQuery = useQuery({
    queryKey: [...VARS_KEY, scopeType, scopeId],
    queryFn: () => fetchEnvVars(scopeType, scopeId),
    enabled: !!scopeId,
  });
  const vars = varsQuery.data ?? [];

  const revisionsQuery = useQuery({
    queryKey: [...REVISIONS_KEY, historyVar?.id ?? ""],
    queryFn: () => fetchEnvVarRevisions(historyVar!.id),
    enabled: Boolean(historyVar),
  });

  const createMut = useMutation({
    mutationFn: (input: { key: string; value: string }) => createEnvVar(scopeType, scopeId, input),
    onSuccess: () => {
      // Only cleared/refreshed once the API confirmed the write, so a failed
      // add never looks like a success.
      setKey("");
      setValue("");
      void queryClient.invalidateQueries({ queryKey: [...VARS_KEY, scopeType, scopeId] });
      void queryClient.invalidateQueries({ queryKey: REVISIONS_KEY });
    },
  });

  const deleteMut = useMutation({
    mutationFn: (varId: string) => deleteEnvVar(varId),
    onSuccess: (_data, varId) => {
      void queryClient.invalidateQueries({ queryKey: [...VARS_KEY, scopeType, scopeId] });
      void queryClient.invalidateQueries({ queryKey: REVISIONS_KEY });
      // The deleted var's revision dialog must not linger on a stale id.
      if (historyVar?.id === varId) setHistoryVar(null);
    },
  });

  const handleAdd = (e: React.FormEvent) => {
    e.preventDefault();
    if (!key.trim() || !scopeId) return;
    createMut.mutate({ key: key.trim(), value });
  };

  const closeRevisions = () => setHistoryVar(null);
  const actionError = createMut.isError
    ? `Could not save the variable: ${errorMessage(createMut.error)}`
    : deleteMut.isError
      ? `Could not delete the variable: ${errorMessage(deleteMut.error)}`
      : null;

  return (
    <Card>
      <CardHeader title={title} />
      <form onSubmit={handleAdd} className="flex gap-2 border-b border-white/[0.06] p-3">
        <input
          value={key}
          onChange={(e) => setKey(e.target.value)}
          placeholder="KEY"
          className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 font-mono text-sm text-white placeholder:text-gray-500 focus:border-[color-mix(in_srgb,var(--brand)_50%,transparent)] focus:outline-none"
          required
        />
        <input
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="value"
          className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:border-[color-mix(in_srgb,var(--brand)_50%,transparent)] focus:outline-none"
        />
        <Btn type="submit" disabled={createMut.isPending || !key.trim()}>
          <Plus size={14} />
        </Btn>
      </form>
      {actionError && (
        <p role="alert" className="border-b border-white/[0.06] px-3 py-2 text-sm text-red-400">{actionError}</p>
      )}
      {varsQuery.isError ? (
        <div className="p-3">
          <div
            role="alert"
            className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-400"
          >
            <span>Could not load environment variables: {errorMessage(varsQuery.error)}</span>
            <button type="button" onClick={() => void varsQuery.refetch()} className="underline hover:text-red-300">
              Retry
            </button>
          </div>
        </div>
      ) : varsQuery.isPending && scopeId ? (
        <div className="p-6 text-sm text-slate-400">Loading...</div>
      ) : vars.length === 0 ? (
        <EmptyState message="No variables defined" />
      ) : (
        <div className="divide-y divide-white/[0.06]">
          {vars.map((v) => (
            <div key={v.id} className="flex items-center justify-between px-4 py-2.5">
              <div className="flex min-w-0 items-center gap-2">
                <span className="font-mono text-sm text-slate-200">{v.key}</span>
                <span className="text-xs text-slate-500">v{v.version}</span>
                {v.isSensitive && <Pill tone="yellow">Sensitive</Pill>}
                {revealed[v.id] && (
                  <span className="truncate font-mono text-xs text-slate-400">
                    {v.value || "value not returned by the API"}
                  </span>
                )}
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setRevealed({ ...revealed, [v.id]: !revealed[v.id] })}
                  className="text-slate-500 hover:text-slate-300"
                  title={revealed[v.id] ? "Hide" : "Show"}
                  type="button"
                >
                  {revealed[v.id] ? <EyeOff size={14} /> : <Eye size={14} />}
                </button>
                <button
                  onClick={() => setHistoryVar(v)}
                  className="text-slate-500 hover:text-[var(--brand)]"
                  title="Revision history"
                  type="button"
                >
                  <History size={14} />
                </button>
                <button
                  onClick={() => deleteMut.mutate(v.id)}
                  disabled={deleteMut.isPending}
                  className="text-red-500 hover:text-red-400 disabled:opacity-50"
                  type="button"
                >
                  <Trash2 size={14} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
      {historyVar && (
        <Dialog
          open={Boolean(historyVar)}
          title={`Revision History — ${historyVar.key}`}
          description={`Version history for ${historyVar.key} (current v${historyVar.version})`}
          closeAction={closeRevisions}
          className="max-w-lg"
        >
          {revisionsQuery.isPending ? (
            <div className="p-6 text-sm text-slate-400">Loading revisions...</div>
          ) : revisionsQuery.isError ? (
            <div
              role="alert"
              className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-400"
            >
              <span>Could not load revision history: {errorMessage(revisionsQuery.error)}</span>
              <button type="button" onClick={() => void revisionsQuery.refetch()} className="underline hover:text-red-300">
                Retry
              </button>
            </div>
          ) : (revisionsQuery.data ?? []).length === 0 ? (
            <p className="p-4 text-sm text-slate-500">No revision history available.</p>
          ) : (
            <div className="divide-y divide-white/[0.06] rounded-lg border border-white/10">
              {(revisionsQuery.data ?? []).map((r) => (
                <div key={r.id} className="flex items-center justify-between px-4 py-2.5">
                  <div className="flex items-center gap-2">
                    <span className="rounded bg-[color-mix(in_srgb,var(--brand)_20%,transparent)] px-2 py-0.5 text-xs font-semibold text-[var(--brand)]">v{r.version}</span>
                    {r.createdBy && <span className="text-xs text-slate-400">by {r.createdBy}</span>}
                  </div>
                  <span className="text-xs text-slate-500">{new Date(r.createdAt).toLocaleString()}</span>
                </div>
              ))}
            </div>
          )}
        </Dialog>
      )}
    </Card>
  );
}
