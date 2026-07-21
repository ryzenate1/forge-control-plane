import { fetchJSON, postJSON, patchJSON, deleteJSON } from "./http";

export type NotificationChannelType = "slack" | "discord" | "telegram" | "email" | "webhook";

export type NotificationChannel = {
  id: string;
  type: NotificationChannelType;
  name: string;
  config: Record<string, unknown>;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
};

export type NotificationEventSubscription = {
  id: string;
  channelId: string;
  eventType: string;
  template: string;
  lastSentAt?: string;
  deliveryStatus: "pending" | "delivered" | "failed";
  createdAt: string;
  updatedAt: string;
};

export type NotificationLog = {
  id: string;
  channelId: string;
  eventType: string;
  status: "pending" | "delivered" | "failed";
  error?: string;
  sentAt: string;
};

export const AVAILABLE_EVENTS = [
  "server.crash",
  "server.install.complete",
  "backup.complete",
  "backup.failed",
  "deployment.complete",
  "deployment.failed",
  "node.down",
  "node.up",
] as const;

export type AvailableEvent = (typeof AVAILABLE_EVENTS)[number];

export const EVENT_LABELS: Record<AvailableEvent, string> = {
  "server.crash": "Server Crash",
  "server.install.complete": "Server Install Complete",
  "backup.complete": "Backup Complete",
  "backup.failed": "Backup Failed",
  "deployment.complete": "Deployment Complete",
  "deployment.failed": "Deployment Failed",
  "node.down": "Node Offline",
  "node.up": "Node Online",
};

type ChannelsResponse = { channels: NotificationChannel[] };
type SubscriptionsResponse = { subscriptions: NotificationEventSubscription[] };
type LogsResponse = { logs: NotificationLog[] };

export async function fetchNotificationChannels(): Promise<NotificationChannel[]> {
  const res = await fetchJSON<ChannelsResponse>("/notification-channels");
  return res.channels;
}

export async function createNotificationChannel(data: {
  type: NotificationChannelType;
  name: string;
  config: Record<string, unknown>;
  enabled: boolean;
}): Promise<NotificationChannel> {
  return postJSON<NotificationChannel>("/notification-channels", data);
}

export async function getNotificationChannel(id: string): Promise<NotificationChannel> {
  return fetchJSON<NotificationChannel>(`/notification-channels/${id}`);
}

export async function updateNotificationChannel(
  id: string,
  data: { name?: string; config?: Record<string, unknown>; enabled?: boolean },
): Promise<NotificationChannel> {
  return patchJSON<NotificationChannel>(`/notification-channels/${id}`, data);
}

export async function deleteNotificationChannel(id: string): Promise<void> {
  return deleteJSON(`/notification-channels/${id}`);
}

export async function testNotificationChannel(id: string): Promise<void> {
  await postJSON(`/notification-channels/${id}/test`);
}

export async function fetchSubscriptions(channelId: string): Promise<NotificationEventSubscription[]> {
  const res = await fetchJSON<SubscriptionsResponse>(`/notification-channels/${channelId}/subscribe`);
  return res.subscriptions;
}

export async function createSubscription(channelId: string, eventType: string, template?: string): Promise<NotificationEventSubscription> {
  return postJSON<NotificationEventSubscription>(`/notification-channels/${channelId}/subscribe`, {
    eventType,
    template: template ?? "",
  });
}

export async function deleteSubscription(channelId: string, subId: string): Promise<void> {
  return deleteJSON(`/notification-channels/${channelId}/subscribe/${subId}`);
}

export async function fetchNotificationLogs(channelId?: string, limit = 100, offset = 0): Promise<NotificationLog[]> {
  const params = new URLSearchParams();
  if (channelId) params.set("channelId", channelId);
  params.set("limit", String(limit));
  params.set("offset", String(offset));
  const res = await fetchJSON<LogsResponse>(`/notification-logs?${params.toString()}`);
  return res.logs;
}
