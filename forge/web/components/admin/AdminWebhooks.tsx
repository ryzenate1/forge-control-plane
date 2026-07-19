"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Globe, Plus, Trash2 } from "lucide-react";
import { fetchJSON, postJSON, patchJSON, deleteJSON, fetchWebhookDeliveries, type ApiWebhook, type ApiWebhookDelivery } from "@/modules/infrastructure/api";
import { Btn, Card, CardHeader, EmptyState, Input, Modal, ModalFooter, Pill, SectionHeader } from "./admin-ui";

type Webhook = ApiWebhook;
type WebhookResponse = Webhook[] | { data?: unknown; error?: unknown; message?: unknown };
type DeliveryResponse = ApiWebhookDelivery[] | { data?: unknown; error?: unknown; message?: unknown };

function responseError(response: unknown, resource: string): Error {
  if (response && typeof response === "object") {
    const body = response as { error?: unknown; message?: unknown };
    if (typeof body.message === "string" && body.message.trim()) return new Error(body.message);
    if (typeof body.error === "string" && body.error.trim()) return new Error(body.error);
  }
  return new Error(`The ${resource} response did not contain a list.`);
}

function responseList<T>(response: T[] | { data?: unknown; error?: unknown; message?: unknown }, resource: string): T[] {
  if (Array.isArray(response)) return response;
  if (response && typeof response === "object" && Array.isArray(response.data)) return response.data as T[];
  throw responseError(response, resource);
}

function webhookEvents(events: unknown): string[] {
  return Array.isArray(events) ? events.filter((event): event is string => typeof event === "string") : [];
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message.trim() ? error.message : fallback;
}

const AVAILABLE_EVENTS = [
  "server:created", "server:deleted",
  "server:started", "server:stopped",
  "server:installed", "server:reinstalled",
  "server:suspended", "server:unsuspended",
  "server:transferred",
  "node:created", "node:deleted", "node:updated",
  "user:created", "user:deleted",
  "backup:created", "backup:deleted", "backup:restored",
];

