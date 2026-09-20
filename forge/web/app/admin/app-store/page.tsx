"use client";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
import { queryKeys } from "@/lib/api/query-keys";

import { useState, useEffect, useMemo, useCallback } from "react";
import Image from "next/image";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Search, Grid3X3, ChevronLeft, Package, Download, Trash2,
  RefreshCw, RotateCcw, Settings, ExternalLink, Server, Tag,
} from "lucide-react";
import {
  ErrorAlert, SkeletonList, useAppToast,
} from "@/components/shared";
import { OfflineBanner } from "@/components/shared/states-offline";
import { Pagination } from "@/components/ui/primitives";
import { AdminPageLayout, Btn, Card, CardHeader, Modal, EmptyState, SectionHeader } from "@/components/admin/admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { cn } from "@/lib/utils";
import * as appStoreApi from "@/lib/api/app-store";
import type { AppStoreApp, AppStoreInstall, InstallRequest } from "@/lib/api/app-store";
import { ApiError } from "@/lib/api/http";
import { fetchAllNodes } from "@/lib/api";
import { safeExternalUrl } from "@/lib/safe-url";

/**
 * Retry policy for app-store mutations. The API client no longer retries inline
 * (see lib/api/app-store.ts), so react-query owns it here: transient failures
 * only (network/408/429/5xx), because re-sending a request the API rejected with
 * 4xx would just repeat the same validation error and burn rate-limit budget.
 */
function retryTransientMutation(failureCount: number, error: unknown): boolean {
  const status = error instanceof ApiError ? error.status : 0;
  const transient = status === 0 || status === 408 || status === 429 || status >= 500;
  return transient && failureCount < 2;
}

const categories = [
  { key: "", label: "All" },
  { key: "database", label: "Databases" },
  { key: "web-server", label: "Web Servers" },
  { key: "cache", label: "Caches" },
  { key: "proxy", label: "Proxies" },
  { key: "management", label: "Management" },
];

