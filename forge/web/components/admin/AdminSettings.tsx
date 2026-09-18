"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Archive, Bell, Globe, Mail, Settings as SettingsIcon, Shield, Workflow, Wrench } from "lucide-react";
import {
  fetchAdvancedSettings,
  fetchMailSettings,
  fetchPanelSettings,
  saveAdvancedSettings,
  saveMailSettings,
  savePanelSettings,
  testMailSettings,
  type ApiPanelAdvancedSettings,
  type ApiPanelMailSettings,
  type ApiPanelSettings,
} from "@/lib/api";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminSelect,
  AdminTabs,
  Btn,
  Card,
  CardHeader,
  Input,
  SectionHeader,
  cn,
} from "./admin-ui";
import { FreshnessBadge } from "./telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { errorMessage } from "@/lib/utils";

type Tab = "general" | "security" | "mail" | "monitoring" | "orchestration" | "backups" | "advanced";

const TABS: Array<{ id: Tab; label: string; icon: typeof SettingsIcon }> = [
  { id: "general", label: "General", icon: SettingsIcon },
  { id: "security", label: "Security", icon: Shield },
  { id: "mail", label: "Mail", icon: Mail },
  { id: "monitoring", label: "Monitoring", icon: Bell },
  { id: "orchestration", label: "Orchestration", icon: Workflow },
  { id: "backups", label: "Backups", icon: Archive },
  { id: "advanced", label: "Advanced", icon: Wrench },
];

/**
 * The value of one form field while it is being edited.
 *
 * `null` means "the box is empty". It is deliberately not `""` collapsed into
 * `0`, and it is not defaulted: `PUT /admin/settings`, `PATCH …/mail` and
 * `PATCH …/advanced` each parse the whole body into one Go struct
 * (`internal/http/handlers_settings.go:44`, `handlers_settings_extras.go:41`),
 * so every field the panel omits arrives as that field's zero value. A blank
 * therefore has to be *rejected*, not silently converted — `0` minutes of
 * session, `0` retention days and `false` 2FA are real configuration, and this
 * page used to write them whenever an operator cleared a box (`Number("")`).
 */
type FieldValue = string | boolean | null;

function hydrate(document: Record<string, unknown> | undefined): Record<string, FieldValue> {
  const out: Record<string, FieldValue> = {};
  if (!document) return out;
  for (const [key, value] of Object.entries(document)) {
    if (value === null || value === undefined) out[key] = null;
    else if (typeof value === "boolean") out[key] = value;
    else out[key] = String(value);
  }
  return out;
}

function isDirty(baseline: Record<string, FieldValue>, values: Record<string, FieldValue>): boolean {
  const keys = new Set([...Object.keys(baseline), ...Object.keys(values)]);
  for (const key of keys) if (baseline[key] !== values[key]) return true;
  return false;
}

function fieldLabel(key: string): string {
  return key.replace(/([A-Z])/g, " $1").replace(/^./, (c) => c.toUpperCase());
}

type Validation = { ok: true; payload: Record<string, unknown> } | { ok: false; errors: string[] };

/**
 * Merge the draft over the document that was actually read.
 *
 * Every field the read returned is sent back, including the ones this form does
 * not render, because the handler parses one whole struct and zero-fills what it
 * does not receive. A field the read left absent stays absent; a field the
 * operator *cleared* is an error rather than a silent `0`/`""`.
 */
function buildPayload(baseline: Record<string, FieldValue>, values: Record<string, FieldValue>, numericKeys: string[]): Validation {
  const errors: string[] = [];
  const payload: Record<string, unknown> = {};
  const all = new Set([...Object.keys(baseline), ...Object.keys(values)]);
  const numeric = new Set(numericKeys);
  for (const key of all) {
    const value = values[key] === undefined ? baseline[key] : values[key];
    if (value === null || value === undefined) {
      if (baseline[key] === null || baseline[key] === undefined) continue;
      errors.push(`${fieldLabel(key)} was cleared. Restore its value or enter a replacement — a blank field is not a zero.`);
      continue;
    }
    if (numeric.has(key)) {
      const trimmed = String(value).trim();
      const parsed = Number(trimmed);
      if (trimmed === "" || !Number.isFinite(parsed) || parsed < 0) {
        errors.push(`${fieldLabel(key)} must be a number of 0 or more.`);
        continue;
      }
      payload[key] = parsed;
      continue;
    }
    payload[key] = value;
  }
  return errors.length > 0 ? { ok: false, errors } : { ok: true, payload };
}

