"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, Globe, MessageSquare, Mail, Send, Terminal, Trash2, Plus, RefreshCw, Zap } from "lucide-react";
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
  type NotificationEventSubscription,
  AVAILABLE_EVENTS,
  EVENT_LABELS,
} from "@/lib/api/notifications";
import { Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader } from "./admin-ui";

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
  const channels = channelsQuery.data ?? [];

  const logsQuery = useQuery({
    queryKey: ["notification-logs"],
    queryFn: () => fetchNotificationLogs(),
    enabled: view === "logs",
  });
  const logs = logsQuery.data ?? [];

  const subsQuery = useQuery({
    queryKey: ["notification-subs", detailId],
    queryFn: () => (detailId ? fetchSubscriptions(detailId) : Promise.resolve([])),
    enabled: !!detailId,
  });
  const subscriptions = subsQuery.data ?? [];

  function buildConfig(): Record<string, unknown> {
    switch (type) {
      case "slack":
      case "discord":
        return { webhook_url: webhookUrl };
      case "telegram":
        return { bot_token: botToken, chat_id: chatId };
      case "email":
        return { recipients: recipients.split(",").map((s) => s.trim()).filter(Boolean) };
      case "webhook":
        return { url: customUrl, headers: headers ? JSON.parse(headers) : {} };
    }
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
    mutationFn: () =>
      createNotificationChannel({ type, name, config: buildConfig(), enabled }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["notification-channels"] });
      setShowCreate(false);
      resetForm();
    },
  });

  const updateMut = useMutation({
    mutationFn: () =>
      updateNotificationChannel(editId!, { name, config: buildConfig(), enabled }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["notification-channels"] });
      setEditId(null);
      resetForm();
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteNotificationChannel(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-channels"] }),
  });

  const testMut = useMutation({
    mutationFn: (id: string) => testNotificationChannel(id),
  });

  const subscribeMut = useMutation({
    mutationFn: ({ channelId, eventType }: { channelId: string; eventType: string }) =>
      createSubscription(channelId, eventType),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-subs", detailId] }),
  });

  const unsubscribeMut = useMutation({
    mutationFn: ({ channelId, subId }: { channelId: string; subId: string }) =>
      deleteSubscription(channelId, subId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-subs", detailId] }),
  });

  function renderConfigForm() {
    return (
      <div className="space-y-4">
        <div>
          <label className="mb-1.5 block text-sm font-medium text-slate-300">Channel Type</label>
          <select
            className="min-h-9 w-full rounded-lg border border-white/10 bg-surface-card-header px-3 text-sm text-slate-200"
            value={type}
            onChange={(e) => setType(e.target.value as NotificationChannelType)}
          >
            <option value="slack">Slack</option>
            <option value="discord">Discord</option>
            <option value="telegram">Telegram</option>
            <option value="email">Email</option>
            <option value="webhook">Webhook</option>
          </select>
        </div>
        <Input label="Channel Name" value={name} onChange={setName} placeholder="My Slack Channel" required />
        <div className="flex items-center gap-2">
          <input type="checkbox" id="enabled" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} className="rounded" />
          <label htmlFor="enabled" className="text-sm text-slate-300">Enabled</label>
        </div>
        {type === "slack" || type === "discord" ? (
          <Input label={type === "slack" ? "Slack Webhook URL" : "Discord Webhook URL"} value={webhookUrl} onChange={setWebhookUrl} placeholder="https://hooks.slack.com/services/..." />
        ) : type === "telegram" ? (
          <>
            <Input label="Bot Token" value={botToken} onChange={setBotToken} placeholder="123456:ABC-DEF..." />
            <Input label="Chat ID" value={chatId} onChange={setChatId} placeholder="-100123456789" />
          </>
        ) : type === "email" ? (
          <Input label="Recipients (comma-separated)" value={recipients} onChange={setRecipients} placeholder="admin@example.com, team@example.com" />
        ) : type === "webhook" ? (
          <>
            <Input label="Webhook URL" value={customUrl} onChange={setCustomUrl} placeholder="https://example.com/hooks/..." />
            <label className="block text-sm font-medium text-slate-300">
              <span className="mb-1.5 block">Headers (JSON)</span>
              <textarea
                className="min-h-0 w-full rounded-lg border border-white/10 bg-surface-card-header px-3 py-2 font-mono text-xs text-slate-200"
                rows={4}
                value={headers}
                onChange={(e) => setHeaders(e.target.value)}
                placeholder='{"Authorization": "Bearer ..."}'
              />
            </label>
          </>
        ) : null}
      </div>
    );
  }

  function renderSubscriptions(ch: NotificationChannel) {
    return (
      <div className="space-y-4">
        <p className="text-xs text-slate-500">Select which events this channel will receive notifications for.</p>
        <div className="grid gap-2 sm:grid-cols-2">
          {AVAILABLE_EVENTS.map((ev) => {
            const isSubscribed = subscriptions.some((s) => s.eventType === ev);
            return (
              <label key={ev} className="flex cursor-pointer items-center gap-3 rounded-lg border border-white/[0.06] bg-[#151b27] p-3 hover:bg-[#1a2235]">
                <input
                  type="checkbox"
                  checked={isSubscribed}
                  onChange={() => {
                    if (isSubscribed) {
                      const sub = subscriptions.find((s) => s.eventType === ev);
                      if (sub) unsubscribeMut.mutate({ channelId: ch.id, subId: sub.id });
                    } else {
                      subscribeMut.mutate({ channelId: ch.id, eventType: ev });
                    }
                  }}
                  className="rounded"
                />
                <span className="text-sm text-slate-200">{EVENT_LABELS[ev]}</span>
              </label>
            );
          })}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Notifications"
        sub="Configure multi-channel notifications for platform events."
        action={
          <div className="flex gap-2">
            <Btn onClick={() => setView(view === "channels" ? "logs" : "channels")}>
              <RefreshCw size={14} /> {view === "channels" ? "View Logs" : "View Channels"}
            </Btn>
            {view === "channels" ? (
              <Btn onClick={() => { resetForm(); setShowCreate(true); }}>
                <Plus size={14} /> Add Channel
              </Btn>
            ) : null}
          </div>
        }
      />

      {view === "channels" ? (
        <>
          <Card>
            <CardHeader title="Notification Channels" icon={Bell} />
            {channelsQuery.isLoading ? (
              <div className="py-10 text-center text-sm text-slate-500">Loading...</div>
            ) : channelsQuery.isError ? (
              <div className="p-4">
                <div className="flex items-start justify-between gap-4 rounded-lg border border-red-500/20 bg-red-950/10 p-3 text-sm text-red-200">
                  <span>Could not load channels: {channelsQuery.error.message}</span>
                  <Btn size="sm" tone="ghost" onClick={() => void channelsQuery.refetch()}>Retry</Btn>
                </div>
              </div>
            ) : channels.length === 0 ? (
              <EmptyState icon={Bell} message="No notification channels configured." />
            ) : (
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
                    <th className="px-4 py-3">Name</th>
                    <th className="px-4 py-3">Type</th>
                    <th className="px-4 py-3">Status</th>
                    <th className="px-4 py-3">Actions</th>
                    <th className="px-4 py-3" />
                  </tr>
                </thead>
                <tbody className="divide-y divide-white/[0.04]">
                  {channels.map((ch) => (
                    <tr key={ch.id} className="hover:bg-white/[0.02]">
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          {channelIcon(ch.type)}
                          <span className="font-medium text-slate-200">{ch.name}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <Pill tone="blue">{CHANNEL_LABELS[ch.type] ?? ch.type}</Pill>
                      </td>
                      <td className="px-4 py-3">
                        <Pill tone={ch.enabled ? "green" : "neutral"}>{ch.enabled ? "Enabled" : "Disabled"}</Pill>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex gap-1">
                          <Btn size="sm" tone="ghost" onClick={() => { setDetailId(detailId === ch.id ? null : ch.id); }}>
                            <Zap size={14} /> Events
                          </Btn>
                          <Btn size="sm" tone="ghost" onClick={() => { loadForm(ch); }}>
                            Edit
                          </Btn>
                          <Btn size="sm" tone="ghost" onClick={() => testMut.mutate(ch.id)} disabled={testMut.isPending}>
                            <Send size={14} /> Test
                          </Btn>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <button className="text-red-400 hover:text-red-300" onClick={() => { if (confirm("Delete this channel?")) deleteMut.mutate(ch.id); }}>
                          <Trash2 size={14} />
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Card>

          {detailId ? (
            <Card>
              <CardHeader title="Event Subscriptions" icon={Zap} action={<Btn size="sm" tone="ghost" onClick={() => setDetailId(null)}>Close</Btn>} />
              <div className="p-4">
                {(() => {
                  const ch = channels.find((c) => c.id === detailId);
                  return ch ? renderSubscriptions(ch) : <p className="text-sm text-slate-500">Channel not found.</p>;
                })()}
              </div>
            </Card>
          ) : null}

          {showCreate || editId ? (
            <Card>
              <CardHeader title={editId ? "Edit Channel" : "New Channel"} icon={Plus} action={<Btn size="sm" tone="ghost" onClick={() => { setShowCreate(false); setEditId(null); resetForm(); }}>Cancel</Btn>} />
              <div className="p-4 space-y-4">
                {renderConfigForm()}
                <Btn onClick={() => (editId ? updateMut.mutate() : createMut.mutate())} disabled={createMut.isPending || updateMut.isPending}>
                  {editId ? "Save Changes" : "Create Channel"}
                </Btn>
                {createMut.isError ? (
                  <p className="text-sm text-red-400">{createMut.error.message}</p>
                ) : null}
                {updateMut.isError ? (
                  <p className="text-sm text-red-400">{updateMut.error.message}</p>
                ) : null}
              </div>
            </Card>
          ) : null}
        </>
      ) : (
        <Card>
          <CardHeader title="Delivery Logs" icon={RefreshCw} />
          {logsQuery.isLoading ? (
            <div className="py-10 text-center text-sm text-slate-500">Loading...</div>
          ) : logs.length === 0 ? (
            <EmptyState icon={Bell} message="No delivery logs yet." />
          ) : (
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
                  <th className="px-4 py-3">Date</th>
                  <th className="px-4 py-3">Event</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Error</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.04]">
                {logs.map((log) => (
                  <tr key={log.id} className="hover:bg-white/[0.02]">
                    <td className="px-4 py-3 text-xs text-slate-400">{new Date(log.sentAt).toLocaleString()}</td>
                    <td className="px-4 py-3 text-slate-200">{EVENT_LABELS[log.eventType as keyof typeof EVENT_LABELS] ?? log.eventType}</td>
                    <td className="px-4 py-3">
                      <Pill tone={log.status === "delivered" ? "green" : log.status === "failed" ? "red" : "yellow"}>{log.status}</Pill>
                    </td>
                    <td className="px-4 py-3 text-xs text-red-400">{log.error ?? "-"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      )}
    </div>
  );
}
