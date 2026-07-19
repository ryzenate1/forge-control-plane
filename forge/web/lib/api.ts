import type {
  ApiUser, ApiServer, ApiNode, ApiAllocationNode, ApiAllocation, ApiDatabase, ApiBackup,
  ApiScheduleTask, ApiDatabaseHost, ApiDatabaseHostConnectionTestResult, ApiServerDatabaseDeleteResult, ApiMount, ApiRegion, ApiLocation,
  ApiNest, ApiEgg, ApiRole, ApiAdminAuditEvent, ApiStats,
  AdminScopes, ApiKey, ApiSSHKey, ApiOAuthClient, ApiOAuthClientCreation,
  ApiPlugin, ApiWebhook, ApiWebhookDelivery, ApiMigration, ApiMigrationHistory, ApiMigrationStatus, CreateMigrationInput,
  ApiPanelMailSettings, ApiPanelAdvancedSettings, TwoFactorSetup, ApiNodeConfiguration,
  ApiActivityLog, ApiAuditEvent, ApiFileEntry, ApiServerSubuser,
  ApiStartupVariable, ApiHealthCheck, ApiHealthReport, ApiTemplate, ApiUserSearchResult,
  ApiEvacuationResult, ApiEvacuationPlan, ApiRecoveryItem, ApiRecoveryPlan, ApiReservation, CreateRecoveryPlanInput, ApiLegacyTransferStatus,
  ApiPanelSettings, ApiPublicPanelSettings, ApiNodeLifecycle,
  ApiSetupStatus, ApiSetupRequest, ApiWSTicket, ApiUserSession, LoginResponse,
  ApiOrphanRemediations, ApiDatabaseOrphanRemediation, ApiServerOrphanRemediation,
  ApiSchedule, ServerCreateInput, ServerUpdateInput,
  ScheduleCreateInput, ScheduleUpdateInput,
  ScheduleTaskCreateInput, ScheduleTaskUpdateInput,
  CreateServerDatabaseInput,
  CreateNodeInput, UpdateNodeInput, CreateAllocationInput,
  UpdateAllocationInput, CreateDatabaseHostInput, UpdateDatabaseHostInput,
  CreateMountInput, AssignMountInput, ApiMountAssignmentResponse, RenameFileInput, PatchScheduleTaskInput,
  CreateEggInput, UpdateEggInput, SocialProvider,
} from './api/types';

// Keep the legacy `@/lib/api` entry point compatible with the modular auth API.
export * from './api/auth';

export type {
  ApiUser, ApiServer, ApiNode, ApiAllocationNode, ApiAllocation, ApiDatabase, ApiBackup,
  ApiScheduleTask, ApiDatabaseHost, ApiDatabaseHostConnectionTestResult, ApiServerDatabaseDeleteResult, ApiMount, ApiRegion, ApiLocation,
  ApiNest, ApiEgg, ApiRole, ApiAdminAuditEvent, ApiStats,
  AdminScopes, ApiKey, ApiSSHKey, ApiOAuthClient, ApiOAuthClientCreation,
  ApiPlugin, ApiWebhook, ApiWebhookDelivery, ApiMigration, ApiMigrationHistory, ApiMigrationStatus, CreateMigrationInput,
  ApiPanelMailSettings, ApiPanelAdvancedSettings, TwoFactorSetup, ApiNodeConfiguration,
  ApiActivityLog, ApiAuditEvent, ApiFileEntry, ApiServerSubuser,
  ApiStartupVariable, ApiHealthCheck, ApiHealthReport, ApiTemplate, ApiUserSearchResult,
  ApiEvacuationResult, ApiEvacuationPlan, ApiRecoveryItem, ApiRecoveryPlan, ApiReservation, CreateRecoveryPlanInput, ApiLegacyTransferStatus,
  ApiPanelSettings, ApiPublicPanelSettings, ApiNodeLifecycle, ApiUserSession,
  LoginResponse, ApiSetupStatus, ApiSetupRequest, ApiWSTicket,
  ApiOrphanRemediations, ApiDatabaseOrphanRemediation, ApiServerOrphanRemediation,
  ApiSchedule, ServerCreateInput, ServerUpdateInput,
  ScheduleCreateInput, ScheduleUpdateInput,
  ScheduleTaskCreateInput, ScheduleTaskUpdateInput,
  CreateServerDatabaseInput,
  CreateNodeInput, UpdateNodeInput, CreateAllocationInput,
  UpdateAllocationInput, CreateDatabaseHostInput, UpdateDatabaseHostInput,
  CreateMountInput, AssignMountInput, ApiMountAssignmentResponse, RenameFileInput, PatchScheduleTaskInput,
  CreateEggInput, UpdateEggInput, SocialProvider,
};

// Production deployments use the panel's origin unless an explicit API URL is configured.
// This avoids sending an administrator's browser to its own localhost.
const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ??
  (process.env.NODE_ENV === "development" ? "http://localhost:8080/api/v1" : "/api/v1");

