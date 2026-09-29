"use client";

/**
 * The state vocabulary reference.
 *
 * The product has three state systems that operators actually see, plus a
 * fourth that only the router sees. This page demos the first three side by side
 * and names the fourth, so "loading is not empty", "an error is not empty",
 * "undefined is not zero" and "stale is not healthy" are one visible contract
 * rather than four opinions. A page that cannot render one of these has to
 * explain why, not invent its own.
 */

import { useState } from "react";
import { notFound, useRouter } from "next/navigation";
import {
  AdminErrorState,
  AdminLoadingRows,
  AdminLoadingState,
  AdminPageLayout,
  AdminSection,
  Btn,
  EmptyState,
  PermissionDeniedState,
  Pill,
  SectionHeader,
} from "@/components/admin/admin-ui";
import {
  AggregateTile,
  DataState,
  FreshnessBadge,
  MetricSeriesChart,
  MetricTile,
  NotReported,
  PanelCard as TelemetryPanelCard,
  PartialFleetNotice,
  Reading,
  SourceRibbon,
  StatusIcon,
  StatusPill,
} from "@/components/admin/telemetry-ui";
import { reportedTotal, type SourceState } from "@/lib/admin/telemetry";
import { PanelCard } from "@/components/ui/panel-card";
import type { ToneInput } from "@/components/ui/forge/status";
import {
  BuildStatus,
  CertStatus,
  CustomStatus,
  DBStatus,
  DeploymentStatus,
  EmptyList,
  EmptySearch,
  ErrorAlert,
  ErrorNetwork,
  ErrorNotFound,
  ErrorPermission,
  ErrorRateLimit,
  OfflineBanner,
  PermissionGate,
  RoleGate,
  ScopeGate,
  ServerStatus,
  SkeletonDetail,
  SkeletonForm,
  SkeletonList,
  SpinnerInline,
  SpinnerPage,
  VerificationStatus,
} from "@/components/shared";
import { AlertTriangle, CheckCircle2, PauseCircle, XCircle, Server, Rocket } from "lucide-react";

/* ------------------------------------------------------------------ *
 * Demo fixtures
 *
 * Every value below is a fixture for a presentational component and is
 * labelled as such on screen. Nothing here is presented as a platform
 * reading — that is the rule the rest of the product is measured against.
 * ------------------------------------------------------------------ */

const NOW = Date.now();

const states: Record<string, SourceState> = {
  loading: { status: "loading", updatedAt: null, stale: false, refreshing: false },
  ready: { status: "ready", updatedAt: NOW - 4_000, stale: false, refreshing: false },
  refreshing: { status: "ready", updatedAt: NOW - 60_000, stale: false, refreshing: true },
  stale: { status: "ready", updatedAt: NOW - 9 * 60_000, stale: true, refreshing: false },
  error: { status: "error", updatedAt: NOW - 3 * 60_000, stale: false, refreshing: false, message: "Beacon did not answer within 5s" },
  restricted: { status: "restricted", updatedAt: null, stale: false, refreshing: false, message: "Permission restricted" },
};

type GateKey = keyof typeof states;

const statusTones = ["ok", "warn", "danger", "info", "pending", "neutral", "unknown"] as const;

function ToggleSection({ title, initiallyVisible = true, children }: { title: string; initiallyVisible?: boolean; children: React.ReactNode }) {
  const [visible, setVisible] = useState(initiallyVisible);
  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <Btn
          ariaLabel={visible ? `Hide ${title}` : `Show ${title}`}
          size="sm"
          tone="ghost"
          onClick={() => setVisible(!visible)}
        >
          {visible ? "Hide" : "Show"}
        </Btn>
        <h3 className="t-title text-text">{title}</h3>
      </div>
      {visible ? <div className="space-y-4">{children}</div> : null}
    </div>
  );
}

function DemoRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-start gap-4 rounded-lg border border-line bg-overlay-subtle px-4 py-3">
      <span className="ui-label w-40 shrink-0">{label}</span>
      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">{children}</div>
    </div>
  );
}

