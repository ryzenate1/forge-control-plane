"use client";

import { useEffect, useRef, useState } from "react";
import { Upload, X } from "lucide-react";
import { formatBytes } from "@/lib/utils";

interface UploadProgressProps {
  loaded: number;
  total: number;
  fileName?: string;
  onCancel?: () => void;
}

export function UploadProgress({ loaded, total, fileName, onCancel }: UploadProgressProps) {
  const [speed, setSpeed] = useState(0);
  const [eta, setEta] = useState<string>("");
  const lastLoaded = useRef(0);
  const lastTime = useRef(Date.now());
  const lastDisplayUpdate = useRef(Date.now());
  const percent = total > 0 ? Math.min(100, Math.round((loaded / total) * 100)) : 0;

  useEffect(() => {
    const now = Date.now();
    if (now - lastDisplayUpdate.current < 500) return;
    lastDisplayUpdate.current = now;
    const elapsed = (now - lastTime.current) / 1000;
    if (elapsed > 0) {
      const bytesPerSec = (loaded - lastLoaded.current) / elapsed;
      setSpeed(bytesPerSec > 0 ? bytesPerSec : 0);
      if (bytesPerSec > 0 && loaded < total) {
        const remaining = (total - loaded) / bytesPerSec;
        if (remaining < 60) {
          setEta(`${Math.round(remaining)}s`);
        } else if (remaining < 3600) {
          setEta(`${Math.floor(remaining / 60)}m ${Math.round(remaining % 60)}s`);
        } else {
          setEta(`${Math.floor(remaining / 3600)}h ${Math.floor((remaining % 3600) / 60)}m`);
        }
      } else {
        setEta("");
      }
    }
    lastLoaded.current = loaded;
    lastTime.current = now;
  }, [loaded, total]);

  return (
    <div className="rounded-lg border border-white/[0.08] bg-[var(--surface-raised)] p-3" role="status" aria-label={`Upload ${percent}% complete`}>
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <Upload className="h-4 w-4 shrink-0 text-red-400" />
          <span className="truncate text-xs font-medium text-slate-200">
            {fileName || "Uploading files..."}
          </span>
        </div>
        {onCancel && (
          <button
            className="shrink-0 rounded p-0.5 text-slate-500 hover:text-slate-300"
            onClick={onCancel}
            type="button"
            aria-label="Cancel upload"
          >
            <X size={14} />
          </button>
        )}
      </div>
      <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-800">
        <div
          className="h-full rounded-full bg-red-600 transition-all duration-300"
          style={{ width: `${percent}%` }}
        />
      </div>
      <div className="mt-1 flex items-center justify-between text-[10px] text-slate-500">
        <span>
          {formatBytes(loaded)} / {formatBytes(total)}
        </span>
        <span className="flex items-center gap-2">
          {speed > 0 && <span>{formatBytes(Math.round(speed))}/s</span>}
          {eta && <span>ETA: {eta}</span>}
        </span>
        <span>{percent}%</span>
      </div>
    </div>
  );
}
