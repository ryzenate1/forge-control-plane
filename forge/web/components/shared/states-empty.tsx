"use client";

import {
  Archive,
  Blocks,
  Cloud,
  Container,
  FileCode,
  Globe,
  HardDrive,
  Rocket,
  Search,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

function EmptyCard({
  icon,
  title,
  description,
  action,
}: {
  icon: LucideIcon;
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-white/10 px-6 py-14 text-center">
      <div className="grid h-12 w-12 place-items-center rounded-full bg-white/[0.05] text-slate-500">
        {icon({ size: 22, strokeWidth: 1.5, "aria-hidden": true } as Record<string, unknown>)}
      </div>
      <h3 className="mt-4 text-base font-semibold text-slate-200">{title}</h3>
      <p className="mt-2 max-w-md text-sm leading-6 text-slate-400">
        {description}
      </p>
      {action ? <div className="mt-5">{action}</div> : null}
    </div>
  );
}

export function EmptyList({
  icon: Icon = Blocks,
  title = "No items yet",
  description = "There are no items to display. Create one to get started.",
  action,
}: {
  icon?: LucideIcon;
  title?: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <EmptyCard
      icon={Icon}
      title={title}
      description={description}
      action={action}
    />
  );
}

export function EmptySearch({
  query,
  onClear,
}: {
  query?: string;
  onClear?: () => void;
}) {
  return (
    <EmptyCard
      icon={Search}
      title="No results found"
      description={
        query
          ? `No items matched "${query}". Try adjusting your search or filters.`
          : "No items matched your current filters. Try adjusting or clearing them."
      }
      action={
        onClear ? (
          <button
            className="rounded-lg border border-white/10 px-4 py-2 text-sm font-semibold text-slate-300 hover:bg-white/[0.06]"
            onClick={onClear}
            type="button"
          >
            Clear filters
          </button>
        ) : undefined
      }
    />
  );
}

export function EmptyDeployments({ action }: { action?: ReactNode }) {
  return (
    <EmptyCard
      icon={Rocket}
      title="No deployments yet"
      description="Deploy your app to push it live. Configure your source and trigger your first deployment."
      action={action}
    />
  );
}

export function EmptyBackups({ action }: { action?: ReactNode }) {
  return (
    <EmptyCard
      icon={HardDrive}
      title="No backups yet"
      description="Create a backup to protect your data. Backups can be used to restore your app to a previous state."
      action={action}
    />
  );
}

export function EmptyDomains({ action }: { action?: ReactNode }) {
  return (
    <EmptyCard
      icon={Globe}
      title="No domains configured"
      description="Add a custom domain to point your DNS to this app. Domains can be verified and managed here."
      action={action}
    />
  );
}

export function EmptyServices({ action }: { action?: ReactNode }) {
  return (
    <EmptyCard
      icon={Container}
      title="No services configured"
      description="Add services like databases, caches, or background workers to compose your app infrastructure."
      action={action}
    />
  );
}

export function EmptyGit({ action }: { action?: ReactNode }) {
  return (
    <EmptyCard
      icon={FileCode}
      title="No source repository configured"
      description="Connect a Git repository to enable continuous deployment from your source code."
      action={action}
    />
  );
}

export function EmptyCertificates({ action }: { action?: ReactNode }) {
  return (
    <EmptyCard
      icon={Cloud}
      title="No TLS certificates"
      description="Issue a TLS certificate for your domains to enable HTTPS. Certificates are managed via Let's Encrypt."
      action={action}
    />
  );
}

export function EmptyDNSProviders({ action }: { action?: ReactNode }) {
  return (
    <EmptyCard
      icon={Cloud}
      title="No DNS providers configured"
      description="Add a DNS provider integration to automate domain verification and certificate issuance."
      action={action}
    />
  );
}

export function EmptyOrganizations({ action }: { action?: ReactNode }) {
  return (
    <EmptyCard
      icon={Archive}
      title="No organizations"
      description="Create an organization to collaborate with your team and manage shared resources."
      action={action}
    />
  );
}
