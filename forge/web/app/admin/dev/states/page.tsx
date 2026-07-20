"use client";

import { useState } from "react";
import {
  ArrowLeft,
  Bug,
  ChevronDown,
  ChevronUp,
  Eye,
  EyeOff,
  Plus,
  Rocket,
  Server,
} from "lucide-react";
import Link from "next/link";
import { Button } from "@/components/ui/primitives";
import { PanelCard } from "@/components/ui/panel-card";
import {
  SkeletonList,
  SkeletonDetail,
  SkeletonForm,
  SpinnerInline,
  SpinnerPage,
  SpinnerButton,
  EmptyList,
  EmptySearch,
  EmptyDeployments,
  EmptyBackups,
  EmptyDomains,
  EmptyServices,
  EmptyGit,
  EmptyCertificates,
  EmptyDNSProviders,
  EmptyOrganizations,
  ErrorAlert,
  ErrorNotFound,
  ErrorPermission,
  ErrorNetwork,
  ErrorRateLimit,
  PermissionGate,
  RoleGate,
  ScopeGate,
  ServerStatus,
  BuildStatus,
  DeploymentStatus,
  CertStatus,
  DBStatus,
  VerificationStatus,
  CustomStatus,
  OfflineBanner,
} from "@/components/shared";

function ToggleSection({
  title,
  initiallyVisible = true,
  children,
}: {
  title: string;
  initiallyVisible?: boolean;
  children: React.ReactNode;
}) {
  const [visible, setVisible] = useState(initiallyVisible);
  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <button
          aria-label={visible ? `Hide ${title}` : `Show ${title}`}
          className="grid h-7 w-7 place-items-center rounded border border-white/10 text-slate-400 hover:bg-white/[0.06]"
          onClick={() => setVisible(!visible)}
          type="button"
        >
          {visible ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
        </button>
        <h3 className="text-base font-semibold text-slate-200">{title}</h3>
      </div>
      {visible ? <div className="space-y-4 pl-9">{children}</div> : null}
    </div>
  );
}

function DemoRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-4 rounded-lg border border-white/[0.06] bg-[#161b28] px-4 py-3">
      <span className="min-w-[140px] text-xs font-semibold uppercase tracking-wider text-slate-500">
        {label}
      </span>
      <div className="flex flex-wrap items-center gap-2">{children}</div>
    </div>
  );
}

