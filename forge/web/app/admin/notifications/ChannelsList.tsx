"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, Globe, MessageSquare, Mail, Send, Terminal, Trash2, Plus, RefreshCw, Zap, Edit2, Play, Pause } from "lucide-react";
import {
  fetchNotificationChannels,
  createNotificationChannel,
  updateNotificationChannel,
  deleteNotificationChannel,
  testNotificationChannel,
  type NotificationChannel,
  type NotificationChannelType,
} from "@/lib/api/notifications";
import { Btn, Card, CardHeader, EmptyState, Input, Pill, SectionHeader, Modal, Badge } from "@/components/admin/admin-ui";

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

const CHANNEL_COLORS: Record<NotificationChannelType, string> = {
  slack: "bg-purple-500/20 text-purple-400",
  discord: "bg-indigo-500/20 text-indigo-400",
  telegram: "bg-blue-500/20 text-blue-400",
  email: "bg-green-500/20 text-green-400",
  webhook: "bg-orange-500/20 text-orange-400",
};

export function ChannelsList() {
  const qc = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [detailId, setDetailId] = useState<string | null>(null);

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

  const createMut = useMutation({
    mutationFn: () =>
      createNotificationChannel({ 
        type, 
        name, 
        config: buildConfig(), 
        enabled 
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["notification-channels"] });
      setShowCreate(false);
      resetForm();
    },
  });

  const updateMut = useMutation({
    mutationFn: () =>
      updateNotificationChannel(editId!, { 
        name, 
        config: buildConfig(), 
        enabled 
      }),
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

  function buildConfig() {
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
      default:
        return {};
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

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Notification Channels"
        sub="Configure multi-channel notifications for platform events."
        action={
          <Btn onClick={() => { resetForm(); setShowCreate(true); }}>
            <Plus size={14} /> Add Channel
          </Btn>
        }
      />

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
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-white/[0.06] text-left text-xs text-slate-500 uppercase tracking-wider">
                  <th className="px-4 py-3">Name</th>
                  <th className="px-4 py-3">Type</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Actions</th>
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
                      <Badge className={CHANNEL_COLORS[ch.type] || "bg-blue-500/20 text-blue-400"}>
                        {CHANNEL_LABELS[ch.type] ?? ch.type}
                      </Badge>
                    </td>
                    <td className="px-4 py-3">
                      <Pill tone={ch.enabled ? "green" : "neutral"}>
                        {ch.enabled ? "Enabled" : "Disabled"}
                      </Pill>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex gap-1">
                        <Btn size="sm" tone="ghost" onClick={() => { loadForm(ch); }}>
                          <Edit2 size={14} /> Edit
                        </Btn>
                        <Btn size="sm" tone="ghost" onClick={() => testMut.mutate(ch.id)} disabled={testMut.isPending}>
                          <Send size={14} /> Test
                        </Btn>
                        <Btn size="sm" tone="ghost" onClick={() => { 
                          if (confirm("Delete this channel?")) deleteMut.mutate(ch.id); 
                        }}>
                          <Trash2 size={14} /> Delete
                        </Btn>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {showCreate || editId ? (
        <Modal
          onClose={() => { setShowCreate(false); setEditId(null); resetForm(); }}
          title={editId ? "Edit Channel" : "New Channel"}
          wide
        >
          <div className="space-y-4">
            {renderConfigForm()}
            <Btn 
              onClick={() => (editId ? updateMut.mutate() : createMut.mutate())} 
              disabled={createMut.isPending || updateMut.isPending}
            >
              {editId ? "Save Changes" : "Create Channel"}
            </Btn>
            {createMut.isError ? (
              <p className="text-sm text-red-400">{createMut.error.message}</p>
            ) : null}
            {updateMut.isError ? (
              <p className="text-sm text-red-400">{updateMut.error.message}</p>
            ) : null}
          </div>
        </Modal>
      ) : null}
    </div>
  );
}