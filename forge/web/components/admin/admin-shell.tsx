"use client";

import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { usePathname, useRouter } from "next/navigation";
import { LogOut } from "lucide-react";
import { cn } from "@/lib/utils";
import { fetchCurrentUser, logout } from "@/modules/infrastructure/api";
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
          <h1 className="text-xl font-bold text-slate-100">Unable to verify admin access</h1>
          <p className="text-sm text-red-300">
            The current user could not be loaded. Admin content remains hidden until the API responds.
          </p>
          <button
            className="rounded-lg bg-[#dc2626] px-4 py-2 text-sm font-bold text-white hover:bg-[#b91c1c] disabled:opacity-60"
            disabled={userQuery.isFetching}
            onClick={() => void userQuery.refetch()}
            type="button"
          >
            {userQuery.isFetching ? "Retrying…" : "Retry"}
          </button>
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
    <div className="flex h-dvh flex-col overflow-hidden bg-[#0f1419]">
      {/* Top bar */}
      <header className="flex h-14 shrink-0 items-center justify-between border-b border-white/[0.06] bg-[#111827] px-4 sm:px-6">
        <button
          className="text-lg font-bold text-slate-100"
          onClick={() => router.push("/servers")}
          type="button"
        >
          {companyName}
        </button>
        <div className="flex items-center gap-3 text-sm text-slate-400">
          <button
            className="hover:text-white transition"
            onClick={() => router.push("/servers")}
            type="button"
          >
            My Servers
          </button>
        </div>
      </header>

      <div className="flex min-h-0 flex-1 flex-col lg:flex-row">
        {/* Sidebar */}
        <aside className="shrink-0 border-b border-white/[0.06] bg-[#151920] lg:flex lg:h-full lg:w-56 lg:flex-col lg:overflow-hidden lg:border-b-0 lg:border-r lg:border-white/[0.06]">
          <div className="flex gap-2 overflow-x-auto px-3 py-3 lg:min-h-0 lg:flex-1 lg:block lg:overflow-y-auto lg:px-4 lg:py-5">
            {navGroups.map((group) => (
              <div key={group.title} className="flex shrink-0 gap-1 lg:mb-3 lg:block">
                <p className="hidden px-3 pb-1 text-[10px] font-bold uppercase tracking-widest text-slate-600 lg:block">
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
                        "flex w-auto shrink-0 items-center gap-2.5 whitespace-nowrap rounded-lg px-3 py-2 text-sm font-medium transition lg:w-full",
                        active
                          ? "bg-[#dc2626]/10 text-[#dc2626]"
                          : "text-slate-400 hover:bg-white/[0.04] hover:text-slate-200",
                      )}
                      onClick={() => router.push(item.href)}
                      type="button"
                    >
                      <Icon size={15} />
                      {item.label}
                    </button>
                  );
                })}
              </div>
            ))}
          </div>
          <div className="shrink-0 border-t border-white/[0.06] px-4 py-3">
            <button
              onClick={handleLogout}
              className="flex items-center gap-2 w-full px-3 py-2 text-sm text-neutral-secondary hover:text-white hover:bg-surface-elevated rounded-lg transition-colors"
              type="button"
            >
              <LogOut className="w-4 h-4" />
              Sign Out
            </button>
          </div>
        </aside>

        {/* Content */}
        <section className="min-h-0 min-w-0 flex-1 overflow-y-auto p-4 sm:p-6 lg:p-8">
          {children}
        </section>
      </div>
    </div>
  );
}
