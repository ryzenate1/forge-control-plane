// File management API functions
import {
  fetchJSON,
  postJSON,
  deleteJSON,
  requestBlob,
  requestText,
  requestVoid,
} from './http';
import type { ApiFileEntry } from './types';

export async function fetchServerFiles(serverId: string, path?: string): Promise<ApiFileEntry[]> {
  const url = path
    ? `/servers/${encodeURIComponent(serverId)}/files?path=${encodeURIComponent(path)}`
    : `/servers/${encodeURIComponent(serverId)}/files`;
  const files = await fetchJSON<ApiFileEntry[]>(url);
  // The daemon returns modTime; the web API type uses modifiedAt. Normalize so
  // consumers never read an undefined timestamp.
  return (files ?? []).map((file) => ({
    ...file,
    modifiedAt: file.modifiedAt ?? file.createdAt,
  }));
}

export async function downloadServerFile(serverId: string, path: string): Promise<Blob> {
  return requestBlob(
    `/servers/${encodeURIComponent(serverId)}/files/download?path=${encodeURIComponent(path)}`,
  );
}

export async function getServerFileDownloadURL(
  serverId: string,
  path: string,
): Promise<{ url: string; expires: string }> {
  return fetchJSON<{ url: string; expires: string }>(
    `/servers/${encodeURIComponent(serverId)}/files/download-url?path=${encodeURIComponent(path)}`,
  );
}

export async function writeServerFile(
  serverId: string,
  path: string,
  content: string | Blob | ArrayBuffer,
): Promise<void> {
  let contentType = 'application/octet-stream';
  if (typeof content === 'string') {
    contentType = 'text/plain; charset=utf-8';
  } else if (content instanceof Blob && content.type) {
    contentType = content.type;
  }

  await requestVoid(
    `/servers/${encodeURIComponent(serverId)}/files/content?path=${encodeURIComponent(path)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': contentType },
      body: content,
    },
  );
}

export async function deleteServerFile(serverId: string, path: string): Promise<void> {
  await deleteJSON(
    `/servers/${encodeURIComponent(serverId)}/files/delete?path=${encodeURIComponent(path)}`,
  );
}

export async function renameServerFile(serverId: string, from: string, to: string): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/files/rename`, {
    from,
    to,
  });
}

export async function copyServerFile(serverId: string, from: string, to: string): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/files/copy`, {
    from,
    to,
  });
}

export async function chmodServerFile(serverId: string, path: string, mode: string): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/files/chmod`, {
    path,
    mode,
  });
}

export async function deleteServerFiles(serverId: string, paths: string[]): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/files/delete-batch`, {
    paths,
  });
}

export async function renameServerFiles(
  serverId: string,
  files: Array<{ from: string; to: string }>,
): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/files/rename-batch`, {
    files,
  });
}

export async function chmodServerFiles(
  serverId: string,
  files: Array<{ path: string; mode: string }>,
): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/files/chmod-batch`, {
    files,
  });
}

export async function createServerDirectory(serverId: string, path: string): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/files/create-directory`, {
    path,
  });
}

export async function compressServerFiles(serverId: string, path: string): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/files/archive`, {
    path,
  });
}

export async function decompressServerFiles(serverId: string, path: string): Promise<void> {
  await postJSON<void>(`/servers/${encodeURIComponent(serverId)}/files/decompress`, {
    path,
  });
}

/**
 * Canonical pull — POST /servers/:id/files/pull (spec contract).
 * Backend also accepts POST /servers/:id/files/download as legacy alias
 * (see `downloadFileToServer` in @/lib/api), both handled by PullRemoteFile.
 */
export async function pullServerFile(
  serverId: string,
  url: string,
  path?: string,
): Promise<{ ok: boolean; path: string; size: number }> {
  return postJSON<{ ok: boolean; path: string; size: number }>(
    `/servers/${encodeURIComponent(serverId)}/files/pull`,
    { url, path },
  );
}

// Legacy alias re-export for callers still importing from files module.
export const downloadFileToServerViaFiles = pullServerFile;

export async function readServerFile(serverId: string, path: string): Promise<string> {
  return requestText(
    `/servers/${encodeURIComponent(serverId)}/files/content?path=${encodeURIComponent(path)}`,
    { headers: { Accept: 'text/plain' } },
  );
}

export async function archiveServerFile(serverId: string, path: string): Promise<Blob> {
  return requestBlob(
    `/servers/${encodeURIComponent(serverId)}/files/archive?path=${encodeURIComponent(path)}`,
    { method: 'POST' },
  );
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
  const uploadId = crypto.randomUUID();

  while (offset < totalSize) {
    const end = Math.min(offset + chunkSize, totalSize);
    const chunk = file.slice(offset, end);
    const isLast = end >= totalSize;

    const params = new URLSearchParams({
      path,
      uploadId,
      offset: String(offset),
    });
    if (isLast) params.set('final', 'true');

    await requestVoid(
      `/servers/${encodeURIComponent(serverId)}/files/upload?${params}`,
      {
        method: 'PUT',
        headers: { 'Content-Type': 'application/octet-stream' },
        credentials: 'include',
        body: chunk,
      },
    );

    offset = end;
    onProgress?.(offset, totalSize);
  }
}
