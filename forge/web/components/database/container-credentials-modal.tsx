"use client";

import { useQuery } from "@tanstack/react-query";
import { Check, Copy, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { Modal } from "@/components/admin/admin-ui";
import { getDBContainerCredentials } from "@/lib/api/database-containers";

function CopyField({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // silently fail
    }
  };

  return (
    <div>
      <label className="mb-1 block text-xs font-medium text-slate-400 uppercase tracking-wider">{label}</label>
      <div className="flex items-center gap-2">
        <code className="flex-1 rounded-lg bg-[#0f141f] px-3 py-2 text-sm text-emerald-300 break-all font-mono">
          {value}
        </code>
        <button
          className="grid h-8 w-8 shrink-0 place-items-center rounded text-slate-400 hover:bg-white/[0.06] hover:text-slate-200"
          onClick={copy}
          title={`Copy ${label}`}
          type="button"
        >
          {copied ? <Check size={14} className="text-emerald-400" /> : <Copy size={14} />}
        </button>
      </div>
    </div>
  );
}

export function DBContainerCredentialsModal({
  containerId,
  containerName,
  onClose,
}: {
  containerId: string;
  containerName: string;
  onClose: () => void;
}) {
  const credsQuery = useQuery({
    queryKey: ["db-container-creds", containerId],
    queryFn: () => getDBContainerCredentials(containerId),
  });

  return (
    <Modal title={`Credentials: ${containerName}`} onClose={onClose} wide>
      {credsQuery.isLoading ? (
        <div className="py-6 text-center text-sm text-slate-500">Loading credentials...</div>
      ) : credsQuery.isError ? (
        <div className="flex items-start gap-3 rounded-lg border border-red-500/20 bg-red-950/10 p-4 text-sm text-red-200">
          <TriangleAlert size={16} className="mt-0.5 shrink-0" />
          <span>Failed to load credentials: {credsQuery.error.message}</span>
        </div>
      ) : credsQuery.data ? (
        <div className="space-y-5">
          <div className="flex items-start gap-3 rounded-lg border border-amber-500/20 bg-amber-950/10 p-3 text-xs text-amber-300">
            <TriangleAlert size={14} className="mt-0.5 shrink-0" />
            <span>
              These credentials are shown once. Store them securely. If you lose them, you may need to reset the
              container credentials.
            </span>
          </div>

          {credsQuery.data.connectionString && (
            <CopyField label="Connection String" value={credsQuery.data.connectionString} />
          )}

          {credsQuery.data.credentials && Object.keys(credsQuery.data.credentials).length > 0 && (
            <div className="space-y-3">
              <p className="text-xs font-semibold uppercase tracking-wider text-slate-400">Credentials</p>
              <div className="rounded-lg bg-[#0f141f] p-3 space-y-3">
                {Object.entries(credsQuery.data.credentials).map(([key, value]) => (
                  <CopyField key={key} label={key} value={value} />
                ))}
              </div>
            </div>
          )}

          <div className="flex justify-end">
            <button
              className="rounded-lg bg-[#1e2536] px-4 py-2 text-sm font-medium text-slate-300 hover:bg-[#2a3348]"
              onClick={onClose}
              type="button"
            >
              Close
            </button>
          </div>
        </div>
      ) : null}
    </Modal>
  );
}
