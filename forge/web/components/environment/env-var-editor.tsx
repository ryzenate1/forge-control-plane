"use client";

import { useState, useEffect } from "react";
import { Eye, EyeOff, Plus, Trash2 } from "lucide-react";
import { Btn, Card, CardHeader, EmptyState, Pill } from "@/components/admin/admin-ui";
import { fetchEnvVars, createEnvVar, deleteEnvVar, type EnvVarResponse } from "@/lib/api/env-vars";

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
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [revealed, setRevealed] = useState<Record<string, boolean>>({});

  const loadVars = async () => {
    if (!scopeId) return;
    setLoading(true);
    try {
      const data = await fetchEnvVars(scopeType, scopeId);
      setVars(data ?? []);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadVars();
  }, [scopeId, scopeType]);

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!key.trim() || !scopeId) return;
    try {
      await createEnvVar(scopeType, scopeId, { key: key.trim(), value });
      setKey("");
      setValue("");
      await loadVars();
    } catch {
      // Error handled by caller
    }
  };

  const handleDelete = async (varId: string) => {
    try {
      await deleteEnvVar(varId);
      await loadVars();
    } catch {
      // Error handled by caller
    }
  };

  return (
    <Card>
      <CardHeader title={title} />
      <form onSubmit={handleAdd} className="flex gap-2 border-b border-white/[0.06] p-3">
        <input
          value={key}
          onChange={(e) => setKey(e.target.value)}
          placeholder="KEY"
          className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 font-mono text-sm text-white placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none"
          required
        />
        <input
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="value"
          className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-1.5 text-sm text-white placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none"
        />
        <Btn type="submit">
          <Plus size={14} />
        </Btn>
      </form>
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
                >
                  {revealed[v.id] ? <EyeOff size={14} /> : <Eye size={14} />}
                </button>
                <button
                  onClick={() => handleDelete(v.id)}
                  className="text-red-500 hover:text-red-400"
                >
                  <Trash2 size={14} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}
