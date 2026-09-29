"use client";

import { AdminPageLayout, SectionHeader } from "@/components/admin/admin-ui";
import { NotificationsManager } from "@/components/admin/notifications-manager";
import { AdminNotifications } from "@/components/admin/AdminNotifications";

/**
 * One route, two backends.
 *
 * `/notifications/channels*` is the user-scoped engine; `/notification-channels*`
 * is the admin-only console with its own records plus the delivery log. The
 * server itself flags the ambiguity ("the user-scoped list wins over the dormant
 * admin-only registrar", `handlers_notifications_crud.go:20`), so the honest thing
 * this page can do is say which half is which — one `<h1>` for the route and two
 * labelled sections under it, instead of two page headers claiming the same title
 * for different data.
 */
export default function NotificationsPage() {
  return (
    <AdminPageLayout className="space-y-8">
      <SectionHeader
        sub="Two surfaces, two backends: the notification engine (your channels and subscriptions) and the admin console (global channels and the delivery log). A channel created in one does not appear in the other."
        info={{
          title: "Notifications",
          triggerLabel: "About notifications",
          description: "Where each notification list comes from, and what a test actually proves.",
          sections: [
            {
              title: "Two backends, one route",
              content: "The engine reads and writes /notifications/channels and is scoped to you (admins also see global and org channels). The console below reads /notification-channels, an admin-only set of global channels, and is the only place the delivery log is shown. They are separate records: creating a channel in one never lists it in the other.",
            },
            {
              title: "What a test proves",
              content: "A test is a real outbound send through that channel, executed synchronously by the control plane. Success means the provider accepted the message; it does not prove your event subscriptions are wired, and it says nothing about the other backend's channels.",
            },
            {
              title: "Stored credentials",
              content: "This API returns a channel's webhook URL and bot token to your session in full. The edit forms mask them for the screen with an explicit reveal control; the server does not treat them as write-only.",
            },
          ],
        }}
      />
      <NotificationsManager />
      <AdminNotifications />
    </AdminPageLayout>
  );
}
