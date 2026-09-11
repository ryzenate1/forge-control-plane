import { fetchJSON, postJSON, patchJSON, deleteJSON } from "./http";

// Client for the admin HashiCorp Vault provider surface
// (/api/v1/admin/vault). Mirrors forge/api/internal/services/vaultprovider and
// forge/api/internal/store/store_vault_connections wire types: a registered
// Vault connection (endpoint + KV mount + auth method) that environment
// variables can reference as `vault:<connection-id>/<secret-path>#<field>`.
//
// The backend never returns a raw token or secret id: list/create/update all
// yield a *safe* view where only masked hints (e.g. "****abcd") and the plain
// role id are exposed. `token`/`secretId` are write-only fields.

export type VaultAuthMethod = "token" | "approle";

export type VaultConnection = {
  id: string;
  /** Absent for a global connection (registered by an admin). */
  orgId?: string;
  name: string;
  baseUrl: string;
  mountPath: string;
  namespace?: string;
  /** KV secrets engine version: 1 or 2. */
  engineVersion: number;
  authMethod: VaultAuthMethod;
  enabled: boolean;
  /** Only meaningful for AppRole; never a secret. */
  roleId?: string;
  /** Masked display hints — the plaintext credential is never returned. */
  tokenHint?: string;
  secretIdHint?: string;
  createdBy?: string;
  createdAt: string;
  updatedAt: string;
};

export type CreateVaultConnectionInput = {
  name: string;
  baseUrl: string;
  mountPath?: string;
  namespace?: string;
  engineVersion?: number;
  authMethod: VaultAuthMethod;
  /** Write-only. Omit on edit to preserve the stored credential. */
  token?: string;
  roleId?: string;
  /** Write-only. Omit on edit to preserve the stored credential. */
  secretId?: string;
  enabled?: boolean;
};

export type UpdateVaultConnectionInput = Partial<CreateVaultConnectionInput>;

export type VaultReferenceCheck = {
  /** Whether the submitted value carried the `vault:` reference prefix. */
  reference: boolean;
  /** Only present when `reference` is true. */
  found?: boolean;
  /** Length of the resolved secret (never the value itself). */
  length?: number;
  /** Resolve failure reason, when not found. */
  reason?: string;
};

export async function listVaultConnections(): Promise<VaultConnection[]> {
  const res = await fetchJSON<{ data: VaultConnection[] }>("/admin/vault");
  return res.data ?? [];
}

export async function createVaultConnection(input: CreateVaultConnectionInput): Promise<VaultConnection> {
  return postJSON<VaultConnection>("/admin/vault", input);
}

export async function updateVaultConnection(id: string, input: UpdateVaultConnectionInput): Promise<VaultConnection> {
  return patchJSON<VaultConnection>(`/admin/vault/${encodeURIComponent(id)}`, input);
}

export async function deleteVaultConnection(id: string): Promise<void> {
  await deleteJSON<{ ok: boolean }>(`/admin/vault/${encodeURIComponent(id)}`);
}

export async function testVaultConnection(id: string): Promise<{ ok: boolean }> {
  return postJSON<{ ok: boolean }>(`/admin/vault/${encodeURIComponent(id)}/test`);
}

/**
 * Dry-run parse + resolve for a candidate reference value. Returns only whether
 * the secret was found (and its length) — never the resolved value itself.
 */
export async function validateVaultReference(value: string): Promise<VaultReferenceCheck> {
  return postJSON<VaultReferenceCheck>("/admin/vault/validate-reference", { value });
}
