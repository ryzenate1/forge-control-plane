"use client";

export default function ErrorFallback({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <div role="alert" className="rounded-xl border border-red-300 bg-red-wash p-6 text-center">
      <h2 className="text-lg font-semibold text-red-dark mb-2">Something went wrong</h2>
      <p className="text-sm text-muted mb-4">
        {process.env.NODE_ENV === 'production'
          ? 'An unexpected error occurred.'
          : error.message}
      </p>
      <button onClick={reset} className="rounded-lg border border-line bg-paper px-5 py-2 text-sm font-bold text-ink hover:bg-surface-hover transition-colors">
        Try again
      </button>
    </div>
  );
}