// Beacon accepts either the panel origin or its /api/v1 base. Derive the value
// from the API client configuration instead of the browser origin: web and API
// deployments may be hosted on different origins.
export function getBeaconPanelURL(): string {
  if (/^https?:\/\//i.test(API_BASE_URL)) return API_BASE_URL.replace(/\/$/, "");
  if (typeof window !== "undefined") return new URL(API_BASE_URL, window.location.origin).toString().replace(/\/$/, "");
  return "https://panel.example.com/api/v1";
}
const UPLOAD_CHUNK_SIZE = 8 * 1024 * 1024;

const API_WS_URL = API_BASE_URL.replace(/^http:/, "ws:").replace(
  /^https:/,
  "wss:",
);

/**
 * Gets the CSRF token from the cookie for state-changing requests
 */
function getCSRFToken(): string | null {
  if (typeof window === "undefined") return null;
  try {
    const cookies = document.cookie.split("; ");
    for (const cookie of cookies) {
      if (cookie.startsWith("__Host-forge_csrf=")) {
        return cookie.split("=")[1];
      }
    }
  } catch {
    // Cookie access can fail in privacy-restricted contexts
  }
  return null;
}

/**
 * Builds headers for API requests
 * - For mutations (POST, PATCH, DELETE): includes CSRF token
 * - For GET/HEAD: no CSRF needed
 */
function buildHeaders(
  method: string,
  extraHeaders: Record<string, string> = {},
): Record<string, string> {
  const headers: Record<string, string> = {
    Accept: "application/json",
    ...extraHeaders,
  };

  // Add CSRF token for state-changing methods
  if (["POST", "PATCH", "PUT", "DELETE"].includes(method.toUpperCase())) {
    const csrfToken = getCSRFToken();
    if (csrfToken) {
      headers["X-CSRF-Token"] = csrfToken;
    }
  }

  return headers;
}

/**
 * Core fetch wrapper that uses HttpOnly cookies for authentication
 */
async function apiFetch<T>(
  path: string,
  options: RequestInit = {},
  preserveDataArrayEnvelope = false,
): Promise<T> {
  const method = options.method ?? "GET";
  const headers = buildHeaders(method, options.headers as Record<string, string>);

  const response = await fetch(`${API_BASE_URL}${path}`, {
    ...options,
    headers,
    credentials: "include", // Critical: sends HttpOnly cookies
  });

  if (!response.ok) {
    const errorMessage = await getErrorMessage(response, `API ${method} ${path} failed with`);
    throw new Error(errorMessage);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const text = await response.text();
  if (!text) {
    return undefined as T;
  }

  let body: unknown;
  try {
    body = JSON.parse(text);
  } catch {
    throw new Error(`API ${method} ${path} returned invalid JSON`);
  }

  // Admin list routes may use { data: [...] }, while older routes return the
  // array directly. Preserve envelopes only for callers that expose metadata.
  if (!preserveDataArrayEnvelope && isDataArrayEnvelope(body)) {
    return body.data as T;
  }
  return body as T;
}

function isDataArrayEnvelope(value: unknown): value is { data: unknown[] } {
  return typeof value === "object" && value !== null && Array.isArray((value as { data?: unknown }).data);
}

/**
 * Helper to extract error message from response body
 */
async function getErrorMessage(response: Response, defaultPrefix: string): Promise<string> {
  const fallback = `${defaultPrefix} ${response.status}`;
  try {
    const text = await response.text();
    if (!text) return fallback;
    try {
      const body = JSON.parse(text) as { message?: unknown; error?: unknown };
      if (typeof body.message === "string" && body.message.trim()) return body.message;
      if (typeof body.error === "string" && body.error.trim()) return body.error;
      if (body.error && typeof body.error === "object" && "message" in body.error) {
        const message = (body.error as { message?: unknown }).message;
        if (typeof message === "string" && message.trim()) return message;
      }
    } catch {
      // Preserve a useful non-JSON response below.
    }
    return text;
  } catch {
    return fallback;
  }
}

// No mock fallbacks -- all data comes from the real backend API

export async function fetchPublicPanelSettings(): Promise<ApiPublicPanelSettings> {
  const response = await fetch(`${API_BASE_URL}/panel/settings/public`, {
    headers: { Accept: "application/json" },
    credentials: "include",
  });
  if (!response.ok) {
    throw new Error(`Panel settings request failed with ${response.status}`);
  }
  return response.json() as Promise<ApiPublicPanelSettings>;
}

export async function fetchSetupStatus(): Promise<ApiSetupStatus> {
  const response = await fetch(`${API_BASE_URL}/setup/status`, {
    headers: { Accept: "application/json" },
    credentials: "include",
  });
  if (!response.ok) {
    const errorMessage = await getErrorMessage(response, "Setup status request failed with");
    throw new Error(errorMessage);
  }
  return response.json() as Promise<ApiSetupStatus>;
}

export async function runSetup(
  req: ApiSetupRequest,
): Promise<{ ok: boolean; userId: string; email: string }> {
  const response = await fetch(`${API_BASE_URL}/setup`, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify(req),
    credentials: "include",
  });
  if (!response.ok) {
    const body = await response.text();
    throw new Error(`Setup failed (${response.status}): ${body}`);
  }
  return response.json() as Promise<{ ok: boolean; userId: string; email: string }>;
}

export async function fetchJSON<T>(path: string): Promise<T> {
  return apiFetch<T>(path);
}

async function getJSON<T>(path: string): Promise<T> {
  return fetchJSON<T>(path);
}

export async function postJSON<T>(path: string, body?: unknown): Promise<T> {
  return apiFetch<T>(path, {
    method: "POST",
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

export async function putJSON<T>(path: string, body?: unknown): Promise<T> {
  return apiFetch<T>(path, {
    method: "PUT",
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
}

export async function patchJSON<T>(path: string, body: unknown): Promise<T> {
  return apiFetch<T>(path, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export async function deleteJSON(path: string, body?: unknown): Promise<void> {
  const options: RequestInit = {
    method: "DELETE",
  };
  if (body) {
    options.headers = { "Content-Type": "application/json" };
    options.body = JSON.stringify(body);
  }
  await apiFetch<void>(path, options);
}

export function serverWebSocketURL(
  serverId: string,
  stream: "stats" | "logs" | "console",
): string {
  const base = API_BASE_URL.replace("/api/v1", "");
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const wsBase = base.replace(/^https?:/, protocol);
  // Uses ticket-based auth - ticket is fetched at connection time
  return `${wsBase}/api/v1/servers/${encodeURIComponent(serverId)}/ws/${stream}`;
}

export async function verifyBearerToken(token: string): Promise<ApiUser> {
  const response = await fetch(`${API_BASE_URL}/auth/me`, {
    headers: {
      Accept: "application/json",
      Authorization: `Bearer ${token}`,
    },
    credentials: "include",
  });
  if (!response.ok) {
    const errorMessage = await getErrorMessage(response, "Token verification failed with");
    throw new Error(errorMessage);
  }
  return response.json() as Promise<ApiUser>;
}

export async function fetchUsers(): Promise<ApiUser[]> {
  return apiFetch<ApiUser[]>("/users");
}

export async function fetchNodes(): Promise<ApiNode[]> {
  return apiFetch<ApiNode[]>("/nodes");
}

export async function fetchNode(id: string): Promise<ApiNode> {
  return apiFetch<ApiNode>(`/nodes/${encodeURIComponent(id)}`);
}

export async function createNode(
  input: CreateNodeInput,
): Promise<{ node: ApiNode; token: string }> {
  return apiFetch<{ node: ApiNode; token: string }>("/nodes", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function updateNode(
  id: string,
  input: UpdateNodeInput,
): Promise<ApiNode> {
  return apiFetch<ApiNode>(`/nodes/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteNode(id: string): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/nodes/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function rotateNodeToken(id: string): Promise<{ token: string }> {
  return apiFetch<{ token: string }>(
    `/nodes/${encodeURIComponent(id)}/rotate-token`,
    { method: "POST" },
  );
}

export async function fetchNodeConfiguration(
  id: string,
): Promise<ApiNodeConfiguration> {
  return apiFetch<ApiNodeConfiguration>(`/nodes/${encodeURIComponent(id)}/configuration`);
}

export async function fetchNodeAllocations(
  id: string,
): Promise<ApiAllocation[]> {
  return apiFetch<ApiAllocation[]>(`/nodes/${encodeURIComponent(id)}/allocations`);
}

export async function fetchNodeServers(id: string): Promise<ApiServer[]> {
  return apiFetch<ApiServer[]>(`/nodes/${encodeURIComponent(id)}/servers`);
}

export async function fetchNodeHealth(id: string): Promise<any> {
  return apiFetch<any>(`/nodes/${encodeURIComponent(id)}/health`);
}

export async function fetchNodeLifecycle(id: string): Promise<ApiNodeLifecycle> {
  return apiFetch<ApiNodeLifecycle>(`/nodes/${encodeURIComponent(id)}/lifecycle`);
}

export async function fetchNodeCapacity(id: string): Promise<any> {
  return apiFetch<any>(`/nodes/${encodeURIComponent(id)}/capacity`);
}

export async function sendServerCommand(
  serverId: string,
  command: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/command`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ command }),
    },
  );
}

export async function downloadFileToServer(
  serverId: string,
  url: string,
  path: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/files/download`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url, path }),
    },
  );
}

export async function runServerOperations(
  serverId: string,
  operations: Array<{ action: string; args: Record<string, unknown> }>,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/operations`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ operations }),
    },
  );
}

export async function fetchServerConfiguration(id: string): Promise<any> {
  return apiFetch<any>(`/servers/${encodeURIComponent(id)}/configuration`);
}

export async function fetchServerScheduleTasks(
  serverId: string,
  scheduleId: string,
): Promise<ApiScheduleTask[]> {
  return apiFetch<ApiScheduleTask[]>(
    `/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}/tasks`,
  );
}

export async function updateServerScheduleTask(
  serverId: string,
  scheduleId: string,
  taskId: string,
  input: PatchScheduleTaskInput,
): Promise<ApiScheduleTask> {
  return apiFetch<ApiScheduleTask>(
    `/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}/tasks/${encodeURIComponent(taskId)}`,
    {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    },
  );
}

export async function fetchUser(id: string): Promise<ApiUser> {
  return apiFetch<ApiUser>(`/users/${encodeURIComponent(id)}`);
}

export async function fetchServerSubuser(
  serverId: string,
  userId: string,
): Promise<any> {
  return apiFetch<any>(
    `/servers/${encodeURIComponent(serverId)}/users/${encodeURIComponent(userId)}`,
  );
}

export async function fetchDatabaseHost(id: string): Promise<ApiDatabaseHost> {
  return apiFetch<ApiDatabaseHost>(`/database-hosts/${encodeURIComponent(id)}`);
}

export async function fetchMount(id: string): Promise<any> {
  return apiFetch<any>(`/mounts/${encodeURIComponent(id)}`);
}

export type PaginationMetadata = {
  current: number;
  total: number;
  count: number;
  per_page: number;
  total_records: number;
};

export type PaginatedResponse<T> = {
  data: T[];
  meta?: {
    pagination?: PaginationMetadata;
  };
};

/** Fetch one server page and retain the response pagination metadata. */
export async function fetchServersPage(
  page = 1,
  perPage = 100,
): Promise<PaginatedResponse<ApiServer>> {
  const response = await apiFetch<ApiServer[] | PaginatedResponse<ApiServer>>(
    `/servers?page=${page}&per_page=${perPage}`,
    {},
    true,
  );
  return Array.isArray(response) ? { data: response } : response;
}

/** Fetch every server page so aggregate views are not limited to the API default page size. */
export async function fetchAllServers(): Promise<ApiServer[]> {
  const firstPage = await fetchServersPage();
  const pagination = firstPage.meta?.pagination;
  if (!pagination || pagination.total <= 1) {
    return firstPage.data ?? [];
  }

  const remainingPages = await Promise.all(
    Array.from({ length: pagination.total - 1 }, (_, index) => fetchServersPage(index + 2)),
  );
  return [
    ...firstPage.data,
    ...remainingPages.flatMap((response) => response.data ?? []),
  ];
}

/**
 * Backward-compatible array API. It now includes every server rather than only
 * the API's default page, which keeps aggregate consumers complete.
 */
export async function fetchServers(): Promise<ApiServer[]> {
  return fetchAllServers();
}

export async function fetchServer(id: string): Promise<ApiServer> {
  return apiFetch<ApiServer>(`/servers/${encodeURIComponent(id)}`);
}

export async function fetchTemplates(): Promise<ApiEgg[]> {
  return apiFetch<ApiEgg[]>("/eggs");
}

export async function fetchAllocationNodes(): Promise<ApiAllocationNode[]> {
  return apiFetch<ApiAllocationNode[]>("/allocations/nodes");
}

export async function fetchAllocations(): Promise<ApiAllocation[]> {
  return apiFetch<ApiAllocation[]>("/allocations");
}

export async function updateServerAllocation(
  serverId: string,
  allocationId: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/allocations/${encodeURIComponent(allocationId)}`,
    { method: "POST" },
  );
}

function requireArrayResponse<T>(value: unknown, endpoint: string): T[] {
  if (!Array.isArray(value)) {
    throw new Error(`Unexpected response from ${endpoint}: expected an array`);
  }
  return value as T[];
}

function requireHealthReport(value: unknown): ApiHealthReport {
  if (
    !value ||
    typeof value !== "object" ||
    !Array.isArray((value as { checks?: unknown }).checks) ||
    typeof (value as { status?: unknown }).status !== "string" ||
    typeof (value as { checkedAt?: unknown }).checkedAt !== "string"
  ) {
    throw new Error("Unexpected response from /health: expected a diagnostic report");
  }
  return value as ApiHealthReport;
}

export async function fetchHealthStatus(): Promise<ApiHealthReport> {
  // This is the diagnostic report. Readiness and liveness have automation-only semantics.
  return requireHealthReport(await apiFetch<unknown>("/health"));
}

export async function fetchReservations(): Promise<ApiReservation[]> {
  return requireArrayResponse<ApiReservation>(await apiFetch<unknown>("/reservations"), "/reservations");
}

export async function fetchRecoveryPlans(): Promise<ApiRecoveryPlan[]> {
  return requireArrayResponse<ApiRecoveryPlan>(await apiFetch<unknown>("/recovery-plans"), "/recovery-plans");
}

export async function fetchRecoveryPlan(id: string): Promise<ApiRecoveryPlan> {
  return apiFetch<ApiRecoveryPlan>(`/recovery-plans/${encodeURIComponent(id)}`);
}

export async function fetchRegions(): Promise<ApiRegion[]> {
  return apiFetch<ApiRegion[]>("/regions");
}

export async function createAllocation(input: CreateAllocationInput): Promise<ApiAllocation[]> {
  return apiFetch<ApiAllocation[]>("/allocations", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function updateAllocation(
  id: string,
  input: UpdateAllocationInput,
): Promise<ApiAllocation> {
  return apiFetch<ApiAllocation>(`/allocations/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteAllocation(id: string): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/allocations/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

/** Global admin allocation alias endpoint. */
export async function setAdminAllocationAlias(id: string, alias: string): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/allocations/${encodeURIComponent(id)}/alias`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ alias }),
  });
}



/** Deletes free allocations through the admin allocation inventory endpoint. */
export async function deleteAllocations(ids: string[]): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>("/allocations/bulk", {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ids }),
  });
}

export async function fetchAuditEvents(): Promise<ApiAdminAuditEvent[]> {
  return apiFetch<ApiAdminAuditEvent[]>("/admin/audit");
}

export async function fetchServerStats(serverId: string): Promise<ApiStats> {
  return apiFetch<ApiStats>(`/servers/${encodeURIComponent(serverId)}/stats`);
}

export async function fetchServerLogs(serverId: string): Promise<string> {
  const response = await fetch(`${API_BASE_URL}/servers/${encodeURIComponent(serverId)}/logs`, {
    headers: { Accept: "application/json" },
    credentials: "include",
  });
  if (!response.ok) {
    throw new Error(`Logs request failed with ${response.status}`);
  }
  return response.text();
}

export async function fetchServerStartup(
  serverId: string,
): Promise<{ startupCommand: string; startup_command?: string; raw_startup_command?: string; docker_images?: Record<string, string>; variables: Record<string, string> | ApiStartupVariable[] }> {
  return apiFetch<{ startupCommand: string; startup_command?: string; raw_startup_command?: string; docker_images?: Record<string, string>; variables: Record<string, string> | ApiStartupVariable[] }>(
    `/servers/${encodeURIComponent(serverId)}/startup`,
  );
}

export async function fetchServerDatabases(
  serverId: string,
): Promise<ApiDatabase[]> {
  return apiFetch<ApiDatabase[]>(
    `/servers/${encodeURIComponent(serverId)}/databases`,
  );
}

export async function createServerDatabase(
  serverId: string,
  input: CreateServerDatabaseInput,
): Promise<ApiDatabase> {
  return apiFetch<ApiDatabase>(
    `/servers/${encodeURIComponent(serverId)}/databases`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    },
  );
}

export async function rotateServerDatabasePassword(
  serverId: string,
  databaseId: string,
): Promise<{ password: string }> {
  return apiFetch<{ password: string }>(
    `/servers/${encodeURIComponent(serverId)}/databases/${encodeURIComponent(databaseId)}/rotate-password`,
    { method: "POST" },
  );
}

export async function deleteServerDatabase(
  serverId: string,
  databaseId: string,
  force = false,
): Promise<ApiServerDatabaseDeleteResult> {
  const query = force ? "?force=true" : "";
  return apiFetch<ApiServerDatabaseDeleteResult>(
    `/servers/${encodeURIComponent(serverId)}/databases/${encodeURIComponent(databaseId)}${query}`,
    { method: "DELETE" },
  );
}

export async function fetchOrphanRemediations(status?: "pending" | "resolved"): Promise<ApiOrphanRemediations> {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  return apiFetch<ApiOrphanRemediations>(`/admin/orphan-remediations/${query}`);
}

export async function resolveDatabaseOrphanRemediation(id: string): Promise<ApiDatabaseOrphanRemediation> {
  return apiFetch<ApiDatabaseOrphanRemediation>(`/admin/orphan-remediations/databases/${encodeURIComponent(id)}/resolve`, { method: "POST" });
}

export async function resolveServerOrphanRemediation(id: string): Promise<ApiServerOrphanRemediation> {
  return apiFetch<ApiServerOrphanRemediation>(`/admin/orphan-remediations/servers/${encodeURIComponent(id)}/resolve`, { method: "POST" });
}

export async function fetchDatabaseHosts(): Promise<ApiDatabaseHost[]> {
  return apiFetch<ApiDatabaseHost[]>("/database-hosts");
}

export async function createDatabaseHost(
  input: CreateDatabaseHostInput,
): Promise<ApiDatabaseHost> {
  return apiFetch<ApiDatabaseHost>("/database-hosts", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function updateDatabaseHost(
  id: string,
  input: UpdateDatabaseHostInput,
): Promise<ApiDatabaseHost> {
  return apiFetch<ApiDatabaseHost>(`/database-hosts/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteDatabaseHost(id: string): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/database-hosts/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export function testDatabaseHostConnection(id: string): Promise<ApiDatabaseHostConnectionTestResult>;
export function testDatabaseHostConnection(input: CreateDatabaseHostInput): Promise<ApiDatabaseHostConnectionTestResult>;
export async function testDatabaseHostConnection(inputOrID: CreateDatabaseHostInput | string): Promise<ApiDatabaseHostConnectionTestResult> {
  if (typeof inputOrID === "string") {
    return apiFetch<ApiDatabaseHostConnectionTestResult>(
      `/database-hosts/${encodeURIComponent(inputOrID)}/test`,
      { method: "POST" },
    );
  }

  return apiFetch<ApiDatabaseHostConnectionTestResult>("/database-hosts/test", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(inputOrID),
  });
}

export async function fetchMounts(): Promise<ApiMount[]> {
  return apiFetch<ApiMount[]>("/mounts");
}

export async function createMount(input: CreateMountInput): Promise<ApiMount> {
  return apiFetch<ApiMount>("/mounts", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function updateMount(
  id: string,
  input: Partial<CreateMountInput>,
): Promise<ApiMount> {
  return apiFetch<ApiMount>(`/mounts/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteMount(id: string): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/mounts/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function fetchServerMounts(serverId: string): Promise<ApiMount[]> {
  return apiFetch<ApiMount[]>(`/servers/${encodeURIComponent(serverId)}/mounts`);
}

export async function assignMountToServer(
  serverId: string,
  input: AssignMountInput,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/mounts`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    },
  );
}

export async function unassignMountFromServer(
  serverId: string,
  mountId: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/mounts/${encodeURIComponent(mountId)}`,
    { method: "DELETE" },
  );
}

export async function fetchServerFiles(
  serverId: string,
  path: string = "/",
): Promise<any> {
  return apiFetch<any>(
    `/servers/${encodeURIComponent(serverId)}/files?path=${encodeURIComponent(path)}`,
  );
}

export async function writeServerFile(
  serverId: string,
  path: string,
  content: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/files${encodeURIComponent(path)}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content }),
    },
  );
}

export async function deleteServerFile(
  serverId: string,
  path: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/files${encodeURIComponent(path)}`,
    { method: "DELETE" },
  );
}

export async function renameServerFile(
  serverId: string,
  input: RenameFileInput,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/files/rename`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    },
  );
}

export async function createServerArchive(
  serverId: string,
  path: string,
): Promise<{ ok: boolean; name: string }> {
  return apiFetch<{ ok: boolean; name: string }>(
    `/servers/${encodeURIComponent(serverId)}/files/archive`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path }),
    },
  );
}

export async function extractServerArchive(
  serverId: string,
  archive: string,
  path: string = "/",
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/files/extract`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ archive, path }),
    },
  );
}

