"use client";

import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { usePathname, useRouter } from "next/navigation";
import { LogOut, AlertTriangle } from "lucide-react";
import { cn } from "@/lib/utils";
import { fetchCurrentUser, logout } from "@/lib/api";
import { API_BASE_URL } from "@/lib/api/http";
import { useBranding } from "@/components/branding";
import { useServerStore } from "@/stores/use-server-store";
import { adminPagesForRole } from "./admin-registry";

export function AdminShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { companyName } = useBranding();
  const { currentUser, setCurrentUser } = useServerStore();
  const userQuery = useQuery({
    queryKey: ["current-user"],
    queryFn: fetchCurrentUser,
    staleTime: 30_000,
    retry: 1,
  });
  const user = userQuery.data === null ? null : userQuery.data ?? currentUser;

  useEffect(() => {
    if (userQuery.data === null) {
      router.replace("/");
    } else if (user && user.role !== "admin") {
      router.replace("/servers");
    }
  }, [router, user, userQuery.data]);

  const navGroups = adminPagesForRole(user?.role);

  const handleLogout = async () => {
      await logout();
      setCurrentUser(null);
      router.push("/");
    };

  if (userQuery.isPending) {
      return (
        <div className="grid min-h-screen place-items-center bg-[#0f1419] p-4 text-sm text-slate-400">
          Redirecting to sign in…
        </div>
      );
    }

    if (!userQuery.data) {
    return (
      <div className="grid min-h-screen place-items-center bg-[#0f1419] p-4">
        <div className="w-full max-w-md space-y-4 rounded-xl border border-red-500/30 bg-[#1e2536] p-6 text-center" role="alert">
          <AlertTriangle size={28} className="mx-auto text-amber-400" strokeWidth={1.5} />
          <h1 className="text-xl font-bold text-slate-100">Unable to verify admin access</h1>
          <p className="text-sm text-red-300">
            {userQuery.isError
              ? `API not reachable at ${API_BASE_URL}. Make sure the Go backend is running.`
              : "The current user could not be loaded. Admin content remains hidden until the API responds."
            }
          </p>
          <div className="flex justify-center gap-2">
            <button
              className="rounded-lg bg-[#dc2626] px-4 py-2 text-sm font-bold text-white hover:bg-[#b91c1c] disabled:opacity-60 transition-colors"
              disabled={userQuery.isFetching}
              onClick={() => void userQuery.refetch()}
              type="button"
            >
              {userQuery.isFetching ? "Retrying…" : "Retry"}
            </button>
          </div>
        </div>
      </div>
    );
  }

  if (!user) {
    return (
      <div className="grid min-h-screen place-items-center bg-[#0f1419] p-4 text-sm text-slate-400">
        Verifying admin access…
      </div>
    );
  }

  return (
    <div className="h-screen overflow-hidden bg-[#0a0e14]">
      {/* Top bar — fixed */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-white/[0.06] bg-[#0f1520] px-4 sm:px-6">
        <button
          className="text-lg font-bold text-slate-100 tracking-tight"
          onClick={() => router.push("/servers")}
          type="button"
        >
          {companyName}
        </button>
        <div className="flex items-center gap-3 text-sm text-slate-400">
          <button
            className="hover:text-white transition-colors"
            onClick={() => router.push("/servers")}
            type="button"
          >
            My Servers
          </button>
        </div>
      </header>

      <div className="flex h-[calc(100vh-56px)]">
        {/* Sidebar — independently scrollable */}
        <aside className="flex flex-col border-r border-white/[0.06] bg-[#11161f] w-56 shrink-0">
          <nav className="flex-1 overflow-y-auto px-3 py-5">
            {navGroups.map((group) => (
              <div key={group.title} className="mb-4">
                <p className="px-3 pb-1.5 text-[10px] font-bold uppercase tracking-[0.12em] text-slate-600">
                  {group.title}
                </p>
                {group.items.map((item) => {
                  const Icon = item.icon;
                  const active = pathname === item.href;
                  return (
                    <button
                      key={item.href}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-all text-left",
                        active
                          ? "bg-red-500/10 text-red-400 shadow-sm shadow-red-950/20"
                          : "text-slate-400 hover:bg-white/[0.04] hover:text-slate-200",
                      )}
                      onClick={() => router.push(item.href)}
                      type="button"
                    >
                      <Icon size={15} className="shrink-0" />
                      <span className="truncate">{item.label}</span>
                    </button>
                  );
                })}
              </div>
            ))}
          </nav>
          <div className="shrink-0 border-t border-white/[0.06] px-3 py-3">
            <button
              onClick={handleLogout}
              className="flex items-center gap-2.5 w-full px-3 py-2 text-sm text-slate-500 hover:text-slate-200 hover:bg-white/[0.04] rounded-lg transition-all"
              type="button"
            >
              <LogOut size={15} />
              Sign Out
            </button>
          </div>
        </aside>

        {/* Content — independently scrollable */}
        <main className="flex-1 overflow-y-auto min-w-0 p-4 sm:p-6 lg:p-8">
          {children}
        </main>
      </div>
    </div>
  );
}
