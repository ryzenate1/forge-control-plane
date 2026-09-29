"use client";

import { useState, type ChangeEvent } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useRouter } from "next/navigation";
import { AlertTriangle, CheckCircle, Upload, XCircle } from "lucide-react";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageHeader,
  AdminPageLayout,
  AdminSection,
  AdminSelect,
  Btn,
  Card,
  Input,
} from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";
import { validateCompose, createComposeStack, type ComposeValidateResult } from "@/lib/api/compose";
import { fetchNodes } from "@/lib/api";
import { errorMessage } from "@/lib/utils";

const TEMPLATES = [
  {
    name: "nginx",
    label: "Nginx Web Server",
    yaml: `services:\n  web:\n    image: nginx:alpine\n    ports:\n      - "80:80"\n    restart: unless-stopped`,
  },
  {
    name: "node-postgres",
    label: "Node.js + PostgreSQL",
    yaml: `services:\n  app:\n    image: node:20-alpine\n    ports:\n      - "3000:3000"\n    depends_on:\n      - db\n  db:\n    image: postgres:16-alpine\n    environment:\n      POSTGRES_PASSWORD: changeme\n    volumes:\n      - pgdata:/var/lib/postgresql/data\nvolumes:\n  pgdata:`,
  },
  {
    name: "redis",
    label: "Redis Cache",
    yaml: `services:\n  cache:\n    image: redis:7-alpine\n    ports:\n      - "6379:6379"\n    restart: unless-stopped`,
  },
];

/**
 * Create a compose stack.
 *
 * The document is validated by the control plane (`POST /compose/validate`) and
 * Deploy unlocks only for the exact text that passed. That gate is real — the
 * server rejects an unvalidated document — so it is kept, but it no longer
 * behaves like a dead button: validation runs when the editor is left, the
 * result is stamped with the text it applies to, and the reason Deploy is
 * unavailable is written next to it.
 */

