"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { OfflineBanner } from "@/components/shared/states-offline";
import { AdminCard, AdminPageLayout } from "@/components/admin/admin-layout";
import * as api from "@/lib/api/forgefile";
import { sanitizeError } from "@/lib/sanitize";

const SAMPLE = `project:
  name: demo
  slug: demo
deploy:
  - name: web
    type: app
    source:
      repo: github.com/example/demo
      branch: main
    build:
      builder: nixpacks
    ports: [8080]
    env:
      NODE_ENV: production
    resources:
      replicas: 1
database:
  name: db
  kind: postgres
  version: "16"
environments: [dev, prod]
`;

export function ForgefileManager() {
  const queryClient = useQueryClient();
  const [content, setContent] = useState(SAMPLE);
  const [selected, setSelected] = useState<string>("");

  // Server state (the manifest slug list) lives in react-query; validate/apply
  // and the single-manifest lookup are request-driven actions, so they are
  // modelled as mutations whose results feed the panels below. Nothing is
  // announced as applied/valid until the corresponding call has resolved.
  const manifestsQuery = useQuery({
    queryKey: ["forgefile-manifests"],
    queryFn: api.listManifests,
  });
  const manifests = manifestsQuery.data ?? [];

  const validateMut = useMutation({ mutationFn: (text: string) => api.validateForgefile(text) });
  const applyMut = useMutation({
    mutationFn: (text: string) => api.applyForgefile(text),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["forgefile-manifests"] });
    },
  });
  const detailMut = useMutation({ mutationFn: (slug: string) => api.getManifest(slug) });

  const validateRes = validateMut.data ?? null;
  const applyRes = applyMut.data ?? null;
  const manifestDetail = detailMut.data ?? null;

  function handleValidate(text: string) {
    // Only one of the two result panels can be live at a time, so each action
    // clears the other's cached result instead of leaving a stale banner.
    applyMut.reset();
    validateMut.mutate(text);
  }

  function handleApply(text: string) {
    validateMut.reset();
    applyMut.mutate(text);
  }

  function handleGet(slug: string) {
    setSelected(slug);
    // Drop the previous lookup first so a failed fetch cannot leave a stale
    // detail panel rendered under the newly selected slug.
    detailMut.reset();
    detailMut.mutate(slug);
  }

  const invalidMessage = validateRes && !validateRes.valid ? validateRes.error || "Invalid" : null;
  const actionError = manifestsQuery.isError
    ? sanitizeError(manifestsQuery.error instanceof Error ? manifestsQuery.error.message : "Load manifests failed")
    : validateMut.isError
      ? sanitizeError(validateMut.error instanceof Error ? validateMut.error.message : "Validate failed")
      : applyMut.isError
        ? sanitizeError(applyMut.error instanceof Error ? applyMut.error.message : "Apply failed")
        : detailMut.isError
          ? sanitizeError(detailMut.error instanceof Error ? detailMut.error.message : "Get manifest failed")
          : null;
  const error = actionError ?? invalidMessage;
  const success = applyRes
    ? `Applied ${applyRes.projectSlug} v${applyRes.version}`
    : validateRes?.valid
      ? "Valid forge.yaml"
      : null;

  function dismissStatus() {
    validateMut.reset();
    applyMut.reset();
  }

  return (
    <AdminPageLayout
      title="Forgefile (env-as-code)"
      description="Declarative forge.yaml — validate with checkKeys + apply via apphosting materialization. Max 256 KiB, unknown keys warned. Apply is idempotent upsert on project slug (version++)."
    >
      <OfflineBanner onRetry={() => void manifestsQuery.refetch()} />
      {error && (
        <div role="alert" className="flex items-center justify-between gap-3 rounded-xl border border-red-500/25 bg-red-500/[0.09] p-4 text-sm text-red-200">
          <span>{error}</span>
          <div className="flex items-center gap-2">
            {manifestsQuery.isError && <button onClick={() => void manifestsQuery.refetch()} className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Retry</button>}
            <button onClick={dismissStatus} className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
          </div>
        </div>
      )}
      {success && (
        <div role="status" className="flex items-center justify-between gap-3 rounded-xl border border-emerald-500/25 bg-emerald-500/[0.09] p-4 text-sm text-emerald-200">
          <span>{success}</span> <button onClick={dismissStatus} className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <AdminCard title="Editor" description="Paste forge.yaml or JSON {content: '<yaml>'} — backend accepts both. GET /forgefile/validate?manifest=… also supported.">
          <textarea value={content} onChange={(e) => setContent(e.target.value)} rows={20} className="w-full rounded-lg border border-[var(--line)] bg-surface p-3 font-mono text-xs" spellCheck={false} />
          <div className="mt-3 flex gap-2">
            <button onClick={() => handleValidate(content)} disabled={validateMut.isPending} className="rounded bg-[var(--brand)] px-4 py-2 text-xs font-bold text-white disabled:opacity-50">Validate</button>
            <button onClick={() => handleApply(content)} disabled={applyMut.isPending} className="rounded bg-[var(--brand)] px-4 py-2 text-xs font-bold text-white disabled:opacity-50">{applyMut.isPending ? "Applying…" : "Apply"}</button>
            <button onClick={() => setContent(SAMPLE)} className="rounded border border-[var(--line)] px-4 py-2 text-xs">Reset Sample</button>
          </div>
          {validateRes && (
            <div className={`mt-3 rounded-lg border p-3 text-xs ${validateRes.valid ? "border-emerald-500/25 bg-emerald-500/[0.09]" : "border-red-500/25 bg-red-500/[0.09]"}`}>
              <p className="font-bold">{validateRes.valid ? "Valid" : "Invalid"} {validateRes.error ? `· ${validateRes.error}` : ""}</p>
              {validateRes.warnings.length > 0 && (
                <ul className="mt-2 list-disc pl-5 text-[var(--text-subtle)]">
                  {validateRes.warnings.map((w, i) => (
                    <li key={i}>{w}</li>
                  ))}
                </ul>
              )}
            </div>
          )}
          {applyRes && (
            <div className="mt-3 rounded-lg border border-emerald-500/25 bg-emerald-500/[0.09] p-3 text-xs">
              <p className="font-bold">Applied {applyRes.projectSlug} v{applyRes.version}</p>
              <p className="text-[var(--text-subtle)]">Links: {Object.entries(applyRes.links).map(([k, v]) => `${k}=${v}`).join(", ") || "—"}</p>
              <p className="text-[var(--text-subtle)]">Apps: {applyRes.apps.map((a) => `${a.appName} (${a.domain})`).join(", ") || "—"}</p>
              {applyRes.warnings.length > 0 && (
                <ul className="mt-1 list-disc pl-5 text-amber-700">
                  {applyRes.warnings.map((w, i) => (
                    <li key={i}>{w}</li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </AdminCard>

        <AdminCard title="Manifests" description="GET /forgefile lists slugs visible to caller (admin sees all, others only own). GET /forgefile/:slug returns version + updatedAt + manifest (requires ownership).">
          <div className="flex gap-2">
            <button onClick={() => void manifestsQuery.refetch()} className="rounded border border-[var(--line)] px-3 py-1.5 text-xs">Refresh</button>
            <span className="text-xs text-[var(--text-subtle)] py-1.5">{manifests.length} manifest(s)</span>
          </div>
          {manifestsQuery.isError ? (
            <p role="alert" className="mt-3 text-sm text-red-200">The manifest list could not be loaded — the count above is unknown, not zero.</p>
          ) : manifests.length === 0 ? (
            <p className="mt-3 text-sm text-[var(--text-subtle)]">No manifests. Apply one to create a project slug row (FORGEFILE_BASE_DOMAIN env controls app link base domain; empty → http://&lt;name&gt;.local/deploy/&lt;appId&gt;).</p>
          ) : (
            <div className="mt-3 space-y-2">
              {manifests.map((slug) => (
                <button key={slug} onClick={() => handleGet(slug)} className={`w-full text-left rounded-lg border px-3 py-2 text-sm ${selected === slug ? "border-[var(--brand)] bg-[color-mix(in_srgb,var(--brand)_10%,transparent)]" : "border-[var(--line)] bg-surface"}`}>
                  {slug}
                </button>
              ))}
            </div>
          )}
          {manifestDetail && (
            <div className="mt-4 rounded-lg border border-[var(--line)] bg-surface p-3 text-xs">
              <p className="font-bold">{manifestDetail.slug} · v{manifestDetail.version} · {new Date(manifestDetail.updatedAt).toLocaleString()}</p>
              <pre className="mt-2 max-h-64 overflow-auto rounded bg-[var(--surface-input)] p-2 font-mono text-[11px]">{JSON.stringify(manifestDetail.manifest, null, 2)}</pre>
            </div>
          )}
          <div className="mt-4 rounded-lg border border-[var(--line)] bg-[var(--surface-input)] p-3 text-xs text-[var(--text-subtle)]">
            <p className="font-bold text-[var(--text)]">Schema</p>
            <p>Top-level: project (name, slug), deploy[] (name, type app|compose|db, source.repo, build.builder dockerfile|nixpacks|heroku|static, ports, env, resources), database (name, kind, version), environments[]</p>
            <p className="mt-1">Base domain: <code className="rounded bg-surface px-1">{process.env.NEXT_PUBLIC_FORGEFILE_BASE_DOMAIN || "FORGEFILE_BASE_DOMAIN env (server)"}</code></p>
          </div>
        </AdminCard>
      </div>
    </AdminPageLayout>
  );
}
