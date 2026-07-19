"use client";

import { useEffect, useState } from "react";
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
import { Btn, Card, CardHeader, Input, SectionHeader, cn } from "./admin-ui";

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

const DEFAULT_GENERAL: ApiPanelSettings = {
  companyName: "Forge Control Plane",
  shortName: "Forge",
  productName: "GamePanel",
  browserTitle: "GamePanel",
  footerText: "",
  logoUrl: "",
  faviconUrl: "",
  loginBackgroundUrl: "",
  themePreset: "default",
  require2FA: "none",
  defaultLocale: "en",
  defaultTimezone: "UTC",
  dateFormat: "yyyy-MM-dd",
  numberFormat: "en-US",
  currencyFormat: "USD",
  defaultDashboard: "overview",
  landingPage: "servers",
  sidebarLayout: "expanded",
  compactMode: false,
  advancedMode: false,
  requireEmailVerification: false,
  passwordComplexity: "standard",
  passwordExpirationDays: 0,
  sessionDurationMinutes: 1440,
  loginRateLimitEnabled: true,
  loginAttemptThreshold: 5,
  accountLockoutMinutes: 15,
  geoRestrictions: "",
  apiTokenTtlDays: 0,
  apiRotationDays: 0,
  allowedOrigins: "",
  trustedNetworks: "",
  metricsRetentionDays: 30,
  logsRetentionDays: 30,
  auditRetentionDays: 365,
  metricsSamplingRate: 100,
  monitoringPollIntervalSeconds: 30,
  emailAlertsEnabled: false,
  webhookAlertsEnabled: false,
  discordWebhookUrl: "",
  slackWebhookUrl: "",
  telegramBotToken: "",
  placementStrategy: "balanced",
  antiAffinityRules: "",
  resourceReservationsEnabled: true,
  nodePrioritization: "capacity",
  recoveryStrategy: "manual",
  failoverThresholdSeconds: 300,
  heartbeatThresholdSeconds: 60,
  reservationDurationMinutes: 30,
  reservationCleanupMinutes: 60,
  capacityBufferPercent: 10,
  backupProvider: "local",
  backupRetentionDays: 7,
  backupLimit: 0,
  backupAutoCleanup: true,
  backupEncryptionEnabled: false,
  backupKeyRotationDays: 90,
};

export function AdminSettings() {
  const [tab, setTab] = useState<Tab>("general");
  return (
    <div className="space-y-6">
      <SectionHeader title="Settings Center" sub="Global controls for branding, security, monitoring, orchestration, and platform behavior." />
      <nav className="flex flex-wrap gap-1 border-b border-white/[0.06]">
        {TABS.map((t) => (
          <button
            key={t.id}
            className={cn(
              "flex items-center gap-1.5 px-3 py-2 text-sm font-medium transition",
              tab === t.id ? "border-b-2 border-[#dc2626] text-[#dc2626]" : "text-slate-400 hover:text-slate-200",
            )}
            onClick={() => setTab(t.id)}
            type="button"
          >
            <t.icon size={14} /> {t.label}
          </button>
        ))}
      </nav>
      {tab === "general" && <PanelSettingsTab mode="general" />}
      {tab === "security" && <PanelSettingsTab mode="security" />}
      {tab === "mail" && <MailTab />}
      {tab === "monitoring" && <PanelSettingsTab mode="monitoring" />}
      {tab === "orchestration" && <PanelSettingsTab mode="orchestration" />}
      {tab === "backups" && <PanelSettingsTab mode="backups" />}
      {tab === "advanced" && <AdvancedTab />}
    </div>
  );
}

function Toggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (value: boolean) => void }) {
  return (
    <label className="flex items-center gap-2 text-sm text-slate-300">
      <input className="accent-[#dc2626]" checked={checked} onChange={(event) => onChange(event.target.checked)} type="checkbox" />
      <span>{label}</span>
    </label>
  );
}