export async function chmodServerFile(
  serverId: string,
  path: string,
  mode: number,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/files/chmod`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path, mode }),
    },
  );
}

export async function getFileDownloadUrl(
  serverId: string,
  path: string,
): Promise<{ url: string; expires: string }> {
  return apiFetch<{ url: string; expires: string }>(
    `/servers/${encodeURIComponent(serverId)}/files/download-url?path=${encodeURIComponent(path)}`,
  );
}

export async function fetchBackups(serverId: string): Promise<ApiBackup[]> {
  return apiFetch<ApiBackup[]>(`/servers/${encodeURIComponent(serverId)}/backups`);
}

export async function createBackup(
  serverId: string,
  name?: string,
  ignored?: string[],
): Promise<{ uuid: string; name: string }> {
  return apiFetch<{ uuid: string; name: string }>(
    `/servers/${encodeURIComponent(serverId)}/backups`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, ignored }),
    },
  );
}

export async function restoreBackup(
  serverId: string,
  backupName: string,
  truncate?: boolean,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/backups/${encodeURIComponent(backupName)}/restore`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ truncate }),
    },
  );
}

export async function deleteBackup(
  serverId: string,
  backupName: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/backups/${encodeURIComponent(backupName)}`,
    { method: "DELETE" },
  );
}

export async function fetchAdminScopes(): Promise<AdminScopes> {
  return apiFetch<AdminScopes>("/admin-scopes");
}

export async function fetchApiKeys(): Promise<ApiKey[]> {
  return apiFetch<ApiKey[]>("/api-keys");
}

export async function createApiKey(input: {
  description: string;
  scopes: string[];
  allowedIps?: string[];
}): Promise<ApiKey> {
  return apiFetch<ApiKey>("/api-keys", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteApiKey(id: string): Promise<void> {
  await apiFetch<void>(`/api-keys/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function fetchSSHKeys(): Promise<ApiSSHKey[]> {
  return apiFetch<ApiSSHKey[]>("/ssh-keys");
}

export async function createSSHKey(input: {
  name: string;
  publicKey: string;
}): Promise<ApiSSHKey> {
  return apiFetch<ApiSSHKey>("/ssh-keys", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteSSHKey(fingerprint: string): Promise<void> {
  await apiFetch<void>(`/ssh-keys/${encodeURIComponent(fingerprint)}`, {
    method: "DELETE",
  });
}

export async function setupTwoFactor(): Promise<TwoFactorSetup> {
  return apiFetch<TwoFactorSetup>("/account/two-factor", {
    method: "GET",
  });
}

export async function enableTwoFactor(input: {
  code: string;
  password: string;
}): Promise<{ tokens: string[] }> {
  return apiFetch<{ tokens: string[] }>("/account/two-factor", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function disableTwoFactor(password: string): Promise<void> {
  await apiFetch<void>("/account/two-factor", {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ password }),
  });
}

export async function fetchActivityLogs(): Promise<ApiActivityLog[]> {
  return apiFetch<ApiActivityLog[]>("/account/activity");
}

export type AdminActivityFilter = {
  actorId?: string;
  subjectType?: string;
  subjectId?: string;
  event?: string;
  level?: string;
  source?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
};

export type AdminActivityPage = {
  events: ApiActivityLog[];
  total: number;
};

/** Fetch recent platform-wide activity for admin monitoring. */
export async function fetchPlatformActivityLogs(): Promise<ApiActivityLog[]> {
  const page = await fetchAdminActivity({ limit: 50 });
  return page.events;
}

function adminActivityQuery(filter: AdminActivityFilter = {}): string {
  const params = new URLSearchParams();
  if (filter.actorId) params.set("actorId", filter.actorId);
  if (filter.subjectType) params.set("subjectType", filter.subjectType);
  if (filter.subjectId) params.set("subjectId", filter.subjectId);
  if (filter.event) params.set("event", filter.event);
  if (filter.level) params.set("level", filter.level);
  if (filter.source) params.set("source", filter.source);
  if (filter.from) params.set("from", filter.from);
  if (filter.to) params.set("to", filter.to);
  if (filter.limit != null) params.set("limit", String(filter.limit));
  if (filter.offset != null) params.set("offset", String(filter.offset));
  const query = params.toString();
  return query ? `?${query}` : "";
}

export async function fetchAdminActivity(filter: AdminActivityFilter = {}): Promise<AdminActivityPage> {
  return apiFetch<AdminActivityPage>(`/admin/activity${adminActivityQuery(filter)}`);
}

export async function fetchAdminAudit(): Promise<ApiAdminAuditEvent[]> {
  return apiFetch<ApiAdminAuditEvent[]>("/admin/audit");
}

export async function exportAdminActivity(format: "csv" | "json", filter: AdminActivityFilter = {}): Promise<Blob> {
  const query = new URLSearchParams(adminActivityQuery(filter).slice(1));
  query.set("format", format);
  const response = await fetch(`${API_BASE_URL}/admin/activity/export?${query.toString()}`, {
    headers: { Accept: format === "csv" ? "text/csv" : "application/json" },
    credentials: "include",
  });
  if (!response.ok) {
    const errorMessage = await getErrorMessage(response, "Activity export failed with");
    throw new Error(errorMessage);
  }
  return response.blob();
}

export async function fetchPermissions(): Promise<Record<string, Record<string, string>>> {
  const result = await apiFetch<{ permissions: Record<string, Record<string, string>> }>("/permissions");
  return result.permissions;
}

export async function searchUsers(query: string): Promise<ApiUser[]> {
  return apiFetch<ApiUser[]>(`/users/search?q=${encodeURIComponent(query)}`);
}

export async function fetchNodeSystemInformation(
  nodeId: string,
): Promise<any> {
  return apiFetch<any>(`/nodes/${encodeURIComponent(nodeId)}/system-information`);
}

export async function generateNodeConfigToken(
  nodeId: string,
): Promise<{ token: string }> {
  return apiFetch<{ token: string }>(
    `/nodes/${encodeURIComponent(nodeId)}/configuration/token`,
    { method: "POST" },
  );
}

export async function setAllocationAlias(
  nodeId: string,
  allocationId: string,
  alias: string,
): Promise<void> {
  await apiFetch<void>(
    `/nodes/${encodeURIComponent(nodeId)}/allocations/alias`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ allocation_id: allocationId, alias }),
    },
  );
}

