"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, Globe, MessageSquare, Mail, Send, Trash2, Plus, RefreshCw, Zap, ListChecks, Eye, EyeOff } from "lucide-react";
import {
  fetchNotificationChannels,
  createNotificationChannel,
  updateNotificationChannel,
  deleteNotificationChannel,
  testNotificationChannel,
  fetchSubscriptions,
  createSubscription,
  deleteSubscription,
  fetchNotificationLogs,
  type NotificationChannel,
  type NotificationChannelType,
  AVAILABLE_EVENTS,
  EVENT_LABELS,
} from "@/lib/api/notifications";
import {
  AdminFormSection,
  AdminSection,
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
  Pill,
  Textarea,
} from "./admin-ui";
import { DataState, Reading, FreshnessBadge } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/components/ui/toast";
import { errorMessage, formatDate } from "@/lib/utils";

const CHANNEL_ICONS: Record<NotificationChannelType, typeof Bell> = {
  slack: MessageSquare,
  discord: MessageSquare,
  telegram: Send,
  email: Mail,
  webhook: Globe,
};

const CHANNEL_LABELS: Record<NotificationChannelType, string> = {
  slack: "Slack",
  discord: "Discord",
  telegram: "Telegram",
  email: "Email",
  webhook: "Webhook",
};

function channelIcon(type: NotificationChannelType) {
  const Icon = CHANNEL_ICONS[type] ?? Bell;
  return <Icon size={16} />;
}

