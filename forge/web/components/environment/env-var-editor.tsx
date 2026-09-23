"use client";

import { useState, useEffect, useCallback } from "react";
import { Eye, EyeOff, Plus, Trash2, History } from "lucide-react";
import { Btn, Card, CardHeader, EmptyState, Pill } from "@/components/admin/admin-ui";
import { Dialog } from "@/components/ui/primitives";
import { fetchEnvVars, createEnvVar, deleteEnvVar, type EnvVarResponse } from "@/lib/api/env-vars";
import { fetchEnvVarRevisions, type EnvVarRevision } from "@/lib/api/tenancy";

export function EnvVarEditor({
  scopeType,
  scopeId,
  title = "Environment Variables",
}: {
  scopeType: "project" | "environment";
  scopeId: string;
  title?: string;
}) {
  const [vars, setVars] = useState<EnvVarResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});
  const [historyVar, setHistoryVar] = useState<EnvVarResponse | null>(null);
  const [revisions, setRevisions] = useState<EnvVarRevision[]>([]);
  const [revisionsLoading, setRevisionsLoading] = useState(false);

  const loadVars = useCallback(async () => {
    if (!scopeId) return;
    setLoading(true);
    try {
      const data = await fetchEnvVars(scopeType, scopeId);
      setVars(data ?? []);
    } finally {
      setLoading(false);
    }
  }, [scopeId, scopeType]);

  useEffect(() => {
    loadVars();
  }, [loadVars]);

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!key.trim() || !scopeId) return;
    setError(null);
    try {
      await createEnvVar(scopeType, scopeId, { key: key.trim(), value });
      setKey("");
      setValue("");
      await loadVars();
    } catch (err: unknown) {
      // No caller handles this — surface it in the editor itself.
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const handleDelete = async (varId: string) => {
    setError(null);
    try {
      await deleteEnvVar(varId);
      await loadVars();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const openRevisions = useCallback(async (v: EnvVarResponse) => {
    setHistoryVar(v);
    setRevisionsLoading(true);
    try {
      const data = await fetchEnvVarRevisions(v.id);
      setRevisions(data ?? []);
    } catch {
      setRevisions([]);
    } finally {
      setRevisionsLoading(false);
    }
  }, []);

  const closeRevisions = useCallback(() => {
    setHistoryVar(null);
    setRevisions([]);
  }, []);

  return (
    <Card>
      <CardHeader title={title} />
      <form onSubmit={handleAdd} className="flex gap-2 border-b border-white/[0.06] p-3">
        <input
          value={key}
          onChange={(e) => setKey(e.target.value)}
          placeholder="KEY"
          className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 font-mono text-sm text-white placeholder:text-gray-500 focus:border-[var(--brand)]/50 focus:outline-none"
          required
        />
        <input
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="value"
          className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:border-[var(--brand)]/50 focus:outline-none"
        />
        <Btn type="submit">
          <Plus size={14} />
        </Btn>
      </form>
      {error && <p className="border-b border-white/[0.06] px-3 py-2 text-sm text-red-400" role="alert">{error}</p>}
      {loading ? (
        <div className="p-6 text-sm text-slate-400">Loading...</div>
      ) : vars.length === 0 ? (
        <EmptyState message="No variables defined" />
      ) : (
        <div className="divide-y divide-white/[0.06]">
          {vars.map((v) => (
            <div key={v.id} className="flex items-center justify-between px-4 py-2.5">
              <div className="flex items-center gap-2">
                <span className="font-mono text-sm text-slate-200">{v.key}</span>
                <span className="text-xs text-slate-500">v{v.version}</span>
                {v.isSensitive && <Pill tone="yellow">Sensitive</Pill>}
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
                  onClick={() => openRevisions(v)}
                  className="text-slate-500 hover:text-[var(--brand)]"
                  title="Revision history"
                  type="button"
                >
                  <History size={14} />
                </button>
                <button
                  onClick={() => handleDelete(v.id)}
                  className="text-red-500 hover:text-red-400"
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
          {revisionsLoading ? (
            <div className="p-6 text-sm text-slate-400">Loading revisions...</div>
          ) : revisions.length === 0 ? (
            <p className="p-4 text-sm text-slate-500">No revision history available.</p>
          ) : (
            <div className="divide-y divide-white/[0.06] rounded-lg border border-white/10">
              {revisions.map((r) => (
                <div key={r.id} className="flex items-center justify-between px-4 py-2.5">
                  <div className="flex items-center gap-2">
                    <span className="rounded bg-[var(--brand)]/20 px-2 py-0.5 text-xs font-semibold text-[var(--brand)]">v{r.version}</span>
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