export async function deleteAllocationsBulk(nodeId: string, ids: string[]): Promise<void> {
  await apiFetch<void>(`/nodes/${encodeURIComponent(nodeId)}/allocations/bulk`, {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ allocations: ids.map((id) => ({ id })) }),
  });
}

export async function deleteNodeAllocation(
  nodeId: string,
  allocationId: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/nodes/${encodeURIComponent(nodeId)}/allocations/${encodeURIComponent(allocationId)}`,
    { method: "DELETE" },
  );
}

export async function rotateServerDatabasePasswordByBody(
  serverId: string,
  database: string,
): Promise<{ password: string }> {
  return apiFetch<{ password: string }>(
    `/servers/${encodeURIComponent(serverId)}/databases/${encodeURIComponent(database)}/rotate-password`,
    { method: "POST" },
  );
}

export async function deleteServerDatabaseWithSuffix(
  serverId: string,
  database: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/databases/${encodeURIComponent(database)}`,
    { method: "DELETE" },
  );
}

export async function fetchMailSettings(): Promise<ApiPanelMailSettings> {
  return apiFetch<ApiPanelMailSettings>("/admin/settings/mail");
}

export async function saveMailSettings(input: ApiPanelMailSettings): Promise<void> {
  await apiFetch<void>("/admin/settings/mail", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function testMailSettings(email: string): Promise<{ sent: boolean; status: string; message?: string }> {
  return apiFetch<{ sent: boolean; status: string; message?: string }>("/admin/settings/mail/test", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email }),
  });
}

