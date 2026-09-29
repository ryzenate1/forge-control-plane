import type { AppTemplate } from "@/lib/api/apps";

const STORAGE_KEY = "forge.app-templates.v1";

export const DEFAULT_APP_TEMPLATES: AppTemplate[] = [
  {
    id: "nginx",
    name: "Nginx",
    description: "A lightweight production web server and reverse proxy.",
    type: "image",
    image: "nginx:1.27-alpine",
    defaultPorts: [{ hostPort: 8080, containerPort: 80, protocol: "tcp", name: "http" }],
    defaultEnvVars: {},
    defaultResources: { cpu: "0.5", memory: "256", disk: "1024" },
  },
  {
    id: "node",
    name: "Node.js",
    description: "Build and run a Node.js application from a Git repository.",
    type: "git",
    defaultPorts: [{ hostPort: 3000, containerPort: 3000, protocol: "tcp", name: "http" }],
    defaultEnvVars: { NODE_ENV: "production" },
    defaultResources: { cpu: "1", memory: "512", disk: "2048" },
  },
  {
    id: "python",
    name: "Python Web",
    description: "Deploy a Python web service from a Git repository.",
    type: "git",
    defaultPorts: [{ hostPort: 8000, containerPort: 8000, protocol: "tcp", name: "http" }],
    defaultEnvVars: { PYTHONUNBUFFERED: "1" },
    defaultResources: { cpu: "1", memory: "512", disk: "2048" },
  },
  {
    id: "postgres-compose",
    name: "PostgreSQL",
    description: "A PostgreSQL database stack with persistent storage.",
    type: "compose",
    composeContent: "services:\n  postgres:\n    image: postgres:16-alpine\n    environment:\n      POSTGRES_DB: app\n      POSTGRES_USER: app\n      POSTGRES_PASSWORD: change-me\n    ports:\n      - \"5432:5432\"\n    volumes:\n      - postgres-data:/var/lib/postgresql/data\nvolumes:\n  postgres-data:\n",
    defaultPorts: [{ hostPort: 5432, containerPort: 5432, protocol: "tcp", name: "postgres" }],
    defaultEnvVars: { POSTGRES_DB: "app", POSTGRES_USER: "app", POSTGRES_PASSWORD: "" },
    defaultResources: { cpu: "1", memory: "1024", disk: "10240" },
  },
  {
    id: "redis",
    name: "Redis",
    description: "An in-memory cache and queue service.",
    type: "image",
    image: "redis:7-alpine",
    defaultPorts: [{ hostPort: 6379, containerPort: 6379, protocol: "tcp", name: "redis" }],
    defaultEnvVars: {},
    defaultResources: { cpu: "0.5", memory: "256", disk: "1024" },
  },
];

/** Result of reading the browser-local template store.
 *
 * `error` is non-null when the stored value could not be read or parsed. The
 * page must show that instead of rendering an empty list, because an unreadable
 * store is not the same as a store with no templates in it (the operator's own
 * templates would otherwise silently disappear). */
export type StoredTemplatesResult = {
  templates: AppTemplate[];
  error: string | null;
};

export function readStoredTemplates(): StoredTemplatesResult {
  if (typeof window === "undefined") return { templates: [], error: null };
  const raw = window.localStorage.getItem(STORAGE_KEY);
  if (raw === null || raw === "") return { templates: [], error: null };
  try {
    const value: unknown = JSON.parse(raw);
    if (!Array.isArray(value)) {
      return { templates: [], error: `Stored templates at ${STORAGE_KEY} are not a list, so they could not be loaded.` };
    }
    const parsed = value as AppTemplate[];
    // An element that cannot be a template (no id/name) means the store was
    // written by something else or is truncated. Say so rather than showing a
    // shorter list than the operator saved.
    const usable = parsed.filter((entry) => entry && typeof entry.id === "string" && typeof entry.name === "string");
    const dropped = parsed.length - usable.length;
    return {
      templates: usable,
      error: dropped > 0
        ? `${dropped} stored ${dropped === 1 ? "entry is" : "entries are"} malformed and were not loaded.`
        : null,
    };
  } catch {
    return { templates: [], error: `Stored templates at ${STORAGE_KEY} could not be read (corrupted JSON).` };
  }
}

export function loadUserTemplates(): AppTemplate[] {
  return readStoredTemplates().templates;
}

/** Throws on quota/serialization failure; callers must surface it. */
export function saveUserTemplates(templates: AppTemplate[]): void {
  if (typeof window === "undefined") return;
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(templates));
}

export function getAllTemplates(): AppTemplate[] {
  const userTemplates = loadUserTemplates();
  const defaultIDs = new Set(DEFAULT_APP_TEMPLATES.map((template) => template.id));
  return [...DEFAULT_APP_TEMPLATES, ...userTemplates.filter((template) => !defaultIDs.has(template.id))];
}