export default function NewComposeStackPage() {
  const { toast } = useToast();
  const router = useRouter();
  const [name, setName] = useState("");
  const [composeYaml, setComposeYaml] = useState("");
  const [validatedYaml, setValidatedYaml] = useState<string | null>(null);
  const [validateResult, setValidateResult] = useState<ComposeValidateResult | null>(null);
  const [validateError, setValidateError] = useState<string | null>(null);
  const [composeType, setComposeType] = useState("docker-compose");
  const [nodeId, setNodeId] = useState("");

  const nodesQuery = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const nodes = Array.isArray(nodesQuery.data) ? nodesQuery.data : [];

  const validateMutation = useMutation({
    mutationFn: (content: string) => validateCompose(content),
    onSuccess: (data, content) => {
      setValidateError(null);
      setValidateResult(data);
      // The verdict belongs to the exact bytes that were sent for it.
      setValidatedYaml(data.valid ? content : null);
      if (data.valid) toast({ tone: "success", title: "Document is valid" });
    },
    onError: (err) => {
      setValidateResult(null);
      setValidatedYaml(null);
      setValidateError(errorMessage(err, "The validation request did not complete."));
    },
  });

  const deployMutation = useMutation({
    mutationFn: () =>
      createComposeStack({
        name,
        composeYaml,
        composeType,
        sourceType: "raw",
        nodeId: nodeId || undefined,
      }),
    onSuccess: (stack) => {
      toast({ tone: "success", title: "Stack created", message: "The stack is being deployed." });
      router.push(`/admin/compose/${encodeURIComponent(stack.id)}`);
    },
    onError: (err: Error) =>
      toast({ tone: "error", title: "Stack not created", message: errorMessage(err) }),
  });

  function handleFileUpload(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (ev) => {
      const content = typeof ev.target?.result === "string" ? ev.target.result : "";
      if (!content) {
        toast({ tone: "error", title: "File was empty or could not be read" });
        return;
      }
      setComposeYaml(content);
      setValidatedYaml(null);
      setValidateResult(null);
      if (!name) setName(file.name.replace(/\.(yml|yaml)$/i, ""));
    };
    reader.onerror = () => toast({ tone: "error", title: "File could not be read" });
    reader.readAsText(file);
    e.target.value = "";
  }

  function selectTemplate(t: (typeof TEMPLATES)[0]) {
    setComposeYaml(t.yaml);
    setValidatedYaml(null);
    setValidateResult(null);
    if (!name) setName(t.name);
  }

  const trimmedName = name.trim();
  const isCurrent = validatedYaml !== null && validatedYaml === composeYaml;
  const canDeploy = Boolean(trimmedName) && Boolean(composeYaml.trim()) && isCurrent;

  const deployReason = !trimmedName
    ? "Give the stack a name to continue."
    : !composeYaml.trim()
      ? "Paste, upload or choose a compose document to continue."
      : validateError
        ? `Deploy stays closed until the server has validated this document — the last validation attempt failed: ${validateError}`
        : !isCurrent
          ? "Deploy unlocks once the control plane has validated the exact document in the editor. It validates when you leave the editor, or press Validate now."
          : undefined;

  const summary = validateResult?.summary;

  return (
    <AdminPageLayout className="max-w-4xl">
      <AdminPageHeader
        // Create route: not a registry row, so the step is named here.
        title="New Compose Stack"
        description="Paste, upload or choose a compose document. The control plane validates it before the stack can be created."
        backAction={() => router.push("/admin/compose")}
        backLabel="Compose Stacks"
        action={
          <Btn tone="ghost" size="sm" onClick={() => router.push("/admin/apps/new")}>
            Single-service app instead
          </Btn>
        }
      />
      <OfflineBanner onRetry={() => window.location.reload()} />

      <div className="grid gap-6 lg:grid-cols-2">
        <div className="space-y-5">
          <AdminSection title="Stack settings">
            <Card className="space-y-4 p-4">
              <Input label="Stack name" value={name} onChange={setName} placeholder="my-stack" required />

              <div>
                {nodesQuery.isPending ? (
                  <AdminLoadingState label="Loading nodes…" />
                ) : nodesQuery.isError ? (
                  <AdminErrorState
                    message={`Nodes could not be listed: ${errorMessage(nodesQuery.error)}`}
                    retry={() => void nodesQuery.refetch()}
                  />
                ) : (
                  <AdminSelect
                    label="Target node"
                    value={nodeId}
                    onChange={setNodeId}
                    options={[
                      { value: "", label: "Let Forge choose" },
                      ...nodes.map((n) => ({ value: n.id, label: `${n.name} (${n.id.slice(0, 8)})` })),
                    ]}
                  />
                )}
                <p className="ui-hint mt-1.5 block">
                  {nodesQuery.data
                    ? `${nodes.length.toLocaleString()} node${nodes.length === 1 ? "" : "s"} to choose from. Leaving it on “Let Forge choose” hands the decision to the placement policy.`
                    : "Node list not loaded yet — the automatic choice is still available."}
                </p>
              </div>

              <AdminSelect
                label="Compose type"
                value={composeType}
                onChange={setComposeType}
                options={[
                  { value: "docker-compose", label: "Docker Compose" },
                  { value: "stack", label: "Docker Stack" },
                ]}
              />
            </Card>
          </AdminSection>

          <AdminSection title="Compose document">
            <Card className="space-y-3 p-4">
              <div className="flex flex-wrap gap-2">
                {TEMPLATES.map((t) => (
                  <Btn key={t.name} size="sm" tone="ghost" onClick={() => selectTemplate(t)}>
                    {t.label}
                  </Btn>
                ))}
                <label className="flex cursor-pointer items-center gap-1 rounded-lg border border-dashed border-line-strong bg-overlay-subtle px-3 py-1.5 text-xs text-text-subtle transition-colors hover:border-[var(--focus)] hover:text-text">
                  <Upload aria-hidden="true" className="h-3 w-3" /> Upload
                  <input
                    accept=".yml,.yaml"
                    className="hidden"
                    onChange={handleFileUpload}
                    type="file"
                  />
                </label>
              </div>

              {/* `Textarea` from admin-ui has no onBlur; validation is bound to blur,
                  so the editor is a labelled raw control with the shared field classes. */}
              <label className="block">
                <span className="ui-label mb-1.5">Compose YAML</span>
                <textarea
                  className="ui-input min-h-0 w-full font-mono"
                  onBlur={() => {
                    if (composeYaml.trim() && validatedYaml !== composeYaml) {
                      validateMutation.mutate(composeYaml);
                    }
                  }}
                  onChange={(e) => {
                    setComposeYaml(e.target.value);
                    setValidateResult(null);
                    setValidatedYaml(null);
                  }}
                  placeholder={'services:\n  app:\n    image: nginx:latest\n    ports:\n      - "8080:80"'}
                  rows={18}
                  value={composeYaml}
                />
              </label>

              <div className="flex flex-wrap items-center gap-3">
                <Btn
                  tone="ghost"
                  onClick={() => validateMutation.mutate(composeYaml)}
                  disabled={!composeYaml.trim() || validateMutation.isPending}
                >
                  {validateMutation.isPending ? "Validating…" : isCurrent ? "Re-validate" : "Validate"}
                </Btn>
                <Btn tone="primary" onClick={() => deployMutation.mutate()} disabled={!canDeploy || deployMutation.isPending}>
                  {deployMutation.isPending ? "Creating…" : "Create stack"}
                </Btn>
              </div>

              {/* The disabled primary button used to be a mystery for the whole time
                  you typed. The gate is now stated. */}
              {!canDeploy && deployReason ? (
                <p className="ui-hint block">{deployReason}</p>
              ) : null}

              {validateMutation.isPending ? (
                <p className="text-meta text-text-subtle" role="status">
                  Validating this document with the control plane…
                </p>
              ) : validateError ? (
                <AdminErrorState
                  message={`Validation did not run: ${validateError}`}
                  retry={() => validateMutation.mutate(composeYaml)}
                />
              ) : validateResult ? (
                <div
                  className={`ui-alert ${validateResult.valid ? "ui-alert-success" : "ui-alert-danger"}`}
                  role="status"
                >
                  <span className="flex items-center gap-2 font-medium">
                    {validateResult.valid ? (
                      <CheckCircle aria-hidden="true" className="h-4 w-4" />
                    ) : (
                      <XCircle aria-hidden="true" className="h-4 w-4" />
                    )}
                    {validateResult.valid ? "Valid" : "Not valid"}
                  </span>
                  {Array.isArray(validateResult.errors) && validateResult.errors.length > 0 ? (
                    <ul className="mt-1 space-y-1">
                      {validateResult.errors.map((err, i) => (
                        <li key={`${err.field}-${i}`}>
                          <span className="font-semibold">{err.field}</span>: {err.message}
                        </li>
                      ))}
                    </ul>
                  ) : null}
                  {Array.isArray(validateResult.warnings) && validateResult.warnings.length > 0 ? (
                    <ul className="mt-1 space-y-1">
                      {validateResult.warnings.map((w, i) => (
                        <li className="flex items-start gap-1.5" key={`${w.field}-${i}`}>
                          <AlertTriangle aria-hidden="true" className="mt-0.5 h-3 w-3 shrink-0" />
                          <span>
                            <span className="font-semibold">{w.field}</span>: {w.message}
                          </span>
                        </li>
                      ))}
                    </ul>
                  ) : null}
                  {summary?.services ? (
                    <div className="mt-2 space-y-0.5">
                      <p>
                        {summary.services.length.toLocaleString()} service
                        {summary.services.length === 1 ? "" : "s"},{" "}
                        {Array.isArray(summary.networks)
                          ? summary.networks.length.toLocaleString()
                          : "no count reported for"}{" "}
                        network
                        {Array.isArray(summary.networks) && summary.networks.length === 1 ? "" : "s"}
                        {", "}
                        {Array.isArray(summary.volumes)
                          ? summary.volumes.length.toLocaleString()
                          : "no count reported for"}{" "}
                        volume
                        {Array.isArray(summary.volumes) && summary.volumes.length === 1 ? "" : "s"}
                      </p>
                      <ul className="space-y-0.5">
                        {summary.services.map((s) => (
                          <li key={s.name}>
                            {s.name}
                            {s.image ? ` (${s.image})` : " (image not reported)"}
                          </li>
                        ))}
                      </ul>
                    </div>
                  ) : null}
                </div>
              ) : composeYaml.trim() ? (
                <p className="text-meta text-text-subtle">
                  Not validated yet. Validation runs when you leave the editor.
                </p>
              ) : null}
            </Card>
          </AdminSection>
        </div>

        <AdminSection title="Preview">
          <Card>
            <pre className="max-h-125 overflow-auto p-4 font-mono text-meta text-text-subtle">
              {composeYaml || "Nothing to preview yet — paste, upload or choose a compose document."}
            </pre>
          </Card>
        </AdminSection>
      </div>
    </AdminPageLayout>
  );
}
