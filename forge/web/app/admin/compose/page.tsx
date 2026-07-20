"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { Upload, Trash2, Download, Eye, EyeOff, CheckCircle, XCircle, AlertTriangle } from "lucide-react";

const API_BASE = process.env.NEXT_PUBLIC_API_URL ?? (process.env.NODE_ENV === "development" ? "http://localhost:8080/api/v1" : "/api/v1");

interface ComposeProject {
  id: string;
  name: string;
  composeContent: string;
  parsedConfig: any;
  status: string;
  revision: number;
  createdAt: string;
  updatedAt: string;
}

interface ValidateResult {
  valid: boolean;
  errors: { field: string; message: string }[];
  warnings: { field: string; message: string }[];
  summary: any;
}

export default function ComposePage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [yamlContent, setYamlContent] = useState("");
  const [projectName, setProjectName] = useState("");
  const [validateResult, setValidateResult] = useState<ValidateResult | null>(null);
  const [selectedProject, setSelectedProject] = useState<ComposeProject | null>(null);
  const [showYaml, setShowYaml] = useState(false);

  const { data: projects = [], isLoading } = useQuery<ComposeProject[]>({
    queryKey: ["compose-projects"],
    queryFn: async () => {
      const res = await fetch(`${API_BASE}/compose/projects`, { credentials: "include" });
      if (!res.ok) throw new Error("Failed to fetch projects");
      return res.json();
    },
  });

  const validateMutation = useMutation({
    mutationFn: async (content: string) => {
      const res = await fetch(`${API_BASE}/compose/validate`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ content }),
      });
      return res.json() as Promise<ValidateResult>;
    },
    onSuccess: (data) => {
      setValidateResult(data);
      if (data.valid) {
        toast({ tone: "success", title: "Compose file is valid" });
      }
    },
    onError: () => toast({ tone: "error", title: "Validation failed" }),
  });

  const importMutation = useMutation({
    mutationFn: async ({ name, content }: { name: string; content: string }) => {
      const res = await fetch(`${API_BASE}/compose/import`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify({ name, content }),
      });
      if (!res.ok) throw new Error("Import failed");
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-projects"] });
      setYamlContent("");
      setProjectName("");
      setValidateResult(null);
      toast({ tone: "success", title: "Compose project imported" });
    },
    onError: () => toast({ tone: "error", title: "Import failed" }),
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`${API_BASE}/compose/projects/${id}`, {
        method: "DELETE",
        credentials: "include",
      });
      if (!res.ok) throw new Error("Delete failed");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["compose-projects"] });
      setSelectedProject(null);
      toast({ tone: "success", title: "Project deleted" });
    },
    onError: () => toast({ tone: "error", title: "Delete failed" }),
  });

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (ev) => {
      const content = ev.target?.result as string;
      setYamlContent(content);
      if (!projectName) {
        setProjectName(file.name.replace(/\.(yml|yaml)$/i, ""));
      }
    };
    reader.readAsText(file);
  };

  const handleValidate = () => {
    if (!yamlContent.trim()) {
      toast({ tone: "error", title: "Please enter or upload a Compose YAML file" });
      return;
    }
    validateMutation.mutate(yamlContent);
  };

  const handleImport = () => {
    if (!projectName.trim()) {
      toast({ tone: "error", title: "Project name is required" });
      return;
    }
    if (!validateResult?.valid) {
      toast({ tone: "error", title: "Please fix validation errors before importing" });
      return;
    }
    importMutation.mutate({ name: projectName, content: yamlContent });
  };

  const handleExport = async (project: ComposeProject) => {
    const res = await fetch(`${API_BASE}/compose/projects/${project.id}/export`, {
      credentials: "include",
    });
    if (!res.ok) return;
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${project.name}.yml`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="space-y-6 p-6">
      <h1 className="text-2xl font-bold text-slate-100">Compose Manager</h1>

      <div className="grid gap-6 lg:grid-cols-2">
        <div className="space-y-4 rounded-xl border border-slate-700/50 bg-[#1a2332] p-6">
          <h2 className="text-lg font-semibold text-slate-100">Import Compose File</h2>

          <div>
            <label className="mb-1 block text-sm text-slate-400">Project Name</label>
            <input
              type="text"
              value={projectName}
              onChange={(e) => setProjectName(e.target.value)}
              placeholder="my-compose-app"
              className="w-full rounded-lg border border-slate-600 bg-[#0f1419] px-3 py-2 text-sm text-slate-200 placeholder:text-slate-500 focus:border-blue-500 focus:outline-none"
            />
          </div>

          <div>
            <label className="mb-1 block text-sm text-slate-400">
              Compose YAML
              <span className="ml-2 text-xs text-slate-500">(paste or upload)</span>
            </label>
            <label className="flex cursor-pointer items-center gap-2 rounded-lg border border-dashed border-slate-600 bg-[#0f1419] px-3 py-2 text-sm text-slate-400 hover:border-blue-500 hover:text-blue-400 transition-colors">
              <Upload className="h-4 w-4" />
              Upload .yml file
              <input type="file" accept=".yml,.yaml" onChange={handleFileUpload} className="hidden" />
            </label>
            <textarea
              value={yamlContent}
              onChange={(e) => setYamlContent(e.target.value)}
              placeholder={`services:\n  app:\n    image: nginx:latest\n    ports:\n      - "8080:80"`}
              rows={14}
              className="mt-2 w-full rounded-lg border border-slate-600 bg-[#0f1419] px-3 py-2 text-sm font-mono text-slate-200 placeholder:text-slate-500 focus:border-blue-500 focus:outline-none resize-y"
            />
          </div>

          <div className="flex gap-3">
            <button
              onClick={handleValidate}
              disabled={validateMutation.isPending}
              className="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              {validateMutation.isPending ? "Validating..." : "Validate"}
            </button>
            <button
              onClick={handleImport}
              disabled={importMutation.isPending || !validateResult?.valid}
              className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
            >
              {importMutation.isPending ? "Importing..." : "Import"}
            </button>
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
              {validateResult.errors.map((err, i) => (
                <div key={i} className="mt-2 text-xs text-red-400">
                  <strong>{err.field}:</strong> {err.message}
                </div>
              ))}
              {validateResult.warnings.map((w, i) => (
                <div key={i} className="mt-1 flex items-center gap-1 text-xs text-amber-400">
                  <AlertTriangle className="h-3 w-3" />
                  <strong>{w.field}:</strong> {w.message}
                </div>
              ))}
              {validateResult.summary?.services && (
                <div className="mt-3 text-xs text-slate-400">
                  Detected {validateResult.summary.services.length} service(s), {validateResult.summary.networks?.length || 0} network(s), {validateResult.summary.volumes?.length || 0} volume(s)
                </div>
              )}
            </div>
          )}
        </div>

        <div className="space-y-4 rounded-xl border border-slate-700/50 bg-[#1a2332] p-6">
          <h2 className="text-lg font-semibold text-slate-100">Projects</h2>

          {isLoading ? (
            <p className="text-sm text-slate-400">Loading...</p>
          ) : projects.length === 0 ? (
            <p className="text-sm text-slate-500">No compose projects imported yet.</p>
          ) : (
            <div className="space-y-2">
              {projects.map((project) => (
                <div
                  key={project.id}
                  className={`rounded-lg border p-3 cursor-pointer transition-colors ${
                    selectedProject?.id === project.id
                      ? "border-blue-500/50 bg-blue-500/10"
                      : "border-slate-700/50 bg-[#0f1419] hover:border-slate-600"
                  }`}
                  onClick={() => {
                    setSelectedProject(project);
                    setShowYaml(false);
                  }}
                >
                  <div className="flex items-center justify-between">
                    <div>
                      <span className="text-sm font-medium text-slate-200">{project.name}</span>
                      <span className="ml-2 text-xs text-slate-500">v{project.revision}</span>
                    </div>
                    <div className="flex gap-1">
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          handleExport(project);
                        }}
                        className="rounded p-1 text-slate-500 hover:text-slate-300 hover:bg-slate-700"
                      >
                        <Download className="h-4 w-4" />
                      </button>
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          if (confirm(`Delete "${project.name}"?`)) deleteMutation.mutate(project.id);
                        }}
                        className="rounded p-1 text-slate-500 hover:text-red-400 hover:bg-slate-700"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </div>
                  </div>
                  <div className="mt-1 text-xs text-slate-500">
                    Created {new Date(project.createdAt).toLocaleDateString()}
                  </div>
                </div>
              ))}
            </div>
          )}

          {selectedProject && (
            <div className="mt-4 space-y-3 rounded-lg border border-slate-700/50 bg-[#0f1419] p-4">
              <div className="flex items-center justify-between">
                <h3 className="text-sm font-medium text-slate-200">{selectedProject.name}</h3>
                <button
                  onClick={() => setShowYaml(!showYaml)}
                  className="rounded p-1 text-slate-500 hover:text-slate-300"
                >
                  {showYaml ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
              <div className="flex gap-4 text-xs text-slate-400">
                <span>Status: <span className="text-slate-300">{selectedProject.status}</span></span>
                <span>Revision: {selectedProject.revision}</span>
              </div>
              {showYaml && (
                <pre className="max-h-60 overflow-auto rounded bg-slate-950 p-3 text-xs text-slate-300 font-mono">
                  {selectedProject.composeContent}
                </pre>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
