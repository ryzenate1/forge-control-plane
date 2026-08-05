"use client";
import { queryKeys } from "@/lib/api/query-keys";

import { useState, useEffect, useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  Box, Cloud, Container, GitBranch, Globe, Layers,
  Play, Zap, CheckCircle2, AlertCircle, LoaderCircle, Cpu, ShieldCheck,
} from "lucide-react";
import {
  createApp, fetchAppTemplates, typeLabel,
  type AppType, type AppPort, type AppVolume, type CreateAppInput,
} from "@/lib/api/apps";
import { fetchNodes, fetchRegions } from "@/lib/api";
import { validateCompose as validateComposeServer } from "@/lib/api/compose";
import { AdminFormSection, AdminPageLayout, Btn, Card, CardHeader, Input, Modal, ModalFooter, Pill, SectionHeader, cn } from "@/components/admin/admin-ui";
import { EnvVarEditor, PortMapper, VolumeEditor } from "@/components/admin/AdminAppsShared";
import { adminPageGuides } from "@/components/admin/admin-page-guides";
import { useToast } from "@/components/ui/toast";
import { Switch } from "@/components/ui/primitives";

const SOURCE_TYPES: { id: AppType; label: string; icon: typeof Box; desc: string }[] = [
  { id: "image", label: "Docker Image", icon: Box, desc: "Deploy from any Docker registry image" },
  { id: "git", label: "Git Repository", icon: GitBranch, desc: "Build and deploy from a Git repository" },
  { id: "compose", label: "Docker Compose", icon: Container, desc: "Deploy a multi-service Docker Compose stack" },
];

const STEPS = ["source", "template", "configure", "review"] as const;

type GitProviderType = "github" | "gitlab" | "bitbucket" | "gitea" | "other";

const GIT_URL_PATTERNS: Record<Exclude<GitProviderType, "other">, RegExp> = {
  github: /^(?:https?:\/\/|git@)(?:www\.)?github\.com[:\/][\w.-]+\/[\w.-]+(?:\.git)?\/?$/i,
  gitlab: /^(?:https?:\/\/|git@)(?:www\.)?gitlab\.com[:\/][\w.-]+\/[\w.-]+(?:\.git)?\/?$/i,
  bitbucket: /^(?:https?:\/\/|git@)(?:www\.)?bitbucket\.org[:\/][\w.-]+\/[\w.-]+(?:\.git)?\/?$/i,
  gitea: /^(?:https?:\/\/|git@)(?:www\.)?gitea\.com[:\/][\w.-]+\/[\w.-]+(?:\.git)?\/?$/i,
};

const VALID_GIT_URL = /^(?:https?:\/\/|git@).+\/.+\/.+$/i;

function detectGitProvider(url: string): GitProviderType {
  if (GIT_URL_PATTERNS.github.test(url)) return "github";
  if (GIT_URL_PATTERNS.gitlab.test(url)) return "gitlab";
  if (GIT_URL_PATTERNS.bitbucket.test(url)) return "bitbucket";
  if (GIT_URL_PATTERNS.gitea.test(url)) return "gitea";
  return "other";
}

function parseGitUrl(url: string): { owner: string; repo: string } | null {
  try {
    const normalized = url.replace(/^git@([^:]+):/, "https://$1/");
    const u = new URL(normalized);
    const parts = u.pathname.replace(/\.git\/?$/, "").split("/").filter(Boolean);
    if (parts.length >= 2) return { owner: parts[0], repo: parts.slice(1).join("/") };
    return null;
  } catch {
    return null;
  }
}

type GitUrlStatus = "idle" | "valid" | "invalid";

function useDebounce<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

type Step = (typeof STEPS)[number];

