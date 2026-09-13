"use client";

import { Activity, AlertTriangle, Play, Square, Trash2, Upload, RefreshCw } from "lucide-react";
import { cn } from "@/lib/utils";

export interface ActivityEvent {
  id: string;
  action: string;
  createdAt: string;
  actorEmail?: string;
  metadata?: string | Record<string, unknown>;
}

function metadataText(raw: string | Record<string, unknown> | undefined): string | null {
  if (!raw) return null;
  if (typeof raw === "string") return raw;
  return Object.entries(raw).map(([key, value]) => `${key.replace(/[_-]/g, " ")}: ${typeof value === "object" ? JSON.stringify(value) : String(value)}`).join(" · ");
}

function actionTone(action: string): { icon: typeof Activity; text: string; iconColor: string } {
  if (action.includes("delete") || action.includes("remove") || action.includes("kill")) {
    return { icon: Trash2, text: "text-rose-300", iconColor: "text-rose-400" };
  }
  if (action.includes("start") || action.includes("create") || action.includes("upload") || action.includes("install")) {
    return { icon: Play, text: "text-emerald-300", iconColor: "text-emerald-400" };
  }
  if (action.includes("stop") || action.includes("shutdown")) {
    return { icon: Square, text: "text-amber-300", iconColor: "text-amber-400" };
  }
  if (action.includes("restart") || action.includes("rebuild") || action.includes("reinstall")) {
    return { icon: RefreshCw, text: "text-sky-300", iconColor: "text-sky-400" };
  }
  if (action.includes("fail") || action.includes("error")) {
    return { icon: AlertTriangle, text: "text-rose-300", iconColor: "text-rose-400" };
  }
  if (action.includes("deploy") || action.includes("promote") || action.includes("backup")) {
    return { icon: Upload, text: "text-violet-300", iconColor: "text-violet-400" };
  }
  return { icon: Activity, text: "text-slate-300", iconColor: "text-slate-400" };
}

function relativeTime(iso: string) {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 1000));
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  if (seconds < 2592000) return `${Math.floor(seconds / 86400)}d ago`;
  if (seconds < 31536000) return `${Math.floor(seconds / 2592000)}mo ago`;
  return `${Math.floor(seconds / 31536000)}y ago`;
}

export function ActivityTimeline({ events, className }: { events: ActivityEvent[]; className?: string }) {
  if (events.length === 0) {
    return (
      <div className={cn("flex items-center gap-2 rounded-xl border border-white/[0.07] bg-white/[0.018] p-4 text-sm text-slate-300", className)}>
        <Activity size={14} className="shrink-0 text-slate-400" />
        No lifecycle events recorded yet.
      </div>
    );
  }

  return (
    <ol className={cn("space-y-0", className)}>
      {events.map((event, index) => {
        const tone = actionTone(event.action);
        const Icon = tone.icon;
        const meta = metadataText(event.metadata);
        return (
          <li key={event.id} className="relative flex gap-3 pb-5 last:pb-0">
            {index < events.length - 1 ? <span aria-hidden className="absolute left-[9px] top-6 h-full w-px bg-white/[0.07]" /> : null}
            <span className={cn("mt-1 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-white/[0.04]", tone.iconColor)}>
              <Icon size={12} />
            </span>
            <div className="min-w-0">
              <div className="flex items-baseline gap-2">
                <p className={cn("truncate text-sm font-medium", tone.text)}>{event.action}</p>
                <span className="shrink-0 text-xs text-slate-600">{relativeTime(event.createdAt)}</span>
              </div>
              <p className="mt-0.5 text-xs text-slate-400">
                {event.actorEmail ? `by ${event.actorEmail}` : "system"}
                {meta ? <span className="ml-2 truncate text-slate-600">{meta}</span> : null}
              </p>
            </div>
          </li>
        );
      })}
    </ol>
  );
}

export function activityToneForCheck(action: string) {
  const tone = actionTone(action);
  return { text: tone.text, iconColor: tone.iconColor };
}