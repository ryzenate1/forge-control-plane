"use client";

import { useState, useEffect } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Bell, Globe, MessageSquare, Mail, Send, Check, X } from "lucide-react";
import {
  createNotificationChannel,
  updateNotificationChannel,
  type NotificationChannel,
  type NotificationChannelType,
} from "@/lib/api/notifications";
import { Btn, Input, Modal } from "@/components/admin/admin-ui";

interface ChannelFormProps {
  channel?: NotificationChannel;
  onClose: () => void;
  onSuccess: () => void;
}

const CHANNEL_ICONS: Record<NotificationChannelType, typeof Bell> = {
  slack: MessageSquare,
  discord: MessageSquare,
  telegram: Send,
  email: Mail,
  webhook: Globe,
};

const CHANNEL_CONFIG_FIELDS: Record<NotificationChannelType, { fields: string[]; labels: Record<string, string> }> = {
  slack: {
    fields: ["webhookUrl"],
    labels: { webhookUrl: "Slack Webhook URL" }
  },
  discord: {
    fields: ["webhookUrl"],
    labels: { webhookUrl: "Discord Webhook URL" }
  },
  telegram: {
    fields: ["botToken", "chatId"],
    labels: { botToken: "Bot Token", chatId: "Chat ID" }
  },
  email: {
    fields: ["recipients"],
    labels: { recipients: "Recipients (comma-separated)" }
  },
  webhook: {
    fields: ["customUrl", "headers"],
    labels: { customUrl: "Webhook URL", headers: "Headers (JSON)" }
  }
};

