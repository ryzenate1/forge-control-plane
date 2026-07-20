"use client";

import { RefreshCw, WifiOff } from "lucide-react";
import { useEffect, useState } from "react";

export function OfflineBanner({
  onRetry,
}: {
  onRetry?: () => void;
}) {
  const [online, setOnline] = useState(
    typeof navigator !== "undefined" ? navigator.onLine : true,
  );

  useEffect(() => {
    const goOnline = () => setOnline(true);
    const goOffline = () => setOnline(false);
    window.addEventListener("online", goOnline);
    window.addEventListener("offline", goOffline);
    return () => {
      window.removeEventListener("online", goOnline);
      window.removeEventListener("offline", goOffline);
    };
  }, []);

  if (online) return null;

  return (
    <div
      className="sticky top-0 z-50 flex items-center justify-center gap-3 border-b border-amber-500/30 bg-amber-500/[0.12] px-4 py-2.5 text-sm text-amber-100 backdrop-blur"
      role="alert"
    >
      <WifiOff aria-hidden="true" className="h-4 w-4 shrink-0" />
      <span>You are offline. Some features may be unavailable.</span>
      {onRetry ? (
        <button
          className="inline-flex items-center gap-1 rounded bg-amber-600 px-2.5 py-1 text-xs font-semibold text-white hover:bg-amber-500"
          onClick={onRetry}
          type="button"
        >
          <RefreshCw size={12} />
          Retry
        </button>
      ) : null}
    </div>
  );
}
