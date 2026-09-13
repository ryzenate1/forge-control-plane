"use client";

/** Skeletons and spinners. All geometry and colour come from `.ui-skeleton`. */

import { LoaderCircle } from "lucide-react";
import { cn } from "@/lib/utils";

export function Skeleton({ className }: { className?: string }) { return <div aria-hidden="true" className={cn("ui-skeleton", className)} />; }
export function LoadingSpinner({ className, label = "Loading" }: { className?: string; label?: string }) { return <span className="inline-flex items-center justify-center" role="status"><LoaderCircle aria-hidden="true" className={cn("size-6 animate-spin text-brand", className)} /><span className="sr-only">{label}</span></span>; }
export function FullPageSpinner({ label = "Loading" }: { label?: string }) { return <div className="grid min-h-screen place-items-center bg-canvas text-xs text-text-subtle" role="status"><div className="text-center"><LoadingSpinner label={label} /><p aria-hidden="true" className="mt-3">{label}</p></div></div>; }
export function PageSkeleton() { return <main aria-label="Loading page" className="mx-auto w-full max-w-page space-y-5 px-4 py-6 sm:px-6" role="status"><Skeleton className="h-7 w-52" /><Skeleton className="h-3.5 w-80 max-w-full" /><div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3"><CardSkeleton /><CardSkeleton /><CardSkeleton /></div></main>; }
export function CardSkeleton() { return <div aria-label="Loading content" className="ui-surface overflow-hidden shadow-card" role="status"><div className="ui-card-header"><Skeleton className="h-4 w-36" /></div><div className="space-y-3 p-4 sm:p-5"><Skeleton className="h-3.5 w-full" /><Skeleton className="h-3.5 w-3/4" /><Skeleton className="h-9 w-1/2" /></div></div>; }
export function TableSkeleton({ rows = 5 }: { rows?: number }) { return <div aria-label="Loading table" className="ui-surface overflow-hidden shadow-card" role="status"><div className="ui-card-header"><Skeleton className="h-4 w-32" /></div><div className="divide-y divide-line">{Array.from({ length: rows }).map((_, index) => <div className="flex items-center gap-4 px-4 py-2.5" key={index}><Skeleton className="h-3.5 w-8" /><Skeleton className="h-3.5 w-2/5" /><Skeleton className="h-3.5 w-1/4" /></div>)}</div></div>; }