function Notice({ tone, children }: { tone: "ok" | "danger" | "warn"; children: React.ReactNode }) {
  return (
    <div
      className={cn(
        "ui-alert mt-3",
        tone === "danger" && "ui-alert-danger",
        tone === "ok" && "ui-alert-success",
        tone === "warn" && "ui-alert-warn",
      )}
      role={tone === "danger" ? "alert" : "status"}
    >
      {children}
    </div>
  );
}

/**
 * Every settings tab ends with the same three facts: when the document was read,
 * whether the draft differs from it, and the save control. A save result and a
 * save failure no longer look identical, and "Loading" here is never a claim
 * that the panel holds defaults.
 */
function SaveFooter({
  state, dirty, pending, label, pendingLabel, disabled, extra,
}: {
  state: ReturnType<typeof sourceState>;
  dirty: boolean;
  pending: boolean;
  label: string;
  pendingLabel: string;
  disabled?: boolean;
  extra?: React.ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center justify-end gap-3">
      <FreshnessBadge state={state} />
      <span className="text-[11px] text-text-muted">{dirty ? "Unsaved changes" : "No changes"}</span>
      {extra}
      <Btn disabled={disabled || pending || !dirty} tone="primary" type="submit">
        {pending ? pendingLabel : label}
      </Btn>
    </div>
  );
}

export function AdminSettings() {
  const [tab, setTab] = useState<Tab>("general");
  const [dirtyTab, setDirtyTab] = useState<Tab | null>(null);
  const [confirm, renderConfirm] = useConfirm();

  // Stable so the children's dirty effect does not re-fire on every parent render.
  const reportDirty = useCallback((target: Tab) => (dirty: boolean) => {
    setDirtyTab((previous) => {
      if (dirty) return target;
      return previous === target ? null : previous;
    });
  }, []);

  const changeTab = async (next: Tab) => {
    if (next === tab) return;
    if (dirtyTab === tab) {
      const discard = await confirm({
        title: "Discard unsaved changes?",
        description: `${tab.charAt(0).toUpperCase() + tab.slice(1)} has edits that have not been saved. Switching tabs discards them.`,
        confirmLabel: "Discard",
        danger: true,
      });
      if (!discard) return;
      setDirtyTab(null);
    }
    setTab(next);
  };

  return (
    <div className="space-y-6">
      <SectionHeader
        sub="Panel-wide configuration. Each tab saves only its own document; nothing here writes until the current values have been read successfully."
        info={{
          title: "Platform Settings",
          triggerLabel: "About platform settings",
          description: "How this page reads and writes panel configuration.",
          sections: [
            {
              title: "Save is a full-document write",
              content: "The control plane parses each settings body into one complete record, so a field the form does not send would be stored as its zero value. This page only enables Save after a successful read, and it refuses a blank numeric box instead of turning it into 0.",
            },
            {
              title: "Scope",
              content: "This is global panel configuration. Per-domain TLS and headers live under Security and mTLS; identities, roles and API keys live under Users, Roles and API Keys; outbound SMTP has its own Mail page, which writes a different record than the Mail tab here.",
            },
            {
              title: "Masked secrets",
              content: "Values the API masks (SMTP password, webhook tokens, reCAPTCHA secret) come back as ********. Leaving them untouched keeps the stored value; typing over them replaces it.",
            },
          ],
        }}
      />
      <AdminTabs
        tabs={TABS.map((t) => ({ id: t.id, label: t.label, icon: t.icon }))}
        active={tab}
        onChange={(id) => void changeTab(id as Tab)}
        label="Settings sections"
      />
      {tab === "general" && <PanelSettingsTab mode="general" onDirtyChange={reportDirty("general")} />}
      {tab === "security" && <PanelSettingsTab mode="security" onDirtyChange={reportDirty("security")} />}
      {tab === "mail" && <MailTab onDirtyChange={reportDirty("mail")} />}
      {tab === "monitoring" && <PanelSettingsTab mode="monitoring" onDirtyChange={reportDirty("monitoring")} />}
      {tab === "orchestration" && <PanelSettingsTab mode="orchestration" onDirtyChange={reportDirty("orchestration")} />}
      {tab === "backups" && <PanelSettingsTab mode="backups" onDirtyChange={reportDirty("backups")} />}
      {tab === "advanced" && <AdvancedTab onDirtyChange={reportDirty("advanced")} />}
      {renderConfirm()}
    </div>
  );
}

