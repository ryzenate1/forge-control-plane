"use client";

import { useState, useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import {
  ArrowLeft, Box, Cloud, Container, GitBranch, Globe, Layers,
  Play, Zap,
} from "lucide-react";
import {
  createApp, fetchAppTemplates, typeLabel,
  type AppType, type AppPort, type AppVolume, type CreateAppInput,
} from "@/lib/api/apps";
import { fetchNodes, fetchRegions } from "@/lib/api";
import { Btn, Card, CardHeader, Input, SectionHeader, Pill, cn } from "@/components/admin/admin-ui";
import { EnvVarEditor, PortMapper, VolumeEditor } from "@/components/admin/AdminAppsShared";

const SOURCE_TYPES: { id: AppType; label: string; icon: typeof Box; desc: string }[] = [
  { id: "image", label: "Docker Image", icon: Box, desc: "Deploy from any Docker registry image" },
  { id: "git", label: "Git Repository", icon: GitBranch, desc: "Build and deploy from a Git repository" },
  { id: "compose", label: "Docker Compose", icon: Container, desc: "Deploy a multi-service Docker Compose stack" },
];

export default function CreateAppPage() {
  const router = useRouter();
  const qc = useQueryClient();
  const [step, setStep] = useState<"source" | "template" | "configure" | "review">("source");
  const [sourceType, setSourceType] = useState<AppType>("image");
  const [name, setName] = useState("");
  const [image, setImage] = useState("");
  const [registryUrl, setRegistryUrl] = useState("");
  const [registryUser, setRegistryUser] = useState("");
  const [registryPass, setRegistryPass] = useState("");
  const [gitUrl, setGitUrl] = useState("");
  const [gitBranch, setGitBranch] = useState("main");
  const [gitProvider, setGitProvider] = useState("github");
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
  const [domains, setDomains] = useState<string>("");
  const [enableTls, setEnableTls] = useState(false);
  const [autoDeploy, setAutoDeploy] = useState(false);
  const [composeValidation, setComposeValidation] = useState<{ valid: boolean; errors: string[] } | null>(null);

  const { data: templates = [] } = useQuery({
    queryKey: ["app-templates"],
    queryFn: fetchAppTemplates,
  });
  const { data: nodes = [] } = useQuery({ queryKey: ["nodes"], queryFn: fetchNodes });
  const { data: regions = [] } = useQuery({ queryKey: ["regions"], queryFn: fetchRegions });

  useEffect(() => {
    if (composeFile) {
      const reader = new FileReader();
      reader.onload = (e) => {
        setComposeContent(e.target?.result as string ?? "");
      };
      reader.readAsText(composeFile);
    }
  }, [composeFile]);

  useEffect(() => {
    if (sourceType === "compose" && composeContent.length > 100) {
      validateCompose(composeContent);
    } else {
      setComposeValidation(null);
    }
  }, [sourceType, composeContent]);

  const validateCompose = (content: string) => {
    const errors: string[] = [];
    const hasServices = /^\s*services\s*:/m.test(content);
    if (!hasServices) errors.push("No 'services' section found");
    const hasVersion = /^\s*version\s*:/m.test(content);
    if (!hasVersion) errors.push("No 'version' field found (optional but recommended)");
    setComposeValidation({ valid: errors.length === 0, errors });
  };

  const handleTemplateSelect = (templateId: string) => {
    const tpl = templates.find((t) => t.id === templateId);
    if (!tpl) return;
    setSelectedTemplate(templateId);
    setSourceType(tpl.type);
    if (tpl.image) setImage(tpl.image);
    if (tpl.gitUrl) { setGitUrl(tpl.gitUrl); setGitBranch("main"); }
    if (tpl.composeContent) setComposeContent(tpl.composeContent);
    setPorts(tpl.defaultPorts ?? []);
    setEnvVars(tpl.defaultEnvVars ?? {});
    setCpu(tpl.defaultResources.cpu ?? "1.0");
    setMemory(tpl.defaultResources.memory ?? "1024");
    setDisk(tpl.defaultResources.disk ?? "10240");
  };

  const validationError = !name.trim()
    ? "App name is required."
    : sourceType === "image" && !image.trim()
      ? "Docker image is required."
      : sourceType === "git" && !gitUrl.trim()
        ? "Git repository URL is required."
        : sourceType === "compose" && !composeContent.trim()
          ? "Docker Compose content is required."
          : null;

  const createMut = useMutation({
    mutationFn: () => {
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
      void qc.invalidateQueries({ queryKey: ["apps"] });
      router.push(`/admin/apps/${result.id}`);
    },
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Btn tone="ghost" size="sm" onClick={() => router.push("/admin/apps")}>
          <ArrowLeft size={14} />
        </Btn>
        <SectionHeader
          title="Create Application"
          sub="Deploy a Docker image, Git repository, or Compose stack"
        />
      </div>

      <div className="flex gap-2 border-b border-white/[0.06] pb-4">
        {(["source", "template", "configure", "review"] as const).map((s, i) => (
          <div key={s} className="flex items-center gap-2">
            <div className={cn(
              "flex h-8 w-8 items-center justify-center rounded-full text-xs font-bold",
              step === s
                ? "bg-[#dc2626] text-white"
                : i < (["source", "template", "configure", "review"].indexOf(step))
                  ? "bg-emerald-500/20 text-emerald-400"
                  : "bg-white/[0.06] text-slate-500",
            )}>
              {i < (["source", "template", "configure", "review"].indexOf(step)) ? <Zap size={12} /> : i + 1}
            </div>
            <span className={cn("text-xs font-medium capitalize", step === s ? "text-white" : "text-slate-500")}>{s}</span>
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
                    "flex flex-col items-center gap-3 rounded-xl border p-6 text-center transition hover:border-[#dc2626]/30",
                    sourceType === id
                      ? "border-[#dc2626] bg-[#dc2626]/5"
                      : "border-white/[0.08] bg-white/[0.02]",
                  )}
                  onClick={() => setSourceType(id)}
                >
                  <Icon size={28} className={sourceType === id ? "text-[#dc2626]" : "text-slate-500"} />
                  <div>
                    <p className="font-semibold text-slate-200">{label}</p>
                    <p className="mt-1 text-xs text-slate-500">{desc}</p>
                  </div>
                </button>
              ))}
            </div>
          </Card>

          <div className="flex justify-end gap-3">
            <Btn tone="ghost" onClick={() => router.push("/admin/apps")}>Cancel</Btn>
            <Btn tone="primary" onClick={() => setStep("template")}>Next: Templates</Btn>
          </div>
        </div>
      )}

      {step === "template" && (
        <div className="space-y-6">
          <Card>
            <CardHeader title="Quick Templates (Optional)" icon={Zap} />
            <div className="p-4">
              {templates.length === 0 ? (
                <p className="text-sm text-slate-500">No templates available.</p>
              ) : (
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {templates.map((tpl) => (
                    <button
                      key={tpl.id}
                      type="button"
                      className={cn(
                        "rounded-xl border p-4 text-left transition hover:border-[#dc2626]/30",
                        selectedTemplate === tpl.id
                          ? "border-[#dc2626] bg-[#dc2626]/5"
                          : "border-white/[0.08] bg-white/[0.02]",
                      )}
                      onClick={() => handleTemplateSelect(tpl.id)}
                    >
                      <Pill tone="neutral">{typeLabel(tpl.type)}</Pill>
                      <p className="mt-2 font-semibold text-slate-200">{tpl.name}</p>
                      <p className="mt-1 text-xs text-slate-500">{tpl.description}</p>
                    </button>
                  ))}
                </div>
              )}
            </div>
          </Card>

          <div className="flex justify-end gap-3">
            <Btn tone="ghost" onClick={() => setStep("source")}>Back</Btn>
            <Btn tone="primary" onClick={() => setStep("configure")}>Next: Configure</Btn>
          </div>
        </div>
      )}

      {step === "configure" && (
        <div className="space-y-6">
          <Card>
            <CardHeader title="Basic Information" icon={Layers} />
            <div className="grid gap-4 p-4 sm:grid-cols-2">
              <Input label="App Name" value={name} onChange={setName} placeholder="my-app" required />
              <Input label="Type" value={typeLabel(sourceType)} onChange={() => {}} />
            </div>
          </Card>

          {sourceType === "image" && (
            <Card>
              <CardHeader title="Docker Image" icon={Box} />
              <div className="grid gap-4 p-4 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <Input label="Image Name" value={image} onChange={setImage} placeholder="nginx:latest" required />
                </div>
                <Input label="Registry URL" value={registryUrl} onChange={setRegistryUrl} placeholder="registry.example.com" />
                <Input label="Username" value={registryUser} onChange={setRegistryUser} placeholder="(optional)" />
                <Input label="Password" value={registryPass} onChange={setRegistryPass} type="password" placeholder="(optional)" />
              </div>
            </Card>
          )}

          {sourceType === "git" && (
            <Card>
              <CardHeader title="Git Repository" icon={GitBranch} />
              <div className="grid gap-4 p-4 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <Input label="Repository URL" value={gitUrl} onChange={setGitUrl} placeholder="https://github.com/user/repo.git" required />
                </div>
                <Input label="Branch" value={gitBranch} onChange={setGitBranch} placeholder="main" />
                <div>
                  <label className="mb-1.5 block text-sm font-medium text-slate-300">Provider</label>
                  <select
                    className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none"
                    value={gitProvider}
                    onChange={(e) => setGitProvider(e.target.value)}
                  >
                    <option value="github">GitHub</option>
                    <option value="gitlab">GitLab</option>
                    <option value="bitbucket">Bitbucket</option>
                    <option value="gitea">Gitea</option>
                    <option value="other">Other</option>
                  </select>
                </div>
                <div className="sm:col-span-2">
                  <label className="flex items-center gap-2 text-sm text-slate-400">
                    <input
                      type="checkbox"
                      checked={autoDeploy}
                      onChange={(e) => setAutoDeploy(e.target.checked)}
                      className="h-3 w-3 rounded border-white/20 bg-[#161b28] accent-[#dc2626]"
                    />
                    Enable auto-deploy on push
                  </label>
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
                ) : undefined}
              />
              <div className="space-y-4 p-4">
                <div>
                  <label className="mb-1.5 block text-sm font-medium text-slate-300">Upload compose file</label>
                  <input
                    type="file"
                    accept=".yml,.yaml"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) setComposeFile(file);
                    }}
                    className="block w-full text-xs text-slate-400 file:mr-4 file:rounded-lg file:border-0 file:bg-[#dc2626]/20 file:px-4 file:py-2 file:text-xs file:font-semibold file:text-[#dc2626] hover:file:bg-[#dc2626]/30"
                  />
                </div>
                <div>
                  <label className="mb-1.5 block text-sm font-medium text-slate-300">
                    Or paste compose file content
                  </label>
                  <textarea
                    className="h-48 w-full rounded-lg border border-white/10 bg-[#161b28] p-3 font-mono text-xs text-slate-100 outline-none resize-none"
                    value={composeContent}
                    onChange={(e) => setComposeContent(e.target.value)}
                    placeholder={`version: "3.8"\nservices:\n  web:\n    image: nginx:latest\n    ports:\n      - "8080:80"`}
                  />
                </div>
                {composeValidation && !composeValidation.valid && (
                  <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">
                    {composeValidation.errors.map((e, i) => (
                      <div key={i}>- {e}</div>
                    ))}
                  </div>
                )}
              </div>
            </Card>
          )}

          <Card>
            <CardHeader title="Deployment Target" icon={Cloud} />
            <div className="grid gap-4 p-4 sm:grid-cols-2">
              <div>
                <label className="mb-1.5 block text-sm font-medium text-slate-300">Node</label>
                <select
                  className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none"
                  value={nodeId}
                  onChange={(e) => setNodeId(e.target.value)}
                >
                  <option value="">Auto-select</option>
                  {nodes.map((n) => (
                    <option key={n.id} value={n.id}>{n.name}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="mb-1.5 block text-sm font-medium text-slate-300">Region</label>
                <select
                  className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none"
                  value={regionId}
                  onChange={(e) => setRegionId(e.target.value)}
                >
                  <option value="">Auto-select</option>
                  {regions.map((r) => (
                    <option key={r.id} value={r.id}>{r.name}</option>
                  ))}
                </select>
              </div>
            </div>
          </Card>

          <Card>
            <CardHeader title="Resources" icon={Zap} />
            <div className="grid gap-4 p-4 sm:grid-cols-3">
              <Input label="CPU (cores)" value={cpu} onChange={setCpu} placeholder="1.0" />
              <Input label="Memory (MiB)" value={memory} onChange={setMemory} placeholder="1024" />
              <Input label="Disk (MiB)" value={disk} onChange={setDisk} placeholder="10240" />
            </div>
          </Card>

          <Card>
            <CardHeader title="Environment Variables" icon={Play} />
            <div className="p-4">
              <EnvVarEditor envVars={envVars} onChange={setEnvVars} />
            </div>
          </Card>

          <Card>
            <CardHeader title="Port Mapping" icon={Box} />
            <div className="p-4">
              <PortMapper ports={ports} onChange={setPorts} />
            </div>
          </Card>

          <Card>
            <CardHeader title="Volume Mounts" icon={Box} />
            <div className="p-4">
              <VolumeEditor volumes={volumes} onChange={setVolumes} />
            </div>
          </Card>

          <Card>
            <CardHeader title="Domain & TLS" icon={Globe} />
            <div className="space-y-4 p-4">
              <Input
                label="Domains (comma-separated)"
                value={domains}
                onChange={setDomains}
                placeholder="example.com, www.example.com"
              />
              <label className="flex items-center gap-2 text-sm text-slate-400">
                <input
                  type="checkbox"
                  checked={enableTls}
                  onChange={(e) => setEnableTls(e.target.checked)}
                  className="h-3 w-3 rounded border-white/20 bg-[#161b28] accent-[#dc2626]"
                />
                Enable TLS/SSL
              </label>
            </div>
          </Card>

          <div className="flex justify-end gap-3">
            <Btn tone="ghost" onClick={() => setStep("template")}>Back</Btn>
            <Btn tone="primary" onClick={() => setStep("review")}>Next: Review</Btn>
          </div>
        </div>
      )}

      {step === "review" && (
        <div className="space-y-6">
          <Card>
            <CardHeader title="Review Configuration" icon={Layers} />
            <div className="divide-y divide-white/[0.06] text-sm">
              <div className="flex justify-between px-4 py-3">
                <span className="text-slate-400">Name</span>
                <span className="font-semibold text-slate-200">{name}</span>
              </div>
              <div className="flex justify-between px-4 py-3">
                <span className="text-slate-400">Type</span>
                <Pill tone="neutral">{typeLabel(sourceType)}</Pill>
              </div>
              {sourceType === "image" && (
                <div className="flex justify-between px-4 py-3">
                  <span className="text-slate-400">Image</span>
                  <span className="font-mono text-xs text-slate-300">{image}</span>
                </div>
              )}
              {sourceType === "git" && (
                <>
                  <div className="flex justify-between px-4 py-3">
                    <span className="text-slate-400">Repository</span>
                    <span className="font-mono text-xs text-slate-300">{gitUrl}</span>
                  </div>
                  <div className="flex justify-between px-4 py-3">
                    <span className="text-slate-400">Branch</span>
                    <span className="text-slate-200">{gitBranch}</span>
                  </div>
                  <div className="flex justify-between px-4 py-3">
                    <span className="text-slate-400">Provider</span>
                    <span className="text-slate-200">{gitProvider}</span>
                  </div>
                </>
              )}
              {sourceType === "compose" && (
                <div className="px-4 py-3">
                  <span className="text-slate-400">Compose File</span>
                  <pre className="mt-1 max-h-24 overflow-y-auto rounded border border-white/[0.06] bg-[#0a0e14] p-2 font-mono text-xs text-slate-400 whitespace-pre-wrap">
                    {composeContent.slice(0, 300)}{composeContent.length > 300 ? "..." : ""}
                  </pre>
                </div>
              )}
              <div className="flex justify-between px-4 py-3">
                <span className="text-slate-400">Resources</span>
                <span className="text-slate-200">{cpu} CPU / {memory} MiB / {disk} MiB</span>
              </div>
              <div className="flex justify-between px-4 py-3">
                <span className="text-slate-400">Ports</span>
                <span className="text-slate-200">{ports.length > 0 ? ports.map((p) => `${p.hostPort}:${p.containerPort}/${p.protocol}`).join(", ") : "None"}</span>
              </div>
              <div className="flex justify-between px-4 py-3">
                <span className="text-slate-400">Env Vars</span>
                <span className="text-slate-200">{Object.keys(envVars).length} variable{Object.keys(envVars).length === 1 ? "" : "s"}</span>
              </div>
              <div className="flex justify-between px-4 py-3">
                <span className="text-slate-400">Volumes</span>
                <span className="text-slate-200">{volumes.length} mount{volumes.length === 1 ? "" : "s"}</span>
              </div>
              {domains && (
                <div className="flex justify-between px-4 py-3">
                  <span className="text-slate-400">Domains</span>
                  <span className="text-slate-200">{domains}</span>
                </div>
              )}
              {nodeId && (
                <div className="flex justify-between px-4 py-3">
                  <span className="text-slate-400">Node</span>
                  <span className="text-slate-200">{nodes.find((n) => n.id === nodeId)?.name ?? nodeId}</span>
                </div>
              )}
              {regionId && (
                <div className="flex justify-between px-4 py-3">
                  <span className="text-slate-400">Region</span>
                  <span className="text-slate-200">{regions.find((r) => r.id === regionId)?.name ?? regionId}</span>
                </div>
              )}
            </div>
          </Card>

          {validationError && (
            <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">
              {validationError}
            </div>
          )}
          {createMut.error && (
            <div className="rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-300">
              {createMut.error.message}
            </div>
          )}

          <div className="flex justify-end gap-3">
            <Btn tone="ghost" onClick={() => setStep("configure")}>Back</Btn>
            <Btn
              tone="primary"
              onClick={() => createMut.mutate()}
              disabled={!!validationError || createMut.isPending}
            >
              {createMut.isPending ? "Creating..." : "Create Application"}
            </Btn>
          </div>
        </div>
      )}
    </div>
  );
}
