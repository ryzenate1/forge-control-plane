"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, Globe, Mail, MessageSquare, Plus, Send, Trash2, Zap } from "lucide-react";
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
  AdminSelect,
  AdminTabs,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  Pill,
  SectionHeader,
  Textarea,
} from "./admin-ui";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { useToast } from "@/components/ui/toast";
import { TableSkeleton } from "@/components/ui/loading-skeleton";

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

function channelScope(ch: EngineChannel): { label: string; tone: "neutral" | "blue" | "yellow" } {
  if (ch.orgId) return { label: "Org", tone: "yellow" };
  if (!ch.userId) return { label: "Global", tone: "blue" };
  return { label: "Mine", tone: "neutral" };
}

export function NotificationsManager() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [confirmDialog, renderConfirm] = useConfirm();
  const [tab, setTab] = useState("channels");
  const [form, setForm] = useState<FormState | null>(null);

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
    setForm({ ...BLANK_FORM });
  }

  function openEdit(ch: EngineChannel) {
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

  const loading = channelsQuery.isLoading || catalogQuery.isLoading || subscriptionsQuery.isLoading;

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Notifications Engine"
        sub="Configure Discord, Telegram, Slack, email and webhook channels, then pick the events each channel should receive. Templated messages are rendered and fanned out by the server."
        action={
          <div className="flex gap-2">
            <Btn
              tone="ghost"
              onClick={() => testAllMut.mutate()}
              disabled={testAllMut.isPending || channels.length === 0}
            >
              <Zap size={14} /> {testAllMut.isPending ? "Testing…" : "Test all"}
            </Btn>
            <Btn onClick={openCreate}>
              <Plus size={14} /> Add channel
            </Btn>
          </div>
        }
      />

      <AdminTabs
        tabs={[
          { id: "channels", label: "Channels", icon: Bell },
          { id: "matrix", label: "Subscriptions", icon: Zap },
        ]}
        active={tab}
        onChange={setTab}
      />

      {loading ? (
        <TableSkeleton rows={4} />
      ) : tab === "channels" ? (
        <Card>
          <CardHeader title="Channels" icon={Bell} />
          {channels.length === 0 ? (
            <EmptyState icon={Bell} title="No channels yet" message="Create a Discord, Telegram, Slack, email or webhook channel to start receiving event notifications." />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-xs uppercase tracking-wider text-slate-500">
                    <th className="px-4 py-3">Channel</th>
                    <th className="px-4 py-3">Type</th>
                    <th className="px-4 py-3">Scope</th>
                    <th className="px-4 py-3">Status</th>
                    <th className="px-4 py-3">Events</th>
                    <th className="px-4 py-3" />
                  </tr>
                </thead>
                <tbody>
                  {channels.map((ch) => {
                    const Icon = CHANNEL_ICONS[ch.type] ?? Bell;
                    const scope = channelScope(ch);
                    const subCount = subscriptions.filter((sub) => sub.channelId === ch.id).length;
                    return (
                      <tr key={ch.id} className="border-b border-white/[0.04] last:border-0">
                        <td className="px-4 py-3 font-medium text-slate-100">{ch.name}</td>
                        <td className="px-4 py-3">
                          <span className="inline-flex items-center gap-1.5 text-slate-300">
                            <Icon size={14} /> {CHANNEL_LABELS[ch.type] ?? ch.type}
                          </span>
                        </td>
                        <td className="px-4 py-3">
                          <Pill tone={scope.tone}>{scope.label}</Pill>
                        </td>
                        <td className="px-4 py-3">
                          <button
                            type="button"
                            onClick={() => toggleMut.mutate({ id: ch.id, enabled: !ch.enabled })}
                            className="cursor-pointer"
                            title={ch.enabled ? "Click to disable" : "Click to enable"}
                          >
                            <Pill tone={ch.enabled ? "green" : "neutral"}>{ch.enabled ? "Enabled" : "Disabled"}</Pill>
                          </button>
                        </td>
                        <td className="px-4 py-3 text-slate-400">{subCount}</td>
                        <td className="px-4 py-3">
                          <div className="flex justify-end gap-1">
                            <Btn size="sm" tone="ghost" onClick={() => testMut.mutate(ch.id)} disabled={testMut.isPending}>
                              <Send size={14} /> Test
                            </Btn>
                            <Btn size="sm" tone="ghost" onClick={() => openEdit(ch)}>
                              Edit
                            </Btn>
                            <button
                              type="button"
                              className="text-red-400 hover:text-red-300"
                              title="Delete channel"
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
                            </button>
                          </div>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      ) : (
        <div className="space-y-4">
          {channels.length === 0 ? (
            <Card>
              <EmptyState icon={Zap} title="No channels to subscribe" message="Create a channel first, then wire it to events here." />
            </Card>
          ) : groupedEvents.length === 0 ? (
            <TableSkeleton rows={3} />
          ) : (
            groupedEvents.map(([category, categoryEvents]) => (
              <Card key={category}>
                <CardHeader title={category.charAt(0).toUpperCase() + category.slice(1)} icon={Zap} />
                <div className="overflow-x-auto p-1">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-white/[0.06] text-left text-xs uppercase tracking-wider text-slate-500">
                        <th className="px-4 py-3">Event</th>
                        {channels.map((ch) => (
                          <th key={ch.id} className="px-3 py-3 text-center font-medium normal-case tracking-normal">
                            <span className="inline-flex items-center gap-1 text-slate-300" title={ch.name}>
                              {CHANNEL_ICONS[ch.type] ? (() => {
                                const Icon = CHANNEL_ICONS[ch.type] ?? Bell;
                                return <Icon size={13} />;
                              })() : null}
                              <span className="max-w-[9rem] truncate">{ch.name}</span>
                            </span>
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {categoryEvents.map((event) => (
                        <tr key={event.type} className="border-b border-white/[0.04] last:border-0">
                          <td className="px-4 py-3">
                            <div className="flex flex-wrap items-center gap-2">
                              <span className="font-mono text-xs text-slate-200">{event.type}</span>
                              <Pill tone={SEVERITY_TONES[event.severity] ?? "neutral"}>{event.severity}</Pill>
                            </div>
                            <p className="mt-0.5 text-xs text-slate-500">{event.description}</p>
                          </td>
                          {channels.map((ch) => {
                            const { subscribed, subscriptionId } = headerCellState(event, ch);
                            const pending = subscribeMut.isPending || unsubscribeMut.isPending;
                            return (
                              <td key={ch.id} className="px-3 py-3 text-center">
                                <input
                                  type="checkbox"
                                  className="h-4 w-4 cursor-pointer accent-[var(--brand)] disabled:cursor-not-allowed"
                                  checked={subscribed}
                                  disabled={pending || !ch.enabled}
                                  aria-label={`${subscribed ? "Unsubscribe" : "Subscribe"} ${ch.name} from ${event.type}`}
                                  onChange={() => {
                                    if (subscribed && subscriptionId) {
                                      unsubscribeMut.mutate(subscriptionId);
                                    } else {
                                      subscribeMut.mutate({ channelId: ch.id, eventType: event.type });
                                    }
                                  }}
                                />
                              </td>
                            );
                          })}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </Card>
            ))
          )}
        </div>
      )}

      {form ? (
        <Modal
          title={form.editId ? "Edit channel" : "New notification channel"}
          description="Configuration is encrypted at rest and validated before saving."
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
              <label className="flex items-center gap-2 text-sm text-slate-300">
                <input
                  type="checkbox"
                  className="h-4 w-4 accent-[var(--brand)]"
                  checked={form.enabled}
                  onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                />
                Enabled
              </label>
            </AdminFormSection>

            <AdminFormSection title="Destination" description="Values are re-validated by the server (HTTPS-only, SSRF-checked) on save and at send time.">
              {form.type === "slack" || form.type === "discord" ? (
                <Input
                  label="Webhook URL"
                  value={form.webhookUrl}
                  onChange={(value) => setForm({ ...form, webhookUrl: value })}
                  placeholder="https://hooks.discord.com/…"
                  mono
                />
              ) : form.type === "telegram" ? (
                <div className="grid gap-4 sm:grid-cols-2">
                  <Input
                    label="Bot token"
                    value={form.botToken}
                    onChange={(value) => setForm({ ...form, botToken: value })}
                    placeholder="123456:ABC-def…"
                    mono
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
    </div>
  );
}
