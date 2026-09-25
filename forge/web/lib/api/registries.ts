import { fetchJSON, postJSON, putJSON, deleteJSON } from "./http";

// Private Docker registry credentials (forge/api /registries). Credential is
// returned masked by the API; send plaintext only on create/update.

export type DockerRegistry = {
  id: string;
  userId?: string;
  name: string;
  serverAddress: string;
  username: string;
  credential?: string;
  email?: string;
  isGlobal: boolean;
  createdAt: string;
  updatedAt: string;
};

export type RegistryInput = {
  name: string;
  serverAddress: string;
  username?: string;
  credential?: string;
  email?: string;
  isGlobal?: boolean;
};

export async function listRegistries(): Promise<DockerRegistry[]> {
  const res = await fetchJSON<{ data: DockerRegistry[] }>("/registries");
  return res.data ?? [];
}

export async function getRegistry(id: string): Promise<DockerRegistry> {
  return fetchJSON<DockerRegistry>(`/registries/${encodeURIComponent(id)}`);
}

export async function createRegistry(input: RegistryInput): Promise<DockerRegistry> {
  return postJSON<DockerRegistry>("/registries", input);
}

export async function updateRegistry(id: string, input: RegistryInput): Promise<DockerRegistry> {
  return putJSON<DockerRegistry>(`/registries/${encodeURIComponent(id)}`, input);
}

export async function deleteRegistry(id: string): Promise<void> {
  await deleteJSON<void>(`/registries/${encodeURIComponent(id)}`);
}

export async function verifyRegistry(id: string): Promise<{ ok: boolean; verified?: boolean; error?: string }> {
  return postJSON<{ ok: boolean; verified?: boolean; error?: string }>(`/registries/${encodeURIComponent(id)}/verify`, {});
}
