"use client";

import { useCallback, useEffect, useState, type ReactNode } from "react";
import { useParams, usePathname } from "next/navigation";
import { AlertCircle, RefreshCw } from "lucide-react";
import { fetchCurrentUser, fetchServer, type ApiServer, type ApiUser } from "@/lib/api";
import { ServerNav, type ServerTab } from "@/components/server/server-nav";
import { ServerProvider, type ServerAccess, useOptionalServerContext } from "@/components/server/server-context";
import { errorMessage } from "@/lib/utils";

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
  const serverId = String(params.id ?? "");
  const [server, setServer] = useState<ApiServer | null>(null);
  const [user, setUser] = useState<ApiUser | null>(null);
  const [permissions, setPermissions] = useState<string[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    if (!serverId) return;
    setLoading(true);
    setError(null);
    try {
      const [nextServer, nextUser] = await Promise.all([fetchServer(serverId), fetchCurrentUser()]);
      if (!nextUser) throw new Error("Your session has expired. Sign in again to manage this server.");
      setServer(nextServer);
      setUser(nextUser);

      const isAdmin = nextUser.role === "admin";
      const isOwner = nextServer.ownerId === nextUser.id;
      if (isAdmin || isOwner) {
        setPermissions([]);
      } else if (nextServer.permissions?.includes("*")) {
        setPermissions(["*"]);
      } else {
        setPermissions(nextServer.permissions ?? null);
      }
    } catch (loadError) {
      setError(message(loadError));
      setServer(null);
    } finally {
      setLoading(false);
    }
  }, [serverId]);

  useEffect(() => { void load(); }, [load]);

  if (loading) {
    return <div className="grid min-h-screen place-items-center bg-[#0a0e16] text-slate-300" role="status"><div className="text-center"><div className="mx-auto h-9 w-9 animate-spin rounded-full border-2 border-slate-700 border-t-red-500" /><p className="mt-3 text-sm">Loading server…</p></div></div>;
  }

  if (error || !server) {
    return <div className="grid min-h-screen place-items-center bg-[#0a0e16] p-6 text-slate-200"><div className="max-w-md rounded-xl border border-red-500/30 bg-red-500/10 p-6 text-center" role="alert"><AlertCircle className="mx-auto text-red-300" /><h1 className="mt-3 text-lg font-bold">Unable to load server</h1><p className="mt-2 text-sm text-red-100">{error ?? "Server not found."}</p><button className="mt-4 inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-bold text-white hover:bg-red-500" onClick={() => void load()} type="button"><RefreshCw size={15} /> Try again</button></div></div>;
  }

  const activeTab = activeTabProp ?? (pathname.split("/").at(-1) === serverId ? "console" : pathname.split("/").at(-1) as ServerTab);

  const access: ServerAccess = {
    user,
    permissions,
    isAdmin: user?.role === "admin",
    isOwner: Boolean(user && server.ownerId === user.id),
  };
  const content = typeof children === "function" ? children(server) : children;

  return (
    <ServerProvider value={{ server, access, refreshServer: load }}>
      <div className="min-h-screen bg-[#0a0e16] text-slate-200 md:flex">
        <ServerNav activeTab={activeTab} access={access} server={server} serverId={serverId} />
        <main className="min-w-0 flex-1 md:h-screen md:overflow-y-auto">
          <div className="mx-auto max-w-7xl p-4 sm:p-6 lg:p-8">{content}</div>
        </main>
      </div>
    </ServerProvider>
  );
}
