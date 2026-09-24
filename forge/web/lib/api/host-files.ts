import { fetchJSON, postJSON, requestText, requestBlob, requestVoid } from './http';

export interface FileEntry {
  name: string;
  path: string;
  size: number;
  mode: string;
  isDir: boolean;
  modTime: string;
}

// The backend resolves the target Beacon node from the optional `nodeId`
// query parameter (falling back to the first active node), so every host
// file operation threads it through the URL.
function nodeQuery(nodeId?: string): string {
  return nodeId ? `&nodeId=${encodeURIComponent(nodeId)}` : '';
}

function nodePath(path: string, nodeId?: string): string {
  return nodeId ? `${path}?nodeId=${encodeURIComponent(nodeId)}` : path;
}

export function listFiles(path: string = '/', nodeId?: string): Promise<FileEntry[]> {
  return fetchJSON<FileEntry[]>(`/host/files/list?path=${encodeURIComponent(path)}${nodeQuery(nodeId)}`);
}

export async function readFile(path: string, nodeId?: string): Promise<string> {
  return requestText(nodePath('/host/files/read', nodeId), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Accept: 'text/plain' },
    body: JSON.stringify({ path }),
  });
}

export async function writeFile(path: string, content: string, nodeId?: string): Promise<void> {
  await postJSON<void>(nodePath('/host/files/write', nodeId), { path, content });
}

export async function createDir(path: string, nodeId?: string): Promise<void> {
  await postJSON<void>(nodePath('/host/files/mkdir', nodeId), { path });
}

export async function renameFile(oldPath: string, newPath: string, nodeId?: string): Promise<void> {
  await postJSON<void>(nodePath('/host/files/rename', nodeId), { oldPath, newPath });
}

export async function copyFile(sourcePath: string, destPath: string, nodeId?: string): Promise<void> {
  await postJSON<void>(nodePath('/host/files/copy', nodeId), { sourcePath, destPath });
}

export async function deleteFile(path: string, nodeId?: string): Promise<void> {
  await postJSON<void>(nodePath('/host/files/remove', nodeId), { path });
}

export async function chmodFile(path: string, mode: string, nodeId?: string): Promise<void> {
  await postJSON<void>(nodePath('/host/files/chmod', nodeId), { path, mode });
}

export async function uploadFile(path: string, file: File, nodeId?: string): Promise<void> {
  const formData = new FormData();
  formData.append('files', file);
  // requestVoid routes through the canonical primitive (CSRF + credentials);
  // the browser sets the multipart Content-Type boundary because no explicit
  // Content-Type is passed.
  await requestVoid(`/host/files/upload?path=${encodeURIComponent(path)}${nodeQuery(nodeId)}`, {
    method: 'POST',
    body: formData,
  });
}

export async function downloadFile(path: string, nodeId?: string): Promise<Blob> {
  return requestBlob(`/host/files/download?path=${encodeURIComponent(path)}${nodeQuery(nodeId)}`);
}

/**
 * Client-side pull of an ARBITRARY external URL (used by the host file manager
 * when no dedicated beacon pull endpoint exists). This deliberately does NOT go
 * through the API primitive: the target is a third-party origin, so we must not
 * prepend `API_BASE_URL`, attach cookies or sign CSRF.
 */
export async function pullRemoteFile(url: string): Promise<Blob> {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`Fetch failed: ${response.status}`);
  }
  return response.blob();
}