function SelectField({ label, value, options, onChange }: { label: string; value: string; options: string[]; onChange: (value: string) => void }) {
  return (
    <label className="block text-sm">
      <span className="mb-1.5 block font-medium text-slate-300">{label}</span>
      <select className="h-9 w-full rounded-lg border border-white/10 bg-[#161b28] px-3 text-slate-100" onChange={(event) => onChange(event.target.value)} value={value}>
        {options.map((option) => <option key={option} value={option}>{option}</option>)}
      </select>
    </label>
  );
}

function PanelSettingsTab({ mode }: { mode: Exclude<Tab, "mail" | "advanced"> }) {
  const qc = useQueryClient();
  const { data: settings, isLoading } = useQuery({ queryKey: ["panel-settings"], queryFn: fetchPanelSettings });
  type RequiredSettings = Required<ApiPanelSettings>;
  const [form, setForm] = useState<RequiredSettings>(DEFAULT_GENERAL as RequiredSettings);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    if (settings) setForm((previous) => ({ ...previous, ...settings } as RequiredSettings));
  }, [settings]);

  const set = <K extends keyof RequiredSettings>(key: K, value: RequiredSettings[K]) => {
    setForm((previous) => ({ ...previous, [key]: value }));
  };

  const numberSet = (key: keyof RequiredSettings, value: string) => {
    setForm((previous) => ({ ...previous, [key]: Number(value) as RequiredSettings[typeof key] }));
  };

  const saveMut = useMutation({
    mutationFn: () => savePanelSettings(form),
    onSuccess: async (saved) => {
      setNotice("Settings saved.");
      qc.setQueryData(["panel-settings"], saved);
      qc.setQueryData(["public-panel-settings"], {
        companyName: saved.companyName,
        shortName: saved.shortName,
        productName: saved.productName,
        browserTitle: saved.browserTitle,
        footerText: saved.footerText,
        logoUrl: saved.logoUrl,
        faviconUrl: saved.faviconUrl,
        loginBackgroundUrl: saved.loginBackgroundUrl,
        themePreset: saved.themePreset,
        defaultLocale: saved.defaultLocale,
      });
      await qc.invalidateQueries({ queryKey: ["public-panel-settings"] });
      await qc.invalidateQueries({ queryKey: ["panel-settings"] });
    },
    onError: (error) => setNotice(error instanceof Error ? error.message : "Settings could not be saved."),
  });

  if (isLoading) return <div className="p-8 text-center text-sm text-slate-500">Loading...</div>;

  return (
    <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); saveMut.mutate(); }}>
      {mode === "general" ? (
        <>
          <Card>
            <CardHeader title="Branding" icon={Globe} />
            <div className="grid gap-3 p-4 md:grid-cols-2">
              <Input label="Company Name" value={form.companyName} onChange={(value) => set("companyName", value)} />
              <Input label="Short Name" value={form.shortName} onChange={(value) => set("shortName", value)} />
              <Input label="Product Name" value={form.productName} onChange={(value) => set("productName", value)} />
              <Input label="Browser Title" value={form.browserTitle} onChange={(value) => set("browserTitle", value)} />
              <Input label="Footer Text" value={form.footerText} onChange={(value) => set("footerText", value)} />
              <Input label="Theme Preset" value={form.themePreset} onChange={(value) => set("themePreset", value)} />
              <Input label="Logo URL" value={form.logoUrl} onChange={(value) => set("logoUrl", value)} />
              <Input label="Favicon URL" value={form.faviconUrl} onChange={(value) => set("faviconUrl", value)} />
              <Input label="Login Background URL" value={form.loginBackgroundUrl} onChange={(value) => set("loginBackgroundUrl", value)} />
            </div>
          </Card>
          <Card>
            <CardHeader title="Localization & Experience" icon={SettingsIcon} />
            <div className="grid gap-3 p-4 md:grid-cols-2">
              <Input label="Default Language" value={form.defaultLocale} onChange={(value) => set("defaultLocale", value)} />
              <Input label="Default Timezone" value={form.defaultTimezone} onChange={(value) => set("defaultTimezone", value)} />
              <Input label="Date Format" value={form.dateFormat} onChange={(value) => set("dateFormat", value)} />
              <Input label="Number Format" value={form.numberFormat} onChange={(value) => set("numberFormat", value)} />
              <Input label="Currency Format" value={form.currencyFormat} onChange={(value) => set("currencyFormat", value)} />
              <SelectField label="Default Dashboard" value={form.defaultDashboard} options={["overview", "monitoring", "servers"]} onChange={(value) => set("defaultDashboard", value)} />
              <SelectField label="Landing Page" value={form.landingPage} options={["servers", "admin/overview", "admin/monitoring"]} onChange={(value) => set("landingPage", value)} />
              <SelectField label="Sidebar Layout" value={form.sidebarLayout} options={["expanded", "compact"]} onChange={(value) => set("sidebarLayout", value)} />
              <Toggle label="Compact Mode" checked={form.compactMode} onChange={(value) => set("compactMode", value)} />
              <Toggle label="Advanced Mode" checked={form.advancedMode} onChange={(value) => set("advancedMode", value)} />
            </div>
          </Card>
        </>
      ) : null}

      {mode === "security" ? (
        <Card>
          <CardHeader title="Security Settings" icon={Shield} />
          <div className="grid gap-3 p-4 md:grid-cols-2">
            <SelectField label="Require 2FA" value={form.require2FA} options={["none", "admin", "all"]} onChange={(value) => set("require2FA", value as RequiredSettings["require2FA"])} />
            <Toggle label="Require Email Verification" checked={form.requireEmailVerification} onChange={(value) => set("requireEmailVerification", value)} />
            <SelectField label="Password Complexity" value={form.passwordComplexity} options={["standard", "strong", "strict"]} onChange={(value) => set("passwordComplexity", value)} />
            <Input label="Password Expiration Days" type="number" value={String(form.passwordExpirationDays)} onChange={(value) => numberSet("passwordExpirationDays", value)} />
            <Input label="Session Duration Minutes" type="number" value={String(form.sessionDurationMinutes)} onChange={(value) => numberSet("sessionDurationMinutes", value)} />
            <Toggle label="Login Rate Limiting" checked={form.loginRateLimitEnabled} onChange={(value) => set("loginRateLimitEnabled", value)} />
            <Input label="Login Attempt Threshold" type="number" value={String(form.loginAttemptThreshold)} onChange={(value) => numberSet("loginAttemptThreshold", value)} />
            <Input label="Account Lockout Minutes" type="number" value={String(form.accountLockoutMinutes)} onChange={(value) => numberSet("accountLockoutMinutes", value)} />
            <Input label="Geo Restrictions" value={form.geoRestrictions} onChange={(value) => set("geoRestrictions", value)} />
            <Input label="API Token TTL Days" type="number" value={String(form.apiTokenTtlDays)} onChange={(value) => numberSet("apiTokenTtlDays", value)} />
            <Input label="API Rotation Days" type="number" value={String(form.apiRotationDays)} onChange={(value) => numberSet("apiRotationDays", value)} />
            <Input label="Allowed Origins" value={form.allowedOrigins} onChange={(value) => set("allowedOrigins", value)} />
            <Input label="Trusted Networks" value={form.trustedNetworks} onChange={(value) => set("trustedNetworks", value)} />
          </div>
        </Card>
      ) : null}

      {mode === "monitoring" ? (
        <Card>
          <CardHeader title="Monitoring Settings" icon={Bell} />
          <div className="grid gap-3 p-4 md:grid-cols-2">
            <Input label="Metrics Retention Days" type="number" value={String(form.metricsRetentionDays)} onChange={(value) => numberSet("metricsRetentionDays", value)} />
            <Input label="Logs Retention Days" type="number" value={String(form.logsRetentionDays)} onChange={(value) => numberSet("logsRetentionDays", value)} />
            <Input label="Audit Retention Days" type="number" value={String(form.auditRetentionDays)} onChange={(value) => numberSet("auditRetentionDays", value)} />
            <Input label="Sampling Rate Percent" type="number" value={String(form.metricsSamplingRate)} onChange={(value) => numberSet("metricsSamplingRate", value)} />
            <Input label="Polling Interval Seconds" type="number" value={String(form.monitoringPollIntervalSeconds)} onChange={(value) => numberSet("monitoringPollIntervalSeconds", value)} />
            <Toggle label="Email Alerts" checked={form.emailAlertsEnabled} onChange={(value) => set("emailAlertsEnabled", value)} />
            <Toggle label="Webhook Alerts" checked={form.webhookAlertsEnabled} onChange={(value) => set("webhookAlertsEnabled", value)} />
            <Input label="Discord Webhook URL" value={form.discordWebhookUrl} onChange={(value) => set("discordWebhookUrl", value)} />
            <Input label="Slack Webhook URL" value={form.slackWebhookUrl} onChange={(value) => set("slackWebhookUrl", value)} />
            <Input label="Telegram Bot Token" value={form.telegramBotToken} onChange={(value) => set("telegramBotToken", value)} />
          </div>
        </Card>
      ) : null}

      {mode === "orchestration" ? (
        <Card>
          <CardHeader title="Orchestration Settings" icon={Workflow} />
          <div className="grid gap-3 p-4 md:grid-cols-2">
            <SelectField label="Placement Strategy" value={form.placementStrategy} options={["balanced", "least-loaded", "spread", "binpack"]} onChange={(value) => set("placementStrategy", value)} />
            <Input label="Anti-Affinity Rules" value={form.antiAffinityRules} onChange={(value) => set("antiAffinityRules", value)} />
            <Toggle label="Resource Reservations" checked={form.resourceReservationsEnabled} onChange={(value) => set("resourceReservationsEnabled", value)} />
            <SelectField label="Node Prioritization" value={form.nodePrioritization} options={["capacity", "latency", "region", "manual"]} onChange={(value) => set("nodePrioritization", value)} />
            <SelectField label="Recovery Strategy" value={form.recoveryStrategy} options={["manual", "assisted", "automatic"]} onChange={(value) => set("recoveryStrategy", value)} />
            <Input label="Failover Threshold Seconds" type="number" value={String(form.failoverThresholdSeconds)} onChange={(value) => numberSet("failoverThresholdSeconds", value)} />
            <Input label="Heartbeat Threshold Seconds" type="number" value={String(form.heartbeatThresholdSeconds)} onChange={(value) => numberSet("heartbeatThresholdSeconds", value)} />
            <Input label="Reservation Duration Minutes" type="number" value={String(form.reservationDurationMinutes)} onChange={(value) => numberSet("reservationDurationMinutes", value)} />
            <Input label="Reservation Cleanup Minutes" type="number" value={String(form.reservationCleanupMinutes)} onChange={(value) => numberSet("reservationCleanupMinutes", value)} />
            <Input label="Capacity Buffer Percent" type="number" value={String(form.capacityBufferPercent)} onChange={(value) => numberSet("capacityBufferPercent", value)} />
          </div>
        </Card>
      ) : null}

      {mode === "backups" ? (
        <Card>
          <CardHeader title="Backup Settings" icon={Archive} />
          <div className="grid gap-3 p-4 md:grid-cols-2">
            <SelectField label="Storage Provider" value={form.backupProvider} options={["local", "s3", "cloudflare-r2", "backblaze-b2", "azure-blob", "google-cloud-storage"]} onChange={(value) => set("backupProvider", value)} />
            <Input label="Backup Retention Days" type="number" value={String(form.backupRetentionDays)} onChange={(value) => numberSet("backupRetentionDays", value)} />
            <Input label="Backup Limit" type="number" value={String(form.backupLimit)} onChange={(value) => numberSet("backupLimit", value)} />
            <Toggle label="Automatic Cleanup" checked={form.backupAutoCleanup} onChange={(value) => set("backupAutoCleanup", value)} />
            <Toggle label="Backup Encryption" checked={form.backupEncryptionEnabled} onChange={(value) => set("backupEncryptionEnabled", value)} />
            <Input label="Key Rotation Days" type="number" value={String(form.backupKeyRotationDays)} onChange={(value) => numberSet("backupKeyRotationDays", value)} />
          </div>
        </Card>
      ) : null}

      {notice ? <p className="text-sm text-slate-300">{notice}</p> : null}
      <div className="flex justify-end">
        <Btn tone="primary" type="submit" disabled={saveMut.isPending}>
          {saveMut.isPending ? "Saving..." : "Save"}
        </Btn>
      </div>
    </form>
  );
}