const NUMBER_FIELDS = [
  "passwordExpirationDays", "sessionDurationMinutes", "loginAttemptThreshold", "accountLockoutMinutes",
  "apiTokenTtlDays", "apiRotationDays", "metricsRetentionDays", "logsRetentionDays", "auditRetentionDays",
  "metricsSamplingRate", "monitoringPollIntervalSeconds", "failoverThresholdSeconds", "heartbeatThresholdSeconds",
  "reservationDurationMinutes", "reservationCleanupMinutes", "capacityBufferPercent", "backupRetentionDays",
  "backupLimit", "backupKeyRotationDays",
];

function Field({
  field, values, onChange, label, hint, type = "text", mono,
}: {
  field: string;
  values: Record<string, FieldValue>;
  onChange: (field: string, value: FieldValue) => void;
  label: string;
  hint?: string;
  type?: string;
  mono?: boolean;
}) {
  const value = values[field];
  return (
    <div>
      <Input
        label={label}
        mono={mono}
        onChange={(next) => onChange(field, next === "" ? null : next)}
        type={type}
        value={typeof value === "string" ? value : ""}
      />
      {value === null ? <p className="mt-1 text-[11px] text-warn">No value — a blank field blocks Save.</p> : null}
      {hint ? <p className="mt-1 text-[11px] leading-5 text-text-muted">{hint}</p> : null}
    </div>
  );
}

function Switch({ field, values, onChange, label, hint }: {
  field: string;
  values: Record<string, FieldValue>;
  onChange: (field: string, value: FieldValue) => void;
  label: string;
  hint?: string;
}) {
  return (
    <label className="flex items-start gap-2 text-sm text-text">
      <input
        checked={values[field] === true}
        className="mt-1 accent-[var(--brand)]"
        onChange={(event) => onChange(field, event.target.checked)}
        type="checkbox"
      />
      <span>
        {label}
        {values[field] === null ? <span className="mt-0.5 block text-[11px] text-warn">No value — this blocks Save.</span> : null}
        {hint ? <span className="mt-0.5 block text-[11px] leading-5 text-text-muted">{hint}</span> : null}
      </span>
    </label>
  );
}

function Choice({ field, values, onChange, label, options, hint }: {
  field: string;
  values: Record<string, FieldValue>;
  onChange: (field: string, value: FieldValue) => void;
  label: string;
  options: Array<{ value: string; label: string }>;
  hint?: string;
}) {
  const current = values[field];
  const text = typeof current === "string" ? current : "";
  const known = text === "" || options.some((option) => option.value === text);
  return (
    <div>
      <AdminSelect
        label={label}
        onChange={(next) => onChange(field, next === "" ? null : next)}
        options={known ? options : [{ label: `${text} — stored value, not a listed option`, value: text }, ...options]}
        value={text}
      />
      {current === null ? <p className="mt-1 text-[11px] text-warn">No value — the field is empty, so Save will ask for one.</p> : null}
      {hint ? <p className="mt-1 text-[11px] leading-5 text-text-muted">{hint}</p> : null}
    </div>
  );
}

