"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText, RefreshCw } from "lucide-react";
import { OfflineBanner } from "@/components/shared/states-offline";
import {
  AdminErrorState,
  AdminLoadingRows,
  AdminPageLayout,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Pill,
  SectionHeader,
  cn,
} from "@/components/admin/admin-ui";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
import { useConfirm } from "@/components/ui/confirm-dialog";
import * as api from "@/lib/api/forgefile";
import { sanitizeError } from "@/lib/sanitize";
import { formatDate } from "@/lib/utils";

/** Offered only through the explicit "Load example" action. The editor used to
 * boot with a complete working manifest and a live Apply button, so one mis-click
 * upserted a `demo` project with an app and a database. */
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

const MAX_MANIFEST_BYTES = 256 * 1024;

function sizeOf(text: string): number {
  return new TextEncoder().encode(text).length;
}

export function ForgefileManager() {
  const queryClient = useQueryClient();
  const [confirm, renderConfirm] = useConfirm();
  const [content, setContent] = useState("");
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

  const tooLarge = sizeOf(content) > MAX_MANIFEST_BYTES;

  function handleValidate(text: string) {
    // Only one of the two result panels can be live at a time, so each action
    // clears the other's cached result instead of leaving a stale banner.
    applyMut.reset();
    validateMut.mutate(text);
  }

  async function handleApply(text: string) {
    validateMut.reset();
    const ok = await confirm({
      title: "Apply this Forgefile?",
      // No dry-run endpoint exists (`POST /forgefile/apply` is the only plan
      // surface), so the honest disclosure is the scope plus "cannot be listed
      // in advance", not a fabricated diff.
      description:
        "Applying validates the document and then materialises what it declares: the project it names is created or updated, its applications and database are provisioned or redeployed, and a new manifest version is stored. This build has no dry-run endpoint, so the exact changes cannot be listed before they happen. Cancel to validate first.",
      danger: true,
      confirmLabel: "Apply",
    });
    if (!ok) return;
    applyMut.mutate(text);
  }

  function handleGet(slug: string) {
    if (selected === slug) {
      // Selecting the same row again clears the panel instead of re-fetching.
      setSelected("");
      detailMut.reset();
      return;
    }
    setSelected(slug);
    // Drop the previous lookup first so a failed fetch cannot leave a stale
    // detail panel rendered under the newly selected slug.
    detailMut.reset();
    detailMut.mutate(slug);
  }

  // One collapsed string used to make a validate failure, an apply failure and a
  // list failure indistinguishable; each panel now carries its own.
  const listError = manifestsQuery.isError
    ? sanitizeError(manifestsQuery.error instanceof Error ? manifestsQuery.error.message : "Manifest list could not be loaded")
    : null;
  const validateError = validateMut.isError
    ? sanitizeError(validateMut.error instanceof Error ? validateMut.error.message : "Validation request failed")
    : null;
  const applyError = applyMut.isError
    ? sanitizeError(applyMut.error instanceof Error ? applyMut.error.message : "Apply request failed")
    : null;
  const detailError = detailMut.isError
    ? sanitizeError(detailMut.error instanceof Error ? detailMut.error.message : "Manifest could not be read")
    : null;

  function dismissStatus() {
    validateMut.reset();
    applyMut.reset();
  }

  return (
    <AdminPageLayout>
      <SectionHeader info={adminPageGuides.forgefile} />

      <OfflineBanner onRetry={() => void manifestsQuery.refetch()} />

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader title="Forgefile editor" icon={FileText} />
          <div className="space-y-3 p-4">
            <p className="text-meta text-text-subtle">
              Write the document in YAML. Nothing is sent until you validate or apply it, and the editor
              starts empty so an example can never be applied by accident.
            </p>
            <textarea
              aria-label="Forgefile document"
              value={content}
              onChange={(e) => setContent(e.target.value)}
              rows={20}
              spellCheck={false}
              className="ui-input w-full resize-y font-mono text-xs"
              placeholder={"project:\n  name: my-app\n  slug: my-app\ndeploy:\n  - name: web\n    type: app"}
            />
            <div className="flex flex-wrap items-center gap-2">
              <Btn size="sm" tone="ghost" loading={validateMut.isPending} disabled={content.trim() === "" || tooLarge} onClick={() => handleValidate(content)}>
                {validateMut.isPending ? "Validating…" : "Validate"}
              </Btn>
              <Btn size="sm" tone="primary" disabled={content.trim() === "" || tooLarge || applyMut.isPending} onClick={() => void handleApply(content)}>
                {applyMut.isPending ? "Applying…" : "Apply"}
              </Btn>
              <Btn size="sm" tone="subtle" disabled={content === SAMPLE} onClick={() => setContent(SAMPLE)}>Load example</Btn>
              <Btn size="sm" tone="ghost" disabled={content === ""} onClick={() => { setContent(""); dismissStatus(); }}>Clear</Btn>
              <span className="t-meta">{content === "" ? "Empty document" : `${sizeOf(content)} of ${MAX_MANIFEST_BYTES} bytes`}</span>
            </div>
            {tooLarge ? (
              <div className="ui-alert ui-alert-warning" role="alert">
                <span>This document is larger than the {MAX_MANIFEST_BYTES} byte limit the server accepts; validate and apply are unavailable until it is smaller.</span>
              </div>
            ) : null}
            {content.trim() === "" ? (
              <p className="ui-hint">Apply stays enabled for an empty document on the server, so the buttons here are disabled until there is something to send.</p>
            ) : null}

            {validateError ? (
              <div className="ui-alert ui-alert-danger" role="alert"><span>Validation request failed: {validateError}</span></div>
            ) : null}

            {validateRes ? (
              <div className={cn("ui-alert", validateRes.valid ? "ui-alert-success" : "ui-alert-danger")} role="status">
                <div className="space-y-1.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <Pill tone={validateRes.valid ? "green" : "red"}>{validateRes.valid ? "Valid" : "Not valid"}</Pill>
                    <span className="text-meta">
                      {validateRes.valid
                        ? "The server accepted this document; nothing has been created yet."
                        : "The server rejected this document; nothing was created."}
                    </span>
                  </div>
                  {validateRes.error ? <p className="font-mono text-xs">{validateRes.error}</p> : null}
                  {Array.isArray(validateRes.warnings) && validateRes.warnings.length > 0 ? (
                    <ul className="list-disc space-y-0.5 pl-5 text-xs">
                      {validateRes.warnings.map((w, i) => <li key={`v-${i}`}>{w}</li>)}
                    </ul>
                  ) : null}
                </div>
              </div>
            ) : null}

            {applyError ? (
              <div className="ui-alert ui-alert-danger" role="alert"><span>Apply failed: {applyError}</span></div>
            ) : null}

            {applyRes ? (
              <div className="ui-alert ui-alert-success" role="status">
                <div className="space-y-1.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <Pill tone="green">Apply reported</Pill>
                    <span className="font-mono text-xs">{applyRes.projectSlug} · v{applyRes.version}</span>
                    <span className="text-meta">{formatDate(applyRes.appliedAt, "Unknown")}</span>
                  </div>
                  {/* ApplyResult carries no created/updated/unchanged flag, so the
                      panel says what it does not know instead of implying a change. */}
                  <p className="text-meta">
                    The server does not report whether this apply changed anything, so a repeat of an
                    identical document still returns a version. Treat this as the response it sent, not as
                    proof of a new deployment.
                  </p>
                  <p className="text-xs">
                    Applications: {Array.isArray(applyRes.apps) && applyRes.apps.length > 0
                      ? applyRes.apps.map((a) => `${a.appName} (${a.domain || a.url || "no domain reported"})`).join(", ")
                      : "none reported"}
                  </p>
                  <p className="text-xs">
                    Links: {Object.entries(applyRes.links ?? {}).length > 0
                      ? Object.entries(applyRes.links).map(([k, v]) => `${k}=${v}`).join(", ")
                      : "none reported"}
                  </p>
                  {Array.isArray(applyRes.warnings) && applyRes.warnings.length > 0 ? (
                    <ul className="list-disc space-y-0.5 pl-5 text-xs">
                      {applyRes.warnings.map((w, i) => <li key={`a-${i}`}>{w}</li>)}
                    </ul>
                  ) : null}
                </div>
              </div>
            ) : null}
          </div>
        </Card>

        <Card>
          <CardHeader
            title={manifestsQuery.data ? `Stored manifests (${manifests.length})` : "Stored manifests"}
            icon={FileText}
            action={<Btn size="sm" tone="ghost" ariaLabel="Reload the stored manifest list" loading={manifestsQuery.isFetching} onClick={() => void manifestsQuery.refetch()}><RefreshCw size={12} /> Refresh</Btn>}
          />
          <div className="space-y-3 p-4">
            {manifestsQuery.isLoading ? (
              <AdminLoadingRows rows={3} label="Loading stored manifests…" />
            ) : listError ? (
              <AdminErrorState message={listError} retry={() => void manifestsQuery.refetch()} />
            ) : manifests.length === 0 ? (
              <EmptyState
                icon={FileText}
                title="No manifests stored yet"
                message="A manifest is stored the first time a document is applied successfully."
              />
            ) : (
              <div className="space-y-2">
                <p className="text-meta text-text-subtle">Select one to read its stored version. Selecting it again closes it.</p>
                {manifests.map((slug) => (
                  <button
                    key={slug}
                    aria-pressed={selected === slug}
                    className={cn(
                      "w-full rounded-lg border px-3 py-2 text-left text-sm text-text transition hover:border-line-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]",
                      selected === slug
                        ? "border-[var(--brand)] bg-[color-mix(in_srgb,var(--brand)_10%,transparent)]"
                        : "border-line bg-overlay-subtle",
                    )}
                    onClick={() => handleGet(slug)}
                    type="button"
                  >
                    {slug}
                  </button>
                ))}
              </div>
            )}

            {detailError && selected ? (
              <AdminErrorState message={`${selected}: ${detailError}`} retry={() => detailMut.mutate(selected)} />
            ) : null}

            {manifestDetail && selected === manifestDetail.slug ? (
              <div className="space-y-2 rounded-lg border border-line bg-overlay-subtle p-3">
                <p className="text-sm font-semibold text-text">
                  {manifestDetail.slug}
                  <span className="ml-2 font-mono text-xs text-text-subtle">v{manifestDetail.version}</span>
                  <span className="ml-2 text-xs text-text-subtle">{formatDate(manifestDetail.updatedAt, "Unknown")}</span>
                </p>
                <pre className="max-h-64 overflow-auto rounded bg-overlay p-2 font-mono text-xs text-text-subtle">{JSON.stringify(manifestDetail.manifest, null, 2)}</pre>
              </div>
            ) : null}

            <details className="rounded-lg border border-line bg-overlay-subtle p-3">
              <summary className="cursor-pointer text-sm font-semibold text-text">Document shape (reference)</summary>
              <p className="mt-2 text-xs leading-5 text-text-subtle">
                Top level: <span className="font-mono">project</span> (name, slug),{" "}
                <span className="font-mono">deploy[]</span> (name, type app | compose | db, source.repo,
                build.builder dockerfile | nixpacks | heroku | static, ports, env, resources),{" "}
                <span className="font-mono">database</span> (name, kind, version) and{" "}
                <span className="font-mono">environments[]</span>.
              </p>
              <p className="mt-2 text-xs leading-5 text-text-subtle">
                Application addresses come back in the apply result above; the panel does not guess them.
              </p>
            </details>
          </div>
        </Card>
      </div>

      {renderConfirm()}
    </AdminPageLayout>
  );
}