export default function AppStorePage() {
  const [category, setCategory] = useState("");
  const [searchInput, setSearchInput] = useState("");
  const [debouncedCategory, setDebouncedCategory] = useState("");
  const [view, setView] = useState<"browse" | "installed" | "detail">("browse");
  const [selectedApp, setSelectedApp] = useState<AppStoreApp | null>(null);
  const [showInstallForm, setShowInstallForm] = useState(false);
  const [showUninstallConfirm, setShowUninstallConfirm] = useState<string | null>(null);
  const [confirm, renderConfirm] = useConfirm();
  const queryClient = useQueryClient();
  const { toast } = useAppToast();
  const [appsPage, setAppsPage] = useState(1);
  const [installsPage, setInstallsPage] = useState(1);

  // Debounce search input to prevent excessive API calls
  const [debouncedSearch, setDebouncedSearch] = useState("");
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchInput);
    }, 500); // 500ms debounce delay
    return () => clearTimeout(timer);
  }, [searchInput]);

  // Debounce category changes
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedCategory(category);
    }, 300); // 300ms debounce for category changes
    return () => clearTimeout(timer);
  }, [category]);

  const appsQuery = useQuery({
    queryKey: queryKeys.apps.store(debouncedCategory, debouncedSearch),
    queryFn: () => appStoreApi.listApps(debouncedCategory || undefined, debouncedSearch || undefined),
    staleTime: 5 * 60 * 1000, // 5 minutes - don't refetch if data is recent
    retry: 2, // Retry failed requests up to 2 times
    enabled: view === "browse" || view === "detail", // Only fetch when needed
  });

  const installsQuery = useQuery({
    queryKey: ["app-store", "installs"],
    queryFn: () => appStoreApi.listInstalls(),
    staleTime: 2 * 60 * 1000, // 2 minutes - installed apps don't change often
    retry: 2, // Retry failed requests up to 2 times
    // Browse cards also need installation state.
    refetchInterval: 15_000,
  });

  const installMut = useMutation({
    mutationFn: (req: InstallRequest) => appStoreApi.installApp(req),
    retry: false,
    onSuccess: (installation) => {
      queryClient.invalidateQueries({ queryKey: ["app-store"] });
      setShowInstallForm(false);
      setView("installed");
      toast({ title: installation.status === "error" ? "Application deployment failed" : "Installation created", message: installation.errorMessage || undefined, tone: installation.status === "error" ? "error" : "success" });
    },
    onError: (err: Error) => {
      void queryClient.invalidateQueries({ queryKey: ["app-store", "installs"] });
      toast({ title: err.message, tone: "error" });
    },
  });

  const uninstallMut = useMutation({
    mutationFn: (id: string) => appStoreApi.uninstallApp(id),
    retry: false,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["app-store"] });
      setShowUninstallConfirm(null);
      toast({ title: "App uninstalled", tone: "success" });
    },
    onError: (err: Error) => toast({ title: err.message, tone: "error" }),
  });

  const upgradeMut = useMutation({
    mutationFn: (id: string) => appStoreApi.upgradeApp(id),
    retry: false,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["app-store"] });
      toast({ title: "App upgraded", tone: "success" });
    },
    onError: (err: Error) => toast({ title: err.message, tone: "error" }),
  });

  const syncBundledMut = useMutation({
    mutationFn: () => appStoreApi.syncBundledTemplates(),
    retry: retryTransientMutation,
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.apps.all });
      toast({
        title: "Bundled templates synced",
        message: `${res.imported} imported, ${res.updated} updated, ${res.skipped} skipped`,
        tone: "success",
      });
    },
    onError: (err: Error) => toast({ title: err.message, tone: "error" }),
  });

  const handleAppClick = (app: AppStoreApp) => {
    setSelectedApp(app);
    setView("detail");
  };

  const apps = useMemo(() => {
    const data = appsQuery.data;
    return Array.isArray(data) ? data : [];
  }, [appsQuery.data]);

  const installs = useMemo(() => {
    const data = installsQuery.data;
    return Array.isArray(data) ? data : [];
  }, [installsQuery.data]);

  const APPS_PAGE_SIZE = 12;
  const INSTALLS_PAGE_SIZE = 10;
  const appsTotalPages = Math.max(1, Math.ceil(apps.length / APPS_PAGE_SIZE));
  const installsTotalPages = Math.max(1, Math.ceil(installs.length / INSTALLS_PAGE_SIZE));
  const safeAppsPage = Math.min(appsPage, appsTotalPages);
  const safeInstallsPage = Math.min(installsPage, installsTotalPages);
  const paginatedApps = useMemo(() => {
    const start = (safeAppsPage - 1) * APPS_PAGE_SIZE;
    return apps.slice(start, start + APPS_PAGE_SIZE);
  }, [apps, safeAppsPage]);
  const paginatedInstalls = useMemo(() => {
    const start = (safeInstallsPage - 1) * INSTALLS_PAGE_SIZE;
    return installs.slice(start, start + INSTALLS_PAGE_SIZE);
  }, [installs, safeInstallsPage]);

  const installedKeys = useMemo(() => new Set(installs.map((i) => i.appKey)), [installs]);
  const installsByKey = useMemo(() => new Map(installs.map((i) => [i.appKey, i])), [installs]);

  const refreshData = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.apps.all });
    void queryClient.invalidateQueries({ queryKey: ["app-store", "installs"] });
  }, [queryClient]);

  return (
    <AdminPageLayout>
      <OfflineBanner onRetry={refreshData} />
      <SectionHeader
        info={adminPageGuides.appStore}
        title="App Store"
        sub="Browse, install, and manage pre-built applications."
        action={
          <div className="flex flex-wrap items-center gap-2">
            <button
              onClick={() => { setView("browse"); setSelectedApp(null); setAppsPage(1); }}
              className={cn("rounded-lg border px-4 py-2 text-sm font-medium transition-colors", view === "browse" ? "border-[color-mix(in_srgb,var(--brand)_70%,transparent)] bg-[var(--brand)] text-white" : "border-line bg-white/[0.04] text-text-subtle hover:bg-white/[0.08]")}
            >
              <Grid3X3 className="mr-1.5 inline-block h-4 w-4" />
              Browse
            </button>
            <button
              onClick={() => { setView("installed"); setSelectedApp(null); setInstallsPage(1); }}
              className={cn("rounded-lg border px-4 py-2 text-sm font-medium transition-colors", view === "installed" ? "border-[color-mix(in_srgb,var(--brand)_70%,transparent)] bg-[var(--brand)] text-white" : "border-line bg-white/[0.04] text-text-subtle hover:bg-white/[0.08]")}
            >
              <Package className="mr-1.5 inline-block h-4 w-4" />
              Installed{installsQuery.isSuccess ? ` (${installs.length})` : ""}
            </button>
            <button
              onClick={() => syncBundledMut.mutate()}
              disabled={syncBundledMut.isPending}
              className="rounded-lg border border-line bg-white/[0.04] px-4 py-2 text-sm font-medium text-text-subtle hover:bg-white/[0.08] disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              title="Import the bundled Coolify template catalog"
            >
              <RefreshCw className={`mr-1.5 inline-block h-4 w-4 ${syncBundledMut.isPending ? 'animate-spin' : ''}`} />
              Sync bundled
            </button>
            <button
              onClick={refreshData}
              disabled={appsQuery.isFetching || installsQuery.isFetching}
              className="rounded-lg border border-line bg-white/[0.04] px-4 py-2 text-sm font-medium text-text-subtle hover:bg-white/[0.08] disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              title="Refresh data"
              aria-label="Refresh app store"
            >
              <RefreshCw className={`h-4 w-4 ${appsQuery.isFetching || installsQuery.isFetching ? 'animate-spin' : ''}`} />
            </button>
          </div>
        }
      />

      {installsQuery.isError && (
        <div role="alert" className="rounded-xl border border-line bg-surface p-4 text-sm text-text-subtle">
          Installation status is unavailable. <Btn size="sm" onClick={() => void installsQuery.refetch()}>Retry</Btn>
        </div>
      )}

      {view === "detail" && selectedApp && (
        <AppDetailView
          app={selectedApp}
          installationStateKnown={installsQuery.isSuccess}
          isBusy={upgradeMut.isPending || uninstallMut.isPending}
          isInstalled={installedKeys.has(selectedApp.key)}
          install={installsByKey.get(selectedApp.key)}
          onBack={() => setView("browse")}
          onInstall={() => setShowInstallForm(true)}
          onUninstall={() => {
            const inst = installsByKey.get(selectedApp.key);
            if (inst) setShowUninstallConfirm(inst.id);
          }}
          onUpgrade={() => {
            const inst = installsByKey.get(selectedApp.key);
            if (inst) {
              void (async () => {
                if (await confirm({ title: `Upgrade ${inst.name}?`, description: `This will upgrade ${inst.name} to the latest version.`, confirmLabel: "Upgrade" })) upgradeMut.mutate(inst.id);
              })();
            }
          }}
        />
      )}

      {view === "browse" && !selectedApp && (
        <Card>
          <CardHeader title={appsQuery.isSuccess ? `${apps.length} available applications` : "Available applications"} icon={Grid3X3} />
          {/* Filters */}
          <div className="mb-5 flex flex-wrap items-center gap-3">
            <div className="relative flex-1 max-w-xs">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-subtle" />
              <input
                type="text"
                placeholder="Search apps..."
                aria-label="Search App Store"
                value={searchInput}
                onChange={(e) => { setSearchInput(e.target.value); setAppsPage(1); }}
                className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] py-2 pl-10 pr-4 text-sm text-text placeholder-slate-500 focus:border-[var(--brand)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]"
              />
            </div>
            {categories.map((c) => (
              <button
                key={c.key}
                onClick={() => { setCategory(c.key); setAppsPage(1); }}
                className={cn("rounded-full px-3 py-1 text-xs font-medium transition-colors", category === c.key ? "bg-[var(--brand)] text-white" : "bg-overlay-subtle text-text-subtle hover:bg-overlay-strong")}
              >
                {c.label}
              </button>
            ))}
          </div>

          {/* App Grid */}
          {appsQuery.isError ? (
            <div className="space-y-3"><ErrorAlert error={appsQuery.error} title="Could not load the app catalog" /><Btn onClick={() => void appsQuery.refetch()}>Retry catalog</Btn></div>
          ) : appsQuery.isLoading ? (
            <SkeletonList rows={4} columns={3} />
          ) : !Array.isArray(apps) || apps.length === 0 ? (
            <EmptyState icon={Package} title="No apps found" message="Try adjusting your search or filter" />
          ) : (
            <>
              <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {paginatedApps.map((app) => (
                <button
                  key={app.key}
                  onClick={() => handleAppClick(app)}
                  className="group relative flex min-h-56 flex-col rounded-xl border border-line bg-surface p-5 text-left transition-colors hover:border-brand hover:bg-overlay-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
                >
                  {installedKeys.has(app.key) && (
                    <span className="absolute right-3 top-3 rounded-full bg-emerald-500/20 px-2 py-0.5 text-[10px] font-bold uppercase text-emerald-400">
                      Installed
                    </span>
                  )}
                  <div className="mb-3 flex h-12 w-12 items-center justify-center rounded-lg bg-overlay-subtle">
                    {app.icon ? (
                      <Image src={app.icon} alt={`${app.name} icon`} width={32} height={32} className="h-11 w-11" unoptimized />
                    ) : (
                      <Package className="h-6 w-6 text-text-subtle" />
                    )}
                  </div>
                  <h3 className="mb-1 font-semibold text-text">{app.name}</h3>
                  <p className="mb-4 line-clamp-2 text-sm leading-6 text-text-subtle">{app.shortDesc}</p>
                  <div className="flex flex-wrap gap-1.5">
                    {app.tags?.slice(0, 3).map((t) => (
                      <span key={t} className="rounded-md bg-overlay-subtle px-2 py-0.5 text-[10px] text-text-subtle">
                        {t}
                      </span>
                    ))}
                  </div>
                </button>
              ))}
              </div>
              <Pagination page={safeAppsPage} pageCount={appsTotalPages} onPageChange={setAppsPage} label="App store browse pagination" />
            </>
          )}
          {apps.length > APPS_PAGE_SIZE && (
            <p className="text-center text-xs text-text-muted">Showing {(safeAppsPage - 1) * APPS_PAGE_SIZE + 1}–{Math.min(safeAppsPage * APPS_PAGE_SIZE, apps.length)} of {apps.length}</p>
          )}
        </Card>
      )}

      {view === "installed" && (
        <Card>
          <CardHeader title={installsQuery.isSuccess ? `${installs.length} installed applications` : "Installed applications"} icon={Package} />
          {installsQuery.isLoading ? (
            <SkeletonList rows={4} columns={3} />
          ) : !Array.isArray(installs) || installs.length === 0 ? (
            <EmptyState icon={Package} title="No apps installed" message="Browse the app store and install your first app" />
          ) : (
            <>
              <div className="space-y-2">
              {paginatedInstalls.map((inst) => (
                <div key={inst.id} className="flex flex-wrap items-center gap-4 rounded-xl border border-line bg-surface px-5 py-4">
                  <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-overlay-subtle">
                    <Package className="h-5 w-5 text-text-subtle" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <p className="font-medium text-text">{inst.name}</p>
                      <StatusBadge status={inst.status} />
                    </div>
                    <p className="text-xs text-text-subtle">
                      {inst.appKey} v{inst.appVersion}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    {inst.status === "running" && (
                      <Btn
                        size="sm"
                        tone="ghost"
                        onClick={() => {
                          void (async () => {
                            if (await confirm({ title: `Upgrade ${inst.name}?`, description: `This will upgrade ${inst.name} to the latest version. The existing deployment will be redeployed.`, confirmLabel: "Upgrade" })) upgradeMut.mutate(inst.id);
                          })();
                        }}
                        disabled={upgradeMut.isPending}
                      >
                        <RotateCcw className="mr-1 inline-block h-3 w-3" />
                        Upgrade
                      </Btn>
                    )}
                    <Btn
                      size="sm"
                      tone="danger"
                      onClick={() => setShowUninstallConfirm(inst.id)}
                      disabled={uninstallMut.isPending}
                    >
                      <Trash2 className="mr-1 inline-block h-3 w-3" />
                      Uninstall
                    </Btn>
                  </div>
                </div>
              ))}
              </div>
              <Pagination page={safeInstallsPage} pageCount={installsTotalPages} onPageChange={setInstallsPage} label="Installed apps pagination" />
            </>
          )}
        </Card>
      )}

      {/* Install Form Modal */}
      {showInstallForm && selectedApp && (
        <InstallFormModal
          app={selectedApp}
          onClose={() => setShowInstallForm(false)}
          onInstall={(req) => installMut.mutate(req)}
          isLoading={installMut.isPending}
        />
      )}

      {/* Uninstall Confirm Modal */}
      {showUninstallConfirm && (
        <UninstallConfirmModal
          onClose={() => setShowUninstallConfirm(null)}
          onConfirm={() => uninstallMut.mutate(showUninstallConfirm)}
          isLoading={uninstallMut.isPending}
        />
      )}
      {renderConfirm()}
    </AdminPageLayout>
  );
}