function PanelSettingsTab({ mode, onDirtyChange }: { mode: Exclude<Tab, "mail" | "advanced">; onDirtyChange: (dirty: boolean) => void }) {
  const qc = useQueryClient();
  const query = useQuery({ queryKey: ["panel-settings"], queryFn: fetchPanelSettings });
  const [baseline, setBaseline] = useState<Record<string, FieldValue>>({});
  const [values, setValues] = useState<Record<string, FieldValue>>({});
  const [notice, setNotice] = useState<{ tone: "ok" | "danger"; text: string } | null>(null);

  useEffect(() => {
    if (!query.data) return;
    const next = hydrate(query.data as unknown as Record<string, unknown>);
    setBaseline(next);
    setValues(next);
  }, [query.data]);

  const dirty = useMemo(() => isDirty(baseline, values), [baseline, values]);
  useEffect(() => { onDirtyChange(dirty); }, [dirty, onDirtyChange]);

  const validation = useMemo(
    () => (dirty ? buildPayload(baseline, values, NUMBER_FIELDS) : null),
    [baseline, values, dirty],
  );

  const set = (field: string, value: FieldValue) => setValues((previous) => ({ ...previous, [field]: value }));

  const saveMut = useMutation({
    mutationFn: () => {
      if (!validation || !validation.ok) throw new Error(validation && !validation.ok ? validation.errors.join(" ") : "Nothing to save.");
      return savePanelSettings(validation.payload as unknown as ApiPanelSettings);
    },
    onSuccess: async (saved) => {
      const next = hydrate(saved as unknown as Record<string, unknown>);
      setBaseline(next);
      setValues(next);
      setNotice({ tone: "ok", text: "Settings saved and re-read from the control plane." });
      qc.setQueryData(["panel-settings"], saved);
      await qc.invalidateQueries({ queryKey: ["public-panel-settings"] });
      await qc.invalidateQueries({ queryKey: ["panel-settings"] });
    },
    onError: (error) => setNotice({ tone: "danger", text: errorMessage(error, "Settings could not be saved.") }),
  });

  const state = sourceState(query);

  if (query.isPending) return <div className="space-y-3"><AdminLoadingState label="Reading panel settings…" /></div>;

  if (query.isError) {
    return (
      <div className="space-y-3">
        <AdminErrorState
          message={`Panel settings could not be read (${errorMessage(query.error, "the control plane did not respond")}). Saving is disabled: with nothing loaded, a save would write defaults over your live configuration.`}
          retry={() => void query.refetch()}
        />
      </div>
    );
  }

  const errors = validation && !validation.ok ? validation.errors : [];

  return (
    <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); saveMut.mutate(); }}>
      {mode === "general" ? (
        <>
          <Card>
            <CardHeader title="Branding" icon={Globe} />
            <div className="grid gap-3 p-4 md:grid-cols-2">
              <Field field="companyName" values={values} onChange={set} label="Company name" />
              <Field field="shortName" values={values} onChange={set} label="Short name" />
              <Field field="productName" values={values} onChange={set} label="Product name" />
              <Field field="browserTitle" values={values} onChange={set} label="Browser title" />
              <Field field="footerText" values={values} onChange={set} label="Footer text" />
              <Field field="themePreset" values={values} onChange={set} label="Theme preset" mono hint="Stored as text; the theme system reads the preset name." />
              <Field field="logoUrl" values={values} onChange={set} label="Logo URL" mono />
              <Field field="faviconUrl" values={values} onChange={set} label="Favicon URL" mono />
              <Field field="loginBackgroundUrl" values={values} onChange={set} label="Login background URL" mono />
            </div>
          </Card>
          <Card>
            <CardHeader title="Localization & experience" icon={SettingsIcon} />
            <div className="grid gap-3 p-4 md:grid-cols-2">
              <Field field="defaultLocale" values={values} onChange={set} label="Default language" mono hint="Locale code, e.g. en, fr, ja." />
              <Field field="defaultTimezone" values={values} onChange={set} label="Default timezone" mono />
              <Field field="dateFormat" values={values} onChange={set} label="Date format" mono />
              <Field field="numberFormat" values={values} onChange={set} label="Number format" mono />
              <Field field="currencyFormat" values={values} onChange={set} label="Currency format" mono hint="Billing prices are shown in their own fixed format and do not read this value." />
              <Choice field="defaultDashboard" values={values} onChange={set} label="Default dashboard" options={[{ value: "overview", label: "Overview" }, { value: "monitoring", label: "Monitoring" }, { value: "servers", label: "Servers" }]} />
              <Choice field="landingPage" values={values} onChange={set} label="Landing page" options={[{ value: "servers", label: "Servers" }, { value: "admin/overview", label: "Admin overview" }, { value: "admin/monitoring", label: "Admin monitoring" }]} />
              <Choice field="sidebarLayout" values={values} onChange={set} label="Sidebar layout" options={[{ value: "expanded", label: "Expanded" }, { value: "compact", label: "Compact" }]} />
              <Switch field="compactMode" values={values} onChange={set} label="Compact mode" />
              <Switch field="advancedMode" values={values} onChange={set} label="Advanced mode" />
            </div>
          </Card>
        </>
      ) : null}

      {mode === "security" ? (
        <Card>
          <CardHeader title="Security" icon={Shield} />
          <div className="grid gap-3 p-4 md:grid-cols-2">
            <Choice field="require2FA" values={values} onChange={set} label="Require 2FA" options={[{ value: "none", label: "Not required" }, { value: "admin", label: "Administrators only" }, { value: "all", label: "All users" }]} />
            <Switch field="requireEmailVerification" values={values} onChange={set} label="Require email verification" />
            <Choice field="passwordComplexity" values={values} onChange={set} label="Password complexity" options={[{ value: "standard", label: "Standard" }, { value: "strong", label: "Strong" }, { value: "strict", label: "Strict" }]} />
            <Field field="passwordExpirationDays" type="number" values={values} onChange={set} label="Password expiration (days)" hint="0 means passwords never expire." />
            <Field field="sessionDurationMinutes" type="number" values={values} onChange={set} label="Session duration (minutes)" />
            <Switch field="loginRateLimitEnabled" values={values} onChange={set} label="Login rate limiting" />
            <Field field="loginAttemptThreshold" type="number" values={values} onChange={set} label="Login attempt threshold" />
            <Field field="accountLockoutMinutes" type="number" values={values} onChange={set} label="Account lockout (minutes)" />
            <Field field="geoRestrictions" values={values} onChange={set} label="Geo restrictions" mono />
            <Field field="apiTokenTtlDays" type="number" values={values} onChange={set} label="API token TTL (days)" hint="0 means tokens do not expire." />
            <Field field="apiRotationDays" type="number" values={values} onChange={set} label="API rotation (days)" hint="0 disables scheduled rotation." />
            <Field field="allowedOrigins" values={values} onChange={set} label="Allowed origins" mono />
            <Field field="trustedNetworks" values={values} onChange={set} label="Trusted networks" mono />
          </div>
        </Card>
      ) : null}

      {mode === "monitoring" ? (
        <Card>
          <CardHeader title="Monitoring" icon={Bell} />
          <div className="grid gap-3 p-4 md:grid-cols-2">
            <Field field="metricsRetentionDays" type="number" values={values} onChange={set} label="Metrics retention (days)" />
            <Field field="logsRetentionDays" type="number" values={values} onChange={set} label="Logs retention (days)" />
            <Field field="auditRetentionDays" type="number" values={values} onChange={set} label="Audit retention (days)" />
            <Field field="metricsSamplingRate" type="number" values={values} onChange={set} label="Sampling rate (%)" />
            <Field field="monitoringPollIntervalSeconds" type="number" values={values} onChange={set} label="Polling interval (seconds)" />
            <Switch field="emailAlertsEnabled" values={values} onChange={set} label="Email alerts" />
            <Switch field="webhookAlertsEnabled" values={values} onChange={set} label="Webhook alerts" />
            <Field field="discordWebhookUrl" values={values} onChange={set} label="Discord webhook URL" mono hint="Masked by the API when set; leave unchanged to keep it." />
            <Field field="slackWebhookUrl" values={values} onChange={set} label="Slack webhook URL" mono hint="Masked by the API when set; leave unchanged to keep it." />
            <Field field="telegramBotToken" values={values} onChange={set} label="Telegram bot token" mono hint="Masked by the API when set; leave unchanged to keep it." />
          </div>
        </Card>
      ) : null}

      {mode === "orchestration" ? (
        <Card>
          <CardHeader title="Orchestration" icon={Workflow} />
          <div className="grid gap-3 p-4 md:grid-cols-2">
            <Choice field="placementStrategy" values={values} onChange={set} label="Placement strategy" options={[{ value: "balanced", label: "Balanced" }, { value: "least-loaded", label: "Least loaded" }, { value: "spread", label: "Spread" }, { value: "binpack", label: "Bin pack" }]} />
            <Field field="antiAffinityRules" values={values} onChange={set} label="Anti-affinity rules" mono />
            <Switch field="resourceReservationsEnabled" values={values} onChange={set} label="Resource reservations" />
            <Choice field="nodePrioritization" values={values} onChange={set} label="Node prioritization" options={[{ value: "capacity", label: "Capacity" }, { value: "latency", label: "Latency" }, { value: "region", label: "Region" }, { value: "manual", label: "Manual" }]} />
            <Choice field="recoveryStrategy" values={values} onChange={set} label="Recovery strategy" options={[{ value: "manual", label: "Manual" }, { value: "assisted", label: "Assisted" }, { value: "automatic", label: "Automatic" }]} />
            <Field field="failoverThresholdSeconds" type="number" values={values} onChange={set} label="Failover threshold (seconds)" />
            <Field field="heartbeatThresholdSeconds" type="number" values={values} onChange={set} label="Heartbeat threshold (seconds)" />
            <Field field="reservationDurationMinutes" type="number" values={values} onChange={set} label="Reservation duration (minutes)" />
            <Field field="reservationCleanupMinutes" type="number" values={values} onChange={set} label="Reservation cleanup (minutes)" />
            <Field field="capacityBufferPercent" type="number" values={values} onChange={set} label="Capacity buffer (%)" hint="Headroom the scheduler keeps free on every node. 0 means no buffer." />
          </div>
        </Card>
      ) : null}

      {mode === "backups" ? (
        <Card>
          <CardHeader title="Backups" icon={Archive} />
          <div className="grid gap-3 p-4 md:grid-cols-2">
            <Choice field="backupProvider" values={values} onChange={set} label="Storage provider" options={[
              { value: "local", label: "Local disk" },
              { value: "s3", label: "S3" },
              { value: "cloudflare-r2", label: "Cloudflare R2" },
              { value: "backblaze-b2", label: "Backblaze B2" },
              { value: "azure-blob", label: "Azure Blob" },
              { value: "google-cloud-storage", label: "Google Cloud Storage" },
            ]} />
            <Field field="backupRetentionDays" type="number" values={values} onChange={set} label="Retention (days)" />
            <Field field="backupLimit" type="number" values={values} onChange={set} label="Backup limit" hint="0 means no limit." />
            <Switch field="backupAutoCleanup" values={values} onChange={set} label="Automatic cleanup" />
            <Switch field="backupEncryptionEnabled" values={values} onChange={set} label="Backup encryption" />
            <Field field="backupKeyRotationDays" type="number" values={values} onChange={set} label="Key rotation (days)" />
          </div>
        </Card>
      ) : null}

      {errors.length > 0 ? <Notice tone="danger">{errors.join(" ")}</Notice> : null}
      {notice ? <Notice tone={notice.tone}>{notice.text}</Notice> : null}
      <SaveFooter
        disabled={Boolean(errors.length)}
        dirty={dirty}
        label="Save"
        pending={saveMut.isPending}
        pendingLabel="Saving…"
        state={state}
      />
    </form>
  );
}

