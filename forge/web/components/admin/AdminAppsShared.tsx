"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import { Download } from "lucide-react";
import { Btn, Input, Pill, cn } from "./admin-ui";
import type { AppStatus, DeploymentStatus, AppPort, AppVolume, AppLogEntry } from "@/lib/api/apps";
import { statusTone, deploymentStatusTone } from "@/lib/api/apps";

export function DeployStatusBadge({ status, type = "app" }: { status: AppStatus | DeploymentStatus; type?: "app" | "deployment" }) {
  const tone = type === "app" ? statusTone(status as AppStatus) : deploymentStatusTone(status as DeploymentStatus);
  const label = status.replace(/_/g, " ");
  return <Pill tone={tone}>{label}</Pill>;
}

export function ResourceGauge({ label, value, limit, unit }: { label: string; value: number; limit: number; unit?: string }) {
  const pct = limit > 0 ? Math.min(100, Math.max(0, (value / limit) * 100)) : 0;
  const tone = pct > 90 ? "bg-red-500" : pct > 70 ? "bg-amber-500" : "bg-emerald-500";
  const display = unit ?? (limit >= 1024 ? "MiB" : "cores");

  return (
    <div className="space-y-1">
      <div className="flex justify-between text-xs text-slate-400">
        <span>{label}</span>
        <span className="font-mono text-slate-300">
          {value.toFixed(1)} / {limit.toFixed(0)} {display}
        </span>
      </div>
      <div className="h-2 w-full overflow-hidden rounded-full bg-white/[0.06]">
        <div className={cn("h-full rounded-full transition-all duration-500", tone)} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}

export function LogViewer({ logs, loading }: { logs: AppLogEntry[]; loading?: boolean }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [search, setSearch] = useState("");
  const [autoScroll, setAutoScroll] = useState(true);

  const filtered = search
    ? logs.filter((l) => l.line.toLowerCase().includes(search.toLowerCase()))
    : logs;

  useEffect(() => {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [filtered, autoScroll]);

  const handleDownload = useCallback(() => {
    const text = filtered.map((l) => `[${l.timestamp}] [${l.stream}] ${l.line}`).join("\n");
    const blob = new Blob([text], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `app-logs-${new Date().toISOString().slice(0, 10)}.log`;
    a.click();
    URL.revokeObjectURL(url);
  }, [filtered]);

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3">
        <div className="flex-1">
          <Input
            placeholder="Search logs..."
            value={search}
            onChange={setSearch}
          />
        </div>
        <Btn tone="ghost" size="sm" onClick={() => setAutoScroll(!autoScroll)}>
          <span className={cn("text-xs", autoScroll ? "text-emerald-400" : "text-slate-500")}>Auto-scroll</span>
        </Btn>
        <Btn tone="ghost" size="sm" onClick={handleDownload}>
          <Download size={12} />
        </Btn>
      </div>
      <div
        ref={containerRef}
        className="h-96 overflow-y-auto rounded-lg border border-white/[0.06] bg-[#0a0e14] p-3 font-mono text-xs"
      >
        {loading ? (
          <div className="py-8 text-center text-slate-500">Loading logs...</div>
        ) : filtered.length === 0 ? (
          <div className="py-8 text-center text-slate-500">No logs to display.</div>
        ) : (
          filtered.map((entry, i) => (
            <div
              key={i}
              className={cn(
                "flex gap-2 py-0.5",
                entry.stream === "stderr" && "text-red-400",
                entry.stream === "stdout" && "text-slate-300",
              )}
            >
              <span className="shrink-0 text-slate-600">{entry.timestamp}</span>
              <span className="break-all">{entry.line}</span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

const ENV_KEY_REGEX = /^[a-zA-Z_][a-zA-Z0-9_]*$/;

export function EnvVarEditor({
  envVars,
  onChange,
}: {
  envVars: Record<string, string>;
  onChange: (vars: Record<string, string>) => void;
}) {
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");
  const [keyError, setKeyError] = useState("");

  const add = () => {
    const key = newKey.trim();
    if (!key) return;
    if (!ENV_KEY_REGEX.test(key)) {
      setKeyError("Must start with a letter or underscore, followed by letters, digits, or underscores");
      return;
    }
    setKeyError("");
    if (key in envVars) {
      onChange({ ...envVars, [key]: newValue });
    } else {
      onChange({ ...envVars, [key]: newValue });
    }
    setNewKey("");
    setNewValue("");
  };

  const remove = (key: string) => {
    const next = { ...envVars };
    delete next[key];
    onChange(next);
  };

  const update = (key: string, value: string) => {
    onChange({ ...envVars, [key]: value });
  };

  const entries = Object.entries(envVars);

  return (
    <div className="space-y-3">
      <div className="space-y-2">
        {entries.map(([key, value]) => (
          <div key={key} className="flex items-center gap-2">
            <Input mono value={key} onChange={() => {}} placeholder="KEY" />
            <Input mono value={value} onChange={(v) => update(key, v)} placeholder="VALUE" />
            <Btn tone="danger" size="sm" onClick={() => remove(key)}>X</Btn>
          </div>
        ))}
      </div>
      <div className="flex items-center gap-2">
        <Input
          mono
          value={newKey}
          onChange={(v) => { setNewKey(v); setKeyError(""); }}
          placeholder="NEW_KEY"
        />
        <Input
          mono
          value={newValue}
          onChange={setNewValue}
          placeholder="VALUE"
        />
        <Btn size="sm" onClick={add} disabled={!newKey.trim()}>
          Add
        </Btn>
      </div>
      {keyError && <p className="text-xs text-red-400">{keyError}</p>}
    </div>
  );
}

export function PortMapper({
  ports,
  onChange,
}: {
  ports: AppPort[];
  onChange: (ports: AppPort[]) => void;
}) {
  const add = () => {
    onChange([...ports, { containerPort: 80, hostPort: getNextPort(ports), protocol: "tcp" }]);
  };

  const remove = (idx: number) => {
    onChange(ports.filter((_, i) => i !== idx));
  };

  const update = (idx: number, field: Partial<AppPort>) => {
    onChange(ports.map((p, i) => (i === idx ? { ...p, ...field } : p)));
  };

  return (
    <div className="space-y-2">
      {ports.length === 0 && (
        <p className="text-xs text-slate-500">No ports configured.</p>
      )}
      {ports.map((port, idx) => (
        <div key={idx} className="flex items-center gap-2">
          <input
            className="h-9 w-20 rounded-lg border border-white/10 bg-[#161b28] px-2 text-xs font-mono text-slate-100 outline-none"
            type="number"
            value={port.hostPort}
            onChange={(e) => update(idx, { hostPort: parseInt(e.target.value) || 0 })}
            placeholder="Host"
          />
          <span className="text-xs text-slate-500">:</span>
          <input
            className="h-9 w-20 rounded-lg border border-white/10 bg-[#161b28] px-2 text-xs font-mono text-slate-100 outline-none"
            type="number"
            value={port.containerPort}
            onChange={(e) => update(idx, { containerPort: parseInt(e.target.value) || 0 })}
            placeholder="Container"
          />
          <select
            className="h-9 rounded-lg border border-white/10 bg-[#161b28] px-2 text-xs text-slate-300 outline-none"
            value={port.protocol}
            onChange={(e) => update(idx, { protocol: e.target.value as "tcp" | "udp" })}
          >
            <option value="tcp">TCP</option>
            <option value="udp">UDP</option>
          </select>
          <Btn tone="danger" size="sm" onClick={() => remove(idx)}>X</Btn>
        </div>
      ))}
      <Btn size="sm" tone="ghost" onClick={add}>+ Add Port</Btn>
    </div>
  );
}

function validateVolumeSource(source: string): string | null {
  if (!source.trim()) return "Source path is required";
  if (source.startsWith("/") || /^[a-zA-Z_][a-zA-Z0-9_.-]*$/.test(source)) return null;
  return "Source must be an absolute path (/...) or a valid volume name";
}

function validateVolumeTarget(target: string): string | null {
  if (!target.trim()) return "Target path is required";
  if (!target.startsWith("/")) return "Target must be an absolute path (/...)";
  return null;
}

export function VolumeEditor({
  volumes,
  onChange,
}: {
  volumes: AppVolume[];
  onChange: (vols: AppVolume[]) => void;
}) {
  const add = () => {
    onChange([...volumes, { source: "", target: "", readOnly: false }]);
  };

  const remove = (idx: number) => {
    onChange(volumes.filter((_, i) => i !== idx));
  };

  const update = (idx: number, field: Partial<AppVolume>) => {
    onChange(volumes.map((v, i) => (i === idx ? { ...v, ...field } : v)));
  };

  return (
    <div className="space-y-2">
      {volumes.length === 0 && (
        <p className="text-xs text-slate-500">No volumes configured.</p>
      )}
      {volumes.map((vol, idx) => {
        const sourceErr = vol.source ? validateVolumeSource(vol.source) : null;
        const targetErr = vol.target ? validateVolumeTarget(vol.target) : null;
        return (
          <div key={idx} className="flex items-center gap-2">
            <div className="flex-1">
              <input
                className={cn(
                  "h-9 w-full rounded-lg border bg-[#161b28] px-2 text-xs font-mono text-slate-100 outline-none",
                  sourceErr ? "border-red-400/70" : "border-white/10",
                )}
                value={vol.source}
                onChange={(e) => update(idx, { source: e.target.value })}
                placeholder="/host/path"
              />
              {sourceErr && <p className="mt-0.5 text-[10px] text-red-400">{sourceErr}</p>}
            </div>
            <span className="text-xs text-slate-500">:</span>
            <div className="flex-1">
              <input
                className={cn(
                  "h-9 w-full rounded-lg border bg-[#161b28] px-2 text-xs font-mono text-slate-100 outline-none",
                  targetErr ? "border-red-400/70" : "border-white/10",
                )}
                value={vol.target}
                onChange={(e) => update(idx, { target: e.target.value })}
                placeholder="/container/path"
              />
              {targetErr && <p className="mt-0.5 text-[10px] text-red-400">{targetErr}</p>}
            </div>
            <label className="flex items-center gap-1 text-xs text-slate-400">
              <input
                type="checkbox"
                checked={vol.readOnly}
                onChange={(e) => update(idx, { readOnly: e.target.checked })}
                className="h-3 w-3 rounded border-white/20 bg-[#161b28] accent-[#dc2626]"
              />
              RO
            </label>
            <Btn tone="danger" size="sm" onClick={() => remove(idx)}>X</Btn>
          </div>
        );
      })}
      <Btn size="sm" tone="ghost" onClick={add}>+ Add Volume</Btn>
    </div>
  );
}

function getNextPort(ports: AppPort[]): number {
  const used = new Set(ports.map((p) => p.hostPort));
  let port = 8080;
  while (used.has(port)) port++;
  return port;
}
