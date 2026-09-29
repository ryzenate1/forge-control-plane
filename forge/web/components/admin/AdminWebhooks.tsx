"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Webhook, Plus, RotateCcw, Trash2 } from "lucide-react";
import { fetchJSON, postJSON, patchJSON, deleteJSON, fetchWebhookDeliveries, retryWebhookDelivery, type ApiWebhook, type ApiWebhookDelivery } from "@/lib/api";
import { safeExternalUrl } from "@/lib/safe-url";
import { chart } from "@/lib/design-tokens";
import {
  AdminErrorState,
  AdminFormSection,
  AdminLoadingRows,
  AdminSelect,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  Pill,
  SectionHeader,
} from "./admin-ui";
import { DataState } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/components/ui/toast";
import { errorMessage, formatDate } from "@/lib/utils";

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

function isValidUrl(str: string): boolean {
  try {
    const url = new URL(str);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

/** What `POST /webhooks/:id/test` answers: the delivery row it created (201). */
type TestDeliveryAck = { id?: string; state?: string; eventName?: string };

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
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const webhooksQuery = useQuery({
    queryKey: ["webhooks"],
    queryFn: async () => {
      const response = await fetchJSON<WebhookResponse>("/webhooks");
      return responseList(response, "webhooks");
    },
  });
  const webhooks = useMemo(() => webhooksQuery.data ?? [], [webhooksQuery.data]);

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

  const urlError = url.trim() && !isValidUrl(url.trim()) ? "Must be a valid HTTP or HTTPS URL" : null;
  const safeDiscordAvatarUrl = safeExternalUrl(discordAvatarUrl);

  const resetForm = () => {
    setName(""); setDescription(""); setUrl(""); setWebhookType("regular");
    setEnabled(true); setSecret(""); setEvents([]);
    setDiscordUsername(""); setDiscordAvatarUrl(""); setDiscordContent("");
  };

  const createMut = useMutation({
    mutationFn: () => postJSON<Webhook>("/webhooks", { name, description, url, webhookType, enabled, secret, events, discordUsername, discordAvatarUrl, discordContent }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["webhooks"] }); setShowCreate(false); resetForm(); toast({ tone: "success", title: "Webhook created" }); },
    onError: (error) => toast({ tone: "error", title: "Webhook could not be created", message: errorMessage(error, "The control plane rejected the request.") }),
  });

  const updateMut = useMutation({
    mutationFn: () => patchJSON<Webhook>(`/webhooks/${editId}`, { name, description, url, webhookType, enabled, secret, events, discordUsername, discordAvatarUrl, discordContent }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["webhooks"] }); setEditId(null); resetForm(); toast({ tone: "success", title: "Webhook updated" }); },
    onError: (error) => toast({ tone: "error", title: "Webhook could not be updated", message: errorMessage(error, "The control plane rejected the request.") }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteJSON(`/webhooks/${id}`),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["webhooks"] }); toast({ tone: "success", title: "Webhook deleted" }); },
    onError: (error) => toast({ tone: "error", title: "Webhook could not be deleted", message: errorMessage(error, "The control plane rejected the request.") }),
  });

  /**
   * `POST /webhooks/:id/test` does not deliver anything: it inserts a delivery
   * row for the dispatcher to attempt later and answers 201 with that row
   * (`forge/api/internal/http/handlers_admin.go:1773-1788`). The toast says so,
   * names the row's state when the response carries one, and the failure path is
   * reported — previously a 403/404/503 produced complete silence while a
   * success toast claimed "Test delivery fired" for work that had not run.
   */
  const testMut = useMutation({
    mutationFn: (id: string) => postJSON<TestDeliveryAck>(`/webhooks/${id}/test`, {}),
    onSuccess: (ack) => {
      void qc.invalidateQueries({ queryKey: ["webhook-deliveries"] });
      toast({
        tone: "success",
        title: "Test delivery queued",
        message: `Recorded${ack?.state ? ` as ${ack.state}` : ""} for the dispatcher to attempt. Open Deliveries for the outcome.`,
      });
    },
    onError: (error) => toast({ tone: "error", title: "Test delivery could not be queued", message: errorMessage(error, "The control plane rejected the request.") }),
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

  const closeForm = () => { setShowCreate(false); setEditId(null); resetForm(); };

  return (
    <div className="space-y-6">
      <SectionHeader
        sub="Endpoints Forge POSTs event payloads to, with delivery history and manual retry for each one."
        action={<Btn onClick={() => { resetForm(); setShowCreate(true); }}><Plus size={14} /> New webhook</Btn>}
        info={{
          title: "Webhooks",
          triggerLabel: "About webhooks",
          description: "What a webhook is here, and what the test button proves.",
          sections: [
            {
              title: "Test queues, it does not deliver",
              content: "Test creates a delivery row and returns it; a background dispatcher performs the attempt. A green toast means the request was recorded — read the Deliveries list for the HTTP status and any failure.",
            },
            {
              title: "Signing secret",
              content: "The secret is stored server-side and not returned in full after creation. Enter a value only to replace it; leaving the field untouched keeps what is stored.",
            },
            {
              title: "Deleting a webhook",
              content: "Removes the endpoint and stops every future delivery to it. Recorded delivery history for that endpoint is no longer reachable from this page.",
            },
          ],
        }}
      />

      <Card>
        <CardHeader icon={Webhook} title="Webhook endpoints" />
        <DataState
          emptyMessage="No webhook endpoints are configured. Create one to receive platform events."
          emptyTitle="No webhooks configured"
          isEmpty={webhooks.length === 0}
          loadingLabel="Loading webhooks…"
          onRetry={() => void webhooksQuery.refetch()}
          state={sourceState(webhooksQuery)}
        >
          <AdminTable label="Webhook endpoints">
            <AdminTHead>
              <AdminTh>Name</AdminTh>
              <AdminTh>Type</AdminTh>
              <AdminTh>Events</AdminTh>
              <AdminTh>URL</AdminTh>
              <AdminTh>Status</AdminTh>
              <AdminTh className="text-right">Actions</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {webhooks.map((wh) => (
                <AdminTr key={wh.id}>
                  <AdminTd>
                    <p className="font-medium text-text">{wh.name}</p>
                    {wh.description ? <p className="text-meta text-text-subtle">{wh.description}</p> : null}
                  </AdminTd>
                  <AdminTd>
                    <Pill tone={wh.webhookType === "discord" ? "info" : "neutral"}>
                      {wh.webhookType === "discord" ? "Discord" : "Regular"}
                    </Pill>
                  </AdminTd>
                  <AdminTd>
                    <div className="flex flex-wrap gap-1">
                      {webhookEvents(wh.events).slice(0, 3).map((ev) => (
                        <Pill key={ev} tone="neutral">{ev}</Pill>
                      ))}
                      {webhookEvents(wh.events).length > 3 ? (
                        <span className="text-[11px] text-text-muted">+{webhookEvents(wh.events).length - 3} more</span>
                      ) : null}
                      {webhookEvents(wh.events).length === 0 ? <span className="text-[11px] text-text-muted">No events subscribed</span> : null}
                    </div>
                  </AdminTd>
                  {/* Always rendered and never truncated: the table scrolls, and a URL an
                      operator may need to copy in full does not live only in a tooltip. */}
                  <AdminTd className="max-w-64 break-all font-mono text-xs text-text-subtle">{wh.url}</AdminTd>
                  <AdminTd><Pill tone={wh.enabled ? "ok" : "warn"}>{wh.enabled ? "Active" : "Disabled"}</Pill></AdminTd>
                  <AdminTd className="text-right whitespace-nowrap">
                    <div className="flex justify-end gap-1.5">
                      <Btn ariaLabel={`Show deliveries for ${wh.name}`} onClick={() => setHistoryId(wh.id)} size="sm" tone="ghost">Deliveries</Btn>
                      <Btn ariaLabel={`Edit ${wh.name}`} onClick={() => openEdit(wh)} size="sm" tone="ghost">Edit</Btn>
                      <Btn
                        ariaLabel={`Queue a test delivery to ${wh.name}`}
                        disabled={testMut.isPending}
                        loading={testMut.isPending && testMut.variables === wh.id}
                        onClick={() => testMut.mutate(wh.id)}
                        size="sm"
                        tone="ghost"
                      >Test</Btn>
                      <Btn
                        ariaLabel={`Delete ${wh.name}`}
                        onClick={() => {
                          void (async () => {
                            const ok = await confirm({
                              title: `Delete ${wh.name}?`,
                              description: "The endpoint and its subscription to these events are removed: " +
                                `${webhookEvents(wh.events).length || "no"} event subscription(s), and deliveries to ${wh.url} stop immediately. This cannot be undone.`,
                              danger: true,
                              confirmLabel: "Delete",
                            });
                            if (ok) deleteMut.mutate(wh.id);
                          })();
                        }}
                        size="sm"
                        tone="danger"
                      ><Trash2 size={12} /></Btn>
                    </div>
                  </AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
        </DataState>
      </Card>

      {historyId ? <WebhookDeliveryModal webhookId={historyId} onClose={() => setHistoryId(null)} /> : null}

      {(showCreate || editId) ? (
        <Modal onClose={closeForm} title={editId ? "Edit webhook" : "Create webhook"}>
          <div className="space-y-4">
            <AdminFormSection title="Webhook details">
              <Input label="Name" onChange={setName} placeholder="My webhook" value={name} />
              <Input label="Description" onChange={setDescription} placeholder="Optional description" value={description} />
              <div>
                <Input
                  label="Payload URL"
                  onChange={setUrl}
                  placeholder="https://discord.com/api/webhooks/..."
                  value={url}
                />
                {urlError ? (
                  <p className="ui-field-error mt-1" role="alert">{urlError}</p>
                ) : null}
              </div>
              <AdminSelect label="Type" onChange={(v) => setWebhookType(v as "regular" | "discord")} options={[
                { value: "regular", label: "Regular JSON payload" },
                { value: "discord", label: "Discord embed" },
              ]} value={webhookType} />
              <Input autoComplete="off" label="Signing secret" onChange={setSecret} placeholder={editId ? "leave unchanged to keep the stored secret" : "Optional secret"} type="password" value={secret} />
              <label className="flex items-center gap-3 text-sm text-text">
                <input checked={enabled} className="accent-[var(--brand)]" onChange={(e) => setEnabled(e.target.checked)} type="checkbox" />
                Enabled
              </label>
            </AdminFormSection>

            {webhookType === "discord" && (
              <AdminFormSection title="Discord presentation">
                <Input label="Username override" onChange={setDiscordUsername} placeholder="My bot" value={discordUsername} />
                <Input label="Avatar URL" onChange={setDiscordAvatarUrl} placeholder="https://..." value={discordAvatarUrl} />
                <Input label="Content" onChange={setDiscordContent} placeholder="Optional message content" value={discordContent} />
                <div className="rounded-lg border border-line bg-overlay-subtle p-3">
                  <div className="mb-2 flex items-center gap-2.5">
                    {safeDiscordAvatarUrl ? (
                      <span aria-label="Webhook avatar preview" className="h-6 w-6 rounded-full bg-cover bg-center" role="img" style={{ backgroundImage: `url(${safeDiscordAvatarUrl})` }} />
                    ) : (
                      <span aria-hidden="true" className="h-6 w-6 rounded-full" style={{ backgroundColor: chart.discord }} />
                    )}
                    <span className="text-sm font-medium leading-none text-text">{discordUsername || "Webhook"}</span>
                  </div>
                  {discordContent ? <p className="text-sm leading-relaxed text-text">{discordContent}</p> : null}
                  <div className="mt-2 rounded-lg border-l-4 bg-overlay-subtle p-3" style={{ borderLeftColor: chart.discord }}>
                    <p className="text-sm font-semibold text-text">Event notification</p>
                    <p className="mt-1 text-xs text-text-subtle">Preview of how a delivered message is presented in Discord.</p>
                    {events.length > 0 ? <p className="mt-1 text-xs text-text-subtle">Triggered on: {events.join(", ")}</p> : <p className="mt-1 text-xs text-text-subtle">No events selected yet, so nothing would trigger this webhook.</p>}
                  </div>
                </div>
              </AdminFormSection>
            )}

            <AdminFormSection description="Select the events that create a delivery to this endpoint." title="Events">
              <fieldset className="grid max-h-48 grid-cols-1 gap-2 overflow-y-auto sm:grid-cols-2 md:grid-cols-3">
                <legend className="sr-only">Events delivered to this webhook</legend>
                {AVAILABLE_EVENTS.map((ev) => (
                  <label className="flex cursor-pointer items-center gap-2.5 rounded-lg border border-line bg-overlay-subtle px-3 py-2.5 text-sm text-text" key={ev}>
                    <input checked={events.includes(ev)} className="accent-[var(--brand)]" onChange={() => toggleEvent(ev)} type="checkbox" />
                    {ev}
                  </label>
                ))}
              </fieldset>
            </AdminFormSection>
            {(editId ? updateMut.isError : createMut.isError) ? (
              <AdminErrorState message={errorMessage(editId ? updateMut.error : createMut.error, `The webhook could not be ${editId ? "updated" : "created"}.`)} />
            ) : null}
          </div>
          <ModalFooter
            confirmLabel={editId ? "Save" : "Create"}
            disabled={name.trim() === "" || url.trim() === "" || Boolean(urlError) || (editId ? updateMut.isPending : createMut.isPending)}
            onCancel={closeForm}
            onConfirm={() => { if (urlError) return; if (editId) updateMut.mutate(); else createMut.mutate(); }}
          />
        </Modal>
      ) : null}
      {renderConfirm()}
    </div>
  );
}

function WebhookDeliveryModal({ webhookId, onClose }: { webhookId: string; onClose: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const query = useQuery({
    queryKey: ["webhook-deliveries", webhookId],
    queryFn: async () => responseList(await fetchWebhookDeliveries(webhookId) as DeliveryResponse, "webhook deliveries"),
  });
  const retryMut = useMutation({
    mutationFn: (deliveryId: string) => retryWebhookDelivery(webhookId, deliveryId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["webhook-deliveries", webhookId] });
      toast({ tone: "success", title: "Retry queued", message: "The dispatcher performs the attempt; watch this list for the outcome." });
    },
    onError: (error) => toast({ tone: "error", title: "Retry could not be queued", message: errorMessage(error, "The control plane rejected the request.") }),
  });
  const deliveries = query.data ?? [];
  const failedCount = deliveries.filter((d) => d.state === "failed").length;
  const state = sourceState(query);

  return <Modal description="Failed deliveries can be retried here; the backend dispatcher also retries pending ones." onClose={onClose} title="Delivery history" wide>
    <div className="space-y-4">
      <p className="text-xs text-text-subtle" role="status">
        {query.isPending
          ? "Reading delivery history…"
          : query.isError
            ? "Counts unavailable — the delivery list could not be read."
            : `${deliveries.length} recorded · ${failedCount} failed`}
      </p>
      {query.isPending ? <AdminLoadingRows cols={5} rows={4} label="Loading delivery history" /> : null}
      {query.isError ? (
        <AdminErrorState
          message={`${errorMessage(query.error, "Delivery history could not be loaded.")} The list below is not known to be empty.`}
          retry={() => void query.refetch()}
        />
      ) : null}
      {!query.isPending && !query.isError && deliveries.length === 0 ? (
        <EmptyState icon={Webhook} message="No delivery has been recorded for this endpoint. That means nothing has been queued to it, not that it has been verified." title="No deliveries recorded" />
      ) : null}
      {deliveries.length > 0 ? (
        <AdminTable className="max-h-[60vh] overflow-auto" label="Deliveries for this webhook">
          <AdminTHead>
            <AdminTh>Created</AdminTh>
            <AdminTh>Event</AdminTh>
            <AdminTh>State</AdminTh>
            <AdminTh>HTTP status</AdminTh>
            <AdminTh>Attempts</AdminTh>
            <AdminTh>Failure</AdminTh>
            <AdminTh className="text-right">Action</AdminTh>
          </AdminTHead>
          <AdminTBody>
            {deliveries.map((delivery) => (
              <AdminTr key={delivery.id}>
                <AdminTd className="whitespace-nowrap text-xs">{formatDate(delivery.createdAt, "Timestamp not reported")}</AdminTd>
                <AdminTd className="max-w-40 break-all font-mono text-xs">{delivery.eventName}</AdminTd>
                <AdminTd>
                  <Pill tone={delivery.state === "delivered" ? "ok" : delivery.state === "failed" ? "danger" : "warn"}>
                    {delivery.state}
                  </Pill>
                </AdminTd>
                <AdminTd className="font-mono text-xs">{delivery.responseStatus ?? "—"}</AdminTd>
                <AdminTd className="font-mono text-xs">{delivery.attempt}</AdminTd>
                <AdminTd className="max-w-60 break-words text-xs text-text-subtle">
                  {delivery.lastError ?? (delivery.state === "failed" ? "Failure reason not recorded" : "—")}
                </AdminTd>
                <AdminTd className="text-right">
                  {delivery.state === "failed" ? (
                    <Btn
                      ariaLabel={`Queue a retry for the delivery to ${delivery.eventName}`}
                      disabled={retryMut.isPending}
                      loading={retryMut.isPending && retryMut.variables === delivery.id}
                      onClick={() => {
                        void (async () => {
                          const ok = await confirm({
                            title: "Retry this delivery?",
                            description: "Queues another attempt against the same payload. The endpoint will receive the event again.",
                            confirmLabel: "Retry",
                          });
                          if (ok) retryMut.mutate(delivery.id);
                        })();
                      }}
                      size="sm"
                      tone="ghost"
                    ><RotateCcw size={12} /> Retry</Btn>
                  ) : null}
                </AdminTd>
              </AdminTr>
            ))}
          </AdminTBody>
        </AdminTable>
      ) : null}
      {state.refreshing && !query.isPending ? <p className="text-[11px] text-text-muted" role="status">Refreshing…</p> : null}
      {renderConfirm()}
    </div>
  </Modal>;
}
