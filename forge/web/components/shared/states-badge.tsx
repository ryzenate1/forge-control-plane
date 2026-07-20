"use client";

import {
  AlertTriangle,
  CheckCircle2,
  Clock,
  LoaderCircle,
  PauseCircle,
  ShieldAlert,
  ArrowLeftRight,
  XCircle,
} from "lucide-react";
import { cn } from "@/lib/utils";

type BadgeTone = "neutral" | "success" | "warning" | "danger" | "info" | "loading";

function StatusBadge({
  icon: Icon,
  label,
  tone = "neutral",
  pulse = false,
}: {
  icon: React.ComponentType<{ className?: string; size?: number }>;
  label: string;
  tone?: BadgeTone;
  pulse?: boolean;
}) {
  const tones: Record<BadgeTone, string> = {
    neutral: "border-slate-500/30 bg-slate-500/10 text-slate-300",
    success: "border-emerald-500/30 bg-emerald-500/10 text-emerald-300",
    warning: "border-amber-500/30 bg-amber-500/10 text-amber-300",
    danger: "border-rose-500/30 bg-rose-500/10 text-rose-300",
    info: "border-sky-500/30 bg-sky-500/10 text-sky-300",
    loading: "border-sky-500/30 bg-sky-500/10 text-sky-300",
  };

  return (
    <span
      aria-label={label}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-[11px] font-bold uppercase tracking-wider",
        tones[tone],
      )}
    >
      <Icon
        aria-hidden="true"
        className={cn("h-3 w-3", pulse && "animate-pulse", tone === "loading" && "animate-spin")}
        size={12}
      />
      {label}
    </span>
  );
}

type ServerStatusType =
  | "running"
  | "stopped"
  | "installing"
  | "suspended"
  | "offline"
  | "transferring"
  | "errored";

const serverStatusConfig: Record<
  ServerStatusType,
  { icon: typeof CheckCircle2; tone: BadgeTone; label: string; pulse?: boolean }
> = {
  running: { icon: CheckCircle2, tone: "success", label: "Running" },
  stopped: { icon: PauseCircle, tone: "neutral", label: "Stopped" },
  installing: { icon: LoaderCircle, tone: "loading", label: "Installing" },
  suspended: { icon: ShieldAlert, tone: "warning", label: "Suspended" },
  offline: { icon: XCircle, tone: "danger", label: "Offline" },
  transferring: { icon: ArrowLeftRight, tone: "info", label: "Transferring", pulse: true },
  errored: { icon: AlertTriangle, tone: "danger", label: "Errored" },
};

export function ServerStatus({ status }: { status?: string | null }) {
  const config = serverStatusConfig[status as ServerStatusType] ?? serverStatusConfig.offline;
  return <StatusBadge {...config} />;
}

type BuildStatusType = "running" | "succeeded" | "failed" | "canceled" | "abandoned" | "pending";

const buildStatusConfig: Record<
  BuildStatusType,
  { icon: typeof CheckCircle2; tone: BadgeTone; label: string; pulse?: boolean }
> = {
  running: { icon: LoaderCircle, tone: "loading", label: "Building" },
  succeeded: { icon: CheckCircle2, tone: "success", label: "Succeeded" },
  failed: { icon: XCircle, tone: "danger", label: "Failed" },
  canceled: { icon: PauseCircle, tone: "neutral", label: "Canceled" },
  abandoned: { icon: AlertTriangle, tone: "warning", label: "Abandoned" },
  pending: { icon: Clock, tone: "info", label: "Pending" },
};

export function BuildStatus({ status }: { status?: string | null }) {
  const config = buildStatusConfig[status as BuildStatusType] ?? buildStatusConfig.pending;
  return <StatusBadge {...config} />;
}

type DeploymentStatusType = "pending" | "deploying" | "deployed" | "rolled-back" | "failed";

const deploymentStatusConfig: Record<
  DeploymentStatusType,
  { icon: typeof CheckCircle2; tone: BadgeTone; label: string; pulse?: boolean }