export async function fetchAdvancedSettings(): Promise<ApiPanelAdvancedSettings> {
  return apiFetch<ApiPanelAdvancedSettings>("/admin/settings/advanced");
}

export async function saveAdvancedSettings(input: ApiPanelAdvancedSettings): Promise<void> {
  await apiFetch<void>("/admin/settings/advanced", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function fetchWSTicket(
  serverId: string,
  stream: string,
): Promise<ApiWSTicket> {
  return apiFetch<ApiWSTicket>(
    `/servers/${encodeURIComponent(serverId)}/ws/ticket?stream=${encodeURIComponent(stream)}`,
    { method: "POST" },
  );
}

export async function connectServerWebSocket(
  serverId: string,
  stream: "console" | "stats" | "logs",
): Promise<WebSocket> {
  const ticketResponse = await fetchWSTicket(serverId, stream);
  const url = serverWebSocketURL(serverId, stream) + `?token=${encodeURIComponent(ticketResponse.token)}`;
  return new WebSocket(url);
}

export async function fetchPanelSettings(): Promise<ApiPanelSettings> {
  return apiFetch<ApiPanelSettings>("/admin/settings");
}

export async function savePanelSettings(
  input: ApiPanelSettings,
): Promise<ApiPanelSettings> {
  return apiFetch<ApiPanelSettings>("/admin/settings", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function fetchRoles(): Promise<ApiRole[]> {
  return apiFetch<ApiRole[]>("/admin/roles");
}

export async function createRole(input: {
  key: string;
  name: string;
  isAdmin: boolean;
}): Promise<ApiRole> {
  return apiFetch<ApiRole>("/admin/roles", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteRole(id: string): Promise<void> {
  await apiFetch<void>(`/admin/roles/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function fetchUserRoles(userId: string): Promise<string[]> {
  return apiFetch<string[]>(`/admin/users/${encodeURIComponent(userId)}/roles`);
}

export async function assignUserRoles(
  userId: string,
  roleKeys: string[],
): Promise<void> {
  await apiFetch<void>(`/admin/users/${encodeURIComponent(userId)}/roles/assign`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ roles: roleKeys }),
  });
}

export async function removeUserRoles(
  userId: string,
  roleKeys: string[],
): Promise<void> {
  await apiFetch<void>(`/admin/users/${encodeURIComponent(userId)}/roles/remove`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ roles: roleKeys }),
  });
}

export async function fetchPlugins(): Promise<ApiPlugin[]> {
  return apiFetch<ApiPlugin[]>("/plugins");
}

export async function importPluginFromURL(url: string): Promise<ApiPlugin> {
  return apiFetch<ApiPlugin>("/plugins/import", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
  });
}

export async function deletePlugin(id: string): Promise<void> {
  await apiFetch<void>(`/plugins/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function fetchMyOAuthClients(): Promise<ApiOAuthClient[]> {
  return apiFetch<ApiOAuthClient[]>("/account/oauth-clients");
}

export async function createMyOAuthClient(input: {
  name: string;
  description?: string;
  redirectUri?: string;
  scopes: string[];
  allowedScopes?: string[];
}): Promise<ApiOAuthClient> {
  return apiFetch<ApiOAuthClient>("/account/oauth-clients", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteMyOAuthClient(id: string): Promise<void> {
  await apiFetch<void>(`/account/oauth-clients/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function fetchAdminOAuthClients(userId: string): Promise<ApiOAuthClient[]> {
  return apiFetch<ApiOAuthClient[]>(`/admin/users/${encodeURIComponent(userId)}/oauth-clients`);
}

export async function createAdminOAuthClient(input: {
  userId: string;
  name: string;
  description?: string;
  redirectUri?: string;
  scopes: string[];
  scope?: string;
  ownerId?: string;
  serverId?: string;
  allowedScopes?: string[];
}): Promise<ApiOAuthClient> {
  return apiFetch<ApiOAuthClient>("/admin/oauth-clients", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteAdminOAuthClient(id: string): Promise<void> {
  await apiFetch<void>(`/admin/oauth-clients/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function fetchWebhookDeliveries(
  webhookId: string,
  limit = 100,
): Promise<ApiWebhookDelivery[]> {
  return apiFetch<ApiWebhookDelivery[]>(
    `/admin/webhooks/${encodeURIComponent(webhookId)}/deliveries?limit=${limit}`,
  );
}

export async function fetchMigrations(): Promise<ApiMigration[]> {
  return apiFetch<ApiMigration[]>("/admin/migrations");
}

export async function fetchMigration(id: string): Promise<ApiMigration> {
  return apiFetch<ApiMigration>(`/migrations/${encodeURIComponent(id)}`);
}

export async function createMigration(input: CreateMigrationInput): Promise<ApiMigration> {
  return apiFetch<ApiMigration>("/migrations", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function cancelMigration(id: string): Promise<ApiMigration> {
  return apiFetch<ApiMigration>(`/migrations/${encodeURIComponent(id)}/cancel`, {
    method: "PATCH",
  });
}

export async function prepareMigration(id: string): Promise<ApiMigration> {
  return apiFetch<ApiMigration>(`/migrations/${encodeURIComponent(id)}/prepare`, {
    method: "POST",
  });
}

export async function executeMigration(id: string): Promise<ApiMigration> {
  return apiFetch<ApiMigration>(`/migrations/${encodeURIComponent(id)}/execute`, {
    method: "POST",
  });
}

export async function previewEvacuation(nodeId: string): Promise<ApiEvacuationResult> {
  return apiFetch<ApiEvacuationResult>(`/nodes/${encodeURIComponent(nodeId)}/evacuation-preview`);
}

export async function createEvacuationPlan(nodeId: string): Promise<ApiEvacuationResult> {
  return apiFetch<ApiEvacuationResult>(`/nodes/${encodeURIComponent(nodeId)}/evacuation-plan`, {
    method: "POST",
  });
}

export async function fetchEvacuationPlan(id: string): Promise<ApiEvacuationPlan> {
  return apiFetch<ApiEvacuationPlan>(`/evacuations/${encodeURIComponent(id)}`);
}

export async function executeEvacuationPlan(planId: string): Promise<ApiEvacuationPlan> {
  return apiFetch<ApiEvacuationPlan>("/evacuations", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ planId }),
  });
}

export async function cancelEvacuationPlan(id: string): Promise<ApiEvacuationPlan> {
  return apiFetch<ApiEvacuationPlan>(`/evacuations/${encodeURIComponent(id)}/cancel`, {
    method: "POST",
  });
}

export async function createRecoveryPlan(input: CreateRecoveryPlanInput): Promise<ApiRecoveryPlan> {
  return apiFetch<ApiRecoveryPlan>("/recovery-plans", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function executeRecoveryPlan(planId: string): Promise<ApiRecoveryPlan> {
  return apiFetch<ApiRecoveryPlan>("/recovery", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ planId }),
  });
}

export async function startRecoveryPlan(id: string): Promise<ApiRecoveryPlan> {
  return apiFetch<ApiRecoveryPlan>(`/recovery/${encodeURIComponent(id)}/start`, {
    method: "POST",
  });
}

export async function cancelRecoveryPlan(id: string): Promise<ApiRecoveryPlan> {
  return apiFetch<ApiRecoveryPlan>(`/recovery/${encodeURIComponent(id)}/cancel`, {
    method: "POST",
  });
}

export async function createRegion(input: {
  name: string;
  slug: string;
  description: string;
  enabled: boolean;
}): Promise<ApiRegion> {
  return apiFetch<ApiRegion>("/regions", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function updateRegion(
  id: string,
  input: { name: string; slug: string; description: string; enabled: boolean },
): Promise<ApiRegion> {
  return apiFetch<ApiRegion>(`/regions/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteRegion(id: string): Promise<void> {
  await apiFetch<void>(`/regions/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function fetchLocations(): Promise<ApiLocation[]> {
  return apiFetch<ApiLocation[]>("/locations");
}

export async function fetchLocation(id: string): Promise<ApiLocation> {
  return apiFetch<ApiLocation>(`/locations/${encodeURIComponent(id)}`);
}

export async function createLocation(input: {
  short: string;
  long: string;
}): Promise<ApiLocation> {
  return apiFetch<ApiLocation>("/locations", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function updateLocation(
  id: string,
  input: { short?: string; long?: string },
): Promise<ApiLocation> {
  return apiFetch<ApiLocation>(`/locations/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteLocation(id: string): Promise<void> {
  await apiFetch<void>(`/locations/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function fetchNests(): Promise<ApiNest[]> {
  return apiFetch<ApiNest[]>("/nests");
}

export async function fetchNest(id: string): Promise<ApiNest> {
  return apiFetch<ApiNest>(`/nests/${encodeURIComponent(id)}`);
}

export async function createNest(input: {
  name: string;
  description?: string;
}): Promise<ApiNest> {
  return apiFetch<ApiNest>("/nests", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function updateNest(
  id: string,
  input: { name?: string; description?: string },
): Promise<ApiNest> {
  return apiFetch<ApiNest>(`/nests/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteNest(id: string): Promise<void> {
  await apiFetch<void>(`/nests/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function fetchEggs(nestId: string = "*"): Promise<ApiEgg[]> {
  return apiFetch<ApiEgg[]>(`/nests/${encodeURIComponent(nestId)}/eggs`);
}

export async function fetchEgg(id: string): Promise<ApiEgg> {
  return apiFetch<ApiEgg>(`/eggs/${encodeURIComponent(id)}`);
}

export async function createEgg(input: CreateEggInput): Promise<ApiEgg> {
  return apiFetch<ApiEgg>("/eggs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function updateEgg(
  id: string,
  input: UpdateEggInput,
): Promise<ApiEgg> {
  return apiFetch<ApiEgg>(`/eggs/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteEgg(id: string): Promise<void> {
  await apiFetch<void>(`/eggs/${encodeURIComponent(id)}`, { method: "DELETE" });
}

/**
 * @deprecated - No longer using localStorage for auth. Returns null to indicate cookie-based auth.
 */
export function getToken(): string | null {
  return null;
}

/**
 * @deprecated - No longer using localStorage for auth. No-op.
 */
export function setStoredToken(_token: string | null): void {
  // No-op: authentication is now handled via HttpOnly cookies
}

export async function createUser(input: {
  email: string;
  password: string;
  role: string;
  cpuLimit?: number;
  memoryMbLimit?: number;
  diskMbLimit?: number;
  backupLimit?: number;
  databaseLimit?: number;
  allocationLimit?: number;
  subuserLimit?: number;
  scheduleLimit?: number;
  serverLimit?: number;
}): Promise<ApiUser> {
  return apiFetch<ApiUser>("/users", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function deleteUser(id: string): Promise<void> {
  await apiFetch<void>(`/users/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export async function updateUser(
  id: string,
  input: {
    email?: string;
    role?: string;
    password?: string;
    cpuLimit?: number;
    memoryMbLimit?: number;
    diskMbLimit?: number;
    backupLimit?: number;
    databaseLimit?: number;
    allocationLimit?: number;
    subuserLimit?: number;
    scheduleLimit?: number;
    serverLimit?: number;
  },
): Promise<ApiUser> {
  return apiFetch<ApiUser>(`/users/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function attachEggsToMount(
  mountId: string,
  eggIds: string[],
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/mounts/${encodeURIComponent(mountId)}/eggs`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ eggs: eggIds }),
  });
}

export async function attachNodesToMount(
  mountId: string,
  nodeIds: string[],
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/mounts/${encodeURIComponent(mountId)}/nodes`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ nodes: nodeIds }),
  });
}

export async function detachEggFromMount(
  mountId: string,
  eggId: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/mounts/${encodeURIComponent(mountId)}/eggs/${encodeURIComponent(eggId)}`,
    { method: "DELETE" },
  );
}

export async function detachNodeFromMount(
  mountId: string,
  nodeId: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/mounts/${encodeURIComponent(mountId)}/nodes/${encodeURIComponent(nodeId)}`,
    { method: "DELETE" },
  );
}

export async function assignMount(
  serverId: string,
  mountId: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/servers/${encodeURIComponent(serverId)}/mounts`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ mountId }),
  });
}

export async function removeMount(
  serverId: string,
  mountId: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/mounts/${encodeURIComponent(mountId)}`,
    { method: "DELETE" },
  );
}

/** @deprecated Legacy transfer cancellation is retired and the API returns HTTP 501. */
export async function cancelTransfer(serverId: string): Promise<never> {
  return apiFetch<never>(
    `/servers/${encodeURIComponent(serverId)}/transfer/cancel`,
    { method: "POST" },
  );
}

export async function assignServerMount(serverId: string, mountId: string): Promise<ApiMountAssignmentResponse> {
  return apiFetch<ApiMountAssignmentResponse>(`/servers/${encodeURIComponent(serverId)}/mounts`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ mountId }),
  });
}

export async function removeServerMount(serverId: string, mountId: string): Promise<ApiMountAssignmentResponse> {
  return apiFetch<ApiMountAssignmentResponse>(
    `/servers/${encodeURIComponent(serverId)}/mounts/${encodeURIComponent(mountId)}`,

    { method: "DELETE" },
  );
}

export async function createTemplate(input: {
  name: string;
  image: string;
  startupCommand: string;
  defaultMemoryMb: number;
}): Promise<ApiTemplate> {
  return apiFetch<ApiTemplate>("/templates", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
}

export async function runServerSchedule(serverId: string, scheduleId: string): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}/execute`,
    { method: "POST" },
  );
}

export async function fetchServerScheduleRuns(
  serverId: string,
  scheduleId: string,
): Promise<Array<{ id: string; status: string; startedAt?: string; trigger?: string; error?: string; tasks?: Array<{ id: string; status: string; executedAt?: string; error?: string }>; triggeredAt: string; completedAt?: string; output?: string }>> {
  return apiFetch<Array<{ id: string; status: string; startedAt?: string; trigger?: string; error?: string; tasks?: Array<{ id: string; status: string; executedAt?: string; error?: string }>; triggeredAt: string; completedAt?: string; output?: string }>>(
    `/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}/runs`,
  );
}

/** Read-only compatibility status for the retired transfer workflow. Use migrations for new transfers. */
export async function fetchServerTransferStatus(
  serverId: string,
): Promise<ApiLegacyTransferStatus | null> {
  try {
    return await apiFetch<ApiLegacyTransferStatus>(
      `/servers/${encodeURIComponent(serverId)}/transfer`,
    );
  } catch {
    return null;
  }
}

// Server functions from modules
export async function sendPowerSignal(
  serverId: string,
  signal: 'start' | 'stop' | 'restart' | 'kill',
): Promise<{ serverId: string; signal: string; accepted: boolean }> {
  return apiFetch<{ serverId: string; signal: string; accepted: boolean }>(
    `/servers/${encodeURIComponent(serverId)}/power`,
    { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ signal }) },
  );
}

export async function createServer(input: ServerCreateInput): Promise<ApiServer> {
  return apiFetch<ApiServer>("/servers", {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input),
  });
}

export async function updateServer(id: string, input: ServerUpdateInput): Promise<ApiServer> {
  return patchJSON<ApiServer>(`/servers/${encodeURIComponent(id)}`, input);
}

export async function deleteServer(id: string, force?: boolean): Promise<void> {
  const query = force ? "?force=true" : "";
  await deleteJSON(`/servers/${encodeURIComponent(id)}${query}`);
}

export async function reinstallServer(serverId: string): Promise<{ accepted: boolean }> {
  return apiFetch<{ accepted: boolean }>(`/servers/${encodeURIComponent(serverId)}/reinstall`, {
    method: "POST",
  });
}

export async function assignServerAllocation(serverId: string, allocationId: string): Promise<void> {
  await apiFetch<void>(`/servers/${encodeURIComponent(serverId)}/allocations`, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ allocationId }),
  });
}

export async function unassignServerAllocation(serverId: string, allocationId: string): Promise<void> {
  await deleteJSON(`/servers/${encodeURIComponent(serverId)}/allocations/${encodeURIComponent(allocationId)}`);
}

export async function setPrimaryServerAllocation(serverId: string, allocationId: string): Promise<void> {
  await apiFetch<void>(`/servers/${encodeURIComponent(serverId)}/allocations/${encodeURIComponent(allocationId)}/primary`, {
    method: "POST",
  });
}

export async function lockBackup(serverId: string, backupId: string): Promise<void> {
  await apiFetch<void>(`/servers/${encodeURIComponent(serverId)}/backups/${encodeURIComponent(backupId)}/lock`, {
    method: "POST",
  });
}

export async function unlockBackup(serverId: string, backupId: string): Promise<void> {
  await apiFetch<void>(`/servers/${encodeURIComponent(serverId)}/backups/${encodeURIComponent(backupId)}/unlock`, {
    method: "POST",
  });
}

export async function downloadBackup(serverId: string, backupId: string): Promise<Blob> {
  const response = await fetch(`${API_BASE_URL}/servers/${encodeURIComponent(serverId)}/backups/download?name=${encodeURIComponent(backupId)}`, {
    headers: { Accept: "application/json" },
    credentials: "include",
  });
  if (!response.ok) throw new Error(`Failed to download backup: ${response.status}`);
  return response.blob();
}

export async function fetchServerSchedules(serverId: string): Promise<ApiSchedule[]> {
  return fetchJSON<ApiSchedule[]>(`/servers/${encodeURIComponent(serverId)}/schedules`);
}

export async function createServerSchedule(serverId: string, input: ScheduleCreateInput): Promise<ApiSchedule> {
  return apiFetch<ApiSchedule>(`/servers/${encodeURIComponent(serverId)}/schedules`, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input),
  });
}

export async function updateServerSchedule(serverId: string, scheduleId: string, input: ScheduleUpdateInput): Promise<ApiSchedule> {
  return patchJSON<ApiSchedule>(`/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}`, input);
}

export async function deleteServerSchedule(serverId: string, scheduleId: string): Promise<void> {
  await deleteJSON(`/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}`);
}

export async function createServerScheduleTask(serverId: string, scheduleId: string, input: ScheduleTaskCreateInput): Promise<ApiScheduleTask> {
  return apiFetch<ApiScheduleTask>(`/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}/tasks`, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input),
  });
}

export async function deleteServerScheduleTask(serverId: string, scheduleId: string, taskId: string): Promise<void> {
  await deleteJSON(`/servers/${encodeURIComponent(serverId)}/schedules/${encodeURIComponent(scheduleId)}/tasks/${encodeURIComponent(taskId)}`);
}

export async function updateServerStartupCommand(serverId: string, command: string): Promise<void> {
  await patchJSON<void>(`/servers/${encodeURIComponent(serverId)}/startup/command`, { command });
}

export async function updateServerDockerImage(serverId: string, image: string): Promise<void> {
  await patchJSON<void>(`/servers/${encodeURIComponent(serverId)}/startup/image`, { image });
}

export async function getBackupDownloadURL(serverId: string, backupId: string): Promise<{ url: string }> {
  const ticket = await apiFetch<{ token: string }>(
    `/servers/${encodeURIComponent(serverId)}/backups/download-ticket`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: backupId }),
    },
  );
  return { url: `${API_BASE_URL}/download/file?token=${encodeURIComponent(ticket.token)}` };
}

export async function downloadServerFile(serverId: string, path: string): Promise<Blob> {
  const response = await fetch(
    `${API_BASE_URL}/servers/${encodeURIComponent(serverId)}/files/download?path=${encodeURIComponent(path)}`,
    { headers: { Accept: "application/json" }, credentials: "include" },
  );
  if (!response.ok) throw new Error(`Failed to download file: ${response.status}`);
  return response.blob();
}

export async function copyServerFile(serverId: string, from: string, to: string): Promise<void> {
  await apiFetch<void>(`/servers/${encodeURIComponent(serverId)}/files/copy`, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ from, to }),
  });
}