function MailTab() {
  const qc = useQueryClient();
  const { data: settings, isLoading } = useQuery({ queryKey: ["panel-mail-settings"], queryFn: fetchMailSettings });
  const [form, setForm] = useState<ApiPanelMailSettings>({
    smtpHost: "", smtpPort: 587, smtpEncryption: "tls", smtpUsername: "", smtpPassword: "",
    mailFromAddress: "", mailFromName: "",
  });
  const [notice, setNotice] = useState("");
  const [testRecipient, setTestRecipient] = useState("");
  useEffect(() => { if (settings) setForm(settings); }, [settings]);
  const saveMut = useMutation({
    mutationFn: () => saveMailSettings(form),
    onSuccess: async () => {
      setNotice("Mail settings saved.");
      await qc.invalidateQueries({ queryKey: ["panel-mail-settings"] });
    },
    onError: (error) => setNotice(error instanceof Error ? error.message : "Mail settings could not be saved."),
  });
  const testMut = useMutation({
    mutationFn: () => testMailSettings(testRecipient.trim()),
    onError: (error) => setNotice(error instanceof Error ? error.message : "Test email could not be sent."),
  });
  if (isLoading) return <div className="p-8 text-center text-sm text-slate-500">Loading...</div>;
  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); saveMut.mutate(); }}>
      <Card>
        <CardHeader title="SMTP" icon={Mail} />
        <div className="grid gap-3 p-4 md:grid-cols-2">
              <Input label="SMTP Host" value={form.smtpHost ?? ""} onChange={(v) => setForm((p) => ({ ...p, smtpHost: v }))} required />
              <Input label="SMTP Port" value={String(form.smtpPort ?? 587)} onChange={(v) => setForm((p) => ({ ...p, smtpPort: Number(v) }))} type="number" required />
              <SelectField label="Encryption" value={form.smtpEncryption ?? ""} options={["", "tls", "ssl"]} onChange={(value) => setForm((p) => ({ ...p, smtpEncryption: value }))} />
          <Input label="SMTP Username" value={form.smtpUsername ?? ""} onChange={(v) => setForm((p) => ({ ...p, smtpUsername: v }))} />
          <Input label="SMTP Password" type="password" value={form.smtpPassword ?? ""} onChange={(v) => setForm((p) => ({ ...p, smtpPassword: v }))} placeholder="Leave blank to keep" />
          <Input label="From Address" type="email" value={form.mailFromAddress ?? ""} onChange={(v) => setForm((p) => ({ ...p, mailFromAddress: v }))} required />
          <Input label="From Name" value={form.mailFromName ?? ""} onChange={(v) => setForm((p) => ({ ...p, mailFromName: v }))} />
          <Input label="Test Recipient" type="email" value={testRecipient} onChange={setTestRecipient} placeholder="operator@example.com" />
        </div>
      </Card>
      {testMut.data ? (
        <div className={cn("rounded-lg border p-3 text-sm", testMut.data.sent ? "border-emerald-500/30 bg-emerald-900/10 text-emerald-300" : "border-amber-500/30 bg-amber-900/10 text-amber-300")}>
          {testMut.data.message ?? (testMut.data.sent ? "Test email sent." : "Test email could not be sent.")}
        </div>
      ) : null}
      {notice ? <p className="text-sm text-slate-300">{notice}</p> : null}
      <div className="flex justify-end gap-2">
        <Btn tone="success" type="button" onClick={() => testMut.mutate()} disabled={testMut.isPending || !testRecipient.trim()}>
          {testMut.isPending ? "Sending..." : "Test"}
        </Btn>
        <Btn tone="primary" type="submit" disabled={saveMut.isPending}>
          {saveMut.isPending ? "Saving..." : "Save"}
        </Btn>
      </div>
    </form>
  );
}

