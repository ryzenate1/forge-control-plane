"use client";

import { useState } from "react";
import { ShieldX } from "lucide-react";

interface ChmodDialogProps {
  open: boolean;
  onClose: () => void;
  onApply: (mode: string, recursive: boolean) => void;
  fileName?: string;
  busy?: boolean;
}

const PRESETS = [
  { label: "644", desc: "rw-r--r--", value: "0644" },
  { label: "755", desc: "rwxr-xr-x", value: "0755" },
  { label: "600", desc: "rw-------", value: "0600" },
  { label: "700", desc: "rwx------", value: "0700" },
  { label: "777", desc: "rwxrwxrwx", value: "0777" },
  { label: "444", desc: "r--r--r--", value: "0444" },
];

export function ChmodDialog({ open, onClose, onApply, fileName, busy }: ChmodDialogProps) {
  const [mode, setMode] = useState("0644");
  const [recursive, setRecursive] = useState(false);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div className="w-full max-w-sm rounded-xl border border-white/[0.08] bg-[var(--surface-raised)] p-5 shadow-2xl">
        <div className="mb-4 flex items-center gap-3">
          <div className="grid h-9 w-9 place-items-center rounded-lg bg-amber-500/10 text-amber-400">
            <ShieldX size={16} />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-slate-100">Change Permissions</h3>
            {fileName && <p className="text-xs text-slate-500 truncate">{fileName}</p>}
          </div>
        </div>

        <div className="mb-4">
          <label className="mb-1.5 block text-xs font-medium text-slate-400">Permission Mode (octal)</label>
          <div className="flex gap-2">
            <input
              className="flex-1 rounded-lg border border-white/10 bg-black/30 px-3 py-2 font-mono text-sm text-white outline-none focus:border-amber-500/50"
              value={mode}
              onChange={(e) => {
                const raw = e.target.value.replace(/[^0-7]/g, "").slice(0, 4);
                setMode(raw);
              }}
              placeholder="0644"
              maxLength={4}
            />
          </div>
          <p className="mt-1 text-[10px] text-slate-500">Use 3 or 4 octal digits (e.g. 0644, 0755)</p>
        </div>

        <div className="mb-4">
          <p className="mb-2 text-xs font-medium text-slate-400">Presets</p>
          <div className="flex flex-wrap gap-1.5">
            {PRESETS.map((p) => (
              <button
                key={p.value}
                type="button"
                className={`rounded-lg border px-2.5 py-1.5 text-[11px] font-mono transition ${
                  mode === p.value
                    ? "border-amber-500/50 bg-amber-500/10 text-amber-300"
                    : "border-white/10 text-slate-400 hover:bg-white/[0.04] hover:text-slate-200"
                }`}
                onClick={() => setMode(p.value)}
              >
                <span className="font-semibold">{p.label}</span>
                <span className="ml-1.5 opacity-60">{p.desc}</span>
              </button>
            ))}
          </div>
        </div>

        <label className="mb-5 flex items-center gap-2 text-xs text-slate-400">
          <input
            type="checkbox"
            checked={recursive}
            onChange={(e) => setRecursive(e.target.checked)}
            className="h-3.5 w-3.5 rounded border-white/20 bg-black/30 accent-amber-500"
          />
          Apply recursively to directories
        </label>

        <div className="flex justify-end gap-2">
          <button
            type="button"
            className="rounded-lg border border-white/10 px-4 py-2 text-xs font-medium text-slate-300 hover:bg-white/[0.05]"
            onClick={onClose}
            disabled={busy}
          >
            Cancel
          </button>
          <button
            type="button"
            className="rounded-lg bg-amber-600 px-4 py-2 text-xs font-medium text-white hover:bg-amber-500 disabled:opacity-40"
            onClick={() => onApply(mode, recursive)}
            disabled={busy || !/^0[0-7]{3,4}$/.test(mode)}
          >
            {busy ? "Applying..." : "Apply"}
          </button>
        </div>
      </div>
    </div>
  );
}
