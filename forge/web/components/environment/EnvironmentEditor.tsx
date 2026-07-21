"use client";

import { useCallback, useMemo, useState } from "react";
import { Eye, EyeOff, FileDown, FileUp, Lock, Plus, Search, Trash2, Unlock } from "lucide-react";
import { cn } from "@/lib/utils";

interface EnvVar {
  key: string;
  value: string;
  encrypted: boolean;
}

interface EnvironmentEditorProps {
  variables: EnvVar[];
  onChange: (variables: EnvVar[]) => void;
  readOnly?: boolean;
}

function parseEnvFormat(text: string): EnvVar[] {
  const lines = text.split("\n");
  const vars: EnvVar[] = [];
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const eqIndex = trimmed.indexOf("=");
    if (eqIndex === -1) continue;
    const key = trimmed.slice(0, eqIndex).trim();
    let value = trimmed.slice(eqIndex + 1).trim();
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1);
    }
    if (key) vars.push({ key, value, encrypted: false });
  }
  return vars;
}

function toEnvFormat(vars: EnvVar[]): string {
  return vars.map((v) => `${v.key}=${v.value.includes(" ") ? `"${v.value}"` : v.value}`).join("\n");
}

export function EnvironmentEditor({ variables, onChange, readOnly = false }: EnvironmentEditorProps) {
  const [searchQuery, setSearchQuery] = useState("");
  const [showValues, setShowValues] = useState<Record<string, boolean>>({});
  const [showImport, setShowImport] = useState(false);
  const [importText, setImportText] = useState("");
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");

  const filteredVars = useMemo(() => {
    if (!searchQuery) return variables;
    const q = searchQuery.toLowerCase();
    return variables.filter((v) => v.key.toLowerCase().includes(q) || v.value.toLowerCase().includes(q));
  }, [variables, searchQuery]);

  const addVar = useCallback(() => {
    const trimmedKey = newKey.trim();
    if (!trimmedKey || variables.some((v) => v.key === trimmedKey)) return;
    onChange([...variables, { key: trimmedKey, value: newValue, encrypted: false }]);
    setNewKey("");
    setNewValue("");
  }, [newKey, newValue, onChange, variables]);

  const updateVar = useCallback((index: number, updates: Partial<EnvVar>) => {
    const next = [...variables];
    next[index] = { ...next[index], ...updates };
    onChange(next);
  }, [onChange, variables]);

  const removeVar = useCallback((index: number) => {
    onChange(variables.filter((_, i) => i !== index));
  }, [onChange, variables]);

  const handleImport = useCallback(() => {
    const parsed = parseEnvFormat(importText);
    const merged = [...variables];
    for (const v of parsed) {
      const existing = merged.findIndex((m) => m.key === v.key);
      if (existing >= 0) {
        merged[existing] = { ...merged[existing], value: v.value };
      } else {
        merged.push(v);
      }
    }
    onChange(merged);
    setImportText("");
    setShowImport(false);
  }, [importText, onChange, variables]);

  const handleExport = useCallback(() => {
    const text = toEnvFormat(variables);
    const blob = new Blob([text], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = ".env";
    a.click();
    URL.revokeObjectURL(url);
  }, [variables]);

  const toggleVisibility = useCallback((key: string) => {
    setShowValues((prev) => ({ ...prev, [key]: !prev[key] }));
  }, []);

  const toggleEncryption = useCallback((index: number) => {
    const v = variables[index];
    updateVar(index, { encrypted: !v.encrypted });
  }, [updateVar, variables]);

  return (
    <div className="rounded-lg border border-white/[0.06] bg-[#111722] overflow-hidden">
      <div className="flex items-center justify-between border-b border-white/[0.06] bg-[#161b28] px-4 py-3">
        <h3 className="text-sm font-semibold text-slate-200">
          Environment Variables ({variables.length})
        </h3>
        <div className="flex items-center gap-1">
          {!readOnly && (
            <>
              <button
                className="p-1.5 text-slate-500 hover:text-slate-200 rounded"
                onClick={() => setShowImport(!showImport)}
                title="Import from .env"
                type="button"
              >
                <FileUp size={14} />
              </button>
              <button
                className="p-1.5 text-slate-500 hover:text-slate-200 rounded"
                onClick={handleExport}
                title="Export as .env"
                type="button"
              >
                <FileDown size={14} />
              </button>
            </>
          )}
        </div>
      </div>

      {showImport && !readOnly && (
        <div className="border-b border-white/[0.06] p-4 space-y-3">
          <p className="text-xs text-slate-400">Paste .env format content below to import variables:</p>
          <textarea
            className="w-full h-32 rounded border border-white/10 bg-[#0d131d] p-3 text-xs font-mono text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-red-500/50 resize-y"
            onChange={(e) => setImportText(e.target.value)}
            placeholder="KEY=value&#10;ANOTHER_KEY=another_value"
            value={importText}
          />
          <div className="flex gap-2">
            <button
              className="rounded bg-red-600 px-3 py-1.5 text-xs font-semibold text-white hover:bg-red-500 disabled:opacity-50"
              disabled={!importText.trim()}
              onClick={handleImport}
              type="button"
            >
              Import
            </button>
            <button
              className="rounded border border-white/10 px-3 py-1.5 text-xs font-semibold text-slate-300 hover:bg-white/[0.06]"
              onClick={() => { setShowImport(false); setImportText(""); }}
              type="button"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      <div className="border-b border-white/[0.06] px-4 py-2">
        <div className="relative">
          <Search size={12} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-500" />
          <input
            className="w-full rounded border border-white/10 bg-[#0d131d] py-1.5 pl-7 pr-3 text-xs text-slate-200 placeholder:text-slate-600 focus:outline-none focus:ring-1 focus:ring-red-500/50"
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search variables..."
            type="text"
            value={searchQuery}
          />
        </div>
      </div>

      <div className="divide-y divide-white/[0.06] max-h-96 overflow-y-auto">
        {filteredVars.length === 0 && !readOnly && (
          <div className="flex flex-col items-center py-8 text-sm text-slate-500">
            {searchQuery ? "No variables match your search" : "No environment variables yet"}
          </div>
        )}

        {filteredVars.length === 0 && readOnly && (
          <div className="flex flex-col items-center py-8 text-sm text-slate-500">
            No environment variables configured
          </div>
        )}

        {filteredVars.map((v) => {
          const globalIndex = variables.indexOf(v);
          return (
            <div key={v.key} className="flex items-center gap-2 px-4 py-2 hover:bg-white/[0.02]">
              <div className="flex-1 min-w-0">
                {readOnly ? (
                  <div className="flex items-center gap-2">
                    <code className="text-xs font-semibold text-slate-200">{v.key}</code>
                    {v.encrypted && <Lock size={10} className="text-emerald-400" />}
                  </div>
                ) : (
                  <input
                    className="w-full bg-transparent text-xs font-semibold text-slate-200 outline-none"
                    onChange={(e) => updateVar(globalIndex, { key: e.target.value })}
                    value={v.key}
                  />
                )}
              </div>
              <div className="flex-1 min-w-0 flex items-center gap-1">
                {readOnly ? (
                  <span className="text-xs text-slate-400 font-mono">
                    {v.encrypted ? "••••••••" : v.value || "(empty)"}
                  </span>
                ) : (
                  <>
                    <input
                      className="flex-1 bg-[#0d131d] rounded border border-white/10 px-2 py-1 text-xs font-mono text-slate-300 outline-none focus:ring-1 focus:ring-red-500/50"
                      onChange={(e) => updateVar(globalIndex, { value: e.target.value })}
                      type={v.encrypted && !showValues[v.key] ? "password" : "text"}
                      value={v.value}
                    />
                    <button
                      className="p-1 text-slate-500 hover:text-slate-300"
                      onClick={() => toggleVisibility(v.key)}
                      type="button"
                      title={showValues[v.key] ? "Hide" : "Show"}
                    >
                      {showValues[v.key] ? <EyeOff size={12} /> : <Eye size={12} />}
                    </button>
                  </>
                )}
              </div>
              <div className="flex items-center gap-1 shrink-0">
                {!readOnly && (
                  <>
                    <button
                      className={cn("p-1 rounded", v.encrypted ? "text-emerald-400" : "text-slate-500 hover:text-slate-300")}
                      onClick={() => toggleEncryption(globalIndex)}
                      title={v.encrypted ? "Decrypt" : "Encrypt"}
                      type="button"
                    >
                      {v.encrypted ? <Lock size={12} /> : <Unlock size={12} />}
                    </button>
                    <button
                      className="p-1 text-slate-500 hover:text-red-400 rounded"
                      onClick={() => removeVar(globalIndex)}
                      title="Remove"
                      type="button"
                    >
                      <Trash2 size={12} />
                    </button>
                  </>
                )}
              </div>
            </div>
          );
        })}
      </div>

      {!readOnly && (
        <div className="flex items-center gap-2 border-t border-white/[0.06] px-4 py-2">
          <input
            className="flex-1 bg-[#0d131d] rounded border border-white/10 px-2 py-1.5 text-xs text-slate-200 placeholder:text-slate-600 outline-none focus:ring-1 focus:ring-red-500/50 font-mono"
            onChange={(e) => setNewKey(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") addVar(); }}
            placeholder="KEY_NAME"
            type="text"
            value={newKey}
          />
          <input
            className="flex-1 bg-[#0d131d] rounded border border-white/10 px-2 py-1.5 text-xs text-slate-200 placeholder:text-slate-600 outline-none focus:ring-1 focus:ring-red-500/50"
            onChange={(e) => setNewValue(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter") addVar(); }}
            placeholder="value"
            type="text"
            value={newValue}
          />
          <button
            className="p-1.5 text-slate-500 hover:text-emerald-400 rounded disabled:opacity-30"
            disabled={!newKey.trim() || variables.some((v) => v.key === newKey.trim())}
            onClick={addVar}
            title="Add variable"
            type="button"
          >
            <Plus size={14} />
          </button>
        </div>
      )}
    </div>
  );
}
