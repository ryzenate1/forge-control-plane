import { fetchJSON, postJSON, API_BASE_URL, getAuthHeaders, getCSRFToken } from './http';

export interface FileEntry {
  name: string;
  path: string;
  size: number;
  mode: string;
  isDir: boolean;
  modTime: string;
}

export function listFiles(path: string = '/'): Promise<FileEntry[]> {
  return fetchJSON<FileEntry[]>(`/host/files/list?path=${encodeURIComponent(path)}`);
}

export async function readFile(path: string): Promise<string> {
  const response = await fetch(
    `${API_BASE_URL}/host/files/read`,
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

export async function writeFile(path: string, content: string): Promise<void> {
  await postJSON<void>('/host/files/write', { path, content });
}

export async function createDir(path: string): Promise<void> {
  await postJSON<void>('/host/files/mkdir', { path });
}

export async function renameFile(oldPath: string, newPath: string): Promise<void> {
  await postJSON<void>('/host/files/rename', { oldPath, newPath });
}

export async function copyFile(sourcePath: string, destPath: string): Promise<void> {
  await postJSON<void>('/host/files/copy', { sourcePath, destPath });
}

export async function deleteFile(path: string): Promise<void> {
  await postJSON<void>('/host/files/remove', { path });
}

export async function chmodFile(path: string, mode: string): Promise<void> {
  await postJSON<void>('/host/files/chmod', { path, mode });
}

export async function uploadFile(path: string, file: File): Promise<void> {
  const formData = new FormData();
  formData.append('files', file);
  const headers: Record<string, string> = { ...getAuthHeaders() };
  const csrf = getCSRFToken();
  if (csrf) headers['X-CSRF-Token'] = csrf;
  const response = await fetch(
    `${API_BASE_URL}/host/files/upload?path=${encodeURIComponent(path)}`,
    { method: 'POST', headers, credentials: 'include', body: formData },
  );
  if (!response.ok) {
    throw new Error(`Failed to upload file: ${response.status}`);
  }
}

export async function downloadFile(path: string): Promise<Blob> {
  const response = await fetch(
    `${API_BASE_URL}/host/files/download?path=${encodeURIComponent(path)}`,
    { headers: { ...getAuthHeaders() }, credentials: 'include' },
  );
  if (!response.ok) {
    throw new Error(`Failed to download file: ${response.status}`);
  }
  return response.blob();
}
