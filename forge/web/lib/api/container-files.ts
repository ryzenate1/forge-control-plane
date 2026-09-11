import {
  deleteJSON,
  fetchJSON,
  postJSON,
  postMultipartJSON,
  putJSON,
  requestBlob,
  requestText,
} from './http';

/**
 * Client for the server-scoped container file manager. Unlike
 * `lib/api/docker.ts` (which addresses a raw container id on a node and is
 * admin-only), every call here addresses a *server*: the API resolves the
 * workload container from the server binding on the node, so the browser never
 * handles container ids at all.
 */

export interface ContainerFileEntry {
  name: string;
  path: string;
  size: number;
  mode: string;
  isDir: boolean;
  modified: string;
  type: 'file' | 'dir' | 'symlink' | 'hardlink' | string;
}

export interface ContainerFileListing {
  path: string;
  entries: ContainerFileEntry[];
  truncated: boolean;
}

export interface ContainerFileContent {
  path: string;
  text: string;
  size: number;
  binary: boolean;
}

export interface ContainerUploadResult {
  ok: boolean;
  uploaded?: number;
  failed?: { file: string; error: string }[];
}

function base(serverId: string): string {
  return `/servers/${encodeURIComponent(serverId)}/container/files`;
}

function pathQuery(path: string): string {
  return `path=${encodeURIComponent(path)}`;
}

export function listContainerFiles(serverId: string, path = '/'): Promise<ContainerFileListing> {
  return fetchJSON<ContainerFileListing>(`${base(serverId)}/ls?${pathQuery(path)}`);
}

/**
 * Read a file inline. The daemon answers with the raw bytes and a sniffed
 * content type; a NUL byte in the decoded text is the same heuristic the
 * daemon uses to mark content as binary, so callers can offer a download
 * instead of an editor.
 */
export async function readContainerFile(serverId: string, path: string): Promise<ContainerFileContent> {
  const text = await requestText(`${base(serverId)}/read?${pathQuery(path)}`, {
    headers: { Accept: 'text/plain, application/octet-stream;q=0.9' },
  });
  return { path, text, size: new TextEncoder().encode(text).length, binary: text.indexOf('\u0000') >= 0 };
}

export async function writeContainerFile(serverId: string, path: string, content: string): Promise<void> {
  await putJSON<{ ok: boolean; path: string }>(`${base(serverId)}/write`, { path, content });
}

export async function createContainerDir(serverId: string, path: string): Promise<void> {
  await postJSON<{ ok: boolean; path: string }>(`${base(serverId)}/mkdir`, { path });
}

export async function deleteContainerFile(serverId: string, path: string): Promise<void> {
  await deleteJSON<{ ok: boolean; path: string }>(`${base(serverId)}?${pathQuery(path)}`);
}

/** Upload one or more files into `directory`; the browser sets the multipart boundary. */
export function uploadContainerFiles(
  serverId: string,
  directory: string,
  files: File[],
): Promise<ContainerUploadResult> {
  const form = new FormData();
  for (const file of files) {
    form.append('file', file, file.name);
  }
  return postMultipartJSON<ContainerUploadResult>(`${base(serverId)}/upload?${pathQuery(directory)}`, form);
}

export function downloadContainerFile(serverId: string, path: string): Promise<Blob> {
  return requestBlob(`${base(serverId)}/download?${pathQuery(path)}`);
}
