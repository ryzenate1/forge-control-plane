import { fetchJSON, postJSON, API_BASE_URL, getAuthHeaders, getCSRFToken } from './http';

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
  const response = await fetch(
    `${API_BASE_URL}${nodePath('/host/files/read', nodeId)}`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'text/plain',
        ...getAuthHeaders(),
      },
      credentials: 'include',
      body: JSON.stringify({ path }),
    },
  );
  if (!response.ok) {
    throw new Error(`Failed to read file: ${response.status}`);
  }
  return response.text();
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
  const headers: Record<string, string> = {};
  const csrf = getCSRFToken();
  if (csrf) headers['X-CSRF-Token'] = csrf;
  const response = await fetch(
    `${API_BASE_URL}/host/files/upload?path=${encodeURIComponent(path)}${nodeQuery(nodeId)}`,
    { method: 'POST', headers, credentials: 'include', body: formData },
  );
  if (!response.ok) {
    throw new Error(`Failed to upload file: ${response.status}`);
  }
}

export async function downloadFile(path: string, nodeId?: string): Promise<Blob> {
  const response = await fetch(
    `${API_BASE_URL}/host/files/download?path=${encodeURIComponent(path)}${nodeQuery(nodeId)}`,
    { headers: { ...getAuthHeaders() }, credentials: 'include' },
  );
  if (!response.ok) {
    throw new Error(`Failed to download file: ${response.status}`);
  }
  return response.blob();
}
