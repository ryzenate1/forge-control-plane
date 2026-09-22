"use client";

import { AdminPageLayout, SectionHeader } from "@/components/admin/admin-ui";
import { DockerEventsFeed } from "@/components/admin/docker-events-feed";
import { adminPageGuides } from "@/components/admin/admin-page-guides";

export default function DockerEventsPage() {
  return (
    <AdminPageLayout>
      {/*
        Title and icon come from the registry. The sub is overridden because the
        registry line says "streamed", and this feed polls
        GET /admin/docker/events every 5 seconds — there is no socket behind it.
        The freshness/connection indicator lives in the feed itself, next to the
        query whose state it reports.
      */}
      <SectionHeader
        info={adminPageGuides.dockerEvents}
        sub="Container lifecycle events polled from every Beacon node — start, stop, die, kill, OOM, recreate and destroy, newest first."
      />
      <DockerEventsFeed />
    </AdminPageLayout>
  );
}
