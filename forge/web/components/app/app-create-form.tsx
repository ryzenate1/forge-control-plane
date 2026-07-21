"use client";

import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { useAppToast } from "@/components/shared";
import { createApp } from "@/lib/api/apps";
import type { ApiApp } from "@/lib/api/apps";
import { errorMessage } from "@/lib/utils";

type FormValues = {
  name: string;
  description: string;
  region: string;
  template: string;
  memory: number;
  disk: number;
  cpu: number;
};

type FormErrors = Partial<Record<keyof FormValues, string>>;

const defaultValues: FormValues = {
  name: "",
  description: "",
  region: "",
  template: "",
  memory: 512,
  disk: 1024,
  cpu: 1,
};

function validate(values: FormValues): FormErrors {
  const errors: FormErrors = {};
  if (!values.name.trim()) errors.name = "Name is required";
  else if (values.name.length < 3) errors.name = "Name must be at least 3 characters";
  else if (values.name.length > 64) errors.name = "Name must be 64 characters or less";
  if (!values.region) errors.region = "Region is required";
  if (!values.template) errors.template = "Template is required";
  if (values.memory < 256) errors.memory = "Minimum 256 MB";
  if (values.memory > 32768) errors.memory = "Maximum 32 GB";
  if (values.disk < 512) errors.disk = "Minimum 512 MB";
  if (values.disk > 512000) errors.disk = "Maximum 500 GB";
  if (values.cpu < 0.25) errors.cpu = "Minimum 0.25 vCPU";
  if (values.cpu > 16) errors.cpu = "Maximum 16 vCPU";
  return errors;
}

const fieldClass =
  "mt-1 h-10 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-sm text-slate-100 outline-none placeholder:text-slate-600 focus:border-red-500/50";

export function CreateAppForm({
  onSuccess,
}: {
  onSuccess?: (id: string) => void;
}) {
  const { success: showSuccess, error: showError } = useAppToast();
  const [values, setValues] = useState<FormValues>(defaultValues);
  const [touched, setTouched] = useState<Set<string>>(new Set());
  const errors = validate(values);
  const fieldErrors = Object.fromEntries(
    Object.entries(errors).filter(([key]) => touched.has(key)),
  );

  const createMut = useMutation({
    mutationFn: async (v: FormValues) => {
      const result = await createApp({
        name: v.name.trim(),
        type: "image",
        ports: [],
        envVars: {},
        volumes: [],
        domains: [],
        enableTls: false,
        cpuLimit: String(v.cpu),
        memoryLimit: String(v.memory),
        diskLimit: String(v.disk),
      });
      return result;
    },
    onSuccess: (data) => {
      showSuccess("App", "created");
      onSuccess?.(data.id);
    },
    onError: (error) => showError("App", errorMessage(error, "Failed to create app")),
  });

  function setField<K extends keyof FormValues>(key: K, value: FormValues[K]) {
    setValues((v) => ({ ...v, [key]: value }));
    setTouched((t) => new Set([...t, key]));
  }

  const hasErrors = Object.keys(errors).length > 0;

  return (
    <div className="ui-card">
      <div className="ui-card-header">
        <span className="text-sm font-semibold text-slate-200">Create App</span>
      </div>
      <div className="space-y-5 p-6">
        <label className="block">
          <span className="text-sm font-medium text-slate-300">Name</span>
          <input
            className={fieldClass}
            onChange={(e) => setField("name", e.target.value)}
            placeholder="my-awesome-app"
            value={values.name}
          />
          {fieldErrors.name ? (
            <p className="mt-1 text-xs text-red-400" role="alert">{fieldErrors.name}</p>
          ) : null}
        </label>

        <label className="block">
          <span className="text-sm font-medium text-slate-300">Description</span>
          <input
            className={fieldClass}
            onChange={(e) => setField("description", e.target.value)}
            placeholder="Optional description"
            value={values.description}
          />
        </label>

        <div className="grid gap-5 md:grid-cols-2">
          <label className="block">
            <span className="text-sm font-medium text-slate-300">Region</span>
            <select
              className={fieldClass}
              onChange={(e) => setField("region", e.target.value)}
              value={values.region}
            >
              <option value="">Select region</option>
              <option value="us-east">US East</option>
              <option value="us-west">US West</option>
              <option value="eu-west">EU West</option>
              <option value="ap-south">AP South</option>
            </select>
            {fieldErrors.region ? (
              <p className="mt-1 text-xs text-red-400" role="alert">{fieldErrors.region}</p>
            ) : null}
          </label>

          <label className="block">
            <span className="text-sm font-medium text-slate-300">Template</span>
            <select
              className={fieldClass}
              onChange={(e) => setField("template", e.target.value)}
              value={values.template}
            >
              <option value="">Select template</option>
              <option value="nodejs">Node.js</option>
              <option value="python">Python</option>
              <option value="go">Go</option>
              <option value="static">Static Site</option>
              <option value="docker">Docker</option>
            </select>
            {fieldErrors.template ? (
              <p className="mt-1 text-xs text-red-400" role="alert">{fieldErrors.template}</p>
            ) : null}
          </label>
        </div>

        <div className="grid gap-5 md:grid-cols-3">
          <label className="block">
            <span className="text-sm font-medium text-slate-300">Memory (MB)</span>
            <input
              className={fieldClass}
              min={256}
              onChange={(e) => setField("memory", Number(e.target.value))}
              type="number"
              value={values.memory}
            />
            {fieldErrors.memory ? (
              <p className="mt-1 text-xs text-red-400" role="alert">{fieldErrors.memory}</p>
            ) : null}
          </label>

          <label className="block">
            <span className="text-sm font-medium text-slate-300">Disk (MB)</span>
            <input
              className={fieldClass}
              min={512}
              onChange={(e) => setField("disk", Number(e.target.value))}
              type="number"
              value={values.disk}
            />
            {fieldErrors.disk ? (
              <p className="mt-1 text-xs text-red-400" role="alert">{fieldErrors.disk}</p>
            ) : null}
          </label>

          <label className="block">
            <span className="text-sm font-medium text-slate-300">vCPU</span>
            <input
              className={fieldClass}
              min={0.25}
              onChange={(e) => setField("cpu", Number(e.target.value))}
              step={0.25}
              type="number"
              value={values.cpu}
            />
            {fieldErrors.cpu ? (
              <p className="mt-1 text-xs text-red-400" role="alert">{fieldErrors.cpu}</p>
            ) : null}
          </label>
        </div>

        <button
          className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-5 py-2.5 text-sm font-semibold text-white hover:bg-red-500 disabled:opacity-50"
          disabled={hasErrors || createMut.isPending}
          onClick={() => {
            setTouched(new Set(Object.keys(values)));
            if (!hasErrors) createMut.mutate(values);
          }}
          type="button"
        >
          <Plus size={14} />
          {createMut.isPending ? "Creating…" : "Create App"}
        </button>
      </div>
    </div>
  );
}