function AppDetailView({
  app, isInstalled, installationStateKnown, isBusy, install, onBack, onInstall, onUninstall, onUpgrade,
}: {
  app: AppStoreApp;
  isInstalled: boolean;
  installationStateKnown: boolean;
  isBusy: boolean;
  install?: AppStoreInstall;
  onBack: () => void;
  onInstall: () => void;
  onUninstall: () => void;
  onUpgrade: () => void;
}) {
  return (
    <div className="rounded-xl border border-line bg-surface p-6">
      <button onClick={onBack} className="mb-4 flex items-center gap-1 text-sm text-text-subtle hover:text-white">
        <ChevronLeft className="h-4 w-4" />
        Back to browse
      </button>

      <div className="flex flex-wrap items-start gap-5">
        <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-xl bg-overlay-subtle">
          {app.icon ? <Image src={app.icon} alt={`${app.name} icon`} width={40} height={40} className="h-10 w-10" unoptimized /> : <Package className="h-11 w-11 text-text-subtle" />}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-3">
            <h2 className="text-xl font-bold text-text">{app.name}</h2>
            {isInstalled && install && <StatusBadge status={install.status} />}
            {!isInstalled && <span className="rounded-full bg-overlay-subtle px-2.5 py-0.5 text-[11px] text-text-subtle">v{app.version}</span>}
          </div>
          <p className="mt-1 text-sm text-text-subtle">{app.shortDesc}</p>
          <div className="mt-3 flex flex-wrap gap-2">
            {app.tags?.map((t) => (
              <span key={t} className="inline-flex items-center gap-1 rounded-md bg-overlay-subtle px-2 py-0.5 text-xs text-text-subtle">
                <Tag className="h-3 w-3" />
                {t}
              </span>
            ))}
            {app.maintainer && (
              <span className="inline-flex items-center gap-1 rounded-md bg-overlay-subtle px-2 py-0.5 text-xs text-text-subtle">
                <Server className="h-3 w-3" />
                {app.maintainer}
              </span>
            )}
          </div>
        </div>
        <div className="flex shrink-0 gap-2">
          {isInstalled ? (
            <>
              <button disabled={isBusy} onClick={onUpgrade} className="rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--brand-hover)]">
                <RefreshCw className="mr-1.5 inline-block h-4 w-4" />
                Upgrade
              </button>
              <button disabled={isBusy} onClick={onUninstall} className="rounded-lg bg-rose-500/10 px-4 py-2 text-sm font-medium text-rose-400 hover:bg-rose-500/20">
                <Trash2 className="mr-1.5 inline-block h-4 w-4" />
                Uninstall
              </button>
            </>
          ) : (
            <button disabled={!installationStateKnown} onClick={onInstall} className="rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--brand-hover)]">
              <Download className="mr-1.5 inline-block h-4 w-4" />
              Install
            </button>
          )}
          {safeExternalUrl(app.sourceUrl) && (
            <a href={safeExternalUrl(app.sourceUrl)!} target="_blank" rel="noopener noreferrer" className="rounded-lg bg-overlay-subtle px-3 py-2 text-sm text-text-subtle hover:bg-overlay-strong">
              <ExternalLink className="h-4 w-4" />
            </a>
          )}
        </div>
      </div>

      <div className="mt-6 grid gap-4 sm:grid-cols-3">
        <div className="rounded-lg bg-overlay-subtle px-4 py-3">
          <p className="text-xs text-text-subtle">Min Memory</p>
          <p className="font-medium text-text">{app.minMemoryMb > 0 ? `${app.minMemoryMb} MB` : "N/A"}</p>
        </div>
        <div className="rounded-lg bg-overlay-subtle px-4 py-3">
          <p className="text-xs text-text-subtle">Min Disk</p>
          <p className="font-medium text-text">{app.minDiskMb > 0 ? `${app.minDiskMb} MB` : "N/A"}</p>
        </div>
        <div className="rounded-lg bg-overlay-subtle px-4 py-3">
          <p className="text-xs text-text-subtle">Category</p>
          <p className="font-medium capitalize text-white">{app.category}</p>
        </div>
      </div>

      {app.description && (
        <div className="mt-4">
          <h3 className="mb-2 text-sm font-semibold text-text-subtle">Description</h3>
          <p className="text-sm leading-relaxed text-text-subtle">{app.description}</p>
        </div>
      )}

      {isInstalled && install && (
        <div className="mt-4 rounded-lg border border-line bg-white/[0.02] p-4">
          <h3 className="mb-2 text-sm font-semibold text-text-subtle">Install Details</h3>
          <div className="grid gap-2 text-sm sm:grid-cols-2">
            <div><span className="text-text-subtle">Name:</span> <span className="text-text-subtle">{install.name}</span></div>
            <div><span className="text-text-subtle">Status:</span> <StatusBadge status={install.status} /></div>
            <div><span className="text-text-subtle">Version:</span> <span className="text-text-subtle">v{install.appVersion}</span></div>
            {install.errorMessage && <div className="col-span-2 text-rose-400">Error: {install.errorMessage}</div>}
          </div>
        </div>
      )}
    </div>
  );
}

