"use client";

import { LoaderCircle } from "lucide-react";
import { cn } from "@/lib/utils";

function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden="true" className={cn("animate-pulse rounded-lg bg-white/[0.07]", className)} />;
}

export function SkeletonList({ rows = 5, columns = 4 }: { rows?: number; columns?: number }) {
  return (
    <div aria-label="Loading list" className="ui-card" role="status">
      <div className="ui-card-header">
        <Skeleton className="h-5 w-32" />
      </div>
      <div className="divide-y divide-white/[0.07]">
        {Array.from({ length: rows }).map((_, i) => (
          <div className="flex items-center gap-4 px-5 py-4" key={i}>
            {Array.from({ length: columns }).map((_, j) => (
              <Skeleton
                className={cn(
                  "h-4",
                  j === 0 ? "w-8" : j === 1 ? "w-2/5" : "w-1/4",
                )}
                key={j}
              />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

export function SkeletonDetail() {
  return (
    <div aria-label="Loading details" className="space-y-6" role="status">
      <Skeleton className="h-8 w-60" />
      <div className="ui-card">
        <div className="space-y-4 p-6">
          <Skeleton className="h-6 w-40" />
          <div className="grid gap-4 md:grid-cols-2">
            <Skeleton className="h-20 rounded-xl" />
            <Skeleton className="h-20 rounded-xl" />
            <Skeleton className="h-20 rounded-xl" />
            <Skeleton className="h-20 rounded-xl" />
          </div>
        </div>
      </div>
      <div className="ui-card">
        <div className="space-y-4 p-6">
          <Skeleton className="h-6 w-32" />
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-3/4" />
          <Skeleton className="h-32 w-full" />
        </div>
      </div>
    </div>
  );
}

export function SkeletonForm({ fields = 5 }: { fields?: number }) {
  return (
    <div aria-label="Loading form" className="ui-card" role="status">
      <div className="ui-card-header">
        <Skeleton className="h-5 w-40" />
      </div>
      <div className="space-y-5 p-6">
        {Array.from({ length: fields }).map((_, i) => (
          <div key={i}>
            <Skeleton className="mb-2 h-4 w-20" />
            <Skeleton className="h-10 w-full rounded-lg" />
          </div>
        ))}
        <Skeleton className="h-10 w-32 rounded-lg" />
      </div>
    </div>
  );
}

export function SpinnerInline({ label = "Loading" }: { label?: string }) {
  return (
    <span className="inline-flex items-center gap-2" role="status">
      <LoaderCircle aria-hidden="true" className="h-4 w-4 animate-spin text-slate-400" />
      <span className="text-sm text-slate-400">{label}</span>
    </span>
  );
}

export function SpinnerPage({ message = "Loading…" }: { message?: string }) {
  return (
    <div className="grid min-h-[60vh] place-items-center" role="status">
      <div className="text-center">
        <LoaderCircle aria-hidden="true" className="mx-auto h-10 w-10 animate-spin text-red-500" />
        <p className="mt-4 text-sm text-slate-400">{message}</p>
      </div>
    </div>
  );
}

export function SpinnerButton({ label, className }: { label?: string; className?: string }) {
  return (
    <span className={cn("inline-flex items-center gap-1.5", className)} role="status">
      <LoaderCircle aria-hidden="true" className="h-3.5 w-3.5 animate-spin" />
      {label ? <span>{label}</span> : null}
    </span>
  );
}
