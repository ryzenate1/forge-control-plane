"use client";

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { useParams, usePathname, useRouter } from "next/navigation";
import { AlertCircle, RefreshCw } from "lucide-react";
import { fetchCurrentUser, fetchServer, type ApiServer, type ApiUser } from "@/lib/api";
import { ServerProvider, computeServerAccess } from "@/components/server/server-context";
import { errorMessage } from "@/lib/utils";

/**
 * Console server-detail layout.
 *
 * This owns the workload shell for `/console/servers/[id]/**`. It fetches the
 * server and current user once and publishes them through `ServerProvider`,
 * which lets every descendant `ServerConsoleLayout` collapse to bare content
 * instead of mounting a second sidebar. Without this layout each tab page
 * would render the legacy `ServerNav` — a second, disagreeing navigation whose
 * links leave the console for `/server/**`.
 *
 * The console's own sidebar (`ConsoleNav`) renders the tab list from
 * `console-registry`, so navigation is single-sourced here.
 */
export default function ConsoleServerLayout({ children }: { children: ReactNode }) {
  const params = useParams();
  const pathname = usePathname();
  const router = useRouter();
  const serverId = String(params.id ?? "");
  const [server, setServer] = useState<ApiServer | null>(null);
  const [user, setUser] = useState<ApiUser | null>(null);
  const [sessionExpired, setSessionExpired] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef(false);

  const load = useCallback(async () => {
    if (!serverId) return;
    abortRef.current = false;
    setLoading(true);
    setError(null);
    setSessionExpired(false);
    try {
      const [nextServer, nextUser] = await Promise.all([fetchServer(serverId), fetchCurrentUser()]);
      if (abortRef.current) return;
      // An explicit null user means the session is gone — never render the
      // shell without a user, bounce to sign-in instead.
      if (nextUser == null) {
        setSessionExpired(true);
        return;
      }
      setServer(nextServer);
      setUser(nextUser);
    } catch (loadError) {
      if (abortRef.current) return;
      setError(errorMessage(loadError, "The server could not be loaded."));
      setServer(null);
    } finally {
      if (!abortRef.current) setLoading(false);
    }
  }, [serverId]);

  useEffect(() => {
    void load();
    return () => {
      abortRef.current = true;
    };
  }, [load]);

  useEffect(() => {
    if (sessionExpired) router.replace(`/?reason=session-expired&next=${encodeURIComponent(pathname)}`);
  }, [pathname, router.replace, sessionExpired]);

  if (loading) {
    return (
      <div className="grid min-h-[50vh] place-items-center text-slate-300" role="status">
        <div className="text-center">
          <div className="mx-auto h-9 w-9 animate-spin rounded-full border-2 border-slate-700 border-t-red-500" />
          <p className="mt-3 text-sm">Loading server…</p>
        </div>
      </div>
    );
  }

  if (sessionExpired) {
    return (
      <div className="grid min-h-[50vh] place-items-center text-slate-300" role="status">
        <div className="text-center">
          <div className="mx-auto h-9 w-9 animate-spin rounded-full border-2 border-slate-700 border-t-red-500" />
          <p className="mt-3 text-sm">Session expired — returning to sign in…</p>
        </div>
      </div>
    );
  }

  if (error || !server) {
    return (
      <div className="grid min-h-[50vh] place-items-center">
        <div className="max-w-md rounded-xl border border-red-500/30 bg-red-500/10 p-6 text-center" role="alert">
          <AlertCircle className="mx-auto text-red-300" />
          <h1 className="mt-3 text-lg font-bold text-red-100">Unable to load server</h1>
          <p className="mt-2 text-sm text-red-100/80">{error ?? "Server not found."}</p>
          <button
            className="mt-4 inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-bold text-white hover:bg-red-500"
            onClick={() => void load()}
            type="button"
          >
            <RefreshCw size={15} /> Try again
          </button>
        </div>
      </div>
    );
  }

  const access = computeServerAccess(server, user);

  return (
    <ServerProvider value={{ server, access, refreshServer: load }}>
      <div className="mx-auto max-w-7xl">{children}</div>
    </ServerProvider>
  );
}