export default function StatesDemoPage() {
  const [showOffline, setShowOffline] = useState(false);
  const [showRateLimit, setShowRateLimit] = useState(false);

  return (
    <div className="space-y-8">
      <div className="flex items-center gap-4">
        <Link
          className="grid h-9 w-9 place-items-center rounded-lg border border-white/10 text-slate-400 hover:bg-white/[0.06]"
          href="/admin"
        >
          <ArrowLeft size={16} />
        </Link>
        <div>
          <h1 className="text-xl font-bold text-slate-100">State Components Demo</h1>
          <p className="text-sm text-slate-400">
            QA review page for all loading, empty, error, and permission states
          </p>
        </div>
      </div>

      {/* ===== Loading States ===== */}
      <PanelCard title="Loading States" icon={Bug}>
        <div className="space-y-6 p-6">
          <ToggleSection title="Skeleton Variants">
            <DemoRow label="SkeletonList (5 rows)">
              <div className="w-full max-w-2xl">
                <SkeletonList rows={5} />
              </div>
            </DemoRow>
            <DemoRow label="SkeletonDetail">
              <div className="w-full max-w-2xl">
                <SkeletonDetail />
              </div>
            </DemoRow>
            <DemoRow label="SkeletonForm (4 fields)">
              <div className="w-full max-w-xl">
                <SkeletonForm fields={4} />
              </div>
            </DemoRow>
          </ToggleSection>

          <ToggleSection title="Spinner Variants">
            <DemoRow label="SpinnerInline">
              <SpinnerInline label="Loading data…" />
            </DemoRow>
            <DemoRow label="SpinnerButton">
              <button
                className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm text-white"
                disabled
                type="button"
              >
                <SpinnerButton label="Saving…" />
              </button>
            </DemoRow>
            <DemoRow label="SpinnerPage">
              <div className="h-48 w-full rounded-lg border border-white/[0.06] bg-[#0a0e16]">
                <SpinnerPage message="Loading app details…" />
              </div>
            </DemoRow>
          </ToggleSection>
        </div>
      </PanelCard>

      {/* ===== Empty States ===== */}
      <PanelCard title="Empty States" icon={Bug}>
        <div className="space-y-6 p-6">
          <ToggleSection title="Generic Empty States">
            <DemoRow label="EmptyList">
              <div className="w-full max-w-lg">
                <EmptyList
                  icon={Server}
                  title="No apps yet"
                  description="Deploy your first app to get started."
                  action={
                    <button
                      className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-500"
                      type="button"
                    >
                      <Plus size={14} /> Create App
                    </button>
                  }
                />
              </div>
            </DemoRow>
            <DemoRow label="EmptySearch">
              <div className="w-full max-w-lg">
                <EmptySearch query="xyz-not-found" />
              </div>
            </DemoRow>
          </ToggleSection>

          <ToggleSection title="Feature-Specific Empty States">
            <DemoRow label="EmptyDeployments">
              <div className="w-full max-w-lg">
                <EmptyDeployments
                  action={
                    <button
                      className="inline-flex items-center gap-2 rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-500"
                      type="button"
                    >
                      <Rocket size={14} /> Deploy
                    </button>
                  }
                />
              </div>
            </DemoRow>
            <DemoRow label="EmptyBackups">
              <div className="w-full max-w-lg">
                <EmptyBackups />
              </div>
            </DemoRow>
            <DemoRow label="EmptyDomains">
              <div className="w-full max-w-lg">
                <EmptyDomains />
              </div>
            </DemoRow>
            <DemoRow label="EmptyServices">
              <div className="w-full max-w-lg">
                <EmptyServices />
              </div>
            </DemoRow>
            <DemoRow label="EmptyGit">
              <div className="w-full max-w-lg">
                <EmptyGit />
              </div>
            </DemoRow>
            <DemoRow label="EmptyCertificates">
              <div className="w-full max-w-lg">
                <EmptyCertificates />
              </div>
            </DemoRow>
            <DemoRow label="EmptyDNSProviders">
              <div className="w-full max-w-lg">
                <EmptyDNSProviders />
              </div>
            </DemoRow>
            <DemoRow label="EmptyOrganizations">
              <div className="w-full max-w-lg">
                <EmptyOrganizations />
              </div>
            </DemoRow>
          </ToggleSection>
        </div>
      </PanelCard>

      {/* ===== Error States ===== */}
      <PanelCard title="Error States" icon={Bug}>
        <div className="space-y-6 p-6">
          <ToggleSection title="Error Variants">
            <DemoRow label="ErrorAlert (generic)">
              <div className="w-full max-w-2xl">
                <ErrorAlert
                  error={new Error("Failed to load data from the API. The server returned a 500 Internal Server Error.")}
                  title="Failed to load apps"
                  onRetry={() => {}}
                  showDetails
                />
              </div>
            </DemoRow>
            <DemoRow label="ErrorAlert (string error)">
              <div className="w-full max-w-2xl">
                <ErrorAlert
                  error="Network timeout after 30 seconds"
                  title="Request timed out"
                  onRetry={() => {}}
                />
              </div>
            </DemoRow>
            <DemoRow label="ErrorNotFound (404)">
              <div className="w-full max-w-xl">
                <ErrorNotFound resource="App" homeHref="/servers" />
              </div>
            </DemoRow>
            <DemoRow label="ErrorPermission (403)">
              <div className="w-full max-w-xl">
                <ErrorPermission permission="manage deployments" />
              </div>
            </DemoRow>
            <DemoRow label="ErrorNetwork">
              <div className="w-full max-w-xl">
                <ErrorNetwork onRetry={() => {}} />
              </div>
            </DemoRow>
          </ToggleSection>

          <ToggleSection title="Rate Limit (with countdown)">
            <DemoRow label="ErrorRateLimit">
              <button
                className="mb-2 rounded border border-amber-500/30 px-3 py-1.5 text-xs font-semibold text-amber-300 hover:bg-amber-500/10"
                onClick={() => setShowRateLimit(!showRateLimit)}
                type="button"
              >
                {showRateLimit ? "Reset" : "Show rate limit demo"}
              </button>
              {showRateLimit ? (
                <div className="w-full max-w-xl">
                  <ErrorRateLimit
                    retryAfterSeconds={10}
                    onRetry={() => setShowRateLimit(false)}
                  />
                </div>
              ) : null}
            </DemoRow>
          </ToggleSection>
        </div>
      </PanelCard>

      {/* ===== Status Badges ===== */}
      <PanelCard title="Status Badges" icon={Bug}>
        <div className="space-y-3 p-6">
          <DemoRow label="Server Status">
            <ServerStatus status="running" />
            <ServerStatus status="stopped" />
            <ServerStatus status="installing" />
            <ServerStatus status="suspended" />
            <ServerStatus status="offline" />
            <ServerStatus status="transferring" />
            <ServerStatus status="errored" />
            <ServerStatus status={null} />
          </DemoRow>
          <DemoRow label="Build Status">
            <BuildStatus status="running" />
            <BuildStatus status="succeeded" />
            <BuildStatus status="failed" />
            <BuildStatus status="canceled" />
            <BuildStatus status="abandoned" />
            <BuildStatus status="pending" />
          </DemoRow>
          <DemoRow label="Deployment Status">
            <DeploymentStatus status="pending" />
            <DeploymentStatus status="deploying" />
            <DeploymentStatus status="deployed" />
            <DeploymentStatus status="rolled-back" />
            <DeploymentStatus status="failed" />
          </DemoRow>
          <DemoRow label="Certificate Status">
            <CertStatus status="valid" />
            <CertStatus status="issuing" />
            <CertStatus status="expiring" expiresAt={new Date(Date.now() + 5 * 24 * 3600 * 1000).toISOString()} />
            <CertStatus status="expired" expiresAt={new Date(Date.now() - 3600 * 1000).toISOString()} />
            <CertStatus status="failed" />
          </DemoRow>
          <DemoRow label="Database Status">
            <DBStatus status="provisioning" />
            <DBStatus status="running" />
            <DBStatus status="backing-up" />
            <DBStatus status="stopped" />
            <DBStatus status="error" />
          </DemoRow>
          <DemoRow label="Verification Status">
            <VerificationStatus status="pending" />
            <VerificationStatus status="verified" />
            <VerificationStatus status="failed" />
          </DemoRow>
          <DemoRow label="Custom Status">
            <CustomStatus
              mapping={{
                healthy: { icon: statusIcons.success, tone: "success", label: "Healthy" },
                degraded: { icon: statusIcons.warning, tone: "warning", label: "Degraded" },
                down: { icon: statusIcons.danger, tone: "danger", label: "Down" },
              }}
              status="healthy"
            />
            <CustomStatus
              mapping={{
                healthy: { icon: statusIcons.success, tone: "success", label: "Healthy" },
                degraded: { icon: statusIcons.warning, tone: "warning", label: "Degraded" },
                down: { icon: statusIcons.danger, tone: "danger", label: "Down" },
              }}
              status="degraded"
            />
            <CustomStatus
              mapping={{
                healthy: { icon: statusIcons.success, tone: "success", label: "Healthy" },
                degraded: { icon: statusIcons.warning, tone: "warning", label: "Degraded" },
                down: { icon: statusIcons.danger, tone: "danger", label: "Down" },
              }}
              status="down"
            />
          </DemoRow>
        </div>
      </PanelCard>

      {/* ===== Permission Gates ===== */}
      <PanelCard title="Permission Gates" icon={Bug}>
        <div className="space-y-3 p-6">
          <DemoRow label="PermissionGate (authorized)">
            <PermissionGate
              access={{ isAdmin: false, isOwner: true, permissions: [] }}
              permission="schedule.create"
            >
              <span className="text-sm text-emerald-300">Access granted — you have permission</span>
            </PermissionGate>
          </DemoRow>
          <DemoRow label="PermissionGate (denied)">
            <PermissionGate
              access={{ isAdmin: false, isOwner: false, permissions: ["file.read"] }}
              permission="schedule.create"
            >
              <span className="text-sm text-red-400">Should not render</span>
            </PermissionGate>
          </DemoRow>
          <DemoRow label="RoleGate (admin)">
            <RoleGate currentRole="admin" roles={["admin"]}>
              <span className="text-sm text-emerald-300">Visible to admin users only</span>
            </RoleGate>
          </DemoRow>
          <DemoRow label="RoleGate (denied user)">
            <RoleGate currentRole="user" roles={["admin"]}>
              <span className="text-sm text-red-400">Should not render</span>
            </RoleGate>
          </DemoRow>
          <DemoRow label="ScopeGate (has scope)">
            <ScopeGate currentScopes={["app:read", "app:write"]} requiredScope="app:write">
              <span className="text-sm text-emerald-300">Scope app:write is granted</span>
            </ScopeGate>
          </DemoRow>
          <DemoRow label="ScopeGate (missing scope)">
            <ScopeGate currentScopes={["app:read"]} requiredScope="app:write">
              <span className="text-sm text-red-400">Should not render</span>
            </ScopeGate>
          </DemoRow>
        </div>
      </PanelCard>

      {/* ===== Offline Banner ===== */}
      <PanelCard title="Offline Banner" icon={Bug}>
        <div className="space-y-4 p-6">
          <p className="text-sm text-slate-400">
            The OfflineBanner auto-detects browser connectivity. Toggle the override below to
            preview the banner (toggles navigator.onLine temporarily).
          </p>
          <div className="flex gap-3">
            <Button
              onClick={() => setShowOffline(!showOffline)}
              variant="secondary"
            >
              {showOffline ? <EyeOff size={14} /> : <Eye size={14} />}
              {showOffline ? "Hide offline banner" : "Show offline banner"}
            </Button>
          </div>
          {showOffline ? (
            <div className="rounded-lg overflow-hidden">
              <OfflineBanner onRetry={() => {}} />
            </div>
          ) : null}
        </div>
      </PanelCard>
    </div>
  );
}

import { CheckCircle2, XCircle, AlertTriangle, PauseCircle } from "lucide-react";

const statusIcons = {
  success: CheckCircle2,
  warning: AlertTriangle,
  danger: XCircle,
  neutral: PauseCircle,
};
