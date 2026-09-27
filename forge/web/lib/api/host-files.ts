import { fetchJSON, postJSON, requestText, requestBlob, requestVoid, ApiError } from './http';

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
 * prepend the API base URL, attach cookies or sign CSRF.
 *
 * A default 30s timeout applies when the caller provides no signal; pass
 * `{ signal }` to cancel with your own controller or `{ timeoutMs: false }`
 * to opt out. Failures surface as {@link ApiError} (status 0 for transport
 * errors) so callers handle them like every other API failure.
 */
export async function pullRemoteFile(
  url: string,
  options?: { signal?: AbortSignal; timeoutMs?: number | false },
): Promise<Blob> {
  const timeoutMs = options?.timeoutMs === false
    ? null
    : typeof options?.timeoutMs === 'number' && options.timeoutMs > 0
      ? options.timeoutMs
      : 30000;
  let signal: AbortSignal | undefined = options?.signal;
  if (timeoutMs != null) {
    try {
      const timeoutSignal = AbortSignal.timeout(timeoutMs);
      signal = signal
        ? (typeof AbortSignal.any === 'function' ? AbortSignal.any([signal, timeoutSignal]) : signal)
        : timeoutSignal;
    } catch {
      // Keep the caller signal when timeout construction fails.
    }
  }
  let response: Response;
  try {
    response = await fetch(url, {
      ...(signal ? { signal } : {}),
      credentials: 'omit',
    });
  } catch (err) {
    if (err instanceof Error && (err.name === 'AbortError' || err.name === 'TimeoutError')) throw err;
    throw new ApiError(
      err instanceof TypeError
        ? `Network error fetching ${url} — check your connection`
        : err instanceof Error ? err.message : `Fetch failed: ${url}`,
      0,
    );
  }
  if (!response.ok) {
    throw new ApiError(`Fetch failed: ${response.status}`, response.status);
  }
  return response.blob();
}