function MailTab({ onDirtyChange }: { onDirtyChange: (dirty: boolean) => void }) {
  const qc = useQueryClient();
  // This tab writes `/admin/settings/mail`; `/admin/mail` writes
  // `/admin/mail/settings`. Different records, different handlers — so the two
  // surfaces must not share one cache key, which is what made one page's data
  // fill the other's form. See `components/admin/mail-manager.tsx`.
  const query = useQuery({ queryKey: ["panel-settings-mail"], queryFn: fetchMailSettings });
  const [baseline, setBaseline] = useState<Record<string, FieldValue>>({});
  const [values, setValues] = useState<Record<string, FieldValue>>({});
  const [notice, setNotice] = useState<{ tone: "ok" | "danger"; text: string } | null>(null);
  const [testRecipient, setTestRecipient] = useState("");

  useEffect(() => {
    if (!query.data) return;
    const next = hydrate(query.data as unknown as Record<string, unknown>);
    setBaseline(next);
    setValues(next);
  }, [query.data]);

  const dirty = useMemo(() => isDirty(baseline, values), [baseline, values]);
  useEffect(() => { onDirtyChange(dirty); }, [dirty, onDirtyChange]);
  const validation = useMemo(
    () => (dirty ? buildPayload(baseline, values, ["smtpPort"]) : null),
    [baseline, values, dirty],
  );
  const set = (field: string, value: FieldValue) => setValues((previous) => ({ ...previous, [field]: value }));

  const saveMut = useMutation({
    mutationFn: () => {
      if (!validation || !validation.ok) throw new Error(validation && !validation.ok ? validation.errors.join(" ") : "Nothing to save.");
      return saveMailSettings(validation.payload as unknown as ApiPanelMailSettings);
    },
    onSuccess: async () => {
      setNotice({ tone: "ok", text: "Mail settings saved." });
      await qc.invalidateQueries({ queryKey: ["panel-settings-mail"] });
    },
    onError: (error) => setNotice({ tone: "danger", text: errorMessage(error, "Mail settings could not be saved.") }),
  });

  const testMut = useMutation({
    mutationFn: () => testMailSettings(testRecipient.trim()),
    onError: (error) => setNotice({ tone: "danger", text: errorMessage(error, "The test email could not be sent.") }),
  });

  const state = sourceState(query);

  if (query.isPending) return <AdminLoadingState label="Reading mail settings…" />;
  if (query.isError) {
    return (
      <AdminErrorState
        message={`Mail settings could not be read (${errorMessage(query.error, "the control plane did not respond")}). Saving is disabled rather than writing defaults over the stored SMTP configuration.`}
        retry={() => void query.refetch()}
      />
    );
  }

  const errors = validation && !validation.ok ? validation.errors : [];
  const result = testMut.data;

  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); saveMut.mutate(); }}>
      <Card>
        <CardHeader title="SMTP" icon={Mail} />
        <div className="grid gap-3 p-4 md:grid-cols-2">
          <Field field="smtpHost" values={values} onChange={set} label="SMTP host" />
          <Field field="smtpPort" type="number" values={values} onChange={set} label="SMTP port" />
          <Choice field="smtpEncryption" values={values} onChange={set} label="Encryption" options={[
            { value: "tls", label: "STARTTLS" },
            { value: "ssl", label: "Implicit TLS" },
            { value: "none", label: "None (local relay only)" },
          ]} hint="Sending the stored empty value unchanged keeps the server default." />
          <Field field="smtpUsername" values={values} onChange={set} label="Username" />
          <Field field="smtpPassword" type="password" values={values} onChange={set} label="Password" hint="Masked by the API when set; leave unchanged to keep it." />
          <Field field="mailFromAddress" values={values} onChange={set} label="From address" type="email" />
          <Field field="mailFromName" values={values} onChange={set} label="From name" />
          <div>
            <Input label="Test recipient" onChange={setTestRecipient} placeholder="operator@example.com" type="email" value={testRecipient} />
            <p className="mt-1 text-[11px] leading-5 text-text-muted">Sends immediately over the stored configuration; this endpoint does not queue.</p>
          </div>
        </div>
      </Card>
      {result ? (
        <Notice tone={result.sent ? "ok" : "danger"}>
          {result.message ?? (result.sent ? "Test email delivered." : "Test email failed.")}
        </Notice>
      ) : null}
      {errors.length > 0 ? <Notice tone="danger">{errors.join(" ")}</Notice> : null}
      {notice ? <Notice tone={notice.tone}>{notice.text}</Notice> : null}
      <SaveFooter
        disabled={Boolean(errors.length)}
        dirty={dirty}
        extra={
          <Btn disabled={testMut.isPending || !testRecipient.trim()} onClick={() => testMut.mutate()} tone="ghost">
            {testMut.isPending ? "Sending…" : "Send test"}
          </Btn>
        }
        label="Save"
        pending={saveMut.isPending}
        pendingLabel="Saving…"
        state={state}
      />
    </form>
  );
}

