// File management API functions
import {
  fetchJSON,
  postJSON,
  deleteJSON,
  getAuthHeaders,
  getCSRFToken,
  API_BASE_URL,
} from './http';

export async function fetchServerFiles(serverId: string, path?: string): Promise<any[]> {
  const url = path
    ? `/servers/${encodeURIComponent(serverId)}/files?path=${encodeURIComponent(path)}`
    : `/servers/${encodeURIComponent(serverId)}/files`;
  return fetchJSON<any[]>(url);
}

export async function downloadServerFile(serverId: string, path: string): Promise<Blob> {
  const url = `/servers/${encodeURIComponent(serverId)}/files/download?path=${encodeURIComponent(path)}`;
  const response = await fetch(url, {
    headers: {
      ...getAuthHeaders(),
    },
  });

  if (!response.ok) {
    throw new Error(`Failed to download file: ${response.status}`);
  }

  return response.blob();
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

  const headers: Record<string, string> = {
    'Content-Type': contentType,
    ...getAuthHeaders(),
  };
  const csrf = getCSRFToken();
  if (csrf) headers['X-CSRF-Token'] = csrf;

  const response = await fetch(
    `${API_BASE_URL}/servers/${encodeURIComponent(serverId)}/files/content?path=${encodeURIComponent(path)}`,
    {
      method: 'PUT',
      headers,
      body: content,
    },
  );

  if (!response.ok) {
    throw new Error(`Failed to write file content: ${response.status}`);
  }
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

export async function readServerFile(serverId: string, path: string): Promise<string> {
  const response = await fetch(
    `${API_BASE_URL}/servers/${encodeURIComponent(serverId)}/files/content?path=${encodeURIComponent(path)}`,
    {
      headers: {
        Accept: 'text/plain',
        ...getAuthHeaders(),
      },
      credentials: 'include',
    },
  );
  if (!response.ok) {
    throw new Error(`Failed to read file: ${response.status}`);
  }
  return response.text();
}

export async function archiveServerFile(serverId: string, path: string): Promise<Blob> {
  const headers: Record<string, string> = {
    ...getAuthHeaders(),
  };
  const csrf = getCSRFToken();
  if (csrf) headers['X-CSRF-Token'] = csrf;

  const response = await fetch(
    `${API_BASE_URL}/servers/${encodeURIComponent(serverId)}/files/archive?path=${encodeURIComponent(path)}`,
    {
      method: 'POST',
      headers,
      credentials: 'include',
    },
  );
  if (!response.ok) {
    throw new Error(`Failed to archive file: ${response.status}`);
  }
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

    const headers: Record<string, string> = {
      'Content-Type': 'application/octet-stream',
      ...getAuthHeaders(),
    };
    const csrf = getCSRFToken();
    if (csrf) headers['X-CSRF-Token'] = csrf;

    const response = await fetch(
      `${API_BASE_URL}/servers/${encodeURIComponent(serverId)}/files/upload?${params}`,
      {
        method: 'PUT',
        headers,
        credentials: 'include',
        body: chunk,
      },
    );

    if (!response.ok) {
      throw new Error(`Upload failed at offset ${offset}: ${response.status}`);
    }

    offset = end;
    onProgress?.(offset, totalSize);
  }
}
