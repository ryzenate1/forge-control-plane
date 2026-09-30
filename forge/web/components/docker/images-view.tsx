"use client";

import { useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Trash2, RefreshCw, Download } from "lucide-react";
import { listImages, pullImage, deleteImage, type DockerImage } from "@/lib/api/docker";
import { NodeSelect } from "@/components/docker/node-select";
import { Btn, Card, EmptyState, Input, Modal, ModalFooter, AdminLoadingState } from "@/components/admin/admin-ui";
import { ConfirmDialog, Alert, Pagination } from "@/components/ui/primitives";
import { useToast } from "@/components/ui/toast";

function formatSize(bytes: number): string {
  if (!bytes) return "0B";
  const units = ["B", "KB", "MB", "GB"];
  let i = 0;
  let size = bytes;
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
  return `${size.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

function formatCreated(ts: number): string {
  if (!ts) return "";
  const d = new Date(ts * 1000);
  return d.toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" });
}

export function ImagesView() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [search, setSearch] = useState("");
  const [showPull, setShowPull] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<DockerImage | null>(null);

  const imagesQuery = useQuery({
    queryKey: ["docker", "images"],
    queryFn: listImages,
    refetchInterval: 30_000,
    retry: false,
    staleTime: 10_000,
  });

  const images = useMemo(() => Array.isArray(imagesQuery.data) ? imagesQuery.data : [], [imagesQuery.data]);

  const pullMut = useMutation({
    mutationFn: ({ image, tag, nodeId }: { image: string; tag: string; nodeId: string }) => pullImage(image, tag || undefined, nodeId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["docker", "images"] }); setShowPull(false); },
  });

  const deleteMut = useMutation({
    mutationFn: ({ id, nodeId }: { id: string; nodeId: string }) => deleteImage(id, nodeId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["docker", "images"] }); setDeleteTarget(null); },
    onError: (err) => toast({ tone: "error", title: "Failed to delete image", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const filtered = useMemo(
    () => images.filter(
      (i) => !search || i.tags.toLowerCase().includes(search.toLowerCase()) || i.id.toLowerCase().includes(search.toLowerCase()),
    ),
    [images, search],
  );

  const [page, setPage] = useState(1);
  const PAGE_SIZE = 10;
  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages);
  const paginated = useMemo(() => {
    const start = (safePage - 1) * PAGE_SIZE;
    return filtered.slice(start, start + PAGE_SIZE);
  }, [filtered, safePage]);

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <div className="flex-1">
          <Input placeholder="Search images..." value={search} onChange={setSearch} />
        </div>
        <Btn tone="ghost" onClick={() => void imagesQuery.refetch()}>
          <RefreshCw size={14} />
        </Btn>
        <Btn tone="primary" onClick={() => setShowPull(true)}>
          <Download size={14} /> Pull Image
        </Btn>
      </div>

      <Card>
        {imagesQuery.isLoading ? (
          <div className="p-4"><AdminLoadingState label="Loading images…" /></div>
        ) : imagesQuery.isError ? (
          <div className="space-y-1 p-4 text-sm">
            <p className="text-red-400">Images could not be loaded: {imagesQuery.error instanceof Error ? imagesQuery.error.message : "request failed"}. This is a failed read, not an empty host.</p>
            <p className="text-xs text-slate-500">A node running with DAEMON_ALLOW_MOCK_RUNTIME=true answers every runtime call with “container runtime is unavailable” by design — no image operation can succeed there.</p>
          </div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Download} message={search ? "No images match your search." : "No images found. Pull an image from a registry."} title={search ? "No results" : "No images"} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--line)] bg-[var(--surface-raised)] text-left text-[10px] uppercase tracking-widest text-slate-500">
                  <th className="px-4 py-3">Repository</th>
                  <th className="px-4 py-3">Tag</th>
                  <th className="px-4 py-3">Image ID</th>
                  <th className="px-4 py-3">Size</th>
                  <th className="px-4 py-3">Node</th>
                  <th className="px-4 py-3">Created</th>
                  <th className="px-4 py-3">Actions</th>
                </tr>
              </thead>
              <tbody>
                {paginated.map((img, idx) => {
                  const parts = img.tags.split(",");
                  const firstTag = parts[0] || "<none>:<none>";
                  const [repo, tag] = firstTag.includes(":") ? firstTag.split(":") : [firstTag, ""];
                  return (
                    <tr key={`${img.id}-${img.nodeId}-${idx}`} className="border-b border-white/[0.03] hover:bg-[var(--surface)]">
                      <td className="px-4 py-3 font-medium text-slate-200">{repo}</td>
                      <td className="px-4 py-3 text-slate-400">{tag || "latest"}</td>
                      <td className="px-4 py-3 font-mono text-[11px] text-slate-400">{img.id?.replace("sha256:", "").slice(0, 12) || "-"}</td>
                      <td className="px-4 py-3 text-slate-400">{formatSize(img.size)}</td>
                      <td className="px-4 py-3 text-slate-400">{img.nodeName || img.nodeId?.slice(0, 8)}</td>
                      <td className="px-4 py-3 text-slate-400">{formatCreated(img.created)}</td>
                      <td className="px-4 py-3">
                        <button className="rounded min-h-11 min-w-11 inline-grid place-items-center p-2 text-slate-400 hover:bg-white/[0.06] hover:text-red-400" onClick={() => setDeleteTarget(img)} title="Delete" type="button"><Trash2 size={13} /></button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
        {filtered.length > PAGE_SIZE && (
          <div className="mt-4">
            <Pagination page={safePage} pageCount={totalPages} onPageChange={setPage} label="Images pagination" />
          </div>
        )}
      </Card>

      {showPull && (
        <PullImageModal
          onClose={() => setShowPull(false)}
          onPull={(image, tag, nodeId) => pullMut.mutate({ image, tag, nodeId })}
          loading={pullMut.isPending}
          error={pullMut.error?.message}
        />
      )}

      <ConfirmDialog
        closeAction={() => setDeleteTarget(null)}
        confirmAction={() => deleteMut.mutate({ id: deleteTarget!.id, nodeId: deleteTarget!.nodeId })}
        confirmLabel="Delete"
        destructive
        loading={deleteMut.isPending}
        open={!!deleteTarget}
        title="Delete Image"
        description={`Are you sure you want to delete image "${deleteTarget?.tags || deleteTarget?.id}" from node "${deleteTarget?.nodeName}"?`}
      />
    </div>
  );
}

function PullImageModal({ onClose, onPull, loading, error }: { onClose: () => void; onPull: (image: string, tag: string, nodeId: string) => void; loading: boolean; error?: string }) {
  const [image, setImage] = useState("");
  const [tag, setTag] = useState("latest");
  const [nodeId, setNodeId] = useState("");

  return (
    <Modal onClose={onClose} title="Pull Image">
      <div className="space-y-4">
        {error && <Alert tone="error" title="Pull failed">{error}</Alert>}
        <Input label="Image Name *" placeholder="nginx" value={image} onChange={setImage} />
        <Input label="Tag" placeholder="latest" value={tag} onChange={setTag} />
        <NodeSelect value={nodeId} onChange={setNodeId} />
        <ModalFooter
          onCancel={onClose}
          onConfirm={() => onPull(image, tag, nodeId)}
          confirmLabel={loading ? "Pulling..." : "Pull"}
          disabled={!image || !nodeId || loading}
        />
      </div>
    </Modal>
  );
}