function InstallFormModal({
  app, onClose, onInstall, isLoading,
}: {
  app: AppStoreApp;
  onClose: () => void;
  onInstall: (req: InstallRequest) => void;
  isLoading: boolean;
}) {
  const [name, setName] = useState(app.name.toLowerCase().replace(/\s+/g, "-"));
  const [nodeId, setNodeId] = useState("");
  const [memoryMb, setMemoryMb] = useState(app.minMemoryMb || 256);
  const [diskMb, setDiskMb] = useState(app.minDiskMb || 1024);
  const [params, setParams] = useState<Record<string, string>>({});
  const nodesQuery = useQuery({ queryKey: ["nodes", "all"], queryFn: () => fetchAllNodes() });

  useEffect(() => {
    if (app.params && typeof app.params === "object") {
      const defaults: Record<string, string> = {};
      for (const [key, field] of Object.entries(app.params)) {
        defaults[key] = String(field.default ?? "");
      }
      setParams(defaults);
    }
  // Depend on the stable app key: `app` is a fresh object each render, so
  // [app] would reset user-edited params in a loop.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [app.key]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (isLoading || !nodeId || !name.trim() || !Number.isFinite(memoryMb) || !Number.isFinite(diskMb)) return;
    onInstall({ appKey: app.key, name: name.trim(), nodeId, memoryMb, cpuShares: 512, diskMb, params });
  };

  return (
    <Modal title={`Install ${app.name}`} description="Choose a target node and review the configuration before deploying." onClose={() => { if (!isLoading) onClose(); }}>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="install-name" className="mb-1 block text-xs font-medium text-text-subtle">App Name</label>
            <input
              type="text"
              id="install-name" value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm text-text placeholder:text-text-muted focus:border-[var(--brand)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]"
              required
            />
          </div>
          <div>
            <label htmlFor="install-node" className="mb-1 block text-xs font-medium text-text-subtle">Target node</label>
            <select id="install-node" required value={nodeId} onChange={(e) => setNodeId(e.target.value)} disabled={nodesQuery.isLoading || nodesQuery.isError || isLoading} className="w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 py-2 text-sm text-text">
              <option value="">Select a node</option>
              {nodesQuery.data?.map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}
            </select>
            {nodesQuery.isLoading && <p className="mt-2 text-xs text-text-subtle">Loading nodes…</p>}
            {nodesQuery.isError && <p role="alert" className="mt-2 text-xs text-text-subtle">Could not load nodes. <button type="button" onClick={() => void nodesQuery.refetch()} className="underline">Retry</button></p>}
            {nodesQuery.isSuccess && nodesQuery.data.length === 0 && <p className="mt-2 text-xs text-text-subtle">Add a node before installing an application.</p>}
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label htmlFor="install-memory" className="mb-1 block text-xs font-medium text-text-subtle">Memory (MB)</label>
              <input
                type="number"
                min={Math.max(1, app.minMemoryMb)}
                required
                id="install-memory" value={memoryMb}
                onChange={(e) => setMemoryMb(Number(e.target.value))}
                className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm text-text placeholder:text-text-muted focus:border-[var(--brand)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]"
              />
            </div>
            <div>
              <label htmlFor="install-disk" className="mb-1 block text-xs font-medium text-text-subtle">Disk (MB)</label>
              <input
                type="number"
                min={Math.max(1, app.minDiskMb)}
                required
                id="install-disk" value={diskMb}
                onChange={(e) => setDiskMb(Number(e.target.value))}
                className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm text-text placeholder:text-text-muted focus:border-[var(--brand)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]"
              />
            </div>
          </div>

          {app.params && typeof app.params === "object" && Object.keys(app.params).length > 0 && (
            <div>
              <label className="mb-2 block text-xs font-medium text-text-subtle">
                <Settings className="mr-1 inline-block h-3 w-3" />
                Configuration Parameters
              </label>
              <div className="space-y-3">
                {Object.entries(app.params).map(([key, field]) => (
                  <div key={key}>
                    <label htmlFor={`install-param-${key}`} className="mb-1 block text-xs text-text-subtle">
                      {field.label}
                      {field.description && <span className="ml-1 text-text-muted">({field.description})</span>}
                    </label>
                    {field.type === "number" ? (
                      <input
                        type="number"
                        id={`install-param-${key}`}
                        value={params[key] ?? String(field.default ?? "")}
                        onChange={(e) => setParams((p) => ({ ...p, [key]: e.target.value }))}
                        className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm text-text placeholder:text-text-muted focus:border-[var(--brand)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]"
                      />
                    ) : (
                      <input
                        type="text"
                        id={`install-param-${key}`}
                        value={params[key] ?? String(field.default ?? "")}
                        onChange={(e) => setParams((p) => ({ ...p, [key]: e.target.value }))}
                        className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm text-text placeholder:text-text-muted focus:border-[var(--brand)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]"
                      />
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          <div className="flex justify-end gap-3 pt-2">
            <button type="button" onClick={onClose} className="rounded-lg bg-overlay-subtle px-4 py-2 text-sm text-text-subtle hover:bg-overlay-strong">
              Cancel
            </button>
            <button type="submit" disabled={isLoading || !nodeId || !nodesQuery.isSuccess} className="rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--brand-hover)] disabled:opacity-50">
              {isLoading ? "Installing..." : "Install"}
            </button>
          </div>
        </form>
    </Modal>
  );
}

function UninstallConfirmModal({
  onClose, onConfirm, isLoading,
}: {
  onClose: () => void;
  onConfirm: () => void;
  isLoading: boolean;
}) {
  return (
    <Modal title="Uninstall application" onClose={() => { if (!isLoading) onClose(); }}>
        <p className="mb-4 text-sm text-text-subtle">
          This removes the application installation. Review any persistent data you need to keep before continuing.
        </p>
        <div className="flex justify-end gap-3">
          <button onClick={onClose} className="rounded-lg bg-overlay-subtle px-4 py-2 text-sm text-text-subtle hover:bg-overlay-strong">
            Cancel
          </button>
          <button onClick={onConfirm} disabled={isLoading} className="rounded-lg bg-rose-600 px-4 py-2 text-sm font-medium text-text hover:bg-rose-500 disabled:opacity-50">
            {isLoading ? "Uninstalling..." : "Uninstall"}
          </button>
        </div>
    </Modal>
  );
}

const statusConfig: Record<string, { label: string; className: string }> = {
  installing: { label: "Installing", className: "bg-sky-500/20 text-sky-400" },
  running: { label: "Running", className: "bg-emerald-500/20 text-emerald-400" },
  stopped: { label: "Stopped", className: "bg-slate-500/20 text-text-subtle" },
  error: { label: "Error", className: "bg-rose-500/20 text-rose-400" },
  upgrading: { label: "Upgrading", className: "bg-amber-500/20 text-amber-400" },
  uninstalling: { label: "Uninstalling", className: "bg-slate-500/20 text-text-subtle" },
};

function StatusBadge({ status }: { status: string }) {
  const cfg = statusConfig[status] ?? { label: status, className: "bg-overlay-subtle text-text-subtle" };
  return (
    <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider ${cfg.className}`}>
      {cfg.label}
    </span>
  );
}
