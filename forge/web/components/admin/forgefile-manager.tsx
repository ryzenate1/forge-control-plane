"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { OfflineBanner } from "@/components/shared/states-offline";
import { AdminCard } from "@/components/admin/admin-layout";
import { AdminPageLayout, Btn, SectionHeader } from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
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

  function editContent(text: string) {
    setContent(text);
    validateMut.reset();
    applyMut.reset();
  }

  const actionPending = validateMut.isPending || applyMut.isPending;

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
    <AdminPageLayout>
      <SectionHeader title="Forgefile" sub="Validate and apply project configuration from a forge.yaml manifest." info={adminPageGuides.forgefile} status={<FreshnessBadge state={sourceState(manifestsQuery)} />} />
      <OfflineBanner onRetry={() => void manifestsQuery.refetch()} />
      {error && (
        <div role="alert" className="flex items-center justify-between gap-3 rounded-xl border border-danger-line bg-danger/[0.09] p-4 text-sm text-danger">
          <span>{error}</span>
          <div className="flex items-center gap-2">
            {manifestsQuery.isError && <button onClick={() => void manifestsQuery.refetch()} className="rounded px-2 py-1 text-xs underline hover:bg-overlay-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Retry</button>}
            <button onClick={dismissStatus} className="rounded px-2 py-1 text-xs underline hover:bg-overlay-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
          </div>
        </div>
      )}
      {success && (
        <div role="status" className="flex items-center justify-between gap-3 rounded-xl border border-ok-line bg-ok/[0.09] p-4 text-sm text-ok">
          <span>{success}</span> <button onClick={dismissStatus} className="rounded px-2 py-1 text-xs underline hover:bg-overlay-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <AdminCard title="Editor" description="Edit your manifest, validate its configuration, then apply it to your project.">
          <textarea aria-label="Forgefile manifest" value={content} disabled={actionPending} onChange={(e) => editContent(e.target.value)} rows={20} className="w-full rounded-lg border border-[var(--line)] bg-surface p-3 font-mono text-xs" spellCheck={false} />
          <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-line pt-4">
            <Btn tone="ghost" onClick={() => handleValidate(content)} disabled={actionPending || !content.trim()} loading={validateMut.isPending}>Validate</Btn>
            <Btn onClick={() => handleApply(content)} disabled={actionPending || !content.trim()} loading={applyMut.isPending}>Apply manifest</Btn>
            <Btn tone="subtle" onClick={() => editContent(SAMPLE)} disabled={actionPending}>Reset sample</Btn>
          </div>
          {validateRes && (
            <div className={`mt-3 rounded-lg border p-3 text-xs ${validateRes.valid ? "border-ok-line bg-ok/[0.09]" : "border-danger-line bg-danger/[0.09]"}`}>
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
            <div className="mt-3 rounded-lg border border-ok-line bg-ok/[0.09] p-3 text-xs">
              <p className="font-bold">Applied {applyRes.projectSlug} v{applyRes.version}</p>
              <p className="text-[var(--text-subtle)]">Links: {Object.entries(applyRes.links).map(([k, v]) => `${k}=${v}`).join(", ") || "—"}</p>
              <p className="text-[var(--text-subtle)]">Apps: {applyRes.apps.map((a) => `${a.appName} (${a.domain})`).join(", ") || "—"}</p>
              {applyRes.warnings.length > 0 && (
                <ul className="mt-1 list-disc pl-5 text-warn">
                  {applyRes.warnings.map((w, i) => (
                    <li key={i}>{w}</li>
                  ))}
                </ul>
              )}
            </div>
          )}
        </AdminCard>

        <AdminCard title="Manifests" description="Browse saved project manifests and inspect their latest versions.">
          <div className="flex items-center justify-between gap-3">
            <span className="text-xs text-text-subtle">{manifestsQuery.isSuccess ? `${manifests.length} manifests` : "Saved manifests"}</span>
            <Btn tone="ghost" size="sm" onClick={() => void manifestsQuery.refetch()} disabled={manifestsQuery.isFetching}>Refresh</Btn>
          </div>
          {manifestsQuery.isError ? (
            <p role="alert" className="mt-3 text-sm text-danger">Could not load saved manifests. Try refreshing the list.</p>
          ) : manifestsQuery.isLoading ? (
            <p role="status" className="mt-3 text-sm text-text-subtle">Loading manifests…</p>
          ) : manifests.length === 0 ? (
            <p className="mt-3 text-sm text-[var(--text-subtle)]">No saved manifests yet. Apply a manifest to create your first project.</p>
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
