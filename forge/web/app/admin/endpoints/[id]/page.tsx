"use client";

import { AdminEndpointDetail } from "@/components/admin/AdminEndpointDetail";

// Frame belongs to the component; wrapping it in AdminPageLayout again nested two
// frames and two spacing scales (see AdminFirewall's bare wrapper).
export default function AdminEndpointDetailPage() {
  return <AdminEndpointDetail />;
}
