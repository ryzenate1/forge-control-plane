"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, Globe, Mail, MessageSquare, Plus, Send, Trash2, Zap, Eye, EyeOff } from "lucide-react";
import {
  createEngineChannel,
  deleteEngineChannel,
  fetchEngineChannels,
  fetchEngineSubscriptions,
  fetchEventCatalog,
  subscribeToEvent,
  testAllEngineChannels,
  testEngineChannel,
  unsubscribeFromEvent,
  updateEngineChannel,
  type EngineChannel,
  type EventDescriptor,
  type NotificationChannelType,
} from "@/lib/api/notifications";
import {
  AdminFormSection,
  AdminSection,
  AdminSelect,
  AdminTable,
  AdminTabs,
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
  Textarea,
} from "./admin-ui";
import { DataState, FreshnessBadge, Reading } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/components/ui/toast";

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

const TYPE_OPTIONS = Object.entries(CHANNEL_LABELS).map(([value, label]) => ({ value, label }));

const SEVERITY_TONES: Record<string, "neutral" | "green" | "yellow" | "red" | "blue"> = {
  info: "blue",
  success: "green",
  warning: "yellow",
  error: "red",
  critical: "red",
};

type FormState = {
  editId: string | null;
  type: NotificationChannelType;
  name: string;
  enabled: boolean;
  webhookUrl: string;
  botToken: string;
  chatId: string;
  recipients: string;
  url: string;
  headers: string;
};

const BLANK_FORM: FormState = {
  editId: null,
  type: "discord",
  name: "",
  enabled: true,
  webhookUrl: "",
  botToken: "",
  chatId: "",
  recipients: "",
  url: "",
  headers: "",
};

function buildConfig(form: FormState): Record<string, unknown> {
  switch (form.type) {
    case "slack":
    case "discord":
      return { webhook_url: form.webhookUrl.trim() };
    case "telegram":
      return { bot_token: form.botToken.trim(), chat_id: form.chatId.trim() };
    case "email":
      return {
        recipients: form.recipients
          .split(",")
          .map((entry) => entry.trim())
          .filter(Boolean),
      };
    case "webhook":
      return { url: form.url.trim(), headers: parseHeaders(form.headers) };
    default:
      return {};
  }
}

function parseHeaders(raw: string): Record<string, string> {
  const trimmed = raw.trim();
  if (!trimmed) return {};
  return JSON.parse(trimmed) as Record<string, string>;
}

/**
 * Who owns the channel.
 *
 * The API serialises `userId`/`orgId` with `omitempty`
 * (`forge/api/internal/store/store_notification_channels.go:33-35`), so a
 * platform-global channel (NULL owner) and a record that simply did not report
 * an owner arrive as the same absent field. Only the first two cases can be
 * claimed; the third must not be dressed up as "Global".
 */
function channelScope(ch: EngineChannel): { label: string; tone: "warn" | "neutral" | "unknown"; note?: string } {
  if (ch.orgId) return { label: "Org", tone: "warn" };
  if (ch.userId) return { label: "Mine", tone: "neutral" };
  return {
    label: "Owner not reported",
    tone: "unknown",
    note: "A channel with no owner is platform-global, but the API omits an absent owner and an unreported owner identically.",
  };
}