function AdvancedTab() {
  const qc = useQueryClient();
  const { data: settings, isLoading } = useQuery({ queryKey: ["panel-advanced-settings"], queryFn: fetchAdvancedSettings });
  const [form, setForm] = useState<ApiPanelAdvancedSettings>({
    recaptchaEnabled: false, recaptchaWebsiteKey: "", recaptchaSecretKey: "",
    guzzleConnectTimeout: 30, guzzleRequestTimeout: 30,
    autoAllocEnabled: false, autoAllocStartPort: 25565, autoAllocEndPort: 25600,
  });
  const [notice, setNotice] = useState("");
  useEffect(() => { if (settings) setForm(settings); }, [settings]);
  const saveMut = useMutation({
    mutationFn: () => saveAdvancedSettings(form),
    onSuccess: async () => {
      setNotice("Advanced settings saved.");
      await qc.invalidateQueries({ queryKey: ["panel-advanced-settings"] });
    },
    onError: (error) => setNotice(error instanceof Error ? error.message : "Advanced settings could not be saved."),
  });
  if (isLoading) return <div className="p-8 text-center text-sm text-slate-500">Loading...</div>;
  return (
    <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); saveMut.mutate(); }}>
      <Card>
        <CardHeader title="Runtime Behavior" icon={Wrench} />
        <div className="grid gap-3 p-4 md:grid-cols-2">
          <Toggle label="reCAPTCHA Enabled" checked={form.recaptchaEnabled ?? false} onChange={(value) => setForm((p) => ({ ...p, recaptchaEnabled: value }))} />
          <Input label="Site Key" value={form.recaptchaWebsiteKey ?? ""} onChange={(value) => setForm((p) => ({ ...p, recaptchaWebsiteKey: value }))} />
          <Input label="Secret Key" value={form.recaptchaSecretKey ?? ""} onChange={(value) => setForm((p) => ({ ...p, recaptchaSecretKey: value }))} />
          <Input label="Connect Timeout Seconds" value={String(form.guzzleConnectTimeout)} onChange={(value) => setForm((p) => ({ ...p, guzzleConnectTimeout: Number(value) }))} type="number" />
          <Input label="Request Timeout Seconds" value={String(form.guzzleRequestTimeout)} onChange={(value) => setForm((p) => ({ ...p, guzzleRequestTimeout: Number(value) }))} type="number" />
          <Toggle label="Automatic Allocations" checked={form.autoAllocEnabled ?? false} onChange={(value) => setForm((p) => ({ ...p, autoAllocEnabled: value }))} />
          <Input label="Start Port" value={String(form.autoAllocStartPort)} onChange={(value) => setForm((p) => ({ ...p, autoAllocStartPort: Number(value) }))} type="number" />
          <Input label="End Port" value={String(form.autoAllocEndPort)} onChange={(value) => setForm((p) => ({ ...p, autoAllocEndPort: Number(value) }))} type="number" />
        </div>
      </Card>
      {notice ? <p className="text-sm text-slate-300">{notice}</p> : null}
      <div className="flex justify-end">
        <Btn tone="primary" type="submit" disabled={saveMut.isPending}>
          {saveMut.isPending ? "Saving..." : "Save"}
        </Btn>
      </div>
    </form>
  );
}
