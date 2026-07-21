"use client";

import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Search, Grid3X3, ChevronLeft, ChevronRight, Package, Download, Trash2,
  RefreshCw, RotateCcw, Settings, X, ExternalLink, Server, Tag,
} from "lucide-react";
import {
  SpinnerPage, ErrorAlert, SkeletonList, useAppToast,
} from "@/components/shared";
import { cn } from "@/lib/utils";
import * as appStoreApi from "@/lib/api/app-store";
import type { AppStoreApp, AppStoreInstall, InstallRequest } from "@/lib/api/app-store";

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
  const [search, setSearch] = useState("");
  const [view, setView] = useState<"browse" | "installed" | "detail">("browse");
  const [selectedApp, setSelectedApp] = useState<AppStoreApp | null>(null);
  const [showInstallForm, setShowInstallForm] = useState(false);
  const [showUninstallConfirm, setShowUninstallConfirm] = useState<string | null>(null);
  const queryClient = useQueryClient();
  const { toast } = useAppToast();

  const appsQuery = useQuery({
    queryKey: ["app-store", "apps", category, search],
    queryFn: () => appStoreApi.listApps(category || undefined, search || undefined),
  });

  const installsQuery = useQuery({
    queryKey: ["app-store", "installs"],
    queryFn: () => appStoreApi.listInstalls(),
  });

  const installMut = useMutation({
    mutationFn: (req: InstallRequest) => appStoreApi.installApp(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["app-store"] });
      setShowInstallForm(false);
      setView("installed");
      toast({ title: "App installed successfully", tone: "success" });
    },
    onError: (err: Error) => toast({ title: err.message, tone: "error" }),
  });

  const uninstallMut = useMutation({
    mutationFn: (id: string) => appStoreApi.uninstallApp(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["app-store"] });
      setShowUninstallConfirm(null);
      toast({ title: "App uninstalled", tone: "success" });
    },
    onError: (err: Error) => toast({ title: err.message, tone: "error" }),
  });

  const upgradeMut = useMutation({
    mutationFn: (id: string) => appStoreApi.upgradeApp(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["app-store"] });
      toast({ title: "App upgraded", tone: "success" });
    },
    onError: (err: Error) => toast({ title: err.message, tone: "error" }),
  });

  const handleAppClick = (app: AppStoreApp) => {
    setSelectedApp(app);
    setView("detail");
  };

  if (appsQuery.isError) {
    return <ErrorAlert error={appsQuery.error} title="Failed to load app store" />;
  }

  const apps = appsQuery.data ?? [];
  const installs = installsQuery.data ?? [];

  const installedKeys = new Set(installs.map((i) => i.appKey));
  const installsByKey = new Map(installs.map((i) => [i.appKey, i]));

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">App Store</h1>
          <p className="text-sm text-slate-400">Browse, install, and manage pre-built applications</p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => { setView("browse"); setSelectedApp(null); }}
            className={cn("rounded-lg px-4 py-2 text-sm font-medium transition-colors", view === "browse" ? "bg-indigo-600 text-white" : "bg-white/5 text-slate-300 hover:bg-white/10")}
          >
            <Grid3X3 className="mr-1.5 inline-block h-4 w-4" />
            Browse
          </button>
          <button
            onClick={() => { setView("installed"); setSelectedApp(null); }}
            className={cn("rounded-lg px-4 py-2 text-sm font-medium transition-colors", view === "installed" ? "bg-indigo-600 text-white" : "bg-white/5 text-slate-300 hover:bg-white/10")}
          >
            <Package className="mr-1.5 inline-block h-4 w-4" />
            Installed ({installs.length})
          </button>
        </div>
      </div>

      {view === "detail" && selectedApp && (
        <AppDetailView
          app={selectedApp}
          isInstalled={installedKeys.has(selectedApp.key)}
          install={installsByKey.get(selectedApp.key)}
          onBack={() => setView("browse")}
          onInstall={() => setShowInstallForm(true)}
          onUninstall={() => setShowUninstallConfirm(selectedApp.key)}
          onUpgrade={() => {
            const inst = installsByKey.get(selectedApp.key);
            if (inst) upgradeMut.mutate(inst.id);
          }}
        />
      )}

      {view === "browse" && !selectedApp && (
        <>
          {/* Filters */}
          <div className="flex flex-wrap items-center gap-3">
            <div className="relative flex-1 max-w-xs">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
              <input
                type="text"
                placeholder="Search apps..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="w-full rounded-lg border border-white/10 bg-white/5 py-2 pl-10 pr-4 text-sm text-white placeholder-slate-500 focus:border-indigo-500 focus:outline-none"
              />
            </div>
            {categories.map((c) => (
              <button
                key={c.key}
                onClick={() => setCategory(c.key)}
                className={cn("rounded-full px-3 py-1 text-xs font-medium transition-colors", category === c.key ? "bg-indigo-600 text-white" : "bg-white/5 text-slate-400 hover:bg-white/10")}
              >
                {c.label}
              </button>
            ))}
          </div>

          {/* App Grid */}
          {appsQuery.isLoading ? (
            <SkeletonList rows={4} columns={3} />
          ) : apps.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-slate-500">
              <Package className="mb-3 h-12 w-12" />
              <p className="text-lg font-medium">No apps found</p>
              <p className="text-sm">Try adjusting your search or filter</p>
            </div>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {apps.map((app) => (
                <button
                  key={app.key}
                  onClick={() => handleAppClick(app)}
                  className="group relative overflow-hidden rounded-xl border border-white/10 bg-white/[0.03] p-5 text-left transition-all hover:border-indigo-500/40 hover:bg-white/[0.06]"
                >
                  {installedKeys.has(app.key) && (
                    <span className="absolute right-3 top-3 rounded-full bg-emerald-500/20 px-2 py-0.5 text-[10px] font-bold uppercase text-emerald-400">
                      Installed
                    </span>
                  )}
                  <div className="mb-3 flex h-12 w-12 items-center justify-center rounded-lg bg-white/5">
                    {app.icon ? (
                      <img src={app.icon} alt="" className="h-8 w-8" />
                    ) : (
                      <Package className="h-6 w-6 text-slate-400" />
                    )}
                  </div>
                  <h3 className="mb-1 font-semibold text-white">{app.name}</h3>
                  <p className="mb-3 line-clamp-2 text-xs text-slate-400">{app.shortDesc}</p>
                  <div className="flex flex-wrap gap-1.5">
                    {app.tags?.slice(0, 3).map((t) => (
                      <span key={t} className="rounded-md bg-white/5 px-2 py-0.5 text-[10px] text-slate-500">
                        {t}
                      </span>
                    ))}
                  </div>
                </button>
              ))}
            </div>
          )}
        </>
      )}

      {view === "installed" && (
        <>
          {installsQuery.isLoading ? (
            <SkeletonList rows={4} columns={3} />
          ) : installs.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-slate-500">
              <Package className="mb-3 h-12 w-12" />
              <p className="text-lg font-medium">No apps installed</p>
              <p className="text-sm">Browse the app store and install your first app</p>
            </div>
          ) : (
            <div className="space-y-2">
              {installs.map((inst) => (
                <div key={inst.id} className="flex items-center gap-4 rounded-xl border border-white/10 bg-white/[0.03] px-5 py-4">
                  <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-white/5">
                    <Package className="h-5 w-5 text-slate-400" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <p className="font-medium text-white">{inst.name}</p>
                      <StatusBadge status={inst.status} />
                    </div>
                    <p className="text-xs text-slate-500">
                      {inst.appKey} v{inst.appVersion}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    {inst.status === "running" && (
                      <button
                        onClick={() => upgradeMut.mutate(inst.id)}
                        className="rounded-lg bg-white/5 px-3 py-1.5 text-xs font-medium text-slate-300 hover:bg-white/10"
                        disabled={upgradeMut.isPending}
                      >
                        <RotateCcw className="mr-1 inline-block h-3 w-3" />
                        Upgrade
                      </button>
                    )}
                    <button
                      onClick={() => setShowUninstallConfirm(inst.id)}
                      className="rounded-lg bg-rose-500/10 px-3 py-1.5 text-xs font-medium text-rose-400 hover:bg-rose-500/20"
                    >
                      <Trash2 className="mr-1 inline-block h-3 w-3" />
                      Uninstall
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
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
    </div>
  );
}

function AppDetailView({
  app, isInstalled, install, onBack, onInstall, onUninstall, onUpgrade,
}: {
  app: AppStoreApp;
  isInstalled: boolean;
  install?: AppStoreInstall;
  onBack: () => void;
  onInstall: () => void;
  onUninstall: () => void;
  onUpgrade: () => void;
}) {
  return (
    <div className="rounded-xl border border-white/10 bg-white/[0.03] p-6">
      <button onClick={onBack} className="mb-4 flex items-center gap-1 text-sm text-slate-400 hover:text-white">
        <ChevronLeft className="h-4 w-4" />
        Back to browse
      </button>

      <div className="flex items-start gap-5">
        <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-xl bg-white/5">
          {app.icon ? <img src={app.icon} alt="" className="h-10 w-10" /> : <Package className="h-8 w-8 text-slate-400" />}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-3">
            <h2 className="text-xl font-bold text-white">{app.name}</h2>
            {isInstalled && install && <StatusBadge status={install.status} />}
            {!isInstalled && <span className="rounded-full bg-white/5 px-2.5 py-0.5 text-[11px] text-slate-500">v{app.version}</span>}
          </div>
          <p className="mt-1 text-sm text-slate-400">{app.shortDesc}</p>
          <div className="mt-3 flex flex-wrap gap-2">
            {app.tags?.map((t) => (
              <span key={t} className="inline-flex items-center gap-1 rounded-md bg-white/5 px-2 py-0.5 text-xs text-slate-400">
                <Tag className="h-3 w-3" />
                {t}
              </span>
            ))}
            {app.maintainer && (
              <span className="inline-flex items-center gap-1 rounded-md bg-white/5 px-2 py-0.5 text-xs text-slate-400">
                <Server className="h-3 w-3" />
                {app.maintainer}
              </span>
            )}
          </div>
        </div>
        <div className="flex shrink-0 gap-2">
          {isInstalled ? (
            <>
              <button onClick={onUpgrade} className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500">
                <RefreshCw className="mr-1.5 inline-block h-4 w-4" />
                Upgrade
              </button>
              <button onClick={onUninstall} className="rounded-lg bg-rose-500/10 px-4 py-2 text-sm font-medium text-rose-400 hover:bg-rose-500/20">
                <Trash2 className="mr-1.5 inline-block h-4 w-4" />
                Uninstall
              </button>
            </>
          ) : (
            <button onClick={onInstall} className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500">
              <Download className="mr-1.5 inline-block h-4 w-4" />
              Install
            </button>
          )}
          {app.sourceUrl && (
            <a href={app.sourceUrl} target="_blank" rel="noopener noreferrer" className="rounded-lg bg-white/5 px-3 py-2 text-sm text-slate-400 hover:bg-white/10">
              <ExternalLink className="h-4 w-4" />
            </a>
          )}
        </div>
      </div>

      <div className="mt-6 grid gap-4 sm:grid-cols-3">
        <div className="rounded-lg bg-white/5 px-4 py-3">
          <p className="text-xs text-slate-500">Min Memory</p>
          <p className="font-medium text-white">{app.minMemoryMb > 0 ? `${app.minMemoryMb} MB` : "N/A"}</p>
        </div>
        <div className="rounded-lg bg-white/5 px-4 py-3">
          <p className="text-xs text-slate-500">Min Disk</p>
          <p className="font-medium text-white">{app.minDiskMb > 0 ? `${app.minDiskMb} MB` : "N/A"}</p>
        </div>
        <div className="rounded-lg bg-white/5 px-4 py-3">
          <p className="text-xs text-slate-500">Category</p>
          <p className="font-medium capitalize text-white">{app.category}</p>
        </div>
      </div>

      {app.description && (
        <div className="mt-4">
          <h3 className="mb-2 text-sm font-semibold text-slate-300">Description</h3>
          <p className="text-sm leading-relaxed text-slate-400">{app.description}</p>
        </div>
      )}

      {isInstalled && install && (
        <div className="mt-4 rounded-lg border border-white/10 bg-white/[0.02] p-4">
          <h3 className="mb-2 text-sm font-semibold text-slate-300">Install Details</h3>
          <div className="grid gap-2 text-sm sm:grid-cols-2">
            <div><span className="text-slate-500">Name:</span> <span className="text-slate-300">{install.name}</span></div>
            <div><span className="text-slate-500">Status:</span> <StatusBadge status={install.status} /></div>
            <div><span className="text-slate-500">Version:</span> <span className="text-slate-300">v{install.appVersion}</span></div>
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

  useEffect(() => {
    if (app.params && typeof app.params === "object") {
      const defaults: Record<string, string> = {};
      for (const [key, field] of Object.entries(app.params)) {
        defaults[key] = String(field.default ?? "");
      }
      setParams(defaults);
    }
  }, [app]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onInstall({ appKey: app.key, name, nodeId, memoryMb, cpuShares: 512, diskMb, params });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-xl border border-white/10 bg-slate-900 p-6 shadow-2xl">
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-bold text-white">Configure & Install {app.name}</h2>
          <button onClick={onClose} className="text-slate-400 hover:text-white"><X className="h-5 w-5" /></button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-400">App Name</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm text-white focus:border-indigo-500 focus:outline-none"
              required
            />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-400">Node ID</label>
            <input
              type="text"
              value={nodeId}
              onChange={(e) => setNodeId(e.target.value)}
              className="w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm text-white focus:border-indigo-500 focus:outline-none"
              placeholder="Leave empty for auto-selection"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-slate-400">Memory (MB)</label>
              <input
                type="number"
                value={memoryMb}
                onChange={(e) => setMemoryMb(Number(e.target.value))}
                className="w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm text-white focus:border-indigo-500 focus:outline-none"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-slate-400">Disk (MB)</label>
              <input
                type="number"
                value={diskMb}
                onChange={(e) => setDiskMb(Number(e.target.value))}
                className="w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm text-white focus:border-indigo-500 focus:outline-none"
              />
            </div>
          </div>

          {app.params && typeof app.params === "object" && Object.keys(app.params).length > 0 && (
            <div>
              <label className="mb-2 block text-xs font-medium text-slate-400">
                <Settings className="mr-1 inline-block h-3 w-3" />
                Configuration Parameters
              </label>
              <div className="space-y-3">
                {Object.entries(app.params).map(([key, field]) => (
                  <div key={key}>
                    <label className="mb-1 block text-xs text-slate-500">
                      {field.label}
                      {field.description && <span className="ml-1 text-slate-600">({field.description})</span>}
                    </label>
                    {field.type === "number" ? (
                      <input
                        type="number"
                        value={params[key] ?? String(field.default ?? "")}
                        onChange={(e) => setParams((p) => ({ ...p, [key]: e.target.value }))}
                        className="w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm text-white focus:border-indigo-500 focus:outline-none"
                      />
                    ) : (
                      <input
                        type="text"
                        value={params[key] ?? String(field.default ?? "")}
                        onChange={(e) => setParams((p) => ({ ...p, [key]: e.target.value }))}
                        className="w-full rounded-lg border border-white/10 bg-white/5 px-3 py-2 text-sm text-white focus:border-indigo-500 focus:outline-none"
                      />
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          <div className="flex justify-end gap-3 pt-2">
            <button type="button" onClick={onClose} className="rounded-lg bg-white/5 px-4 py-2 text-sm text-slate-300 hover:bg-white/10">
              Cancel
            </button>
            <button type="submit" disabled={isLoading} className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500 disabled:opacity-50">
              {isLoading ? "Installing..." : "Install"}
            </button>
          </div>
        </form>
      </div>
    </div>
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
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div className="w-full max-w-sm rounded-xl border border-white/10 bg-slate-900 p-6 shadow-2xl">
        <h2 className="mb-2 text-lg font-bold text-white">Uninstall App</h2>
        <p className="mb-6 text-sm text-slate-400">
          This will remove the app and all its data. This action cannot be undone.
        </p>
        <div className="flex justify-end gap-3">
          <button onClick={onClose} className="rounded-lg bg-white/5 px-4 py-2 text-sm text-slate-300 hover:bg-white/10">
            Cancel
          </button>
          <button onClick={onConfirm} disabled={isLoading} className="rounded-lg bg-rose-600 px-4 py-2 text-sm font-medium text-white hover:bg-rose-500 disabled:opacity-50">
            {isLoading ? "Uninstalling..." : "Uninstall"}
          </button>
        </div>
      </div>
    </div>
  );
}

const statusConfig: Record<string, { label: string; className: string }> = {
  installing: { label: "Installing", className: "bg-sky-500/20 text-sky-400" },
  running: { label: "Running", className: "bg-emerald-500/20 text-emerald-400" },
  stopped: { label: "Stopped", className: "bg-slate-500/20 text-slate-400" },
  error: { label: "Error", className: "bg-rose-500/20 text-rose-400" },
  upgrading: { label: "Upgrading", className: "bg-amber-500/20 text-amber-400" },
  uninstalling: { label: "Uninstalling", className: "bg-slate-500/20 text-slate-400" },
};

function StatusBadge({ status }: { status: string }) {
  const cfg = statusConfig[status] ?? { label: status, className: "bg-white/5 text-slate-400" };
  return (
    <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-bold uppercase tracking-wider ${cfg.className}`}>
      {cfg.label}
    </span>
  );
}
