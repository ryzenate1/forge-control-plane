"use client";

import { useConfirm } from "@/components/ui/confirm-dialog";
import { Alert, Badge, Button, Dialog, Field, Input, Select, Switch } from "@/components/ui/primitives";
import { ErrorAlert, SpinnerInline, useAppToast } from "@/components/shared";
import { fetchAppDomains } from "@/lib/api/apps";
import {
  DOMAIN_REDIRECT_STATUS_CODES,
  MAX_DOMAIN_REDIRECT_RULES,
  applyDomainRedirectGateway,
  applyDomainRedirectPresets,
  createDomainRedirect,
  deleteDomainRedirect,
  fetchDomainRedirects,
  updateDomainRedirect,
} from "@/lib/api/redirects";
import type {
  ApplyDomainRedirectsResult,
  CreateDomainRedirectInput,
  DomainRedirect,
  DomainRedirectPreset,
  UpdateDomainRedirectInput,
} from "@/lib/api/redirects";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, RefreshCw, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { errorMessage } from "@/lib/utils";

/**
 * Per-application "Domain Redirects & Forwards" settings section.
 *
 * The Dokploy equivalent is a redirects tab on the application page; here it is
 * a self-contained card so it can sit next to Domains/Certificates in any app
 * layout. Everything it shows comes from the API — there is no local shadow
 * state — so a second browser tab or a preset applied by a deployment is visible
 * after one refetch rather than being silently overwritten on the next save.
 */

const redirectsKey = (appId: string) => ["app-domain-redirects", appId] as const;

const permanentStatusCodes = new Set([301, 308]);

function statusLabel(code: number): string {
  switch (code) {
    case 301:
      return "301 · permanent";
    case 302:
      return "302 · temporary";
    case 307:
      return "307 · keep method";
    case 308:
      return "308 · permanent, keep method";
    default:
      return String(code);
  }
}

/** "host + path" for display, with the path hidden when it is the root. */
function endpoint(domain: string, path?: string | null): string {
  if (!path || path === "/") return domain;
  return `${domain}${path}`;
}

type Draft = {
  sourceDomain: string;
  targetDomain: string;
  sourcePath: string;
  targetPath: string;
  statusCode: number;
  enabled: boolean;
};

function draftFromRule(rule: DomainRedirect | null): Draft {
  return {
    sourceDomain: rule?.sourceDomain ?? "",
    targetDomain: rule?.targetDomain ?? "",
    sourcePath: rule?.sourcePath ?? "/",
    // A null target path means "preserve the request path", which the form shows
    // as an empty field; sending it back empty restores the null rather than
    // quietly turning the rule into "everything goes to the target root".
    targetPath: rule?.targetPath ?? "",
    statusCode: rule?.statusCode ?? 301,
    enabled: rule?.enabled ?? true,
  };
}

export interface DomainRedirectsPanelProps {
  appId: string;
}

