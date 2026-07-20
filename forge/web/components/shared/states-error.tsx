"use client";

import {
  AlertTriangle,
  ChevronDown,
  ChevronUp,
  Clock,
  Home,
  Lock,
  RefreshCw,
  Search,
  WifiOff,
} from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";
import { errorMessage } from "@/lib/utils";

function ErrorCard({
  icon: Icon,
  title,
  message,
  actions,
}: {
  icon: React.ComponentType<{ className?: string; size?: number; "aria-hidden"?: boolean }>;
  title: string;
  message: string;
  actions?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-red-500/20 bg-red-500/[0.04] px-6 py-14 text-center">
      <div className="grid h-12 w-12 place-items-center rounded-full bg-red-500/10 text-red-400">
        <Icon size={22} aria-hidden={true} />
      </div>
      <h3 className="mt-4 text-base font-semibold text-red-200">{title}</h3>
      <p className="mt-2 max-w-md text-sm leading-6 text-red-300/80">
        {message}
      </p>
      {actions ? <div className="mt-5 flex flex-wrap justify-center gap-3">{actions}</div> : null}
    </div>
  );
}

export function ErrorAlert({
  error,
  title = "Something went wrong",
  onRetry,
  showDetails = false,
}: {
  error: unknown;
  title?: string;
  onRetry?: () => void;
  showDetails?: boolean;
}) {
  const [expanded, setExpanded] = useState(false);
  const msg = errorMessage(error, "An unexpected error occurred.");
  const full = error instanceof Error ? error.stack ?? error.message : String(error);

  return (
    <ErrorCard
      icon={AlertTriangle}
      title={title}
      message={msg}
      actions={
        <div className="flex flex-col items-center gap-2">
          <div className="flex gap-2">
            {onRetry ? (
              <button
                className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-500"
                onClick={onRetry}
                type="button"
              >
                <RefreshCw size={14} />
                Retry
              </button>
            ) : null}
          </div>
          {showDetails ? (
            <button
              className="mt-1 inline-flex items-center gap-1 text-xs text-slate-500 hover:text-slate-300"
              onClick={() => setExpanded(!expanded)}
              type="button"
            >
              {expanded ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
              {expanded ? "Hide details" : "Show details"}
            </button>
          ) : null}
          {expanded ? (
            <pre className="mt-2 max-h-48 max-w-md overflow-auto rounded-lg border border-white/10 bg-[#0f1419] p-3 text-left text-[11px] leading-relaxed text-slate-500 whitespace-pre-wrap">
              {full}
            </pre>
          ) : null}
        </div>
      }
    />
  );
}

export function ErrorNotFound({
  resource = "Resource",
  homeHref = "/servers",
}: {
  resource?: string;
  homeHref?: string;
}) {
  return (
    <ErrorCard
      icon={Search}
      title={`${resource} not found`}
      message={`The ${resource.toLowerCase()} you are looking for does not exist or has been removed.`}
      actions={
        <Link
          className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-500"
          href={homeHref}
        >
          <Home size={14} />
          Go to dashboard
        </Link>
      }
    />
  );
}

export function ErrorPermission({
  permission = "access this resource",
  message,
}: {
  permission?: string;
  message?: string;
}) {
  return (
    <ErrorCard
      icon={Lock}
      title="Insufficient permissions"
      message={
        message ??
        `You do not have the required permissions to ${permission}. Contact an administrator if you believe this is a mistake.`
      }
    />
  );
}

export function ErrorNetwork({ onRetry }: { onRetry?: () => void }) {
  return (
    <ErrorCard
      icon={WifiOff}
      title="Connection lost"
      message="Unable to reach the server. Check your internet connection and try again."
      actions={
        onRetry ? (
          <button
            className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-500"
            onClick={onRetry}
            type="button"
          >
            <RefreshCw size={14} />
            Retry
          </button>
        ) : null
      }
    />
  );
}

export function ErrorRateLimit({
  retryAfterSeconds,
  onRetry,
}: {
  retryAfterSeconds?: number;
  onRetry?: () => void;
}) {
  const [countdown, setCountdown] = useState(retryAfterSeconds ?? 0);

  useEffect(() => {
    if (retryAfterSeconds && retryAfterSeconds > 0) {
      setCountdown(retryAfterSeconds);
    }
  }, [retryAfterSeconds]);

  useEffect(() => {
    if (countdown <= 0) return;
    const timer = setInterval(() => setCountdown((c) => Math.max(0, c - 1)), 1000);
    return () => clearInterval(timer);
  }, [countdown]);

  return (
    <ErrorCard
      icon={Clock}
      title="Rate limit exceeded"
      message={
        countdown > 0
          ? `Too many requests. Please wait ${countdown} second${countdown === 1 ? "" : "s"} before trying again.`
          : "Too many requests. You can try again now."
      }
      actions={
        onRetry ? (
          <button
            className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-500 disabled:opacity-50"
            disabled={countdown > 0}
            onClick={onRetry}
            type="button"
          >
            <RefreshCw size={14} />
            {countdown > 0 ? `Retry in ${countdown}s` : "Retry now"}
          </button>
        ) : null
      }
    />
  );
}