function AdvancedTab({ onDirtyChange }: { onDirtyChange: (dirty: boolean) => void }) {
  const qc = useQueryClient();
  const query = useQuery({ queryKey: ["panel-advanced-settings"], queryFn: fetchAdvancedSettings });
  const [baseline, setBaseline] = useState<Record<string, FieldValue>>({});
  const [values, setValues] = useState<Record<string, FieldValue>>({});
  const [notice, setNotice] = useState<{ tone: "ok" | "danger"; text: string } | null>(null);

  useEffect(() => {
    if (!query.data) return;
    const next = hydrate(query.data as unknown as Record<string, unknown>);
    setBaseline(next);
    setValues(next);
  }, [query.data]);

  const dirty = useMemo(() => isDirty(baseline, values), [baseline, values]);
  useEffect(() => { onDirtyChange(dirty); }, [dirty, onDirtyChange]);
  const validation = useMemo(
    () => (dirty ? buildPayload(baseline, values, ["guzzleConnectTimeout", "guzzleRequestTimeout", "autoAllocStartPort", "autoAllocEndPort"]) : null),
    [baseline, values, dirty],
  );
  const set = (field: string, value: FieldValue) => setValues((previous) => ({ ...previous, [field]: value }));

  const saveMut = useMutation({
    mutationFn: () => {
      if (!validation || !validation.ok) throw new Error(validation && !validation.ok ? validation.errors.join(" ") : "Nothing to save.");
      return saveAdvancedSettings(validation.payload as unknown as ApiPanelAdvancedSettings);
    },
    onSuccess: async () => {
      setNotice({ tone: "ok", text: "Advanced settings saved." });
      await qc.invalidateQueries({ queryKey: ["panel-advanced-settings"] });
    },
    onError: (error) => setNotice({ tone: "danger", text: errorMessage(error, "Advanced settings could not be saved.") }),
  });

  if (query.isPending) return <AdminLoadingState label="Reading advanced settings…" />;
  if (query.isError) {
    return (
      <AdminErrorState
        message={`Advanced settings could not be read (${errorMessage(query.error, "the control plane did not respond")}). Saving is disabled rather than writing defaults over the stored runtime configuration.`}
        retry={() => void query.refetch()}
      />
    );
  }

  const state = sourceState(query);
  const errors = validation && !validation.ok ? validation.errors : [];

  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); saveMut.mutate(); }}>
      <Card>
        <CardHeader title="Runtime behaviour" icon={Wrench} />
        <div className="grid gap-3 p-4 md:grid-cols-2">
          <Switch field="recaptchaEnabled" values={values} onChange={set} label="reCAPTCHA enabled" />
          <Field field="recaptchaWebsiteKey" values={values} onChange={set} label="Site key" mono />
          <Field field="recaptchaSecretKey" type="password" values={values} onChange={set} label="Secret key" mono hint="Masked by the API when set; leave unchanged to keep it." />
          <Field field="guzzleConnectTimeout" type="number" values={values} onChange={set} label="Connect timeout (seconds)" />
          <Field field="guzzleRequestTimeout" type="number" values={values} onChange={set} label="Request timeout (seconds)" />
          <Switch field="autoAllocEnabled" values={values} onChange={set} label="Automatic allocations" />
          <Field field="autoAllocStartPort" type="number" values={values} onChange={set} label="Start port" />
          <Field field="autoAllocEndPort" type="number" values={values} onChange={set} label="End port" />
        </div>
      </Card>
      {errors.length > 0 ? <Notice tone="danger">{errors.join(" ")}</Notice> : null}
      {notice ? <Notice tone={notice.tone}>{notice.text}</Notice> : null}
      <SaveFooter
        disabled={Boolean(errors.length)}
        dirty={dirty}
        label="Save"
        pending={saveMut.isPending}
        pendingLabel="Saving…"
        state={state}
      />
    </form>
  );
}