export function DomainRedirectsPanel({ appId }: DomainRedirectsPanelProps) {
  const qc = useQueryClient();
  const { success, error, warning } = useAppToast();
  const [confirm, renderConfirm] = useConfirm();

  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<DomainRedirect | null>(null);
  const [applyResult, setApplyResult] = useState<ApplyDomainRedirectsResult | null>(null);

  const rules = useQuery<DomainRedirect[]>({
    queryKey: redirectsKey(appId),
    queryFn: () => fetchDomainRedirects(appId),
    enabled: Boolean(appId),
  });

  // Reuses the domains query key so the panel and the domains tab share one
  // request. The hosts are advisory: they fill the pickers and let the panel say
  // "this source host is not bound to the application", which is the one way a
  // syntactically valid rule can be silently dead.
  const appDomains = useQuery({
    queryKey: ["app-domains", appId],
    queryFn: () => fetchAppDomains(appId),
    enabled: Boolean(appId),
  });

  const boundHosts = (appDomains.data ?? []).map((domain) => domain.domain.toLowerCase());
  // Until the host list has actually arrived, "not bound" is unknown rather than
  // true. Warning about every rule while the domains query is still in flight
  // would be a fabricated problem.
  const hostsKnown = appDomains.isSuccess && boundHosts.length > 0;

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: redirectsKey(appId) });
  };

  const createMut = useMutation({
    mutationFn: (input: CreateDomainRedirectInput) => createDomainRedirect(appId, input),
    onSuccess: () => {
      success("Redirect", "created");
      refresh();
      setEditorOpen(false);
      setEditing(null);
    },
  });

  const updateMut = useMutation({
    mutationFn: (vars: { id: string; input: UpdateDomainRedirectInput }) =>
      updateDomainRedirect(appId, vars.id, vars.input),
    onSuccess: () => {
      success("Redirect", "updated");
      refresh();
      setEditorOpen(false);
      setEditing(null);
    },
  });

  const toggleMut = useMutation({
    mutationFn: (vars: { id: string; enabled: boolean }) =>
      updateDomainRedirect(appId, vars.id, { enabled: vars.enabled }),
    // No toast: a toggle is visible in the row it came from, and every flip
    // firing a notification makes the switch look like a command palette.
    onSuccess: refresh,
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteDomainRedirect(appId, id),
    onSuccess: () => {
      success("Redirect", "deleted");
      refresh();
    },
  });

  const presetMut = useMutation({
    mutationFn: (presets: DomainRedirectPreset[]) => applyDomainRedirectPresets(appId, presets),
    onSuccess: (result) => {
      refresh();
      // The API reports what it refused, so report it instead of claiming the
      // preset was applied when it created nothing.
      if (result.created.length > 0) {
        success("Redirects", "created");
      }
      if (result.skipped.length > 0) {
        const first = result.skipped[0];
        warning(
          result.created.length > 0 ? "Partially applied" : "Nothing was added",
          `${result.skipped.length} rule${result.skipped.length === 1 ? "" : "s"} skipped — ${first.domain ? `${first.domain}: ` : ""}${first.reason}`,
        );
      } else if (result.created.length === 0) {
        warning("Nothing was added", "The preset produced no rules for this application.");
      }
    },
  });

  const applyMut = useMutation({
    mutationFn: () => applyDomainRedirectGateway(appId),
    onSuccess: (result) => setApplyResult(result),
    onError: (err) => {
      setApplyResult(null);
      error("Gateway", errorMessage(err, "Failed to regenerate the gateway configuration"));
    },
  });

  function submitDraft(draft: Draft) {
    const sourcePath = draft.sourcePath.trim() || "/";
    const targetPath = draft.targetPath.trim();
    if (editing) {
      updateMut.mutate({
        id: editing.id,
        input: {
          sourceDomain: draft.sourceDomain.trim(),
          targetDomain: draft.targetDomain.trim(),
          sourcePath,
          // Empty field => "preserve the request path" again.
          clearTargetPath: targetPath === "",
          targetPath: targetPath === "" ? undefined : targetPath,
          statusCode: draft.statusCode,
          enabled: draft.enabled,
        },
      });
      return;
    }
    createMut.mutate({
      sourceDomain: draft.sourceDomain.trim(),
      targetDomain: draft.targetDomain.trim(),
      sourcePath,
      targetPath: targetPath === "" ? null : targetPath,
      statusCode: draft.statusCode,
      enabled: draft.enabled,
    });
  }

  if (rules.isLoading) return <SpinnerInline label="Loading redirects…" />;

  if (rules.isError) {
    return (
      <ErrorAlert
        error={rules.error}
        title="Failed to load redirects"
        onRetry={() => void rules.refetch()}
      />
    );
  }

  const items = rules.data ?? [];
  const atLimit = items.length >= MAX_DOMAIN_REDIRECT_RULES;
  const busy = createMut.isPending || updateMut.isPending;
  const formError = createMut.error ?? updateMut.error;

  return (
    <section className="ui-card">
      {renderConfirm()}
      <datalist id="domain-redirect-hosts">
        {boundHosts.map((host) => (
          <option key={host} value={host} />
        ))}
      </datalist>

      <div className="ui-card-header">
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-[var(--text)]">Domain redirects</h2>
          <p className="mt-1 text-xs leading-5 text-[var(--text-subtle)]">
            {items.length} of {MAX_DOMAIN_REDIRECT_RULES} rules · rendered into the gateway
            (Traefik middlewares or Caddy routes) for this application
          </p>
        </div>
        <div className="ml-auto flex shrink-0 flex-wrap items-center justify-end gap-2">
          <Button
            loading={presetMut.isPending && Boolean(presetMut.variables?.includes("http-to-https"))}
            onClick={() => presetMut.mutate(["http-to-https"])}
            size="sm"
            variant="secondary"
          >
            Enable HTTPS
          </Button>
          <Button
            loading={presetMut.isPending && Boolean(presetMut.variables?.includes("www-apex"))}
            onClick={() => presetMut.mutate(["www-apex"])}
            size="sm"
            variant="secondary"
          >
            Canonicalize www
          </Button>
          <Button
            loading={applyMut.isPending}
            onClick={() => applyMut.mutate()}
            size="sm"
            variant="secondary"
          >
            <RefreshCw aria-hidden="true" className="h-4 w-4" />
            Regenerate config
          </Button>
          <Button
            disabled={atLimit}
            onClick={() => {
              setEditing(null);
              createMut.reset();
              updateMut.reset();
              setEditorOpen(true);
            }}
            size="sm"
          >
            Add rule
          </Button>
        </div>
      </div>

      {applyResult ? (
        <Alert
          className="mt-4"
          tone={applyResult.gatewaySynced ? "success" : "warning"}
          title={
            applyResult.gatewaySynced
              ? `Regenerated ${applyResult.ruleCount} rule${applyResult.ruleCount === 1 ? "" : "s"}`
              : "Config regenerated, gateway not reloaded"
          }
        >
          {applyResult.syncDetail ?? "The generated rules are returned by the API."}
          {applyResult.adapter ? (
            <span className="ml-1 text-[var(--text-subtle)]">(adapter: {applyResult.adapter})</span>
          ) : null}
        </Alert>
      ) : null}

      {items.length === 0 ? (
        <p className="px-1 py-6 text-sm text-[var(--text-subtle)]">
          No redirect rules yet. Use <strong className="text-[var(--text)]">Enable HTTPS</strong>{" "}
          and <strong className="text-[var(--text)]">Canonicalize www</strong> for the two common
          cases, or <strong className="text-[var(--text)]">Add rule</strong> for a path-based 301.
        </p>
      ) : (
        <ul className="divide-y divide-[var(--line)]">
          {items.map((rule) => {
            const sourceBound = boundHosts.includes(rule.sourceDomain.toLowerCase());
            const targetBound = boundHosts.includes(rule.targetDomain.toLowerCase());
            return (
              <li className="flex flex-wrap items-center gap-3 py-3" key={rule.id}>
                <div className="min-w-0 flex-1">
                  <p className="flex min-w-0 flex-wrap items-center gap-x-2 font-mono text-sm text-[var(--text)]">
                    <span className="truncate">{endpoint(rule.sourceDomain, rule.sourcePath)}</span>
                    <ArrowRight aria-hidden="true" className="h-3.5 w-3.5 shrink-0 text-[var(--text-subtle)]" />
                    <span className="truncate">
                      {rule.targetPath === null || rule.targetPath === undefined
                        ? `${rule.targetDomain}${rule.sourcePath === "/" ? "" : "…"}`
                        : endpoint(rule.targetDomain, rule.targetPath)}
                    </span>
                  </p>
                  <p className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-[var(--text-subtle)]">
                    {rule.presetType && rule.presetType !== "custom" ? (
                      <span className="uppercase tracking-wide">{rule.presetType}</span>
                    ) : (
                      <span>custom rule</span>
                    )}
                    <span aria-hidden="true">·</span>
                    <span>
                      {rule.targetPath === null || rule.targetPath === undefined
                        ? "request path preserved"
                        : "request path replaced"}
                    </span>
                    {!hostsKnown ? null : !sourceBound ? (
                      <>
                        <span aria-hidden="true">·</span>
                        <span className="inline-flex items-center gap-1 text-[var(--warning)]">
                          <TriangleAlert aria-hidden="true" className="h-3 w-3" />
                          {rule.sourceDomain} is not bound to this application, so this rule cannot
                          match
                        </span>
                      </>
                    ) : !targetBound ? (
                      <>
                        <span aria-hidden="true">·</span>
                        <span className="text-[var(--warning)]">
                          {rule.targetDomain} is not bound to this application
                        </span>
                      </>
                    ) : null}
                  </p>
                </div>

                <Badge tone={permanentStatusCodes.has(rule.statusCode) ? "success" : "warning"}>
                  {rule.statusCode}
                </Badge>

                <Switch
                  checked={rule.enabled}
                  disabled={toggleMut.isPending}
                  label={rule.enabled ? "Enabled" : "Disabled"}
                  onCheckedChange={(enabled) => toggleMut.mutate({ id: rule.id, enabled })}
                />

                <Button
                  onClick={() => {
                    setEditing(rule);
                    createMut.reset();
                    updateMut.reset();
                    setEditorOpen(true);
                  }}
                  size="sm"
                  variant="ghost"
                >
                  Edit
                </Button>
                <Button
                  loading={deleteMut.isPending && deleteMut.variables === rule.id}
                  onClick={async () => {
                    const label = endpoint(rule.sourceDomain, rule.sourcePath);
                    if (await confirm({
                      confirmLabel: "Remove",
                      danger: true,
                      description: `${label} will stop redirecting. The generated gateway config drops this rule the next time it is regenerated.`,
                      title: "Remove redirect rule?",
                    })) {
                      deleteMut.mutate(rule.id);
                    }
                  }}
                  size="sm"
                  variant="ghost"
                >
                  Remove
                </Button>
              </li>
            );
          })}
        </ul>
      )}

      {editorOpen ? (
        <RedirectEditor
          busy={busy}
          error={formError ? errorMessage(formError, "The rule was rejected") : null}
          onClose={() => setEditorOpen(false)}
          onSubmit={submitDraft}
          rule={editing}
        />
      ) : null}
    </section>
  );
}

