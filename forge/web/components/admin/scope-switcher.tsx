"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { ChevronDown, Check, AlertTriangle, Lock } from "lucide-react";
import { cn } from "@/lib/utils";
import { PlanetDefaultIcon } from "@/components/ui/forge-icons";
import { useTenancyStore } from "@/stores/use-tenancy-store";
import { useDismissOnOutside } from "@/lib/hooks/use-dismiss-on-outside";

/**
 * Organization → Project → Environment scope switcher.
 *
 * This is the navigation surface for Forge's actual tenancy model, and it
 * reports only what the API returned. It replaces a hardcoded popover that
 * always displayed "Default (Production)" and "Staging" regardless of what
 * existed — a stale reading presented as a live one. While the tenancy data is
 * loading it says so; if the request failed it says that and offers the
 * management page; if the account genuinely has no projects it says that too.
 *
 * Data arrives via `TenancyHydrator` (mounted in components/providers.tsx),
 * which owns the react-query fetches and cascades the selection. Selecting
 * here writes to the same store, so the hydrator refetches the level below.
 */
export function ScopeSwitcher() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  useDismissOnOutside(containerRef, open, () => setOpen(false));

  const organizations = useTenancyStore((s) => s.organizations);
  const activeOrg = useTenancyStore((s) => s.activeOrg);
  const projects = useTenancyStore((s) => s.projects);
  const activeProject = useTenancyStore((s) => s.activeProject);
  const environments = useTenancyStore((s) => s.environments);
  const activeEnvironment = useTenancyStore((s) => s.activeEnvironment);
  const loading = useTenancyStore((s) => s.loading);
  const error = useTenancyStore((s) => s.error);
  const setActiveOrg = useTenancyStore((s) => s.setActiveOrg);
  const setActiveProject = useTenancyStore((s) => s.setActiveProject);
  const setActiveEnvironment = useTenancyStore((s) => s.setActiveEnvironment);

  // The trigger must never imply a scope that isn't real.
  const primary = activeProject?.name ?? (loading ? "Loading…" : error ? "Scope unavailable" : "No project");
  const secondary = activeEnvironment?.name ?? activeOrg?.name ?? null;

  const go = (href: string) => {
    setOpen(false);
    router.push(href);
  };

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        aria-expanded={open}
        aria-haspopup="menu"
        aria-label={`Current scope: ${primary}${secondary ? ` / ${secondary}` : ""}. Change scope`}
        onClick={() => setOpen((value) => !value)}
        className="flex w-full items-center justify-between rounded-lg border border-line bg-overlay-subtle px-2.5 py-1.5 text-left transition-colors hover:bg-overlay focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
      >
        <span className="flex min-w-0 items-center gap-2">
          <PlanetDefaultIcon size={16} className="shrink-0" />
          <span className="min-w-0">
            <span className="t-eyebrow block">
              {error ? "Scope" : "Project"}
            </span>
            <span
              className={cn(
                "block truncate text-xs font-semibold",
                activeProject ? "text-text" : "text-text-subtle",
              )}
            >
              {primary}
            </span>
          </span>
        </span>
        <span className="flex shrink-0 items-center gap-1">
          {secondary ? (
            <span className="hidden max-w-[72px] truncate font-mono text-eyebrow text-text-muted xl:inline">
              {secondary}
            </span>
          ) : null}
          <ChevronDown
            size={12}
            className={cn("text-text-muted transition-transform", open && "rotate-180")}
          />
        </span>
      </button>

      {open ? (
        <div
          role="menu"
          aria-label="Scope"
          className="absolute bottom-full left-0 z-50 mb-1.5 w-full divide-y divide-line rounded-lg border border-line bg-surface-raised p-1.5 shadow-popover"
        >
          {error ? (
            <div className="px-2 py-2" role="alert">
              <p className="flex items-center gap-1.5 text-xs font-semibold text-warn">
                <AlertTriangle size={12} />
                Tenancy data unavailable
              </p>
              <p className="ui-hint mt-1">{error}</p>
            </div>
          ) : null}

          <ScopeSection
            title="Organization"
            emptyLabel={loading ? "Loading organizations…" : "No organizations"}
            items={organizations.map((org) => ({
              id: org.id,
              label: org.name,
              selected: org.id === activeOrg?.id,
              onSelect: () => setActiveOrg(org),
            }))}
          />

          <ScopeSection
            title="Project"
            emptyLabel={
              !activeOrg ? "Select an organization first" : loading ? "Loading projects…" : "No projects in this organization"
            }
            items={projects.map((project) => ({
              id: project.id,
              label: project.name,
              selected: project.id === activeProject?.id,
              onSelect: () => setActiveProject(project),
            }))}
          />

          <ScopeSection
            title="Environment"
            emptyLabel={
              !activeProject ? "Select a project first" : loading ? "Loading environments…" : "No environments in this project"
            }
            items={environments.map((environment) => ({
              id: environment.id,
              label: environment.name,
              selected: environment.id === activeEnvironment?.id,
              locked: environment.protected,
              dot: environment.color || undefined,
              onSelect: () => setActiveEnvironment(environment),
            }))}
          />

          <div className="flex flex-col pt-1">
            <button
              type="button"
              role="menuitem"
              onClick={() => go("/admin/projects")}
              className="rounded-lg px-2 py-1 text-left text-xs text-text-subtle transition-colors hover:bg-overlay hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
            >
              Manage projects…
            </button>
            <button
              type="button"
              role="menuitem"
              onClick={() => go("/admin/environments")}
              className="rounded-lg px-2 py-1 text-left text-xs text-text-subtle transition-colors hover:bg-overlay hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]"
            >
              Manage environments…
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

type ScopeItem = {
  id: string;
  label: string;
  selected: boolean;
  locked?: boolean;
  dot?: string;
  onSelect: () => void;
};

function ScopeSection({
  title,
  items,
  emptyLabel,
}: {
  title: string;
  items: ScopeItem[];
  emptyLabel: string;
}) {
  return (
    <div className="py-1">
      <p className="t-eyebrow px-2 py-1">{title}</p>
      {items.length === 0 ? (
        <p className="ui-hint px-2 pb-1">{emptyLabel}</p>
      ) : (
        <div className="max-h-40 space-y-0.5 overflow-y-auto scrollbar-thin">
          {items.map((item) => (
            <button
              key={item.id}
              type="button"
              role="menuitemradio"
              aria-checked={item.selected}
              onClick={item.onSelect}
              className={cn(
                "flex w-full items-center justify-between gap-2 rounded-lg px-2 py-1.5 text-left text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]",
                item.selected
                  ? "bg-brand-subtle font-semibold text-text"
                  : "text-text-subtle hover:bg-overlay hover:text-text",
              )}
            >
              <span className="flex min-w-0 items-center gap-1.5">
                {item.dot ? (
                  <span
                    aria-hidden="true"
                    className="h-1.5 w-1.5 shrink-0 rounded-full"
                    style={{ backgroundColor: item.dot }}
                  />
                ) : null}
                <span className="truncate">{item.label}</span>
                {item.locked ? <Lock size={10} className="shrink-0 text-text-muted" aria-label="Protected environment" /> : null}
              </span>
              {item.selected ? <Check size={12} className="shrink-0 text-brand" /> : null}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