export async function deleteServerFiles(serverId: string, paths: string[]): Promise<void> {
  await apiFetch<void>(`/servers/${encodeURIComponent(serverId)}/files/delete-batch`, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ paths }),
  });
}

export async function renameServerFiles(serverId: string, files: Array<{ from: string; to: string }>): Promise<void> {
  await apiFetch<void>(`/servers/${encodeURIComponent(serverId)}/files/rename-batch`, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ files }),
  });
}

export async function chmodServerFiles(serverId: string, files: Array<{ path: string; mode: string }>): Promise<void> {
  await apiFetch<void>(`/servers/${encodeURIComponent(serverId)}/files/chmod-batch`, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ files }),
  });
}

export async function createServerDirectory(serverId: string, path: string): Promise<void> {
  await apiFetch<void>(`/servers/${encodeURIComponent(serverId)}/files/create-directory`, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ path }),
  });
}

export async function compressServerFiles(serverId: string, path: string): Promise<void> {
  await apiFetch<void>(`/servers/${encodeURIComponent(serverId)}/files/archive`, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ path }),
  });
}

export async function decompressServerFiles(serverId: string, path: string): Promise<void> {
  await apiFetch<void>(`/servers/${encodeURIComponent(serverId)}/files/decompress`, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ path }),
  });
}

