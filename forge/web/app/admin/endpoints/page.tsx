"use client";

import { AdminEndpoints } from "@/components/admin/AdminEndpoints";

// The frame belongs to the component (AdminFirewall / AdminCrossnode split): a
// wrapper that also opens AdminPageLayout nests two frames and two spacing scales.
export default function AdminEndpointsPage() {
  return <AdminEndpoints />;
}
