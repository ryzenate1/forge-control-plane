"use client";

import { AdminPageLayout, SectionHeader } from "@/components/admin/admin-ui";
import { DockerEventsFeed } from "@/components/admin/docker-events-feed";

export default function DockerEventsPage() {
  return (
    <AdminPageLayout>
      <SectionHeader
        title="Docker Events"
        sub="Container lifecycle events streamed from every Beacon node — start, stop, die, kill, OOM, recreate and destroy, newest first."
      />
      <DockerEventsFeed />
    </AdminPageLayout>
  );
}
