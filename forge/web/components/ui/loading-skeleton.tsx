"use client";

import { LoaderCircle } from "lucide-react";
import { cn } from "@/lib/utils";

export function Skeleton({ className }: { className?: string }) { return <div aria-hidden="true" className={cn("animate-pulse rounded-lg bg-white/[0.07]", className)} />; }
export function LoadingSpinner({ className, label = "Loading" }: { className?: string; label?: string }) { return <span className="inline-flex items-center justify-center" role="status"><LoaderCircle aria-hidden="true" className={cn("h-8 w-8 animate-spin text-red-500", className)} /><span className="sr-only">{label}</span></span>; }
export function FullPageSpinner({ label = "Loading" }: { label?: string }) { return <div className="grid min-h-screen place-items-center bg-[var(--canvas)] text-sm text-[var(--text-subtle)]" role="status"><div className="text-center"><LoadingSpinner label={label} /><p className="mt-3">{label}</p></div></div>; }
export function PageSkeleton() { return <main aria-label="Loading page" className="mx-auto max-w-7xl space-y-6 px-4 py-8" role="status"><Skeleton className="h-8 w-52" /><Skeleton className="h-4 w-80 max-w-full" /><div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3"><CardSkeleton /><CardSkeleton /><CardSkeleton /></div></main>; }
export function CardSkeleton() { return <div aria-label="Loading content" className="ui-card" role="status"><div className="ui-card-header"><Skeleton className="h-5 w-36" /></div><div className="space-y-3 p-5"><Skeleton className="h-4 w-full" /><Skeleton className="h-4 w-3/4" /><Skeleton className="h-10 w-1/2" /></div></div>; }
export function TableSkeleton({ rows = 5 }: { rows?: number }) { return <div aria-label="Loading table" className="ui-card" role="status"><div className="ui-card-header"><Skeleton className="h-5 w-32" /></div><div className="divide-y divide-white/[0.07]">{Array.from({ length: rows }).map((_, index) => <div className="flex items-center gap-4 px-5 py-4" key={index}><Skeleton className="h-4 w-8" /><Skeleton className="h-4 w-2/5" /><Skeleton className="h-4 w-1/4" /></div>)}</div></div>; }