function FixtureNote({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-[11px] leading-5 text-text-muted">
      {children}
    </p>
  );
}

export default function StatesDemoPage() {
  const router = useRouter();
  const [gate, setGate] = useState<GateKey>("ready");
  const [showOffline, setShowOffline] = useState(false);
  const [showRateLimit, setShowRateLimit] = useState(false);

  // A component gallery is a development tool, not a feature of the product.
  // It is hidden from the sidebar (`hidden: true` in the admin registry) but
  // hiding a link is not access control — the route answered in production to
  // anyone who typed it, and rendered a page of fake servers, fake
  // deployments and fake error states that looks like real platform data.
  if (process.env.NODE_ENV === "production") {
    notFound();
  }

  const gateState = states[gate];
  const sample = [{ v: 4 }, { v: 6 }, { v: undefined }, { v: 2 }, { v: undefined }];
  const partialTotal = reportedTotal(sample, (record) => record.v);
  const fullTotal = reportedTotal([{ v: 12 }, { v: 30 }], (record) => record.v);

  return (
    <AdminPageLayout className="space-y-8">
      <SectionHeader
        sub="Reference for every state the product renders: the loading → error → empty → ready precedence, unknown readings, freshness and provenance, the metric tiles, the shared app-workspace states and the container primitives. Fixtures are labelled as fixtures."
        backAction={() => router.push("/admin")}
        backLabel="Admin"
        status={<Pill tone="neutral">Dev only</Pill>}
      />

      {/* ---------------------------------------------------------------- */}
      <AdminSection
        title="1 · The precedence gate"
        description="DataState is the only place loading, restricted, error and empty are decided. isEmpty is evaluated after the ready check, which is what makes 'a failed read rendered as an empty list' impossible to write by accident."
      >
        <FixtureNote>
          Fixture — the switch below picks which <code className="font-mono">SourceState</code> is fed to the component. Nothing here is a platform reading.
        </FixtureNote>
        <div className="flex flex-wrap gap-2">
          {(Object.keys(states) as GateKey[]).map((key) => (
            <Btn key={key} size="sm" tone={gate === key ? "primary" : "ghost"} onClick={() => setGate(key)}>{key}</Btn>
          ))}
        </div>
        <DataState
          emptyMessage="Nothing matched, and the read succeeded."
          emptyTitle="Nothing to show"
          isEmpty={gate === "ready"}
          loadingLabel="Reading the source…"
          onRetry={() => setGate("ready")}
          state={gateState}
        >
          <div className="rounded-lg border border-line bg-surface px-4 py-3 text-sm text-text-subtle">
            Ready content would render here. The gate above shows that <strong className="text-text">empty</strong> and <strong className="text-text">error</strong> can never both be true of the same source.
          </div>
        </DataState>
      </AdminSection>

      {/* ---------------------------------------------------------------- */}
      <AdminSection
        title="2 · Unknown is not zero"
        description="Reading renders the value or an em dash with a reason; NotReported requires that reason. A measured zero renders as 0 — the two must look different."
      >
        <DemoRow label="Reading(value)">
          <Reading value="42.7%" />
        </DemoRow>
        <DemoRow label="Reading(undefined)">
          <Reading reason="Beacon has not reported disk usage" value={undefined} />
        </DemoRow>
        <DemoRow label="Reading('')">
          <Reading reason="Empty string is treated as no value" value="" />
        </DemoRow>
        <DemoRow label="Reading(0) — a real zero">
          <Reading value={0} />
        </DemoRow>
        <DemoRow label="NotReported">
          <NotReported reason="The node did not answer" />
        </DemoRow>
      </AdminSection>

      {/* ---------------------------------------------------------------- */}
      <AdminSection
        title="3 · Freshness and provenance"
        description="FreshnessBadge accepts a SourceState and never a timestamp, so it can only say what the query actually knows. It cannot be coerced into claiming liveness — any 'Live' on a page is a page-level fabrication."
      >
        <DemoRow label="loading">
          <FreshnessBadge state={states.loading} />
        </DemoRow>
        <DemoRow label="ready">
          <FreshnessBadge state={states.ready} />
        </DemoRow>
        <DemoRow label="ready + refetching">
          <FreshnessBadge state={states.refreshing} />
        </DemoRow>
        <DemoRow label="stale">
          <FreshnessBadge state={states.stale} />
        </DemoRow>
        <DemoRow label="error">
          <FreshnessBadge state={states.error} />
        </DemoRow>
        <DemoRow label="restricted (403)">
          <FreshnessBadge state={states.restricted} />
        </DemoRow>
        <div className="mt-2">
          <SourceRibbon
            sources={[
              { label: "Nodes", state: states.ready },
              { label: "Beacon host stats", state: states.stale },
              { label: "Container inventory", state: states.error },
              { label: "Budget", state: states.restricted },
            ]}
          />
        </div>
        <FixtureNote>SourceRibbon names each source on a panel that mixes them, so one failed half cannot tint the other.</FixtureNote>
        <div className="mt-4">
          <MetricSeriesChart
            emptyMessage="Fixture: zero points. The axes render and no line is drawn — a flat line at zero would claim a healthy idle fleet."
            emptyTitle="No telemetry reported"
            points={[]}
            series={["cpuPercent", "memoryPercent"]}
          />
        </div>
        <PartialFleetNotice
          failedNodeIds={["node-7"]}
          nodeNames={new Map([["node-7", "fixture-helsinki"]])}
          skippedNodeIds={["node-8", "node-9"]}
        />
      </AdminSection>

      {/* ---------------------------------------------------------------- */}
      <AdminSection
        title="4 · Metrics"
        description="MetricTile requires a kind and renders it, because reading configured capacity as live usage is the most dangerous mistake in this area. It carries no trend or sparkline unless real series are supplied. AggregateTile renders 'N of M reported' so a floor is never read as a total."
      >
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <MetricTile
            context="Fixture reading reported by the host agent"
            kind="live"
            label="CPU"
            unit="%"
            value={38.4}
          />
          <MetricTile
            kind="configured"
            label="Memory capacity"
            missingReason="No node reported memory"
            value={undefined}
          />
          <MetricTile
            context="Fixture read that is older than its refresh interval"
            kind="live"
            label="Disk"
            state={states.stale}
            unit="%"
            value={71.2}
          />
          <MetricTile
            context="Derived from the tiles above"
            kind="derived"
            label="Share"
            unit="%"
            value={12}
          />
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <AggregateTile
            format={(value) => String(value)}
            kind="allocated"
            label="Sum of a partial field"
            noun="records"
            total={partialTotal}
            unit="GiB"
          />
          <AggregateTile
            format={(value) => String(value)}
            kind="allocated"
            label="Sum of a complete field"
            noun="records"
            total={fullTotal}
            unit="GiB"
          />
        </div>
        <FixtureNote>
          The first tile is a <strong className="text-text">floor</strong>:{" "}
          {partialTotal.reported} of {partialTotal.total} records carried the field, so the real total cannot be lower than it and may be higher.
        </FixtureNote>
      </AdminSection>

      {/* ---------------------------------------------------------------- */}
      <AdminSection
        title="5 · Status vocabulary"
        description="Seven tones, one meaning each. Status colours come from the tone, never from a class at the call site. `unknown` is grey and dashed: a reading we do not have is not a healthy reading."
      >
        <DemoRow label="StatusIcon + StatusPill">
          {statusTones.map((tone) => (
            <span className="inline-flex items-center gap-1.5" key={tone}>
              <StatusIcon tone={tone} />
              <StatusPill label={tone} tone={tone} />
            </span>
          ))}
        </DemoRow>
        <DemoRow label="Pill (admin frame)">
          {(["ok", "success", "warn", "danger", "info", "neutral", "unknown", "green", "red"] as ToneInput[]).map((tone) => (
            <Pill key={String(tone)} tone={tone}>{String(tone)}</Pill>
          ))}
        </DemoRow>
        <FixtureNote>
          Colour words (<code className="font-mono">green</code>) and meaning words (<code className="font-mono">success</code>) both resolve through the same
          <code className="font-mono"> resolveTone</code>; anything unrecognised becomes <code className="font-mono">unknown</code>, never healthy.
        </FixtureNote>
      </AdminSection>

      {/* ---------------------------------------------------------------- */}
      <AdminSection
        title="6 · Admin frame states"
        description="The loading and error surfaces every /admin page should use. A failed read must not fall through to the empty state."
      >
        <DemoRow label="AdminLoadingState">
          <div className="w-full max-w-xl"><AdminLoadingState label="Loading…" /></div>
        </DemoRow>
        <DemoRow label="AdminLoadingRows">
          <div className="w-full max-w-xl rounded-lg border border-line"><AdminLoadingRows cols={4} rows={3} /></div>
        </DemoRow>
        <DemoRow label="AdminErrorState">
          <div className="w-full max-w-xl"><AdminErrorState message="The control plane returned 502 while reading this list." retry={() => {}} /></div>
        </DemoRow>
        <DemoRow label="EmptyState">
          <div className="w-full max-w-xl"><EmptyState icon={Server} message="A read that succeeded and genuinely found nothing." title="Nothing here" /></div>
        </DemoRow>
        <DemoRow label="PermissionDeniedState">
          <div className="w-full max-w-xl"><PermissionDeniedState message="Your role cannot read this resource." /></div>
        </DemoRow>
      </AdminSection>

      {/* ---------------------------------------------------------------- */}
      <AdminSection
        description="Two components in the product are exported as PanelCard with different geometry and both render an h3. Call sites pick the wrong one by name alone. Until one is renamed or merged (central change), this row shows why the collision is a bug, not a style choice."
        title="7 · Containers — the PanelCard collision"
      >
        <div className="grid gap-4 lg:grid-cols-2">
          <PanelCard icon={Server} title="ui/PanelCard">
            <p className="px-4 pb-4 text-xs text-text-subtle">
              An adapter over <code className="font-mono">ForgeCard</code>. It exposes no{" "}
              <code className="font-mono">description</code>, <code className="font-mono">action</code> or{" "}
              <code className="font-mono">footer</code> slot — which is exactly why pages reach for the telemetry
              one instead, and how the collision stays alive. Used by <code className="font-mono">components/server/users-view.tsx</code>{" "}
              and this gallery.
            </p>
          </PanelCard>
          <TelemetryPanelCard
            description="components/admin/telemetry-ui — a Card with a header strip, a footer slot and different padding."
            footer="Footer slot — the reason pages reach for this one."
            icon={AlertTriangle}
            title="telemetry-ui/PanelCard"
          >
            <p className="px-4 pb-4 text-xs text-text-subtle">Used by Overview and Monitoring.</p>
          </TelemetryPanelCard>
        </div>
      </AdminSection>

      {/* ---------------------------------------------------------------- */}
      <AdminSection
        description="components/shared/states-* — the set the app-workspace views use, and the only one the old version of this gallery demonstrated. It overlaps section 6: three badge idioms (this set, StatusPill and Pill) say the same thing in three ways, which is a merge decision rather than a page fix."
        title="8 · App-workspace states"
      >
        <ToggleSection title="Loading">
          <DemoRow label="SkeletonList">
            <div className="w-full max-w-2xl"><SkeletonList rows={5} /></div>
          </DemoRow>
          <DemoRow label="SkeletonDetail">
            <div className="w-full max-w-2xl"><SkeletonDetail /></div>
          </DemoRow>
          <DemoRow label="SkeletonForm">
            <div className="w-full max-w-xl"><SkeletonForm fields={4} /></div>
          </DemoRow>
          <DemoRow label="SpinnerInline">
            <SpinnerInline label="Loading data…" />
          </DemoRow>
          <DemoRow label="SpinnerButton">
            <Btn disabled loading size="sm">Saving…</Btn>
          </DemoRow>
          <DemoRow label="SpinnerPage">
            <div className="h-40 w-full rounded-lg border border-line"><SpinnerPage message="Loading app details…" /></div>
          </DemoRow>
        </ToggleSection>

        <ToggleSection title="Empty">
          <DemoRow label="EmptyList">
            <div className="w-full max-w-lg">
              <EmptyList
                action={<Btn size="sm"><Rocket size={14} /> Create app</Btn>}
                description="Deploy your first app to get started."
                icon={Server}
                title="No apps yet"
              />
            </div>
          </DemoRow>
          <DemoRow label="EmptySearch">
            <div className="w-full max-w-lg"><EmptySearch query="xyz-not-found" /></div>
          </DemoRow>
          <FixtureNote>
            The eight domain-specific exports in <code className="font-mono">states-empty.tsx</code>
            {" "}(<code className="font-mono">EmptyDeployments</code>, <code className="font-mono">EmptyBackups</code>,{" "}
            <code className="font-mono">EmptyDomains</code>, <code className="font-mono">EmptyServices</code>,{" "}
            <code className="font-mono">EmptyGit</code>, <code className="font-mono">EmptyCertificates</code>,{" "}
            <code className="font-mono">EmptyDNSProviders</code>, <code className="font-mono">EmptyOrganizations</code>)
            are hand-duplicated copies of <code className="font-mono">EmptyList</code>, one per domain: every new domain
            adds an export instead of a parameter. <code className="font-mono">EmptyState title= message=</code> from
            section 6 is the parameterised form. They stay here rather than being deleted because the app-workspace
            views still import them (removal is a central change).
          </FixtureNote>
        </ToggleSection>

        <ToggleSection title="Error">
          <DemoRow label="ErrorAlert">
            <div className="w-full max-w-2xl">
              <ErrorAlert error={new Error("The server returned 500 while reading the app list.")} onRetry={() => {}} showDetails title="Failed to load apps" />
            </div>
          </DemoRow>
          <DemoRow label="ErrorNotFound">
            <div className="w-full max-w-xl"><ErrorNotFound homeHref="/servers" resource="App" /></div>
          </DemoRow>
          <DemoRow label="ErrorPermission">
            <div className="w-full max-w-xl"><ErrorPermission permission="manage deployments" /></div>
          </DemoRow>
          <DemoRow label="ErrorNetwork">
            <div className="w-full max-w-xl"><ErrorNetwork onRetry={() => {}} /></div>
          </DemoRow>
          <DemoRow label="ErrorRateLimit">
            <div className="w-full max-w-xl">
              <Btn size="sm" tone="ghost" onClick={() => setShowRateLimit(!showRateLimit)}>{showRateLimit ? "Reset" : "Show rate limit"}</Btn>
              {showRateLimit ? <div className="mt-2"><ErrorRateLimit onRetry={() => setShowRateLimit(false)} retryAfterSeconds={10} /></div> : null}
            </div>
          </DemoRow>
        </ToggleSection>

        <ToggleSection title="Status badges (the third idiom)">
          <DemoRow label="ServerStatus">
            <ServerStatus status="running" />
            <ServerStatus status="stopped" />
            <ServerStatus status="installing" />
            <ServerStatus status="suspended" />
            <ServerStatus status="offline" />
            <ServerStatus status="transferring" />
            <ServerStatus status="errored" />
            <ServerStatus status={null} />
          </DemoRow>
          <DemoRow label="BuildStatus">
            <BuildStatus status="running" />
            <BuildStatus status="succeeded" />
            <BuildStatus status="failed" />
            <BuildStatus status="canceled" />
            <BuildStatus status="pending" />
          </DemoRow>
          <DemoRow label="DeploymentStatus">
            <DeploymentStatus status="pending" />
            <DeploymentStatus status="deploying" />
            <DeploymentStatus status="deployed" />
            <DeploymentStatus status="rolled-back" />
            <DeploymentStatus status="failed" />
          </DemoRow>
          <DemoRow label="CertStatus">
            <CertStatus status="valid" />
            <CertStatus expiresAt={new Date(NOW + 5 * 24 * 3600 * 1000).toISOString()} status="expiring" />
            <CertStatus expiresAt={new Date(NOW - 3600 * 1000).toISOString()} status="expired" />
          </DemoRow>
          <DemoRow label="DBStatus / VerificationStatus">
            <DBStatus status="running" />
            <DBStatus status="error" />
            <VerificationStatus status="pending" />
            <VerificationStatus status="verified" />
          </DemoRow>
          <DemoRow label="CustomStatus">
            <CustomStatus
              mapping={{
                healthy: { icon: CheckCircle2, tone: "success", label: "Healthy" },
                degraded: { icon: AlertTriangle, tone: "warning", label: "Degraded" },
                down: { icon: XCircle, tone: "danger", label: "Down" },
                paused: { icon: PauseCircle, tone: "neutral", label: "Paused" },
              }}
              status="degraded"
            />
            <CustomStatus
              mapping={{ healthy: { icon: CheckCircle2, tone: "success", label: "Healthy" } }}
              status="missing-from-mapping"
            />
          </DemoRow>
        </ToggleSection>

        <ToggleSection title="Permission gates">
          <DemoRow label="PermissionGate (granted)">
            <PermissionGate access={{ isAdmin: false, isOwner: true, permissions: [] }} permission="schedule.create">
              <span className="text-sm text-text">The gated child rendered.</span>
            </PermissionGate>
          </DemoRow>
          <DemoRow label="PermissionGate (denied)">
            <PermissionGate access={{ isAdmin: false, isOwner: false, permissions: ["file.read"] }} permission="schedule.create">
              <span className="text-sm text-text">This child must NOT appear.</span>
            </PermissionGate>
          </DemoRow>
          <DemoRow label="RoleGate (admin)">
            <RoleGate currentRole="admin" roles={["admin"]}>
              <span className="text-sm text-text">Visible to admins.</span>
            </RoleGate>
          </DemoRow>
          <DemoRow label="RoleGate (denied)">
            <RoleGate currentRole="user" roles={["admin"]}>
              <span className="text-sm text-text">This child must NOT appear.</span>
            </RoleGate>
          </DemoRow>
          <DemoRow label="ScopeGate (missing scope)">
            <ScopeGate currentScopes={["app:read"]} requiredScope="app:write">
              <span className="text-sm text-text">This child must NOT appear.</span>
            </ScopeGate>
          </DemoRow>
        </ToggleSection>

        <ToggleSection title="Offline">
          <DemoRow label="OfflineBanner">
            <div className="w-full space-y-2">
              <Btn size="sm" tone="ghost" onClick={() => setShowOffline(!showOffline)}>{showOffline ? "Hide" : "Show"} banner</Btn>
              <FixtureNote>
                <code className="font-mono">OfflineBanner</code> reads real browser connectivity; the button only mounts it, so what appears here is the live online state.
              </FixtureNote>
              {showOffline ? <OfflineBanner onRetry={() => {}} /> : null}
            </div>
          </DemoRow>
        </ToggleSection>
      </AdminSection>

      {/* ---------------------------------------------------------------- */}
      <AdminSection
        description="Not demoed, and why: `NodeTelemetryTable` needs node rows, and inventing a node to show it off would put fake platform data on the page whose whole job is the opposite. `app/admin/loading.tsx` and `app/admin/error.tsx` are the fourth system — the route chrome every slow navigation shows — hand-rolled off-token and still pending a central rebuild onto AdminLoadingRows and AdminErrorState. `components/ui/forge/feedback.tsx` is a fifth primitive layer below all of these."
        title="9 · Known gaps in this reference"
      >
        <div className="rounded-lg border border-warn-line bg-warn-subtle px-4 py-3 text-sm leading-6 text-warn" role="status">
          A page measured against this gallery should be able to answer: which gate decides loading vs error vs empty, which component renders an unknown reading, and where freshness comes from. If it cannot, it is using a vocabulary of its own.
        </div>
      </AdminSection>
    </AdminPageLayout>
  );
}