export async function pullServerFile(serverId: string, url: string, path?: string): Promise<{ ok: boolean; path: string; size: number }> {
  return apiFetch<{ ok: boolean; path: string; size: number }>(
    `/servers/${encodeURIComponent(serverId)}/files/pull`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url, path }),
    },
  );
}

export async function fetchServerAllocations(serverId: string): Promise<ApiAllocation[]> {
  return fetchJSON<ApiAllocation[]>(`/servers/${encodeURIComponent(serverId)}/allocations`);
}

export async function fetchServerActivity(
  serverId: string,
  page = 1,
  perPage = 50,
): Promise<{ data: ApiAuditEvent[]; pagination: { page: number; per_page: number; total: number; total_pages: number } }> {
  const result = await apiFetch<ApiAuditEvent[] | { data: ApiAuditEvent[]; pagination: { page: number; per_page: number; total: number; total_pages: number } }>(
    `/servers/${encodeURIComponent(serverId)}/activity?page=${page}&per_page=${perPage}`,
    {},
    true,
  );
  if (Array.isArray(result)) {
    return {
      data: result,
      pagination: { page: 1, per_page: result.length, total: result.length, total_pages: 1 },
    };
  }
  return result;
}

export async function fetchServerUsers(serverId: string): Promise<ApiServerSubuser[]> {
  return fetchJSON<ApiServerSubuser[]>(`/servers/${encodeURIComponent(serverId)}/users`);
}