export default function CreateAppPage() {
  const router = useRouter();
  const qc = useQueryClient();
  const { toast } = useToast();
  const [step, setStep] = useState<Step>("source");
  const [sourceType, setSourceType] = useState<AppType>("image");
  const [name, setName] = useState("");
  const [image, setImage] = useState("");
  const [registryUrl, setRegistryUrl] = useState("");
  const [registryUser, setRegistryUser] = useState("");
  const [registryPass, setRegistryPass] = useState("");
  const [gitUrl, setGitUrl] = useState("");
  const [gitBranch, setGitBranch] = useState("main");
  const [gitProvider, setGitProvider] = useState<GitProviderType>("github");
  const [gitUrlStatus, setGitUrlStatus] = useState<GitUrlStatus>("idle");
  const [gitUrlMessage, setGitUrlMessage] = useState("");
  const [composeContent, setComposeContent] = useState("");
  const [composeFile, setComposeFile] = useState<File | null>(null);
  const [selectedTemplate, setSelectedTemplate] = useState<string>("");
  const [nodeId, setNodeId] = useState("");
  const [regionId, setRegionId] = useState("");
  const [cpu, setCpu] = useState("1.0");
  const [memory, setMemory] = useState("1024");
  const [disk, setDisk] = useState("10240");
  const [ports, setPorts] = useState<AppPort[]>([]);
  const [envVars, setEnvVars] = useState<Record<string, string>>({});
  const [volumes, setVolumes] = useState<AppVolume[]>([]);
  const [domains, setDomains] = useState("");
  const [enableTls, setEnableTls] = useState(false);
  const [autoDeploy, setAutoDeploy] = useState(false);
  const [composeValidation, setComposeValidation] = useState<{ valid: boolean; errors: string[] } | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [fieldsDirty, setFieldsDirty] = useState(false);
  const [templateConfirm, setTemplateConfirm] = useState<string | null>(null);

  const debouncedGitUrl = useDebounce(gitUrl, 400);
  const debouncedCompose = useDebounce(composeContent, 600);
  const [composeValidatorError, setComposeValidatorError] = useState("");

  const { data: templates = [] } = useQuery({
    queryKey: [...queryKeys.apps.all, "templates"],
    queryFn: fetchAppTemplates,
  });
  const { data: nodes = [] } = useQuery({ queryKey: queryKeys.nodes.lists(), queryFn: fetchNodes });
  const { data: regions = [] } = useQuery({ queryKey: ["regions"], queryFn: fetchRegions });

  const recommendedNode = useMemo(() => {
    const list = Array.isArray(nodes) ? nodes as Array<{ id: string; name: string; heartbeatState?: string; memoryMb?: number; diskMb?: number }> : [];
    if (list.length === 0) return null;
    const scored = list.map((n) => ({ node: n, score: (n.heartbeatState === "healthy" ? 50 : 0) + (n.memoryMb ?? 0) / 1024 }));
    scored.sort((a, b) => b.score - a.score);
    return scored[0].node;
  }, [nodes]);
  const pickedNode = useMemo(() => (Array.isArray(nodes) ? (nodes as Array<{ id: string; name: string; heartbeatState?: string; memoryMb?: number; diskMb?: number; regionId?: string; fqdn?: string }>) : []).find((n) => n.id === nodeId) ?? null, [nodes, nodeId]);

  useEffect(() => {
    if (composeFile) {
      const reader = new FileReader();
      reader.onload = (e) => {
        setComposeContent(e.target?.result as string ?? "");
      };
      reader.readAsText(composeFile);
    }
  }, [composeFile]);

  const composeCheckQ = useQuery({
    queryKey: ["compose-validate", debouncedCompose],
    queryFn: () => validateComposeServer(debouncedCompose),
    enabled: sourceType === "compose" && debouncedCompose.trim().length > 50,
    retry: false,
    staleTime: 30_000,
  });

  useEffect(() => {
    if (sourceType !== "compose" || debouncedCompose.trim().length <= 50) {
      setComposeValidation(null);
      setComposeValidatorError("");
      return;
    }
    if (composeCheckQ.data) {
      const errs = [
        ...(composeCheckQ.data.errors ?? []).map((e) => (e.field ? `${e.field}: ${e.message}` : e.message)),
        ...(composeCheckQ.data.warnings ?? []).map((w) => `Warning — ${w.field ? `${w.field}: ` : ""}${w.message}`),
      ];
      setComposeValidation({ valid: composeCheckQ.data.valid, errors: errs });
      setComposeValidatorError("");
    } else if (composeCheckQ.isError) {
      // Validator unreachable: say so and don't block submit on an unknown.
      setComposeValidation(null);
      setComposeValidatorError(`Server validator unreachable (${(composeCheckQ.error as Error).message}) — content will be validated when the stack is created.`);
    }
  }, [sourceType, debouncedCompose, composeCheckQ.data, composeCheckQ.isError, composeCheckQ.error]);

  useEffect(() => {
    if (debouncedGitUrl && sourceType === "git") {
      doValidateGitUrl(debouncedGitUrl);
    } else {
      setGitUrlStatus("idle");
      setGitUrlMessage("");
    }
  }, [debouncedGitUrl, sourceType]);

  const doValidateGitUrl = (url: string) => {
    const trimmed = url.trim();
    if (!trimmed) {
      setGitUrlStatus("idle");
      setGitUrlMessage("");
      return;
    }
    if (!VALID_GIT_URL.test(trimmed)) {
      setGitUrlStatus("invalid");
      setGitUrlMessage("Enter an https:// or git@ repository URL");
      return;
    }
    const parsed = parseGitUrl(trimmed);
    if (!parsed) {
      setGitUrlStatus("invalid");
      setGitUrlMessage("Could not parse repository owner and name");
      return;
    }
    const detected = detectGitProvider(trimmed);
    setGitProvider(detected);
    setGitUrlStatus("valid");
    setGitUrlMessage("");
  };

  const applyTemplate = (templateId: string) => {
    const tpl = (Array.isArray(templates) ? templates : []).find((t) => t.id === templateId);
    if (!tpl) return;
    setSelectedTemplate(templateId);
    setSourceType(tpl.type);
    if (tpl.image) setImage(tpl.image);
    if (tpl.type === "git") {
      setGitUrl(tpl.gitUrl ?? "");
      setGitBranch(tpl.gitBranch ?? "main");
      setGitUrlStatus("idle");
      setGitUrlMessage("");
    }
    if (tpl.composeContent) setComposeContent(tpl.composeContent);
    setPorts(tpl.defaultPorts ?? []);
    setEnvVars(tpl.defaultEnvVars ?? {});
    setCpu(tpl.defaultResources.cpu ?? "1.0");
    setMemory(tpl.defaultResources.memory ?? "1024");
    setDisk(tpl.defaultResources.disk ?? "10240");
    setFieldsDirty(false);
    setTemplateConfirm(null);
  };

  const handleTemplateClick = (templateId: string) => {
    if (fieldsDirty && templateId !== selectedTemplate) {
      setTemplateConfirm(templateId);
      return;
    }
    if (templateId === selectedTemplate) {
      setSelectedTemplate("");
      return;
    }
    applyTemplate(templateId);
  };

  const validateAll = (): Record<string, string> => {
    const errors: Record<string, string> = {};
    if (!name.trim()) errors.name = "App name is required.";
    else if (name.trim().length < 2) errors.name = "Name must be at least 2 characters.";
    else if (!/^[a-z0-9_-]+$/i.test(name.trim())) errors.name = "Only letters, numbers, hyphens, and underscores allowed.";
    if (sourceType === "image") {
      if (!image.trim()) errors.image = "Docker image is required.";
      else if (!/^[a-z0-9._/-]+(:[a-zA-Z0-9._-]+)?(@sha256:[a-f0-9]{64})?$/i.test(image.trim())) errors.image = "Invalid image format. Expected format: [registry/]name[:tag]";
    }
    if (sourceType === "git") {
      if (!gitUrl.trim()) errors.gitUrl = "Git repository URL is required.";
      else if (!VALID_GIT_URL.test(gitUrl.trim())) errors.gitUrl = "Invalid repository URL format.";
    }
    if (sourceType === "compose") {
      if (!composeContent.trim()) errors.composeContent = "Compose file content is required.";
      else if (composeValidation && !composeValidation.valid) errors.composeContent = `Compose validation: ${composeValidation.errors.join("; ")}`;
    }
    if (cpu && (isNaN(Number(cpu)) || Number(cpu) <= 0)) errors.cpu = "CPU must be a positive number.";
    if (memory && (isNaN(Number(memory)) || Number(memory) <= 0)) errors.memory = "Memory must be a positive number.";
    if (disk && (isNaN(Number(disk)) || Number(disk) <= 0)) errors.disk = "Disk must be a positive number.";
    if (domains.trim()) {
      const parts = domains.split(",").map((d) => d.trim()).filter(Boolean);
      const domainRegex = /^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$/;
      const invalid = parts.filter((d) => !domainRegex.test(d));
      if (invalid.length > 0) errors.domains = `Invalid domain(s): ${invalid.join(", ")}`;
    }
    return errors;
  };

  const validationErrors = validateAll();
  const validationError = Object.keys(validationErrors).length > 0
    ? Object.values(validationErrors).join(". ")
    : null;

  const createMut = useMutation({
    mutationFn: () => {
      const errs = validateAll();
      setFieldErrors(errs);
      if (Object.keys(errs).length > 0) throw new Error("Please fix the validation errors above.");
      const input: CreateAppInput = {
        name: name.trim(),
        type: sourceType,
        nodeId: nodeId || undefined,
        regionId: regionId || undefined,
        image: sourceType === "image" ? image.trim() : undefined,
        registryUrl: registryUrl || undefined,
        registryUsername: registryUser || undefined,
        registryPassword: registryPass || undefined,
        gitUrl: sourceType === "git" ? gitUrl.trim() : undefined,
        gitBranch: sourceType === "git" ? gitBranch : undefined,
        gitProvider: sourceType === "git" ? gitProvider : undefined,
        composeContent: sourceType === "compose" ? composeContent : undefined,
        templateId: selectedTemplate || undefined,
        cpuLimit: cpu,
        memoryLimit: memory,
        diskLimit: disk,
        ports,
        envVars,
        volumes,
        domains: domains.split(",").map((d) => d.trim()).filter(Boolean),
        enableTls,
        autoDeploy: sourceType === "git" ? autoDeploy : undefined,
      };
      return createApp(input);
    },
    onSuccess: (result) => {
      void qc.invalidateQueries({ queryKey: queryKeys.apps.lists() });
      router.push(`/admin/apps/${result.id}`);
    },
    onError: (err) => toast({ tone: "error", title: "Failed to create app", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const stepIndex = STEPS.indexOf(step);

  return (
    <AdminPageLayout className="mx-auto max-w-4xl px-1 sm:px-0">
      <SectionHeader
        title="Create Application"
        sub="Deploy a Docker image, Git repository, or Compose stack"
        info={adminPageGuides.appCreation}
        backAction={() => {
          const prev = STEPS[STEPS.indexOf(step) - 1];
          if (prev) { setStep(prev); setFieldErrors({}); } else router.push("/admin/apps");
        }}
        backLabel="Apps"
      />

      <div className="flex gap-1 border-b border-line pb-4 sm:gap-2">
        {STEPS.map((s, i) => (
          <div key={s} className="flex items-center gap-1.5 sm:gap-2">
            <div className={cn(
              "flex h-7 w-7 sm:h-8 sm:w-8 items-center justify-center rounded-full text-[10px] sm:text-xs font-bold shrink-0",
              step === s
                ? "bg-[var(--brand)] text-white"
                : i < stepIndex
                  ? "bg-ok-subtle text-ok"
                  : "bg-overlay-subtle text-text-muted",
            )}>
              {i < stepIndex ? <CheckCircle2 size={12} /> : i + 1}
            </div>
            <span className={cn(
              "text-[11px] sm:text-xs font-medium capitalize truncate",
              step === s ? "text-text" : "text-text-muted",
            )}>
              {s}
            </span>
            {i < STEPS.length - 1 && <div className="ml-1 hidden w-6 border-t border-line sm:block sm:w-10" />}
          </div>
        ))}
      </div>

      {step === "source" && (
        <div className="space-y-6">
          <Card>
            <CardHeader title="Select Source Type" icon={Layers} />
            <div className="grid gap-4 p-4 sm:grid-cols-3">
              {SOURCE_TYPES.filter((s) => s.id !== "game_server").map(({ id, label, icon: Icon, desc }) => (
                <button
                  key={id}
                  type="button"
                  className={cn(
                    "flex flex-col items-center gap-3 rounded-xl border p-5 sm:p-6 text-center transition-all duration-200",
                    sourceType === id
                      ? "border-[var(--brand)] bg-[color-mix(in_srgb,var(--brand)_5%,transparent)] shadow-sm shadow-[color-mix(in_srgb,var(--brand)_10%,transparent)]"
                      : "border-line bg-overlay-subtle hover:border-line-strong hover:bg-overlay-subtle",
                  )}
                  onClick={() => { setSourceType(id); setFieldsDirty(false); setTemplateConfirm(null); }}
                >
                  <Icon size={28} className={cn(
                    "transition-colors duration-200",
                    sourceType === id ? "text-[var(--brand)]" : "text-text-muted",
                  )} />
                  <div>
                    <p className="font-semibold text-text text-sm sm:text-base">{label}</p>
                    <p className="mt-1 text-xs text-text-muted leading-relaxed">{desc}</p>
                  </div>
                </button>
              ))}
            </div>
          </Card>

          <div className="flex justify-between gap-3">
            <Btn tone="ghost" onClick={() => router.push("/admin/apps")}>Cancel</Btn>
            <div className="flex gap-2">
              <Btn tone="ghost" onClick={() => { setStep("configure"); setFieldErrors({}); }}>Skip Templates</Btn>
              <Btn tone="primary" onClick={() => { setStep("template"); setFieldErrors({}); }}>Next: Templates</Btn>
            </div>
          </div>
        </div>
      )}

      {step === "template" && (
        <div className="space-y-6">
          <Card>
            <CardHeader title="Quick Templates (Optional)" icon={Zap} />
            <div className="p-4">
              {!Array.isArray(templates) || templates.length === 0 ? (
                <p className="text-sm text-text-muted">No templates available.</p>
              ) : (
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {templates.map((tpl) => (
                    <button
                      key={tpl.id}
                      type="button"
                      className={cn(
                        "rounded-xl border p-4 text-left transition-all duration-200",
                        selectedTemplate === tpl.id
                          ? "border-[var(--brand)] bg-[color-mix(in_srgb,var(--brand)_5%,transparent)] shadow-sm shadow-[color-mix(in_srgb,var(--brand)_10%,transparent)]"
                          : "border-line bg-overlay-subtle hover:border-line-strong hover:bg-overlay-subtle",
                      )}
                      onClick={() => handleTemplateClick(tpl.id)}
                    >
                      <Pill tone="neutral">{typeLabel(tpl.type)}</Pill>
                      <p className="mt-2 font-semibold text-text text-sm">{tpl.name}</p>
                      <p className="mt-1 text-xs text-text-muted line-clamp-2">{tpl.description}</p>
                    </button>
                  ))}
                </div>
              )}
              {templateConfirm && (
                <Modal title="Overwrite current configuration?" description="Selecting a new template will overwrite your current configuration values." onClose={() => setTemplateConfirm(null)}>
                  <ModalFooter onCancel={() => setTemplateConfirm(null)} onConfirm={() => applyTemplate(templateConfirm)} confirmLabel="Apply Template" destructive />
                </Modal>
              )}
            </div>
          </Card>

          <div className="flex justify-between gap-3">
            <Btn tone="ghost" onClick={() => router.push("/admin/apps")}>Cancel</Btn>
            <div className="flex gap-2">
              <Btn tone="ghost" onClick={() => { setStep("source"); setTemplateConfirm(null); }}>Back</Btn>
              <Btn tone="primary" onClick={() => { setStep("configure"); setFieldErrors({}); }}>Next: Configure</Btn>
            </div>
          </div>
        </div>
      )}

      {step === "configure" && (
        <div className="space-y-6">
          <Card>
            <CardHeader title="Basic Information" icon={Layers} />
            <div className="p-4">
              <AdminFormSection title="Basic Information">
                <div className="grid gap-4 sm:grid-cols-2">
                  <div>
                    <Input
                      label="App Name"
                      value={name}
                      onChange={(v) => { setName(v); setFieldErrors((e) => ({ ...e, name: "" })); setFieldsDirty(true); }}
                      placeholder="my-app"
                      required
                    />
                    {fieldErrors.name && (
                      <p className="mt-1 text-xs text-danger">{fieldErrors.name}</p>
                    )}
                  </div>
                  <div>
                    <label className="mb-1.5 block text-sm font-medium text-text-subtle">Type</label>
                    <div className="flex h-9 items-center rounded-lg border border-line bg-overlay-subtle px-3 text-sm text-text-subtle">
                      <Pill tone="neutral">{typeLabel(sourceType)}</Pill>
                    </div>
                  </div>
                </div>
              </AdminFormSection>
            </div>
          </Card>

          {sourceType === "image" && (
            <Card>
              <CardHeader title="Docker Image" icon={Box} />
              <div className="grid gap-4 p-4 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <Input
                    label="Image Name"
                    value={image}
                    onChange={(v) => { setImage(v); setFieldErrors((e) => ({ ...e, image: "" })); setFieldsDirty(true); }}
                    placeholder="nginx:latest"
                    required
                  />
                  {fieldErrors.image && (
                    <p className="mt-1 text-xs text-danger">{fieldErrors.image}</p>
                  )}
                </div>
                <Input label="Registry URL" value={registryUrl} onChange={setRegistryUrl} placeholder="registry.example.com" />
                <Input label="Username" value={registryUser} onChange={setRegistryUser} placeholder="(optional)" />
                <Input label="Password" value={registryPass} onChange={setRegistryPass} type="password" placeholder="(optional)" autoComplete="off" />
              </div>
            </Card>
          )}

          {sourceType === "git" && (
            <Card>
              <CardHeader title="Git Repository" icon={GitBranch} />
              <div className="grid gap-4 p-4 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <label className="mb-1.5 block text-sm font-medium text-text-subtle">
                    Repository URL
                    <span className="ml-1 text-danger">*</span>
                  </label>
                  <div className="relative">
                    <input
                      className={cn(
                        "block h-10 w-full rounded-lg border px-3.5 pr-10 text-sm outline-none transition-all duration-200",
                        "bg-[var(--surface-input)] text-text shadow-inner  placeholder:text-text-muted",
                        gitUrlStatus === "invalid"
                          ? "border-danger focus:border-danger focus:ring-2 focus:ring-danger/20"
                          : gitUrlStatus === "valid"
                            ? "border-ok focus:border-ok focus:ring-2 focus:ring-ok/20"
                            : "border-line hover:border-line focus:border-danger focus:ring-2 focus:ring-danger/20",
                      )}
                      value={gitUrl}
                      onChange={(e) => { setGitUrl(e.target.value); setFieldErrors((ef) => ({ ...ef, gitUrl: "" })); setFieldsDirty(true); }}
                      placeholder="https://github.com/user/repo.git"
                    />
                    <span className="absolute right-3 top-1/2 -translate-y-1/2">
                      {gitUrlStatus === "valid" && (
                        <CheckCircle2 size={16} className="text-ok" />
                      )}
                      {gitUrlStatus === "invalid" && (
                        <AlertCircle size={16} className="text-danger" />
                      )}
                    </span>
                  </div>
                  {gitUrlMessage && (
                    <p className={cn(
                      "mt-1 text-xs",
                      gitUrlStatus === "invalid" ? "text-danger" : "text-text-muted",
                    )}>{gitUrlMessage}</p>
                  )}
                  {fieldErrors.gitUrl && (
                    <p className="mt-1 text-xs text-danger">{fieldErrors.gitUrl}</p>
                  )}
                </div>
                <Input
                  label="Branch"
                  value={gitBranch}
                  onChange={setGitBranch}
                  placeholder="main"
                />
                <div>
                  <label className="mb-1.5 block text-sm font-medium text-text-subtle">Provider</label>
                  <select
                    className="h-10 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 text-sm text-text outline-none transition hover:border-line focus:border-danger focus:ring-2 focus:ring-danger/20"
                    value={gitProvider}
                    onChange={(e) => setGitProvider(e.target.value as GitProviderType)}
                  >
                    <option value="github">GitHub</option>
                    <option value="gitlab">GitLab</option>
                    <option value="bitbucket">Bitbucket</option>
                    <option value="gitea">Gitea</option>
                    <option value="other">Other</option>
                  </select>
                </div>
                <div className="sm:col-span-2">
                  <Switch
                    checked={autoDeploy}
                    onCheckedChange={setAutoDeploy}
                    label="Auto-deploy on push"
                  />
                </div>
              </div>
            </Card>
          )}

          {sourceType === "compose" && (
            <Card>
              <CardHeader
                title="Docker Compose"
                icon={Container}
                action={composeValidation ? (
                  <Pill tone={composeValidation.valid ? "green" : "red"}>
                    {composeValidation.valid ? "Valid" : `${composeValidation.errors.length} issue${composeValidation.errors.length === 1 ? "" : "s"}`}
                  </Pill>
                ) : composeCheckQ.isFetching ? (
                  <Pill tone="neutral">Checking…</Pill>
                ) : undefined}
              />
              <div className="space-y-4 p-4">
                <div className="rounded-lg border border-line bg-overlay-subtle px-3 py-2 text-xs leading-5 text-text-subtle">
                  Creates an app of type Compose. For full stack lifecycle (logs, rollback, GitOps), create a{" "}
                  <button type="button" onClick={() => router.push("/admin/compose/new")} className="underline hover:text-text">
                    Compose Stack
                  </button>{" "}
                  instead — same file, managed deployment surface.
                </div>
                <div>
                  <label className="mb-1.5 block text-sm font-medium text-text-subtle">Upload compose file</label>
                  <input
                    type="file"
                    accept=".yml,.yaml"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) setComposeFile(file);
                    }}
                    className="block w-full text-xs text-text-subtle file:mr-4 file:cursor-pointer file:rounded-lg file:border-0 file:bg-[color-mix(in_srgb,var(--brand)_20%,transparent)] file:px-4 file:py-2 file:text-xs file:font-semibold file:text-[var(--brand)] hover:file:bg-[color-mix(in_srgb,var(--brand)_30%,transparent)]"
                  />
                </div>
                <div>
                  <label className="mb-1.5 block text-sm font-medium text-text-subtle">
                    Or paste compose file content
                  </label>
                  <textarea
                    className={cn(
                      "h-48 w-full rounded-lg border bg-[var(--surface-input)] p-3 font-mono text-xs text-text outline-none resize-none transition",
                      fieldErrors.composeContent ? "border-danger" : "border-line hover:border-line",
                      "focus:border-danger focus:ring-2 focus:ring-danger/20",
                    )}
                    value={composeContent}
                    onChange={(e) => { setComposeContent(e.target.value); setFieldErrors((ef) => ({ ...ef, composeContent: "" })); setFieldsDirty(true); }}
                    placeholder={`services:\n  web:\n    image: nginx:latest\n    ports:\n      - "8080:80"`}
                  />
                </div>
                {fieldErrors.composeContent && (
                  <p className="text-xs text-danger">{fieldErrors.composeContent}</p>
                )}
                {composeValidatorError && (
                  <p className="rounded-lg border border-warn-line bg-warn-subtle p-3 text-xs leading-5 text-warn">
                    {composeValidatorError}
                  </p>
                )}
                {composeValidation && !composeValidation.valid && (
                  <div className="rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger">
                    <p className="mb-1 text-xs font-semibold uppercase tracking-wider text-danger">Validation Issues</p>
                    {composeValidation.errors.map((e, i) => (
                      <div key={i} className="text-xs leading-relaxed">- {e}</div>
                    ))}
                  </div>
                )}
              </div>
            </Card>
          )}

          <Card>
            <CardHeader title="Deployment target" icon={Cloud} />
            <div className="p-4 space-y-4">
              <div className="rounded-lg border border-line bg-overlay-subtle px-3 py-2 text-xs leading-5 text-text-subtle">
                Select a Beacon and region when this application has a specific destination. Leave either setting automatic for Forge to choose from eligible infrastructure when it deploys.
              </div>
              <AdminFormSection title="Deployment Target">
                <div className="grid gap-4 sm:grid-cols-2">
                  <div>
                    <label className="mb-1.5 block text-sm font-medium text-text-subtle">Beacon</label>
                    <select
                      className="h-10 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 text-sm text-text outline-none transition hover:border-line focus:border-danger focus:ring-2 focus:ring-danger/20"
                      value={nodeId}
                      onChange={(e) => setNodeId(e.target.value)}
                    >
                      <option value="">Auto-select (Forge picks best)</option>
                        {(Array.isArray(nodes) ? nodes : []).map((n: { id: string; name: string; heartbeatState?: string; memoryMb?: number }) => (
                        <option key={n.id} value={n.id}>{n.name} · {(n as { heartbeatState?: string }).heartbeatState ?? "unknown"} · {((n as { memoryMb?: number }).memoryMb ?? "?")} MiB{n.id === recommendedNode?.id ? " · ★ recommended" : ""}</option>
                      ))}
                    </select>
                    {!nodeId && recommendedNode && (
                      <div className="mt-2 flex flex-wrap items-center gap-2">
                        <Btn size="sm" tone="ghost" onClick={() => setNodeId(recommendedNode.id)}>Use recommended: {recommendedNode.name}</Btn>
                        <span className="text-xs text-text-muted">Based on its reported health and configured capacity.</span>
                      </div>
                    )}
                  </div>
                  <div>
                    <label className="mb-1.5 block text-sm font-medium text-text-subtle">Region</label>
                    <select
                      className="h-10 w-full rounded-lg border border-line bg-[var(--surface-input)] px-3 text-sm text-text outline-none transition hover:border-line focus:border-danger focus:ring-2 focus:ring-danger/20"
                      value={regionId}
                      onChange={(e) => setRegionId(e.target.value)}
                    >
                      <option value="">Auto-select</option>
                        {(Array.isArray(regions) ? regions : []).map((r: { id: string; name: string }) => (
                        <option key={r.id} value={r.id}>{r.name}</option>
                      ))}
                    </select>
                    <p className="mt-1 text-xs text-text-muted">Leave this automatic to let the placement policy choose an eligible region.</p>
                  </div>
                </div>
              </AdminFormSection>
              {pickedNode ? (
                <div className="rounded-xl border border-ok-line bg-ok-subtle p-3">
                  <div className="flex items-center justify-between gap-2">
                    <p className="flex items-center gap-1.5 text-xs font-bold uppercase tracking-widest text-ok"><ShieldCheck size={12} /> Placement assessment · {pickedNode.id === recommendedNode?.id ? "Recommended" : "Selected"}</p>
                    <Pill tone={pickedNode.heartbeatState === "healthy" ? "green" : pickedNode.heartbeatState === "degraded" ? "yellow" : "red"}>{pickedNode.heartbeatState ?? "unknown"}</Pill>
                  </div>
                  <ul className="mt-2 grid gap-1 text-xs leading-5 text-text-subtle sm:grid-cols-2">
                    <li className="flex items-center gap-1.5"><span className={pickedNode.heartbeatState === "healthy" ? "text-ok" : "text-danger"}>{pickedNode.heartbeatState === "healthy" ? "✓" : "✗"}</span> Beacon heartbeat healthy</li>
                    <li className="flex items-center gap-1.5"><span className="text-ok">✓</span> FQDN {pickedNode.fqdn ?? "—"}</li>
                    <li className="flex items-center gap-1.5"><Cpu size={10} className="text-text-muted" /> Configured capacity: {pickedNode.memoryMb ?? "?"} MiB memory, {pickedNode.diskMb ?? "?"} MiB disk; request: {memory} / {disk}</li>
                    <li className="flex items-center gap-1.5"><span className="text-ok">✓</span> Region {(Array.isArray(regions) ? regions : []).find((r: { id: string; name: string }) => r.id === regionId)?.name ?? (pickedNode.regionId ? `scoped` : "any")} compatible</li>
                  </ul>
                  <p className="mt-2 text-[11px] leading-4 text-text-muted">Forge evaluates the placement policy and the latest reported node state when it deploys the application.</p>
                </div>
              ) : (
                <div className="rounded-lg border border-line bg-overlay-subtle px-3 py-2 text-xs text-text-muted">Select a Beacon to review its placement details, or leave the choice automatic and Forge will select an eligible target during deployment.</div>
              )}
            </div>
          </Card>

          <Card>
            <CardHeader title="Resources" icon={Zap} />
            <div className="p-4">
              <AdminFormSection title="Resources">
                <div className="grid gap-4 sm:grid-cols-3">
                  <div>
                    <Input label="CPU (cores)" value={cpu} onChange={(v) => { setCpu(v); setFieldErrors((e) => ({ ...e, cpu: "" })); setFieldsDirty(true); }} placeholder="1.0" />
                    {fieldErrors.cpu && <p className="mt-1 text-xs text-danger">{fieldErrors.cpu}</p>}
                  </div>
                  <div>
                    <Input label="Memory (MiB)" value={memory} onChange={(v) => { setMemory(v); setFieldErrors((e) => ({ ...e, memory: "" })); setFieldsDirty(true); }} placeholder="1024" />
                    {fieldErrors.memory && <p className="mt-1 text-xs text-danger">{fieldErrors.memory}</p>}
                  </div>
                  <div>
                    <Input label="Disk (MiB)" value={disk} onChange={(v) => { setDisk(v); setFieldErrors((e) => ({ ...e, disk: "" })); setFieldsDirty(true); }} placeholder="10240" />
                    {fieldErrors.disk && <p className="mt-1 text-xs text-danger">{fieldErrors.disk}</p>}
                  </div>
                </div>
              </AdminFormSection>
            </div>
          </Card>

          <Card>
            <CardHeader title="Environment Variables" icon={Play} />
            <div className="p-4">
              <EnvVarEditor envVars={envVars} onChange={(v) => { setEnvVars(v); setFieldsDirty(true); }} />
            </div>
          </Card>

          <Card>
            <CardHeader title="Port Mapping" icon={Box} />
            <div className="p-4">
              <PortMapper ports={ports} onChange={(v) => { setPorts(v); setFieldsDirty(true); }} />
            </div>
          </Card>

          <Card>
            <CardHeader title="Volume Mounts" icon={Box} />
            <div className="p-4">
              <VolumeEditor volumes={volumes} onChange={(v) => { setVolumes(v); setFieldsDirty(true); }} />
            </div>
          </Card>

          <Card>
            <CardHeader title="Domain & TLS" icon={Globe} />
            <div className="p-4">
              <AdminFormSection title="Domain & TLS">
                <div>
                  <Input
                    label="Domains (comma-separated)"
                    value={domains}
                    onChange={(v) => { setDomains(v); setFieldErrors((e) => ({ ...e, domains: "" })); setFieldsDirty(true); }}
                    placeholder="example.com, www.example.com"
                  />
                  {fieldErrors.domains && (
                    <p className="mt-1 text-xs text-danger">{fieldErrors.domains}</p>
                  )}
                </div>
                <Switch
                  checked={enableTls}
                  onCheckedChange={setEnableTls}
                  label="TLS/SSL"
                />
              </AdminFormSection>
            </div>
          </Card>

          <div className="flex justify-between gap-3">
            <Btn tone="ghost" onClick={() => router.push("/admin/apps")}>Cancel</Btn>
            <div className="flex gap-2">
              <Btn tone="ghost" onClick={() => setStep("template")}>Back</Btn>
              <Btn tone="primary" onClick={() => {
                const errs = validateAll();
                setFieldErrors(errs);
                if (Object.keys(errs).length === 0) setStep("review");
              }}>
                Next: Review
              </Btn>
            </div>
          </div>
        </div>
      )}

      {step === "review" && (
        <div className="space-y-6">
          <Card>
            <CardHeader title="Review Configuration" icon={Layers} />
            <div className="divide-y divide-line text-sm">
              <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <span className="text-text-subtle">Name</span>
                <span className="font-semibold text-text break-all">{name}</span>
              </div>
              <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <span className="text-text-subtle">Type</span>
                <Pill tone="neutral">{typeLabel(sourceType)}</Pill>
              </div>
              {sourceType === "image" && (
                <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                  <span className="text-text-subtle">Image</span>
                  <span className="font-mono text-xs text-text-subtle break-all">{image}</span>
                </div>
              )}
              {sourceType === "git" && (
                <>
                  <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                    <span className="text-text-subtle">Repository</span>
                    <span className="font-mono text-xs text-text-subtle break-all">{gitUrl}</span>
                  </div>
                  <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                    <span className="text-text-subtle">Branch</span>
                    <span className="text-text">{gitBranch}</span>
                  </div>
                  <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                    <span className="text-text-subtle">Provider</span>
                    <Pill tone="neutral">{gitProvider}</Pill>
                  </div>
                  <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                    <span className="text-text-subtle">Auto-deploy</span>
                    <span className={autoDeploy ? "text-ok" : "text-text-muted"}>{autoDeploy ? "Enabled" : "Disabled"}</span>
                  </div>
                </>
              )}
              {sourceType === "compose" && (
                <div className="px-4 py-3">
                  <span className="text-text-subtle">Compose File</span>
                  <pre className="mt-1 max-h-24 overflow-y-auto rounded border border-line bg-[var(--canvas)] p-2 font-mono text-xs text-text-subtle whitespace-pre-wrap break-all">
                    {composeContent.slice(0, 300)}{composeContent.length > 300 ? "..." : ""}
                  </pre>
                </div>
              )}
              <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <span className="text-text-subtle">Resources</span>
                <span className="text-text">{cpu} CPU / {memory} MiB / {disk} MiB</span>
              </div>
              <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <span className="text-text-subtle">Ports</span>
                <span className="text-text text-right">{ports.length > 0 ? ports.map((p) => `${p.hostPort}:${p.containerPort}/${p.protocol}`).join(", ") : "None"}</span>
              </div>
              <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <span className="text-text-subtle">Env Vars</span>
                <span className="text-text">{Object.keys(envVars).length} variable{Object.keys(envVars).length === 1 ? "" : "s"}</span>
              </div>
              <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <span className="text-text-subtle">Volumes</span>
                <span className="text-text">{volumes.length} mount{volumes.length === 1 ? "" : "s"}</span>
              </div>
              {domains.trim() && (
                <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                  <span className="text-text-subtle">Domains</span>
                  <span className="text-text text-right break-all">{domains}</span>
                </div>
              )}
              {enableTls && (
                <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                  <span className="text-text-subtle">TLS/SSL</span>
                  <span className="text-ok">Enabled</span>
                </div>
              )}
              {nodeId && (
                <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                  <span className="text-text-subtle">Node</span>
                   <span className="text-text">{(Array.isArray(nodes) ? nodes : []).find((n: { id: string; name: string }) => n.id === nodeId)?.name ?? nodeId}</span>
                </div>
              )}
              {regionId && (
                <div className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                  <span className="text-text-subtle">Region</span>
                   <span className="text-text">{(Array.isArray(regions) ? regions : []).find((r: { id: string; name: string }) => r.id === regionId)?.name ?? regionId}</span>
                </div>
              )}
            </div>
          </Card>

          {validationError && !createMut.isPending && (
            <div className="rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger">
              <div className="flex items-center gap-2">
                <AlertCircle size={14} />
                <span>{validationError}</span>
              </div>
            </div>
          )}

          {createMut.error && (
            <div className="rounded-lg border border-danger-line bg-danger-subtle p-3 text-sm text-danger">
              <div className="flex items-center gap-2">
                <AlertCircle size={14} />
                <span>{createMut.error.message}</span>
              </div>
            </div>
          )}

          <div className="flex justify-between gap-3">
            <Btn tone="ghost" onClick={() => router.push("/admin/apps")}>Cancel</Btn>
            <div className="flex gap-2">
              <Btn tone="ghost" onClick={() => { setStep("configure"); setFieldErrors({}); }}>Back</Btn>
              <Btn
                tone="primary"
                onClick={() => createMut.mutate()}
                disabled={!!validationError || createMut.isPending}
              >
                {createMut.isPending ? (
                  <span className="flex items-center gap-2">
                    <LoaderCircle size={14} className="animate-spin" />
                    Creating...
                  </span>
                ) : "Create Application"}
              </Btn>
            </div>
          </div>
        </div>
      )}
    </AdminPageLayout>
  );
}