export function AdminNotifications() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirm, renderConfirm] = useConfirm();
  const [showCreate, setShowCreate] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [detailId, setDetailId] = useState<string | null>(null);
  const [view, setView] = useState<"channels" | "logs">("channels");

  // Create/Edit form state
  const [type, setType] = useState<NotificationChannelType>("slack");
  const [name, setName] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [webhookUrl, setWebhookUrl] = useState("");
  const [botToken, setBotToken] = useState("");
  const [chatId, setChatId] = useState("");
  const [recipients, setRecipients] = useState("");
  const [customUrl, setCustomUrl] = useState("");
  const [headers, setHeaders] = useState("");

  const channelsQuery = useQuery({
    queryKey: ["notification-channels"],
    queryFn: fetchNotificationChannels,
  });
  const channels = useMemo(() => channelsQuery.data ?? [], [channelsQuery.data]);

  const logsQuery = useQuery({
    queryKey: ["notification-logs"],
    queryFn: () => fetchNotificationLogs(),
    enabled: view === "logs",
  });
  const logs = useMemo(() => logsQuery.data ?? [], [logsQuery.data]);

  const subsQuery = useQuery({
    queryKey: ["notification-subs", detailId],
    queryFn: () => (detailId ? fetchSubscriptions(detailId) : Promise.resolve([])),
    enabled: !!detailId,
  });
  const subscriptions = useMemo(() => subsQuery.data ?? [], [subsQuery.data]);

  /**
   * `JSON.parse` runs before the mutation is dispatched, so malformed headers
   * are a field error the operator can fix rather than a raw `SyntaxError`
   * surfaced in red text after a request that never happened.
   */
  function buildConfig(): { config: Record<string, unknown>; error?: string } {
    switch (type) {
      case "slack":
      case "discord":
        return { config: { webhook_url: webhookUrl } };
      case "telegram":
        return { config: { bot_token: botToken, chat_id: chatId } };
      case "email":
        return { config: { recipients: recipients.split(",").map((s) => s.trim()).filter(Boolean) } };
      case "webhook": {
        if (!headers.trim()) return { config: { url: customUrl, headers: {} } };
        try {
          return { config: { url: customUrl, headers: JSON.parse(headers) as Record<string, string> } };
        } catch {
          return { config: {}, error: "Headers must be valid JSON, for example {\"Authorization\": \"Bearer …\"}." };
        }
      }
      default:
        return { config: {}, error: "Choose a channel type." };
    }
  }

  const [configError, setConfigError] = useState<string | null>(null);

  function submitForm() {
    const built = buildConfig();
    if (built.error) {
      setConfigError(built.error);
      return;
    }
    setConfigError(null);
    if (editId) updateMut.mutate(built.config);
    else createMut.mutate(built.config);
  }

  function resetForm() {
    setType("slack");
    setName("");
    setEnabled(true);
    setWebhookUrl("");
    setBotToken("");
    setChatId("");
    setRecipients("");
    setCustomUrl("");
    setHeaders("");
  }

  function loadForm(ch: NotificationChannel) {
    setEditId(ch.id);
    setType(ch.type);
    setName(ch.name);
    setEnabled(ch.enabled);
    const cfg = ch.config ?? {};
    setWebhookUrl((cfg.webhook_url as string) ?? "");
    setBotToken((cfg.bot_token as string) ?? "");
    setChatId((cfg.chat_id as string) ?? "");
    setRecipients((cfg.recipients as string[])?.join(", ") ?? "");
    setCustomUrl((cfg.url as string) ?? "");
    setHeaders(cfg.headers ? JSON.stringify(cfg.headers, null, 2) : "");
  }

  const createMut = useMutation({
    mutationFn: (config: Record<string, unknown>) =>
      createNotificationChannel({ type, name, config, enabled }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["notification-channels"] });
      setShowCreate(false);
      resetForm();
      toast({ tone: "success", title: "Channel created" });
    },
    onError: (err) => toast({ tone: "error", title: "Could not create channel", message: errorMessage(err, "The control plane rejected the request.") }),
  });

  const updateMut = useMutation({
    mutationFn: (config: Record<string, unknown>) =>
      updateNotificationChannel(editId!, { name, config, enabled }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["notification-channels"] });
      setEditId(null);
      resetForm();
      toast({ tone: "success", title: "Channel updated" });
    },
    onError: (err) => toast({ tone: "error", title: "Could not update channel", message: errorMessage(err, "The control plane rejected the request.") }),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteNotificationChannel(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-channels"] }),
    onError: (err) => toast({ tone: "error", title: "Failed to delete channel", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const testMut = useMutation({
    mutationFn: (id: string) => testNotificationChannel(id),
    onSuccess: () => toast({ tone: "success", title: "Test notification sent", message: "The control plane accepted the send; check the delivery log for the outcome." }),
    onError: (err) => toast({ tone: "error", title: "Failed to send test notification", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const subscribeMut = useMutation({
    mutationFn: ({ channelId, eventType }: { channelId: string; eventType: string }) =>
      createSubscription(channelId, eventType),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-subs", detailId] }),
    onError: (err) => toast({ tone: "error", title: "Failed to subscribe to event", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const unsubscribeMut = useMutation({
    mutationFn: ({ channelId, subId }: { channelId: string; subId: string }) =>
      deleteSubscription(channelId, subId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-subs", detailId] }),
    onError: (err) => toast({ tone: "error", title: "Failed to unsubscribe from event", message: err instanceof Error ? err.message : "An error occurred" }),
  });

  const [reveal, setReveal] = useState(false);

  function openCreateForm() {
    setReveal(false);
    setConfigError(null);
    resetForm();
    setShowCreate(true);
  }

  function openEditForm(ch: NotificationChannel) {
    setReveal(false);
    setConfigError(null);
    loadForm(ch);
  }

  function closeForm() {
    setShowCreate(false);
    setEditId(null);
    setConfigError(null);
    resetForm();
  }

  const destinationFields = (
    <>
      {type === "slack" || type === "discord" ? (
        <AdminFormSection title="Webhook">
          <Input
            autoComplete="off"
            label={type === "slack" ? "Slack Webhook URL" : "Discord Webhook URL"}
            onChange={setWebhookUrl}
            placeholder="https://hooks.slack.com/services/..."
            type={reveal ? "text" : "password"}
            value={webhookUrl}
          />
          <p className="text-[11px] leading-5 text-text-muted">This API returns the stored value to your session; it is hidden here for the screen, not withheld by the server.</p>
        </AdminFormSection>
      ) : type === "telegram" ? (
        <AdminFormSection title="Telegram">
          <Input
            autoComplete="off"
            label="Bot token"
            onChange={setBotToken}
            placeholder="123456:ABC-DEF..."
            type={reveal ? "text" : "password"}
            value={botToken}
          />
          <Input label="Chat ID" mono onChange={setChatId} placeholder="-100123456789" value={chatId} />
        </AdminFormSection>
      ) : type === "email" ? (
        <AdminFormSection title="Email">
          <Input label="Recipients (comma-separated)" onChange={setRecipients} placeholder="admin@example.com, team@example.com" value={recipients} />
        </AdminFormSection>
      ) : type === "webhook" ? (
        <AdminFormSection title="Webhook">
          <Input autoComplete="off" label="Webhook URL" mono onChange={setCustomUrl} placeholder="https://example.com/hooks/..." type={reveal ? "text" : "password"} value={customUrl} />
          <Textarea label="Headers (JSON)" onChange={setHeaders} placeholder='{"Authorization": "Bearer ..."}' value={headers} />
        </AdminFormSection>
      ) : null}
    </>
  );

  return (
    <AdminSection
      description="The admin console for /notification-channels: global channels, per-channel event toggles and the delivery log. These are separate records from the engine above — a channel created there is not listed here."
      title="Admin notification console"
      action={
        <div className="flex flex-wrap items-center gap-2">
          <FreshnessBadge state={sourceState(view === "logs" ? logsQuery : channelsQuery)} />
          <Btn onClick={() => setView(view === "channels" ? "logs" : "channels")} tone="ghost">
            <ListChecks size={14} /> {view === "channels" ? "Delivery log" : "Channels"}
          </Btn>
          <Btn onClick={() => void (view === "channels" ? channelsQuery.refetch() : logsQuery.refetch())} tone="ghost">
            <RefreshCw size={14} /> Reload
          </Btn>
          {view === "channels" ? (
            <Btn onClick={openCreateForm}>
              <Plus size={14} /> Add channel
            </Btn>
          ) : null}
        </div>
      }
    >
      {view === "channels" ? (
        <>
          <Card>
            <CardHeader title="Notification channels" icon={Bell} />
            <DataState
              emptyMessage="No notification channels are configured in this console."
              emptyTitle="No channels configured"
              isEmpty={!Array.isArray(channels) || channels.length === 0}
              loadingLabel="Loading channels…"
              onRetry={() => void channelsQuery.refetch()}
              state={sourceState(channelsQuery)}
            >
              <AdminTable label="Notification channels">
                <AdminTHead>
                  <AdminTh>Name</AdminTh>
                  <AdminTh>Type</AdminTh>
                  <AdminTh>Status</AdminTh>
                  <AdminTh className="text-right">Actions</AdminTh>
                </AdminTHead>
                <AdminTBody>
                  {channels.map((ch) => (
                    <AdminTr key={ch.id}>
                      <AdminTd>
                        <div className="flex items-center gap-2">
                          {channelIcon(ch.type)}
                          <span className="font-medium text-text">{ch.name}</span>
                        </div>
                      </AdminTd>
                      <AdminTd>
                        <Pill tone="info">{CHANNEL_LABELS[ch.type] ?? ch.type}</Pill>
                      </AdminTd>
                      <AdminTd>
                        <Pill tone={ch.enabled ? "ok" : "neutral"}>{ch.enabled ? "Enabled" : "Disabled"}</Pill>
                      </AdminTd>
                      <AdminTd>
                        <div className="flex justify-end gap-1">
                          <Btn ariaLabel={`${detailId === ch.id ? "Hide" : "Show"} events for ${ch.name}`} onClick={() => setDetailId(detailId === ch.id ? null : ch.id)} size="sm" tone="ghost">
                            <Zap size={14} /> Events
                          </Btn>
                          <Btn ariaLabel={`Edit ${ch.name}`} onClick={() => openEditForm(ch)} size="sm" tone="ghost">
                            Edit
                          </Btn>
                          <Btn
                            ariaLabel={`Send a test through ${ch.name}`}
                            disabled={testMut.isPending}
                            loading={testMut.isPending && testMut.variables === ch.id}
                            onClick={() => testMut.mutate(ch.id)}
                            size="sm"
                            tone="ghost"
                          >
                            <Send size={14} /> Test
                          </Btn>
                          <Btn
                            ariaLabel={`Delete ${ch.name}`}
                            onClick={() => {
                              void (async () => {
                                if (await confirm({ title: `Delete notification channel ${ch.name}?`, description: "Notifications for this channel will stop. This cannot be undone.", danger: true, confirmLabel: "Delete" })) deleteMut.mutate(ch.id);
                              })();
                            }}
                            size="sm"
                            tone="danger"
                          >
                            <Trash2 size={14} />
                          </Btn>
                        </div>
                      </AdminTd>
                    </AdminTr>
                  ))}
                </AdminTBody>
              </AdminTable>
            </DataState>
          </Card>

          {detailId ? (
            <Card>
              <CardHeader title="Event subscriptions" icon={Zap} action={<Btn onClick={() => setDetailId(null)} size="sm" tone="ghost">Close</Btn>} />
              <div className="p-4">
                {(() => {
                  const ch = channels.find((c) => c.id === detailId);
                  if (!ch) return <p className="text-sm text-text-subtle">This channel is no longer in the list.</p>;
                  return (
                    <DataState
                      emptyMessage="No events are subscribed. Tick an event to create a subscription."
                      emptyTitle="No subscriptions"
                      isEmpty={subscriptions.length === 0}
                      loadingLabel="Loading subscriptions…"
                      onRetry={() => void subsQuery.refetch()}
                      state={sourceState(subsQuery)}
                    >
                      <div className="space-y-4">
                        <p className="text-xs text-text-subtle">Select which events this channel will receive notifications for.</p>
                        <div className="grid gap-2 sm:grid-cols-2">
                          {AVAILABLE_EVENTS.map((ev) => {
                            const isSubscribed = subscriptions.some((s) => s.eventType === ev);
                            const pending = subscribeMut.isPending || unsubscribeMut.isPending;
                            return (
                              <label className="flex cursor-pointer items-center gap-3 rounded-lg border border-line bg-overlay-subtle p-3" key={ev}>
                                <input
                                  aria-label={`${isSubscribed ? "Unsubscribe from" : "Subscribe to"} ${EVENT_LABELS[ev] ?? ev}`}
                                  checked={isSubscribed}
                                  className="rounded accent-[var(--brand)]"
                                  disabled={pending}
                                  onChange={() => {
                                    if (isSubscribed) {
                                      const sub = subscriptions.find((s) => s.eventType === ev);
                                      if (sub) unsubscribeMut.mutate({ channelId: ch.id, subId: sub.id });
                                    } else {
                                      subscribeMut.mutate({ channelId: ch.id, eventType: ev });
                                    }
                                  }}
                                  type="checkbox"
                                />
                                <span className="text-sm text-text">{EVENT_LABELS[ev] ?? ev}</span>
                              </label>
                            );
                          })}
                        </div>
                      </div>
                    </DataState>
                  );
                })()}
              </div>
            </Card>
          ) : null}

          {showCreate || editId ? (
            <Card>
              <CardHeader action={<Btn onClick={closeForm} size="sm" tone="ghost">Cancel</Btn>} icon={Plus} title={editId ? "Edit channel" : "New channel"} />
              <div className="space-y-4 p-4">
                <AdminFormSection title="Channel configuration">
                  <AdminSelect label="Channel type" onChange={(v) => setType(v as NotificationChannelType)} options={[
                    { value: "slack", label: "Slack" },
                    { value: "discord", label: "Discord" },
                    { value: "telegram", label: "Telegram" },
                    { value: "email", label: "Email" },
                    { value: "webhook", label: "Webhook" },
                  ]} value={type} />
                  <Input label="Channel name" onChange={setName} placeholder="My Slack channel" required value={name} />
                  <label className="flex cursor-pointer items-center gap-2 text-sm text-text">
                    <input checked={enabled} className="rounded accent-[var(--brand)]" onChange={(e) => setEnabled(e.target.checked)} type="checkbox" />
                    Enabled
                  </label>
                  <div className="flex justify-end">
                    <Btn ariaLabel={reveal ? "Hide credential values" : "Show credential values"} onClick={() => setReveal((value) => !value)} size="sm" tone="ghost">
                      {reveal ? <EyeOff size={14} /> : <Eye size={14} />} {reveal ? "Hide" : "Show"} values
                    </Btn>
                  </div>
                </AdminFormSection>
                {destinationFields}
                {configError ? <p className="ui-alert ui-alert-danger text-sm" role="alert">{configError}</p> : null}
                <Btn disabled={createMut.isPending || updateMut.isPending} onClick={submitForm}>
                  {editId ? "Save changes" : "Create channel"}
                </Btn>
                {createMut.isError ? (
                  <p className="text-sm text-danger" role="alert">{errorMessage(createMut.error, "The channel could not be created.")}</p>
                ) : null}
                {updateMut.isError ? (
                  <p className="text-sm text-danger" role="alert">{errorMessage(updateMut.error, "The channel could not be updated.")}</p>
                ) : null}
              </div>
            </Card>
          ) : null}
        </>
      ) : (
        <Card>
          <CardHeader title="Delivery log" icon={ListChecks} />
          <DataState
            emptyMessage="No delivery has been recorded yet. This is a successful read that found nothing, not a gap in the log."
            emptyTitle="No delivery logs"
            isEmpty={!Array.isArray(logs) || logs.length === 0}
            loadingLabel="Loading delivery log…"
            onRetry={() => void logsQuery.refetch()}
            state={sourceState(logsQuery)}
          >
            <AdminTable label="Notification delivery log">
              <AdminTHead>
                <AdminTh>Date</AdminTh>
                <AdminTh>Event</AdminTh>
                <AdminTh>Status</AdminTh>
                <AdminTh>Error</AdminTh>
              </AdminTHead>
              <AdminTBody>
                {logs.map((log) => (
                  <AdminTr key={log.id}>
                    <AdminTd className="text-xs text-text-subtle">{log.sentAt ? formatDate(log.sentAt, "Timestamp not reported") : <Reading reason="No timestamp reported" value={undefined} />}</AdminTd>
                    <AdminTd className="text-text">{EVENT_LABELS[log.eventType as keyof typeof EVENT_LABELS] ?? log.eventType}</AdminTd>
                    <AdminTd>
                      <Pill tone={log.status === "delivered" ? "ok" : log.status === "failed" ? "danger" : "warn"}>{log.status}</Pill>
                    </AdminTd>
                    <AdminTd className="text-xs text-text-subtle">
                      {log.status === "failed" ? <Reading reason="The failure was not recorded" value={log.error} /> : <Reading reason="No error for a delivered message" value={undefined} />}
                    </AdminTd>
                  </AdminTr>
                ))}
              </AdminTBody>
            </AdminTable>
          </DataState>
        </Card>
      )}
      {renderConfirm()}
    </AdminSection>
  );
}