> = {
  pending: { icon: Clock, tone: "info", label: "Pending" },
  deploying: { icon: LoaderCircle, tone: "loading", label: "Deploying" },
  deployed: { icon: CheckCircle2, tone: "success", label: "Deployed" },
  "rolled-back": { icon: ArrowLeftRight, tone: "warning", label: "Rolled back" },
  failed: { icon: XCircle, tone: "danger", label: "Failed" },
};

export function DeploymentStatus({ status }: { status?: string | null }) {
  const config = deploymentStatusConfig[status as DeploymentStatusType] ?? deploymentStatusConfig.pending;
  return <StatusBadge {...config} />;
}

type CertStatusType = "valid" | "expiring" | "expired" | "issuing" | "failed";

const certStatusConfig: Record<
  CertStatusType,
  { icon: typeof CheckCircle2; tone: BadgeTone; label: string; pulse?: boolean }
> = {
  valid: { icon: CheckCircle2, tone: "success", label: "Valid" },
  expiring: { icon: AlertTriangle, tone: "warning", label: "Expiring" },
  expired: { icon: XCircle, tone: "danger", label: "Expired" },
  issuing: { icon: LoaderCircle, tone: "loading", label: "Issuing" },
  failed: { icon: XCircle, tone: "danger", label: "Failed" },
};

export function CertStatus({ status, expiresAt }: { status?: string | null; expiresAt?: string | null }) {
  let type = status as CertStatusType;
  if (!type) type = "valid";
  if (type === "valid" && expiresAt) {
    const days = Math.ceil((Date.now() - new Date(expiresAt).getTime()) / (1000 * 60 * 60 * 24)) * -1;
    if (days > 0 && days <= 14) type = "expiring";
    if (days <= 0) type = "expired";
  }
  const config = certStatusConfig[type] ?? certStatusConfig.valid;
  return <StatusBadge {...config} />;
}

type DBStatusType = "provisioning" | "running" | "backing-up" | "stopped" | "error";

const dbStatusConfig: Record<
  DBStatusType,
  { icon: typeof CheckCircle2; tone: BadgeTone; label: string; pulse?: boolean }
> = {
  provisioning: { icon: LoaderCircle, tone: "loading", label: "Provisioning" },
  running: { icon: CheckCircle2, tone: "success", label: "Running" },
  "backing-up": { icon: LoaderCircle, tone: "loading", label: "Backing up" },
  stopped: { icon: PauseCircle, tone: "neutral", label: "Stopped" },
  error: { icon: XCircle, tone: "danger", label: "Error" },
};

export function DBStatus({ status }: { status?: string | null }) {
  const config = dbStatusConfig[status as DBStatusType] ?? dbStatusConfig.stopped;
  return <StatusBadge {...config} />;
}

type VerificationStatusType = "pending" | "verified" | "failed";

const verificationStatusConfig: Record<
  VerificationStatusType,
  { icon: typeof CheckCircle2; tone: BadgeTone; label: string; pulse?: boolean }
> = {
  pending: { icon: Clock, tone: "info", label: "Verification pending" },
  verified: { icon: CheckCircle2, tone: "success", label: "Verified" },
  failed: { icon: XCircle, tone: "danger", label: "Verification failed" },
};

export function VerificationStatus({ status }: { status?: string | null }) {
  const config =
    verificationStatusConfig[status as VerificationStatusType] ?? verificationStatusConfig.pending;
  return <StatusBadge {...config} />;
}

export function CustomStatus({
  status,
  mapping,
}: {
  status?: string | null;
  mapping: Record<
    string,
    { icon: typeof CheckCircle2; tone: BadgeTone; label: string; pulse?: boolean }
  >;
}) {
  const defaultConfig = { icon: Clock, tone: "neutral" as BadgeTone, label: status ?? "Unknown" };
  const config = (status && mapping[status]) ? mapping[status] : defaultConfig;
  return <StatusBadge {...config} />;
}
