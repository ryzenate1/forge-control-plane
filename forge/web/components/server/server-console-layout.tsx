"use client";

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { useParams, usePathname, useRouter } from "next/navigation";
import { AlertCircle, RefreshCw } from "lucide-react";
import { fetchCurrentUser, fetchServer, type ApiServer, type ApiUser } from "@/lib/api";
import { ServerNav, type ServerTab } from "@/components/server/server-nav";
import { ServerProvider, computeServerAccess, useOptionalServerContext } from "@/components/server/server-context";
import { workloadTabs } from "@/components/console/console-registry";
import { errorMessage } from "@/lib/utils";

function resolveActiveTab(pathname: string, serverId: string, fallback?: ServerTab): ServerTab {
  if (fallback && workloadTabs.some((tab) => tab.id === fallback)) return fallback;
  const segments = pathname.split("/").filter(Boolean);
  const last = segments.at(-1);
  if (!last || last === serverId) return "overview";
  // The terminal page lives at .../console but its tab id is "terminal".
  if (last === "console") return "terminal";
  return workloadTabs.some((tab) => tab.id === last) ? (last as ServerTab) : "overview";
}

interface ServerConsoleLayoutProps {
  activeTab?: ServerTab;
  children: ReactNode | ((server: ApiServer) => ReactNode);
}

function message(error: unknown) {
  return errorMessage(error, "The server could not be loaded.");
}

export function ServerConsoleLayout(props: ServerConsoleLayoutProps) {
  const parentContext = useOptionalServerContext();

  // Route pages may retain their render-prop wrapper while a parent route layout
  // owns the shell. Reuse that context to avoid a second fetch and shell mount.
  if (parentContext) {
    return <>{typeof props.children === "function" ? props.children(parentContext.server) : props.children}</>;
  }

  return <ServerConsoleShell {...props} />;
}

function ServerConsoleShell({ activeTab: activeTabProp, children }: ServerConsoleLayoutProps) {
  const params = useParams();
  const pathname = usePathname();
  // Only `replace` is needed here, and depending on the method rather than the
  // router object keeps the effect's dependency exact: `useRouter()` can return
  // a fresh object per render, which would re-run the redirect every render.
  const { replace } = useRouter();
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
      setError(message(loadError));
      setServer(null);
    } finally {
      if (!abortRef.current) setLoading(false);
    }
  }, [serverId]);

  useEffect(() => { void load(); return () => { abortRef.current = true; }; }, [load]);

  useEffect(() => {
    if (sessionExpired) replace(`/?reason=session-expired&next=${encodeURIComponent(pathname)}`);
  }, [pathname, replace, sessionExpired]);

  if (loading) {
    return <div className="grid min-h-screen place-items-center bg-[var(--canvas)] text-[var(--text-subtle)]" role="status"><div className="text-center"><div className="mx-auto h-9 w-9 animate-spin rounded-full border-2 border-[var(--line)] border-t-[var(--brand)]" /><p className="mt-3 text-sm">Loading server…</p></div></div>;
  }

  if (sessionExpired) {
    return <div className="grid min-h-screen place-items-center bg-[var(--canvas)] text-[var(--text-subtle)]" role="status"><div className="text-center"><div className="mx-auto h-9 w-9 animate-spin rounded-full border-2 border-[var(--line)] border-t-[var(--brand)]" /><p className="mt-3 text-sm">Session expired — returning to sign in…</p></div></div>;
  }

  if (error || !server) {
    return <div className="grid min-h-screen place-items-center bg-[var(--canvas)] p-6 text-[var(--text)]"><div className="max-w-md rounded-xl border border-[var(--danger-line)] bg-[var(--danger-subtle)] p-6 text-center" role="alert"><AlertCircle className="mx-auto text-[var(--danger)]" /><h1 className="mt-3 text-lg font-bold">Unable to load server</h1><p className="mt-2 text-sm text-[var(--text)]">{error ?? "Server not found."}</p><button className="mt-4 inline-flex items-center gap-2 rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-bold text-white hover:bg-[var(--brand-hover)]" onClick={() => void load()} type="button"><RefreshCw size={15} /> Try again</button></div></div>;
  }

  const activeTab = resolveActiveTab(pathname, serverId, activeTabProp);

  const access = computeServerAccess(server, user);
  const content = typeof children === "function" ? children(server) : children;

  return (
    <ServerProvider value={{ server, access, refreshServer: load }}>
      <div className="min-h-screen bg-[var(--canvas)] text-slate-200 md:flex">
        <ServerNav activeTab={activeTab} access={access} server={server} serverId={serverId} />
        <main className="min-w-0 flex-1 md:h-screen md:overflow-y-auto">
          <div className="mx-auto max-w-7xl p-4 sm:p-6 lg:p-8">{content}</div>
        </main>
      </div>
    </ServerProvider>
  );
}
