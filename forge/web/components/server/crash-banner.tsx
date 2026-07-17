"use client";

import { AlertTriangle, RefreshCw, Skull } from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { sendPowerSignal } from "@/lib/api";
import { fetchServerCrashHistory, resetServerCrashState } from "@/lib/api/servers";
import { useServerContext } from "./server-context";

interface CrashBannerProps {
  serverId: string;
}

export function CrashBanner({ serverId }: CrashBannerProps) {
  const { access, refreshServer } = useServerContext();
  const queryClient = useQueryClient();

  const { data: crashes, isLoading } = useQuery({
    queryKey: ["crash-history", serverId],
    queryFn: () => fetchServerCrashHistory(serverId),
    refetchInterval: 30_000,
  });

  const resetMutation = useMutation({
    mutationFn: () => resetServerCrashState(serverId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["crash-history", serverId] });
    },
  });

  const restartMutation = useMutation({
    mutationFn: () => sendPowerSignal(serverId, "start"),
    onSuccess: () => void refreshServer(),
  });

  if (isLoading || !crashes || crashes.length === 0) return null;

  const recentCrashes = crashes.slice(0, 5);
  const lastCrash = recentCrashes[0];

  return (
    <div className="rounded-xl border border-red-500/30 bg-red-500/10 p-4">
      <div className="flex items-start gap-3">
        <Skull className="mt-0.5 shrink-0 text-red-300" size={19} />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-red-100">
            Server has crashed {crashes.length} time{crashes.length !== 1 ? "s" : ""}
          </p>
          <p className="mt-1 text-xs text-red-200/70">
            Last crash: {new Date(lastCrash.created_at).toLocaleString()}
            {lastCrash.exit_code !== 0 && (
              <> · Exit code: {lastCrash.exit_code}</>
            )}
            {lastCrash.oom_killed && <> · Out of memory</>}
          </p>
          {recentCrashes.length > 1 && (
            <details className="mt-2">
              <summary className="cursor-pointer text-xs text-red-200/50 hover:text-red-200/80">
                Crash history ({recentCrashes.length} events)
              </summary>
              <ul className="mt-2 space-y-1">
                {recentCrashes.map((crash) => (
                  <li key={crash.id} className="flex items-center gap-2 font-mono text-[10px] text-red-200/40">
                    <span>{new Date(crash.created_at).toLocaleString()}</span>
                    {crash.exit_code !== 0 && <span className="text-red-200/60">exit={crash.exit_code}</span>}
                    {crash.oom_killed && <span className="text-red-200/60">OOM</span>}
                    {crash.auto_restarted && <span className="text-emerald-400/60">auto-restarted</span>}
                  </li>
                ))}
              </ul>
            </details>
          )}
          <div className="mt-3 flex flex-wrap gap-2">
            <button
              className="inline-flex items-center gap-1.5 rounded-lg bg-red-600 px-3 py-1.5 text-xs font-bold text-white hover:bg-red-500 disabled:opacity-40"
              disabled={restartMutation.isPending}
              onClick={() => restartMutation.mutate()}
              type="button"
            >
              <RefreshCw size={13} />
              {restartMutation.isPending ? "Starting..." : "Restart server"}
            </button>
            <button
              className="inline-flex items-center gap-1.5 rounded-lg border border-white/10 px-3 py-1.5 text-xs font-bold text-slate-200 hover:bg-white/5 disabled:opacity-40"
              disabled={resetMutation.isPending}
              onClick={() => resetMutation.mutate()}
              type="button"
            >
              <AlertTriangle size={13} />
              {resetMutation.isPending ? "Resetting..." : "Reset crash state"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