export function ChannelForm({ channel, onClose, onSuccess }: ChannelFormProps) {
  const qc = useQueryClient();
  const isEdit = !!channel;
  
  // Form state
  const [type, setType] = useState<NotificationChannelType>(channel?.type || "slack");
  const [name, setName] = useState(channel?.name || "");
  const [enabled, setEnabled] = useState(channel?.enabled ?? true);
  const config = channel?.config ?? {};
  const [webhookUrl, setWebhookUrl] = useState((config as Record<string, string>).webhook_url || "");
  const [botToken, setBotToken] = useState((config as Record<string, string>).bot_token || "");
  const [chatId, setChatId] = useState((config as Record<string, string>).chat_id || "");
  const [recipients, setRecipients] = useState(
    (config as Record<string, string[]>).recipients?.join(", ") || ""
  );
  const [customUrl, setCustomUrl] = useState((config as Record<string, string>).url || "");
  const [headers, setHeaders] = useState(
    (config as Record<string, unknown>).headers
      ? JSON.stringify((config as Record<string, unknown>).headers, null, 2)
      : ""
  );

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
      onSuccess();
      onClose();
    },
  });

  const updateMut = useMutation({
    mutationFn: () =>
      updateNotificationChannel(channel!.id, { 
        name, 
        config: buildConfig(), 
        enabled 
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["notification-channels"] });
      onSuccess();
      onClose();
    },
  });

  function buildConfig() {
    switch (type) {
      case "slack":
      case "discord":
        return { webhook_url: webhookUrl };
      case "telegram":
        return { bot_token: botToken, chat_id: chatId };
      case "email":
        return { recipients: recipients.split(",").map((s: string) => s.trim()).filter(Boolean) };
      case "webhook":
        return { url: customUrl, headers: headers ? JSON.parse(headers) : {} };
      default:
        return {};
    }
  }

  function validateForm(): boolean {
    if (!name.trim()) return false;
    
    switch (type) {
      case "slack":
      case "discord":
        return !!webhookUrl.trim();
      case "telegram":
        return !!botToken.trim() && !!chatId.trim();
      case "email":
        return !!recipients.trim();
      case "webhook":
        return !!customUrl.trim();
      default:
        return true;
    }
  }

  function handleSubmit() {
    if (!validateForm()) return;
    
    if (isEdit) {
      updateMut.mutate();
    } else {
      createMut.mutate();
    }
  }

  const configFields = CHANNEL_CONFIG_FIELDS[type];
  const Icon = CHANNEL_ICONS[type] || Bell;

  return (
    <Modal
      onClose={onClose}
      title={
        <div className="flex items-center gap-2">
          <Icon size={18} />
          {isEdit ? `Edit ${name}` : "New Notification Channel"}
        </div>
      }
      wide
    >
      <div className="space-y-4">
        <div>
          <label className="mb-1.5 block text-sm font-medium text-slate-300">Channel Type</label>
          <select
            className="min-h-9 w-full rounded-lg border border-white/10 bg-surface-card-header px-3 text-sm text-slate-200"
            value={type}
            onChange={(e) => setType(e.target.value as NotificationChannelType)}
            disabled={isEdit}
          >
            <option value="slack">Slack</option>
            <option value="discord">Discord</option>
            <option value="telegram">Telegram</option>
            <option value="email">Email</option>
            <option value="webhook">Webhook</option>
          </select>
        </div>

        <Input 
          label="Channel Name" 
          value={name} 
          onChange={setName} 
          placeholder="My Notification Channel"
          required
        />

        <div className="flex items-center gap-2">
          <input 
            type="checkbox" 
            id="enabled" 
            checked={enabled} 
            onChange={(e) => setEnabled(e.target.checked)} 
            className="rounded"
          />
          <label htmlFor="enabled" className="text-sm text-slate-300">Enabled</label>
        </div>

        {configFields && (
          <div className="space-y-4">
            {configFields.fields.map((field) => {
              const label = configFields.labels[field] || field;
              
              if (field === "headers") {
                return (
                  <div key={field}>
                    <label className="block text-sm font-medium text-slate-300">
                      <span className="mb-1.5 block">{label}</span>
                      <textarea
                        className="min-h-0 w-full rounded-lg border border-white/10 bg-surface-card-header px-3 py-2 font-mono text-xs text-slate-200"
                        rows={4}
                        value={headers}
                        onChange={(e) => setHeaders(e.target.value)}
                        placeholder='{"Authorization": "Bearer ..."}'
                      />
                    </label>
                  </div>
                );
              }

              const valueMap: Record<string, string> = {
                webhookUrl,
                botToken,
                chatId,
                recipients,
                customUrl,
              };

              const setterMap: Record<string, (value: string) => void> = {
                webhookUrl: setWebhookUrl,
                botToken: setBotToken,
                chatId: setChatId,
                recipients: setRecipients,
                customUrl: setCustomUrl,
              };

              const value = valueMap[field] || "";
              const setter = setterMap[field];

              if (setter) {
                return (
                  <Input
                    key={field}
                    label={label}
                    value={value}
                    onChange={setter}
                    placeholder={getPlaceholder(field)}
                  />
                );
              }
              return null;
            })}
          </div>
        )}

        {(createMut.isError || updateMut.isError) && (
          <p className="text-sm text-red-400">
            {createMut.error?.message || updateMut.error?.message}
          </p>
        )}

        <div className="flex justify-end gap-2">
          <Btn size="sm" tone="ghost" onClick={onClose}>
            <X size={14} /> Cancel
          </Btn>
          <Btn 
            size="sm" 
            onClick={handleSubmit}
            disabled={!validateForm() || createMut.isPending || updateMut.isPending}
          >
            <Check size={14} /> {isEdit ? "Save Changes" : "Create Channel"}
          </Btn>
        </div>
      </div>
    </Modal>
  );
}

function getPlaceholder(field: string): string {
  const placeholders: Record<string, string> = {
    webhookUrl: "https://hooks.slack.com/services/...",
    botToken: "123456:ABC-DEF...",
    chatId: "-100123456789",
    recipients: "admin@example.com, team@example.com",
    customUrl: "https://example.com/hooks/...",
  };
  return placeholders[field] || "";
}