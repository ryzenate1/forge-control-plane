"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Trash2, RefreshCw, Download } from "lucide-react";
import { listImages, pullImage, deleteImage, type DockerImage } from "@/lib/api/docker";
import { Btn, Card, EmptyState, Input, Modal, ModalFooter } from "@/components/admin/admin-ui";
import { ConfirmDialog, Alert } from "@/components/ui/primitives";

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
  const [search, setSearch] = useState("");
  const [showPull, setShowPull] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<DockerImage | null>(null);

  const imagesQuery = useQuery({
    queryKey: ["docker", "images"],
    queryFn: listImages,
    refetchInterval: 30_000,
  });

  const images = imagesQuery.data ?? [];

  const pullMut = useMutation({
    mutationFn: ({ image, tag }: { image: string; tag: string }) => pullImage(image, tag || undefined),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["docker", "images"] }); setShowPull(false); },
  });

  const deleteMut = useMutation({
    mutationFn: ({ id, nodeId }: { id: string; nodeId: string }) => deleteImage(id, nodeId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ["docker", "images"] }); setDeleteTarget(null); },
  });

  const filtered = images.filter(
    (i) => !search || i.tags.toLowerCase().includes(search.toLowerCase()) || i.id.toLowerCase().includes(search.toLowerCase()),
  );

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
          <div className="p-8 text-center text-sm text-slate-500">Loading images...</div>
        ) : imagesQuery.isError ? (
          <div className="p-4 text-sm text-red-400">Failed to load images.</div>
        ) : filtered.length === 0 ? (
          <EmptyState icon={Download} message={search ? "No images match your search." : "No images found. Pull an image from a registry."} title={search ? "No results" : "No images"} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] bg-[#161b28] text-left text-[10px] uppercase tracking-widest text-slate-500">
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
                {filtered.map((img, idx) => {
                  const parts = img.tags.split(",");
                  const firstTag = parts[0] || "<none>:<none>";
                  const [repo, tag] = firstTag.includes(":") ? firstTag.split(":") : [firstTag, ""];
                  return (
                    <tr key={`${img.id}-${img.nodeId}-${idx}`} className="border-b border-white/[0.03] hover:bg-white/[0.02]">
                      <td className="px-4 py-3 font-medium text-slate-200">{repo}</td>
                      <td className="px-4 py-3 text-slate-400">{tag || "latest"}</td>
                      <td className="px-4 py-3 font-mono text-[11px] text-slate-500">{img.id?.replace("sha256:", "").slice(0, 12) || "-"}</td>
                      <td className="px-4 py-3 text-slate-400">{formatSize(img.size)}</td>
                      <td className="px-4 py-3 text-slate-400">{img.nodeName || img.nodeId?.slice(0, 8)}</td>
                      <td className="px-4 py-3 text-slate-400">{formatCreated(img.created)}</td>
                      <td className="px-4 py-3">
                        <button className="rounded p-1 text-slate-500 hover:bg-white/[0.06] hover:text-red-400" onClick={() => setDeleteTarget(img)} title="Delete" type="button"><Trash2 size={13} /></button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {showPull && (
        <PullImageModal
          onClose={() => setShowPull(false)}
          onPull={(image, tag) => pullMut.mutate({ image, tag })}
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

function PullImageModal({ onClose, onPull, loading, error }: { onClose: () => void; onPull: (image: string, tag: string) => void; loading: boolean; error?: string }) {
  const [image, setImage] = useState("");
  const [tag, setTag] = useState("latest");

  return (
    <Modal onClose={onClose} title="Pull Image">
      <div className="space-y-4">
        {error && <Alert tone="error" title="Pull failed">{error}</Alert>}
        <Input label="Image Name *" placeholder="nginx" value={image} onChange={setImage} />
        <Input label="Tag" placeholder="latest" value={tag} onChange={setTag} />
        <ModalFooter
          onCancel={onClose}
          onConfirm={() => onPull(image, tag)}
          confirmLabel={loading ? "Pulling..." : "Pull"}
          disabled={!image || loading}
        />
      </div>
    </Modal>
  );
}