interface RedirectEditorProps {
  rule: DomainRedirect | null;
  busy: boolean;
  error: string | null;
  onClose: () => void;
  onSubmit: (draft: Draft) => void;
}

/**
 * Add/edit form.
 *
 * The fields are seeded once from the rule being edited (the dialog is mounted
 * per open, keyed by the rule id), which keeps "what will be sent" visible in
 * the inputs instead of scattered across patch semantics.
 */
function RedirectEditor({ rule, busy, error, onClose, onSubmit }: RedirectEditorProps) {
  const [draft, setDraft] = useState<Draft>(() => draftFromRule(rule));

  const patch = (next: Partial<Draft>) => setDraft((current) => ({ ...current, ...next }));

  const sourcePathInvalid = draft.sourcePath.trim() !== "" && !draft.sourcePath.trim().startsWith("/");
  const targetPathInvalid = draft.targetPath.trim() !== "" && !draft.targetPath.trim().startsWith("/");
  const missingSource = draft.sourceDomain.trim() === "";
  const missingTarget = draft.targetDomain.trim() === "";
  const schemeUpgrade =
    !missingSource && draft.sourceDomain.trim().toLowerCase() === draft.targetDomain.trim().toLowerCase();
  const canSubmit = !missingSource && !missingTarget && !sourcePathInvalid && !targetPathInvalid;

  return (
    <Dialog
      closeAction={onClose}
      description={
        rule
          ? "Changes are validated against the application's other rules, including the reverse redirect that would turn both into a loop."
          : "Requests arriving at the source host and path are answered with the status code below."
      }
      open
      title={rule ? "Edit redirect" : "Add redirect"}
    >
      <form
        className="space-y-4"
        onSubmit={(event) => {
          event.preventDefault();
          if (canSubmit) onSubmit(draft);
        }}
      >
        {error ? (
          <Alert tone="error" title="The rule was rejected">
            {error}
          </Alert>
        ) : null}

        <div className="grid gap-4 sm:grid-cols-2">
          <Field
            error={missingSource ? "A source domain is required" : undefined}
            hint="A host bound to this application, e.g. www.example.com"
            id="redirect-source-domain"
            label="Source domain"
          >
            <Input
              autoComplete="off"
              id="redirect-source-domain"
              invalid={sourcePathInvalid}
              list="domain-redirect-hosts"
              onChange={(event) => patch({ sourceDomain: event.target.value })}
              placeholder="www.example.com"
              value={draft.sourceDomain}
            />
          </Field>

          <Field
            error={missingTarget ? "A target domain is required" : undefined}
            hint={schemeUpgrade ? "Same host: this is an HTTP → HTTPS upgrade rule" : undefined}
            id="redirect-target-domain"
            label="Target domain"
          >
            <Input
              autoComplete="off"
              id="redirect-target-domain"
              list="domain-redirect-hosts"
              onChange={(event) => patch({ targetDomain: event.target.value })}
              placeholder="example.com"
              value={draft.targetDomain}
            />
          </Field>

          <Field
            error={sourcePathInvalid ? "The path must start with /" : undefined}
            hint="Prefix match: /old also covers /old/anything"
            id="redirect-source-path"
            label="Source path"
          >
            <Input
              id="redirect-source-path"
              invalid={sourcePathInvalid}
              onChange={(event) => patch({ sourcePath: event.target.value })}
              placeholder="/"
              value={draft.sourcePath}
            />
          </Field>

          <Field
            error={targetPathInvalid ? "The path must start with /" : undefined}
            hint="Leave empty to keep the request path; / sends everything to the target root"
            id="redirect-target-path"
            label="Target path"
          >
            <Input
              id="redirect-target-path"
              invalid={targetPathInvalid}
              onChange={(event) => patch({ targetPath: event.target.value })}
              placeholder="(preserve request path)"
              value={draft.targetPath}
            />
          </Field>

          <Field id="redirect-status-code" label="Status code">
            <Select
              id="redirect-status-code"
              onChange={(event) => patch({ statusCode: Number(event.target.value) })}
              value={String(draft.statusCode)}
            >
              {DOMAIN_REDIRECT_STATUS_CODES.map((code) => (
                <option key={code} value={code}>
                  {statusLabel(code)}
                </option>
              ))}
            </Select>
          </Field>

          <div className="flex items-end pb-2">
            <Switch
              checked={draft.enabled}
              label="Enabled"
              onCheckedChange={(enabled) => patch({ enabled })}
            />
          </div>
        </div>

        <p className="text-xs leading-5 text-[var(--text-subtle)]">
          An HTTP → HTTPS upgrade is expressed by leaving both hosts identical: the gateway keeps
          the host and path and changes only the scheme.
        </p>

        <div className="flex justify-end gap-2 border-t border-[var(--line)] pt-4">
          <Button disabled={busy} onClick={onClose} variant="ghost">
            Cancel
          </Button>
          <Button disabled={!canSubmit} loading={busy} type="submit">
            {rule ? "Save rule" : "Add rule"}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