export function NotificationsManager() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirmDialog, renderConfirm] = useConfirm();
  const [tab, setTab] = useState("channels");
  const [form, setForm] = useState<FormState | null>(null);
  // Credentials come back from this API in cleartext, so the edit form hides
  // them by default and only shows them on an explicit request.
  const [reveal, setReveal] = useState(false);

  const channelsQuery = useQuery({
    queryKey: ["notifications-engine", "channels"],
    queryFn: fetchEngineChannels,
  });
  const channels = useMemo(() => channelsQuery.data ?? [], [channelsQuery.data]);

  const catalogQuery = useQuery({
    queryKey: ["notifications-engine", "events"],
    queryFn: fetchEventCatalog,
    staleTime: 5 * 60 * 1000,
  });
  const catalogEvents = useMemo(() => catalogQuery.data ?? [], [catalogQuery.data]);

  const subscriptionsQuery = useQuery({
    queryKey: ["notifications-engine", "subscriptions"],
    queryFn: fetchEngineSubscriptions,
  });
  const subscriptions = useMemo(() => subscriptionsQuery.data ?? [], [subscriptionsQuery.data]);

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ["notifications-engine"] });
  };

  const failToast = (title: string) => (err: unknown) =>
    toast({ tone: "error", title, message: err instanceof Error ? err.message : "An error occurred" });

  const saveMut = useMutation({
    mutationFn: async () => {
      if (!form) throw new Error("No channel form open");
      const payload = { type: form.type, name: form.name.trim(), config: buildConfig(form), enabled: form.enabled };
      if (form.editId) return updateEngineChannel(form.editId, payload);
      return createEngineChannel(payload);
    },
    onSuccess: () => {
      setForm(null);
      invalidate();
      toast({ tone: "success", title: form?.editId ? "Channel updated" : "Channel created" });
    },
    onError: failToast("Could not save channel"),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteEngineChannel(id),
    onSuccess: invalidate,
    onError: failToast("Failed to delete channel"),
  });

  const toggleMut = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => updateEngineChannel(id, { enabled }),
    onSuccess: invalidate,
    onError: failToast("Failed to update channel"),
  });

  const testMut = useMutation({
    mutationFn: (id: string) => testEngineChannel(id),
    onSuccess: () => toast({ tone: "success", title: "Test notification delivered" }),
    onError: failToast("Test notification failed"),
  });

  const testAllMut = useMutation({
    mutationFn: testAllEngineChannels,
    onSuccess: (results) => {
      const failed = results.filter((entry) => !entry.ok);
      if (failed.length === 0) {
        toast({ tone: "success", title: `Test sent through ${results.length} channel(s)` });
      } else {
        toast({
          tone: "error",
          title: `${failed.length} of ${results.length} channels failed`,
          message: failed.map((entry) => `${entry.name}: ${entry.error ?? "unknown error"}`).join("\n"),
        });
      }
    },
    onError: failToast("Bulk test failed"),
  });

  const subscribeMut = useMutation({
    mutationFn: ({ channelId, eventType }: { channelId: string; eventType: string }) =>
      subscribeToEvent(channelId, eventType),
    onSuccess: invalidate,
    onError: failToast("Failed to subscribe channel"),
  });

  const unsubscribeMut = useMutation({
    mutationFn: (subscriptionId: string) => unsubscribeFromEvent(subscriptionId),
    onSuccess: invalidate,
    onError: failToast("Failed to unsubscribe channel"),
  });

  function openCreate() {
    setReveal(false);
    setForm({ ...BLANK_FORM });
  }

  function openEdit(ch: EngineChannel) {
    setReveal(false);
    const cfg = (ch.config ?? {}) as Record<string, unknown>;
    setForm({
      editId: ch.id,
      type: ch.type,
      name: ch.name,
      enabled: ch.enabled,
      webhookUrl: (cfg.webhook_url as string) ?? "",
      botToken: (cfg.bot_token as string) ?? "",
      chatId: (cfg.chat_id as string) ?? "",
      recipients: (cfg.recipients as string[])?.join(", ") ?? "",
      url: (cfg.url as string) ?? "",
      headers: cfg.headers ? JSON.stringify(cfg.headers, null, 2) : "",
    });
  }

  function headerCellState(event: EventDescriptor, channel: EngineChannel) {
    const match = subscriptions.find(
      (sub) => sub.channelId === channel.id && sub.eventType === event.type,
    );
    return { subscribed: Boolean(match), subscriptionId: match?.id };
  }

  const groupedEvents = useMemo(() => {
    const groups = new Map<string, EventDescriptor[]>();
    for (const event of catalogEvents) {
      const bucket = groups.get(event.category) ?? [];
      bucket.push(event);
      groups.set(event.category, bucket);
    }
    return [...groups.entries()];
  }, [catalogEvents]);

  const loading = channelsQuery.isPending || catalogQuery.isPending || subscriptionsQuery.isPending;

  return (
    <AdminSection
      description="Your own channels and the ones your admin role can see, served by /notifications/channels. The console below this one is a different backend (/notification-channels) with its own records — a channel created here does not appear there."
      title="Notification engine"
      action={
        <div className="flex flex-wrap items-center gap-2">
          <FreshnessBadge state={sourceState(channelsQuery)} />
          <Btn
            tone="ghost"
            onClick={() => testAllMut.mutate()}
            disabled={testAllMut.isPending || channels.length === 0}
            title={channels.length === 0 ? "No channels to test" : "Send a test through every enabled channel"}
          >
            <Zap size={14} /> {testAllMut.isPending ? "Testing…" : "Test all"}
          </Btn>
          <Btn onClick={openCreate}>
            <Plus size={14} /> Add channel
          </Btn>
        </div>
      }
    >
      <AdminTabs
        tabs={[
          { id: "channels", label: "Channels", icon: Bell },
          { id: "matrix", label: "Subscriptions", icon: Zap },
        ]}
        active={tab}
        onChange={setTab}
        label="Notification engine sections"
      />

      <div className="mt-4 space-y-4">
        {tab === "channels" ? (
          <Card>
            <CardHeader title="Channels" icon={Bell} />
            <DataState
              emptyMessage="Create a Discord, Telegram, Slack, email or webhook channel to start receiving event notifications."
              emptyTitle="No channels yet"
              isEmpty={channels.length === 0}
              loadingLabel="Loading channels…"
              onRetry={() => void channelsQuery.refetch()}
              state={sourceState(channelsQuery)}
            >
              <AdminTable label="Notification channels">
                <AdminTHead>
                  <AdminTh>Channel</AdminTh>
                  <AdminTh>Type</AdminTh>
                  <AdminTh>Scope</AdminTh>
                  <AdminTh>Status</AdminTh>
                  <AdminTh>Events</AdminTh>
                  <AdminTh className="text-right">Actions</AdminTh>
                </AdminTHead>
                <AdminTBody>
                  {channels.map((ch) => {
                    const Icon = CHANNEL_ICONS[ch.type] ?? Bell;
                    const scope = channelScope(ch);
                    const subscriptionsUnavailable = subscriptionsQuery.isError;
                    const subCount = subscriptions.filter((sub) => sub.channelId === ch.id).length;
                    const toggling = toggleMut.isPending && toggleMut.variables?.id === ch.id;
                    return (
                      <AdminTr key={ch.id}>
                        <AdminTd className="font-medium text-text">{ch.name}</AdminTd>
                        <AdminTd>
                          <span className="inline-flex items-center gap-1.5 text-text-subtle">
                            <Icon size={14} aria-hidden="true" /> {CHANNEL_LABELS[ch.type] ?? ch.type}
                          </span>
                        </AdminTd>
                        <AdminTd>
                          <Pill tone={scope.tone} >{scope.label}</Pill>
                          {scope.note ? <span className="mt-1 block max-w-prose text-[11px] leading-5 text-text-muted">{scope.note}</span> : null}
                        </AdminTd>
                        <AdminTd>
                          <Btn
                            ariaLabel={`${ch.enabled ? "Disable" : "Enable"} ${ch.name}`}
                            disabled={toggling}
                            loading={toggling}
                            size="sm"
                            tone="ghost"
                            onClick={() => toggleMut.mutate({ id: ch.id, enabled: !ch.enabled })}
                          >
                            {ch.enabled ? "Enabled" : "Disabled"}
                          </Btn>
                        </AdminTd>
                        <AdminTd>
                          {subscriptionsUnavailable
                            ? <Reading reason="Subscriptions could not be read" value={undefined} />
                            : subCount}
                        </AdminTd>
                        <AdminTd>
                          <div className="flex justify-end gap-1">
                            <Btn
                              size="sm"
                              tone="ghost"
                              loading={testMut.isPending && testMut.variables === ch.id}
                              onClick={() => testMut.mutate(ch.id)}
                              disabled={testMut.isPending}
                            >
                              <Send size={14} /> Test
                            </Btn>
                            <Btn size="sm" tone="ghost" onClick={() => openEdit(ch)} ariaLabel={`Edit ${ch.name}`}>
                              Edit
                            </Btn>
                            <Btn
                              ariaLabel={`Delete ${ch.name}`}
                              size="sm"
                              tone="danger"
                              onClick={() => {
                                void (async () => {
                                  const ok = await confirmDialog({
                                    title: `Delete ${ch.name}?`,
                                    description: "Its subscriptions are removed too and pending notifications stop. This cannot be undone.",
                                    danger: true,
                                    confirmLabel: "Delete",
                                  });
                                  if (ok) deleteMut.mutate(ch.id);
                                })();
                              }}
                            >
                              <Trash2 size={14} />
                            </Btn>
                          </div>
                        </AdminTd>
                      </AdminTr>
                    );
                  })}
                </AdminTBody>
              </AdminTable>
            </DataState>
          </Card>
        ) : (
          <div className="space-y-4">
            <DataState
              emptyMessage="Create a channel first, then wire it to events here."
              emptyTitle="No channels to subscribe"
              isEmpty={channels.length === 0}
              loadingLabel="Loading channels…"
              onRetry={() => void channelsQuery.refetch()}
              state={sourceState(channelsQuery)}
            >
              <DataState
                emptyMessage="The event catalogue returned no events."
                emptyTitle="No events in the catalogue"
                isEmpty={groupedEvents.length === 0}
                loadingLabel="Loading event catalogue…"
                onRetry={() => void catalogQuery.refetch()}
                state={sourceState(catalogQuery)}
              >
                <div className="space-y-4">
                  {groupedEvents.map(([category, categoryEvents]) => (
                    <Card key={category}>
                      <CardHeader title={category.charAt(0).toUpperCase() + category.slice(1)} icon={Zap} />
                      <div className="p-1">
                        <AdminTable label={`Event subscriptions by ${category}`}>
                          <AdminTHead>
                            <AdminTh>Event</AdminTh>
                            {channels.map((ch) => (
                              <AdminTh key={ch.id} className="text-center font-medium normal-case">
                                <span className="inline-flex items-center gap-1 text-text-subtle">
                                  {(() => {
                                    const Icon = CHANNEL_ICONS[ch.type] ?? Bell;
                                    return <Icon size={13} aria-hidden="true" />;
                                  })()}
                                  <span className="max-w-[9rem] truncate">{ch.name}</span>
                                </span>
                              </AdminTh>
                            ))}
                          </AdminTHead>
                          <AdminTBody>
                            {categoryEvents.map((event) => (
                              <AdminTr key={event.type}>
                                <AdminTd>
                                  <div className="flex flex-wrap items-center gap-2">
                                    <span className="font-mono text-xs text-text">{event.type}</span>
                                    <Pill tone={SEVERITY_TONES[event.severity] ?? "neutral"}>{event.severity}</Pill>
                                  </div>
                                  <p className="mt-0.5 text-xs text-text-subtle">{event.description}</p>
                                </AdminTd>
                                {channels.map((ch) => {
                                  const { subscribed, subscriptionId } = headerCellState(event, ch);
                                  const pending = subscribeMut.isPending || unsubscribeMut.isPending;
                                  return (
                                    <AdminTd key={ch.id} className="text-center">
                                      <input
                                        type="checkbox"
                                        className="h-4 w-4 cursor-pointer accent-[var(--brand)] disabled:cursor-not-allowed"
                                        checked={subscribed}
                                        disabled={pending || !ch.enabled}
                                        title={!ch.enabled ? `${ch.name} is disabled` : undefined}
                                        aria-label={`${subscribed ? "Unsubscribe" : "Subscribe"} ${ch.name} from ${event.type}`}
                                        onChange={() => {
                                          if (subscribed && subscriptionId) {
                                            unsubscribeMut.mutate(subscriptionId);
                                          } else {
                                            subscribeMut.mutate({ channelId: ch.id, eventType: event.type });
                                          }
                                        }}
                                      />
                                    </AdminTd>
                                  );
                                })}
                              </AdminTr>
                            ))}
                          </AdminTBody>
                        </AdminTable>
                      </div>
                    </Card>
                  ))}
                </div>
              </DataState>
            </DataState>
            {subscriptionsQuery.isError ? (
              <p className="text-[11px] leading-5 text-warn" role="alert">
                Subscriptions could not be read, so the checkboxes below may not match what the server has.
              </p>
            ) : null}
            {loading ? <p className="text-[11px] text-text-muted" role="status">Loading…</p> : null}
          </div>
        )}
      </div>

      {form ? (
        <Modal
          title={form.editId ? "Edit channel" : "New notification channel"}
          description="The destination is validated by the server on save and again at send time; webhook URLs must be HTTPS and are SSRF-checked."
          onClose={() => setForm(null)}
        >
          <div className="space-y-5">
            <AdminFormSection title="Channel">
              <div className="grid gap-4 sm:grid-cols-2">
                <AdminSelect
                  label="Type"
                  value={form.type}
                  onChange={(value) => setForm({ ...form, type: value as NotificationChannelType })}
                  options={TYPE_OPTIONS}
                  disabled={Boolean(form.editId)}
                />
                <Input
                  label="Name"
                  value={form.name}
                  onChange={(value) => setForm({ ...form, name: value })}
                  placeholder="Ops Discord alerts"
                />
              </div>
              <label className="flex items-center gap-2 text-sm text-text">
                <input
                  type="checkbox"
                  className="h-4 w-4 accent-[var(--brand)]"
                  checked={form.enabled}
                  onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                />
                Enabled
              </label>
            </AdminFormSection>

            <AdminFormSection title="Destination" description="This API returns stored credentials to your session in full — they are masked here for the screen, not for the wire. The server re-validates the destination on save and at send time.">
              <div className="flex justify-end">
                <Btn size="sm" tone="ghost" ariaLabel={reveal ? "Hide credential values" : "Show credential values"} onClick={() => setReveal((value) => !value)}>
                  {reveal ? <EyeOff size={14} /> : <Eye size={14} />} {reveal ? "Hide" : "Show"} values
                </Btn>
              </div>
              {form.type === "slack" || form.type === "discord" ? (
                <Input
                  label="Webhook URL"
                  value={form.webhookUrl}
                  onChange={(value) => setForm({ ...form, webhookUrl: value })}
                  placeholder="https://hooks.discord.com/…"
                  mono
                  type={reveal ? "text" : "password"}
                  autoComplete="off"
                />
              ) : form.type === "telegram" ? (
                <div className="grid gap-4 sm:grid-cols-2">
                  <Input
                    label="Bot token"
                    value={form.botToken}
                    onChange={(value) => setForm({ ...form, botToken: value })}
                    placeholder="123456:ABC-def…"
                    mono
                    type={reveal ? "text" : "password"}
                    autoComplete="off"
                  />
                  <Input
                    label="Chat ID"
                    value={form.chatId}
                    onChange={(value) => setForm({ ...form, chatId: value })}
                    placeholder="-1001234567890"
                    mono
                  />
                </div>
              ) : form.type === "email" ? (
                <Input
                  label="Recipients"
                  value={form.recipients}
                  onChange={(value) => setForm({ ...form, recipients: value })}
                  placeholder="ops@example.com, oncall@example.com"
                />
              ) : (
                <div className="grid gap-4">
                  <Input
                    label="Endpoint URL"
                    value={form.url}
                    onChange={(value) => setForm({ ...form, url: value })}
                    placeholder="https://example.com/hooks/forge"
                    mono
                    type={reveal ? "text" : "password"}
                    autoComplete="off"
                  />
                  <Textarea
                    label="Custom headers (JSON)"
                    value={form.headers}
                    onChange={(value) => setForm({ ...form, headers: value })}
                    rows={4}
                    placeholder='{"Authorization": "Bearer …"}'
                  />
                </div>
              )}
            </AdminFormSection>

            <ModalFooter
              onCancel={() => setForm(null)}
              onConfirm={() => {
                if (!form.name.trim()) {
                  toast({ tone: "error", title: "Channel name is required" });
                  return;
                }
                if (form.type === "webhook") {
                  try {
                    parseHeaders(form.headers);
                  } catch {
                    toast({ tone: "error", title: "Headers must be valid JSON" });
                    return;
                  }
                }
                saveMut.mutate();
              }}
              disabled={saveMut.isPending}
              confirmLabel={form.editId ? "Save changes" : "Create channel"}
            />
          </div>
        </Modal>
      ) : null}

      {renderConfirm()}
    </AdminSection>
  );
}
