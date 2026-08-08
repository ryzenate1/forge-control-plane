"use client";

import dynamic from "next/dynamic";

const BackupManager = dynamic(() => import("@/components/backup-manager").then(m => ({ default: m.BackupManager })), { ssr: false });
const HealthDashboard = dynamic(() => import("@/components/health-dashboard").then(m => ({ default: m.HealthDashboard })), { ssr: false });
const SystemInfoDisplay = dynamic(() => import("@/components/system-info").then(m => ({ default: m.SystemInfoDisplay })), { ssr: false });

export function BackupManagerWrapper() { return <BackupManager />; }
export function HealthDashboardWrapper() { return <HealthDashboard />; }
export function SystemInfoDisplayWrapper() { return <SystemInfoDisplay />; }