export function AdminWebhooks() {
  const qc = useQueryClient();
  const webhooksQuery = useQuery({
    queryKey: ["webhooks"],
    queryFn: async () => {
      const response = await fetchJSON<WebhookResponse>("/webhooks");
      return responseList(response, "webhooks");
    },
  });
  const webhooks = webhooksQuery.data ?? [];

  const [showCreate, setShowCreate] = useState(false);

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [url, setUrl] = useState("");
  const [webhookType, setWebhookType] = useState<"regular" | "discord">("regular");
  const [enabled, setEnabled] = useState(true);
  const [secret, setSecret] = useState("");
  const [events, setEvents] = useState<string[]>([]);
  const [discordUsername, setDiscordUsername] = useState("");
  const [discordAvatarUrl, setDiscordAvatarUrl] = useState("");
  const [discordContent, setDiscordContent] = useState("");

  const [editId, setEditId] = useState<string | null>(null);
  const [historyId, setHistoryId] = useState<string | null>(null);

  const resetForm = () => {
    setName(""); setDescription(""); setUrl(""); setWebhookType("regular");
    setEnabled(true); setSecret(""); setEvents([]);
    setDiscordUsername(""); setDiscordAvatarUrl(""); setDiscordContent("");
  };

  const createMut = useMutation({
    mutationFn: () => postJSON<Webhook>("/webhooks", { name, description, url, webhookType, enabled, secret, events, discordUsername, discordAvatarUrl, discordContent }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["webhooks"] }); setShowCreate(false); resetForm(); },
  });

  const updateMut = useMutation({
    mutationFn: () => patchJSON<Webhook>(`/webhooks/${editId}`, { name, description, url, webhookType, enabled, secret, events, discordUsername, discordAvatarUrl, discordContent }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["webhooks"] }); setEditId(null); resetForm(); },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteJSON(`/webhooks/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["webhooks"] }),
  });

  const openEdit = (wh: Webhook) => {
    setEditId(wh.id);
    setName(wh.name);
    setDescription(wh.description ?? "");
    setUrl(wh.url);
    setWebhookType((wh.webhookType ?? "regular") as "regular" | "discord");
    setEnabled(wh.enabled);
    setSecret(wh.secret ?? "");
    setEvents(wh.events ?? []);
    setDiscordUsername(wh.discordUsername ?? "");
    setDiscordAvatarUrl(wh.discordAvatarUrl ?? "");
    setDiscordContent(wh.discordContent ?? "");
  };

  const toggleEvent = (ev: string) => {
    setEvents((prev) => prev.includes(ev) ? prev.filter((e) => e !== ev) : [...prev, ev]);
  };

  return (
    <div>
      <SectionHeader
        title="Webhooks"
        sub="Event-driven webhook notifications with Discord embed support."
        action={<Btn onClick={() => { resetForm(); setShowCreate(true); }}><Plus size={14} /> New Webhook</Btn>}
      />

      <Card>
        <CardHeader title="Webhook List" icon={Globe} />
        {webhooksQuery.isLoading ? (
          <div className="py-10 text-center text-sm text-slate-500">Loading...</div>
        ) : webhooksQuery.isError ? (
          <div className="p-6 text-sm text-red-300">{errorMessage(webhooksQuery.error, "Webhooks could not be loaded.")}</div>
        ) : webhooks.length === 0 ? (
          <EmptyState icon={Globe} message="No webhooks configured." />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
                <th className="px-4 py-3">Name</th>
                <th className="px-4 py-3">Type</th>
                <th className="px-4 py-3">Events</th>
                <th className="px-4 py-3">URL</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.04]">
              {webhooks.map((wh) => (
                <tr key={wh.id} className="hover:bg-white/[0.02]">
                  <td className="px-4 py-3">
                    <p className="font-medium text-slate-200">{wh.name}</p>
                    {wh.description && <p className="text-xs text-slate-500">{wh.description}</p>}
                  </td>
                  <td className="px-4 py-3">
                    <Pill tone={wh.webhookType === "discord" ? "blue" : "neutral"}>
                      {wh.webhookType === "discord" ? "Discord" : "Regular"}
                    </Pill>
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex flex-wrap gap-1">
                      {webhookEvents(wh.events).slice(0, 3).map((ev) => (
                        <Pill key={ev} tone="neutral">{ev}</Pill>
                      ))}
                      {webhookEvents(wh.events).length > 3 && <span className="text-xs text-slate-500">+{webhookEvents(wh.events).length - 3}</span>}
                    </div>
                  </td>
                  <td className="px-4 py-3 font-mono text-xs text-slate-400 max-w-[200px] truncate">{wh.url}</td>
                  <td className="px-4 py-3"><Pill tone={wh.enabled ? "green" : "yellow"}>{wh.enabled ? "Active" : "Disabled"}</Pill></td>
                  <td className="px-4 py-3 whitespace-nowrap">
                    <Btn size="sm" tone="ghost" onClick={() => setHistoryId(wh.id)}>Deliveries</Btn>
                    <Btn size="sm" tone="ghost" onClick={() => openEdit(wh)}>Edit</Btn>
                    <Btn size="sm" tone="danger" onClick={() => { if (confirm("Delete this webhook?")) deleteMut.mutate(wh.id); }} disabled={deleteMut.isPending}><Trash2 size={12} /></Btn>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      {deleteMut.isError ? <p className="mt-3 text-sm text-red-300">{errorMessage(deleteMut.error, "Webhook could not be deleted.")}</p> : null}

      {historyId ? <WebhookDeliveryModal webhookId={historyId} onClose={() => setHistoryId(null)} /> : null}

      {(showCreate || editId) ? (
        <Modal title={editId ? "Edit Webhook" : "Create Webhook"} onClose={() => { setShowCreate(false); setEditId(null); resetForm(); }}>
          <div className="grid gap-4">
            <Input label="Name" value={name} onChange={setName} placeholder="My Webhook" />
            <Input label="Description" value={description} onChange={setDescription} placeholder="Optional description" />
            <Input label="Payload URL" value={url} onChange={setUrl} placeholder="https://discord.com/api/webhooks/..." />
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="mb-1.5 block text-sm font-medium text-slate-300">Type</label>
                <select className="h-9 w-full rounded-lg border border-white/10 bg-[#0f1419] px-3 text-sm text-slate-100" value={webhookType} onChange={(e) => setWebhookType(e.target.value as "regular" | "discord")}>
                  <option value="regular">Regular</option>
                  <option value="discord">Discord Embed</option>
                </select>
              </div>
              <div>
                <label className="mb-1.5 block font-medium text-sm text-slate-300">Signing Secret</label>
                <input className="h-9 w-full rounded-lg border border-white/10 bg-[#0f1419] px-3 text-sm text-slate-100" type="password" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder={editId ? "Masked; replace to rotate" : "Optional secret"} />
                <p className="mt-1 text-xs text-slate-500">Secrets are masked after creation. Enter a new value to replace the current secret.</p>
              </div>
            </div>
            <label className="flex items-center gap-3 text-sm text-slate-300 cursor-pointer">
              <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="accent-[#dc2626]" />
              Enabled
            </label>

            {webhookType === "discord" && (
              <div className="rounded-lg border border-white/[0.06] bg-[#0f1419] p-4 grid gap-4">
                <h4 className="text-sm font-semibold text-slate-200">Discord Settings</h4>
                <Input label="Username Override" value={discordUsername} onChange={setDiscordUsername} placeholder="My Bot" />
                <Input label="Avatar URL" value={discordAvatarUrl} onChange={setDiscordAvatarUrl} placeholder="https://..." />
                <div>
                  <label className="mb-1.5 block text-sm font-medium text-slate-300">Content</label>
                  <textarea className="h-20 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 py-2 text-sm text-slate-100" value={discordContent} onChange={(e) => setDiscordContent(e.target.value)} placeholder="Optional message content" />
                </div>
                {/* Mini preview */}
                <div className="rounded-lg bg-[#2b2d31] p-3">
                  <div className="flex items-center gap-2 mb-2">
                    {discordAvatarUrl ? <span aria-label="Webhook avatar preview" className="h-6 w-6 rounded-full bg-cover bg-center" role="img" style={{ backgroundImage: `url(${discordAvatarUrl})` }} /> : <div className="h-6 w-6 rounded-full bg-[#5865f2]" />}
                    <span className="text-sm font-medium text-white">{discordUsername || "Webhook"}</span>
                    <span className="text-xs text-[#949ba4]">Today at 12:00</span>
                  </div>
                  {discordContent && <p className="text-sm text-[#dbdee1]">{discordContent}</p>}
                  <div className="mt-2 rounded-lg border border-[#3b3d44] bg-[#2b2d31] p-3">
                    <p className="text-sm font-semibold text-[#dbdee1]">Event Notification</p>
                    <p className="text-xs text-[#949ba4] mt-1">This is a preview of how the webhook will appear in Discord.</p>
                    {events.length > 0 && <p className="text-xs text-[#949ba4] mt-1">Triggered on: {events.join(", ")}</p>}
                  </div>
                </div>
              </div>
            )}

            <div>
              <label className="mb-1.5 block text-sm font-medium text-slate-300">Events</label>
              <div className="grid grid-cols-2 md:grid-cols-3 gap-2 max-h-48 overflow-y-auto">
                {AVAILABLE_EVENTS.map((ev) => (
                  <label key={ev} className="flex items-center gap-2 rounded border border-white/10 bg-[#0f1419] px-3 py-2 text-xs cursor-pointer hover:bg-white/[0.03]">
                    <input type="checkbox" checked={events.includes(ev)} onChange={() => toggleEvent(ev)} className="accent-[#dc2626]" />
                    {ev}
                  </label>
                ))}
              </div>
            </div>
          </div>
          {(editId ? updateMut.isError : createMut.isError) ? <p className="mt-4 text-sm text-red-300">{errorMessage(editId ? updateMut.error : createMut.error, `Webhook could not be ${editId ? "updated" : "created"}.`)}</p> : null}
          <ModalFooter
            onCancel={() => { setShowCreate(false); setEditId(null); resetForm(); }}
            onConfirm={() => editId ? updateMut.mutate() : createMut.mutate()}
            disabled={name.trim() === "" || url.trim() === "" || (editId ? updateMut.isPending : createMut.isPending)}
            confirmLabel={editId ? "Save" : "Create"}
          />
        </Modal>
      ) : null}
    </div>
  );
}

function WebhookDeliveryModal({ webhookId, onClose }: { webhookId: string; onClose: () => void }) {
  const query = useQuery({
    queryKey: ["webhook-deliveries", webhookId],
    queryFn: async () => responseList(await fetchWebhookDeliveries(webhookId) as DeliveryResponse, "webhook deliveries"),
  });
  const deliveries = query.data ?? [];

  return <Modal title="Webhook Delivery History" onClose={onClose} wide>
    <p className="mb-3 text-xs text-slate-400">Delivery retry is not exposed because no retry API exists. Pending deliveries are retried by the backend dispatcher.</p>
    {query.isLoading ? <p className="text-sm text-slate-500">Loading delivery history...</p> : null}
    {query.isError ? <p className="text-sm text-red-300">{errorMessage(query.error, "Delivery history could not be loaded.")}</p> : null}
    {!query.isLoading && !query.isError && deliveries.length === 0 ? <EmptyState icon={Globe} message="No deliveries recorded."/> : null}
    {!query.isLoading && !query.isError && deliveries.length > 0 ? <div className="max-h-[60vh] overflow-auto"><table className="w-full text-xs"><thead><tr className="border-b border-white/10 text-left text-slate-500"><th className="p-2">Created</th><th className="p-2">Event</th><th className="p-2">State</th><th className="p-2">HTTP</th><th className="p-2">Attempts</th><th className="p-2">Failure</th></tr></thead><tbody>{deliveries.map((delivery) => <tr className="border-b border-white/[0.04]" key={delivery.id}><td className="p-2 whitespace-nowrap">{new Date(delivery.createdAt).toLocaleString()}</td><td className="p-2 font-mono">{delivery.eventName}</td><td className="p-2"><Pill tone={delivery.state === "delivered" ? "green" : delivery.state === "failed" ? "red" : "yellow"}>{delivery.state}</Pill></td><td className="p-2">{delivery.responseStatus ?? "—"}</td><td className="p-2">{delivery.attempt}</td><td className="p-2 text-red-300">{delivery.lastError ?? delivery.responseBodyExcerpt ?? "—"}</td></tr>)}</tbody></table></div> : null}
  </Modal>;
}
