"use client";

import { useState } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { useRouter } from "next/navigation";
import { Upload, CheckCircle, XCircle, AlertTriangle, Server } from "lucide-react";
import { AdminFormSection, AdminPageHeader, AdminPageLayout, Btn, Card, CardHeader } from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";
import { validateCompose, createComposeStack, type ComposeValidateResult } from "@/lib/api/compose";
import { fetchNodes } from "@/lib/api";

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

export default function NewComposeStackPage() {
  const { toast } = useToast();
  const router = useRouter();
  const [name, setName] = useState("");
  const [composeYaml, setComposeYaml] = useState("");
  const [validateResult, setValidateResult] = useState<ComposeValidateResult | null>(null);
  const [composeType, setComposeType] = useState("docker-compose");
  const [nodeId, setNodeId] = useState("");
  const { data: nodes = [] } = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });

  const validateMutation = useMutation({
    mutationFn: (content: string) => validateCompose(content),
    onSuccess: (data) => {
      setValidateResult(data);
      if (data.valid) {
        toast({ tone: "success", title: "Compose file is valid" });
      }
    },
    onError: () => toast({ tone: "error", title: "Validation failed" }),
  });

  const deployMutation = useMutation({
    mutationFn: () => createComposeStack({ name, composeYaml, composeType, sourceType: "raw", nodeId: nodeId || undefined }),
    onSuccess: (stack) => {
      toast({ tone: "success", title: "Stack deployed" });
      router.push(`/admin/compose/${(stack as { id: string }).id}`);
    },
    onError: (err: Error) => toast({ tone: "error", title: err.message }),
  });

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (ev) => {
      const content = ev.target?.result as string;
      setComposeYaml(content);
      if (!name) {
        setName(file.name.replace(/\.(yml|yaml)$/i, ""));
      }
    };
    reader.readAsText(file);
  };

  const selectTemplate = (t: (typeof TEMPLATES)[0]) => {
    setComposeYaml(t.yaml);
    if (!name) setName(t.name);
  };

  const canDeploy = name.trim() && composeYaml.trim() && validateResult?.valid;

  return (
    <AdminPageLayout className="max-w-4xl">
      <AdminPageHeader
        title="New Compose Stack"
        description="Paste, upload, or select a template to deploy a Docker Compose stack."
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
        <Card>
          <CardHeader title="Configuration" />
          <div className="p-4 space-y-4">
            <AdminFormSection title="Stack">
              <div>
                <label className="mb-1 block text-sm text-slate-300">Stack Name</label>
                <input
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="my-stack"
                  className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm text-slate-200 placeholder:text-slate-400 focus:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]"
                />
              </div>
              <div>
                <label className="mb-1 block text-sm text-slate-300">Target Node</label>
                <select
                  value={nodeId}
                  onChange={(e) => setNodeId(e.target.value)}
                  className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm text-slate-200 focus:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]"
                >
                  <option value="">Auto-select</option>
                  {(Array.isArray(nodes) ? nodes : []).map((n: { id: string; name: string }) => (
                    <option key={n.id} value={n.id}>{n.name} ({n.id.slice(0, 8)})</option>
                  ))}
                </select>
                <p className="mt-1 text-xs text-slate-500">Choose a node for scheduling; leave auto for scheduler.</p>
              </div>
              <div>
                <label className="mb-1 block text-sm text-slate-300">Compose Type</label>
                <select
                  value={composeType}
                  onChange={(e) => setComposeType(e.target.value)}
                  className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm text-slate-200 focus:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)]"
                >
                  <option value="docker-compose">Docker Compose</option>
                  <option value="stack">Docker Stack</option>
                </select>
              </div>
              <div className="rounded-lg border border-sky-500/20 bg-sky-500/10 px-3 py-2 text-xs text-sky-200">
                <Server size={12} className="inline mr-1" /> Health & status use <code className="rounded bg-white/10 px-1 py-0.5 font-mono text-[11px]">composeStatusTone</code> tones (green running, yellow degraded, red failed) — same Pill system as Apps.
              </div>
            </AdminFormSection>

            <AdminFormSection title="Compose YAML">
              <div className="flex flex-wrap gap-2 mb-2">
                {TEMPLATES.map((t) => (
                  <Btn key={t.name} size="sm" tone="ghost" onClick={() => selectTemplate(t)}>
                    {t.label}
                  </Btn>
                ))}
                <label className="flex cursor-pointer items-center gap-1 rounded-lg border border-dashed border-[var(--line)] bg-[var(--surface-input)] px-3 py-1.5 text-xs text-slate-400 hover:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] hover:text-[var(--brand)] transition-colors">
                  <Upload className="h-3 w-3" /> Upload
                  <input type="file" accept=".yml,.yaml" onChange={handleFileUpload} className="hidden" />
                </label>
              </div>
              <textarea
                value={composeYaml}
                onChange={(e) => {
                  setComposeYaml(e.target.value);
                  setValidateResult(null);
                }}
                placeholder={`services:\n  app:\n    image: nginx:latest\n    ports:\n      - "8080:80"`}
                rows={18}
                className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface-input)] px-3 py-2 text-sm font-mono text-slate-200 placeholder:text-slate-400 focus:border-[color-mix(in_srgb,var(--brand)_70%,transparent)] focus:outline-none focus:ring-2 focus:ring-[color-mix(in_srgb,var(--brand)_15%,transparent)] resize-y"
              />
            </AdminFormSection>

            <div className="flex gap-3">
              <Btn
                tone="ghost"
                onClick={() => validateMutation.mutate(composeYaml)}
                disabled={!composeYaml.trim() || validateMutation.isPending}
              >
                {validateMutation.isPending ? "Validating..." : "Validate"}
              </Btn>
              <Btn
                tone="primary"
                onClick={() => deployMutation.mutate()}
                disabled={!canDeploy || deployMutation.isPending}
              >
                {deployMutation.isPending ? "Deploying..." : "Deploy"}
              </Btn>
            </div>

            {validateResult && (
              <div className={`rounded-lg border p-4 ${validateResult.valid ? "border-emerald-500/50 bg-emerald-500/10" : "border-red-500/50 bg-red-500/10"}`}>
                <div className="flex items-center gap-2">
                  {validateResult.valid ? (
                    <CheckCircle className="h-5 w-5 text-emerald-400" />
                  ) : (
                    <XCircle className="h-5 w-5 text-red-400" />
                  )}
                  <span className={`text-sm font-medium ${validateResult.valid ? "text-emerald-400" : "text-red-400"}`}>
                    {validateResult.valid ? "Valid" : "Invalid"}
                  </span>
                </div>
                {Array.isArray(validateResult.errors) && validateResult.errors.map((err, i) => (
                  <div key={i} className="mt-2 text-xs text-red-400">
                    <strong>{err.field}:</strong> {err.message}
                  </div>
                ))}
                {Array.isArray(validateResult.warnings) && validateResult.warnings.map((w, i) => (
                  <div key={i} className="mt-1 flex items-center gap-1 text-xs text-amber-400">
                    <AlertTriangle className="h-3 w-3" />
                    <strong>{w.field}:</strong> {w.message}
                  </div>
                ))}
                {validateResult.summary?.services && (
                  <div className="mt-3 text-xs text-slate-400">
                    {validateResult.summary.services.length} service(s), {validateResult.summary.networks?.length || 0} network(s), {validateResult.summary.volumes?.length || 0} volume(s)
                    <ul className="mt-1 space-y-0.5">
                      {validateResult.summary.services.map((s) => (
                        <li key={s.name} className="text-slate-400">
                          {s.name} {s.image ? `(${s.image})` : ""}
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>
            )}
          </div>
        </Card>

        <Card>
          <CardHeader title="Preview" />
          <pre className="rounded-lg bg-[var(--surface-input)] p-4 text-xs font-mono text-slate-300 overflow-auto max-h-[500px]">
            {composeYaml || "Paste or select a template to preview..."}
          </pre>
        </Card>
      </div>
    </AdminPageLayout>
  );
}
