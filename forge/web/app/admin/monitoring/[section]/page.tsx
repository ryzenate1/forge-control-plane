import { redirect } from "next/navigation";
import { AdminHealth, type MonitorSection } from "@/components/admin/AdminHealth";

const sectionMap: Record<string, MonitorSection | "logs" | "alerts"> = {
  infrastructure: "infrastructure",
  platform: "platform",
  resources: "resources",
  workloads: "workloads",
  database: "database",
  cache: "cache",
  queue: "queue",
  api: "api",
  "api-runtime": "api",
  daemon: "daemon",
  nodes: "infrastructure",
  orchestration: "orchestration",
  logs: "logs",
  alerts: "alerts",
};

export default async function MonitoringSectionPage({
  params,
}: {
  params: Promise<{ section: string }>;
}) {
  const { section: requestedSection } = await params;
  const section = sectionMap[requestedSection];

  if (!section) {
    redirect("/admin/monitoring");
  }
  if (section === "logs" || section === "alerts") {
    redirect("/admin/activity");
  }

  return <AdminHealth initialSection={section} />;
}