export async function deleteServerUser(serverId: string, userId: string): Promise<void> {
  await deleteJSON(`/servers/${encodeURIComponent(serverId)}/users/${encodeURIComponent(userId)}`);
}

export async function updateServerUser(serverId: string, userId: string, data: { permissions: string[] }): Promise<ApiServerSubuser> {
  return patchJSON<ApiServerSubuser>(`/servers/${encodeURIComponent(serverId)}/users/${encodeURIComponent(userId)}`, data);
}

export async function upsertServerUser(serverId: string, data: { email: string; permissions: string[] }): Promise<ApiServerSubuser> {
  return apiFetch<ApiServerSubuser>(`/servers/${encodeURIComponent(serverId)}/users`, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(data),
  });
}

/** @deprecated Legacy transfer cancellation is retired and the API returns HTTP 501. */
export async function cancelServerTransfer(serverId: string): Promise<never> {
  return apiFetch<never>(`/servers/${encodeURIComponent(serverId)}/transfer/cancel`, { method: "POST" });
}

export async function suspendServer(serverId: string): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/servers/${encodeURIComponent(serverId)}/suspension`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action: "suspend" }),
  });
}

export async function unsuspendServer(serverId: string): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/servers/${encodeURIComponent(serverId)}/suspension`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action: "unsuspend" }),
  });
}

/** @deprecated Legacy transfer creation is retired and the API returns HTTP 501. Create a migration instead. */
export async function transferServer(serverId: string, targetNodeId: string): Promise<never> {
  return apiFetch<never>(
    `/servers/${encodeURIComponent(serverId)}/transfer`, {
      method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ targetNodeId }),
    },
  );
}

export async function getServerFileDownloadURL(serverId: string, path: string): Promise<string> {
  const result = await apiFetch<{ url: string; expires: string }>(
    `/servers/${encodeURIComponent(serverId)}/files/download-url?path=${encodeURIComponent(path)}`,
  );
  return result.url;
}

export async function readServerFile(serverId: string, path: string): Promise<string> {
  const response = await fetch(
    `${API_BASE_URL}/servers/${encodeURIComponent(serverId)}/files/content?path=${encodeURIComponent(path)}`,
    { headers: { Accept: "text/plain" }, credentials: "include" },
  );
  if (!response.ok) throw new Error(`Failed to read file: ${response.status}`);
  return response.text();
}

export async function archiveServerFile(serverId: string, path: string): Promise<Blob> {
  const response = await fetch(
    `${API_BASE_URL}/servers/${encodeURIComponent(serverId)}/files/archive?path=${encodeURIComponent(path)}`,
    { method: "POST", credentials: "include" },
  );
  if (!response.ok) throw new Error(`Failed to archive file: ${response.status}`);
  return response.blob();
}

export async function uploadFileChunked(
  serverId: string,
  path: string,
  file: File | Blob,
  onProgress?: (loaded: number, total: number) => void,
): Promise<void> {
  const chunkSize = 8 * 1024 * 1024;
  const totalSize = file.size;
  let offset = 0;
  while (offset < totalSize) {
    const chunk = file.slice(offset, Math.min(offset + chunkSize, totalSize));
    const isLast = offset + chunkSize >= totalSize;
    const formData = new FormData();
    formData.append("files", chunk, path.split("/").pop() || "file");
    formData.append("directory", path.substring(0, path.lastIndexOf("/")) || "/");
    if (!isLast) formData.append("append", "true");
    const response = await fetch(
      `${API_BASE_URL}/servers/${encodeURIComponent(serverId)}/files/upload`,
      { method: "POST", credentials: "include", body: formData },
    );
    if (!response.ok) throw new Error(`Upload failed at offset ${offset}: ${response.status}`);
    offset += chunk.size;
    onProgress?.(offset, totalSize);
  }
}

export async function updateServerStartupVariable(
  serverId: string,
  variableId: string,
  value: string,
): Promise<void> {
  await putJSON<void>(`/servers/${encodeURIComponent(serverId)}/startup/variable`, {
    key: variableId,
    value,
  });
}
