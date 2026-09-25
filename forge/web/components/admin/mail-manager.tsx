"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AdminCard, AdminPageLayout } from "@/components/admin/admin-layout";
import { OfflineBanner } from "@/components/shared/states-offline";
import * as mail from "@/lib/api/mail";
import { sanitizeError } from "@/lib/sanitize";

const DEFAULT_SETTINGS: mail.PanelMailSettings = {
  driver: "smtp",
  smtpHost: "",
  smtpPort: 587,
  smtpEncryption: "tls",
  smtpUsername: "",
  smtpPassword: "",
  mailFromAddress: "noreply@example.com",
  mailFromName: "Forge",
};

// The panel returns a masked placeholder for the write-only password, so the
// editor is seeded field-by-field and blanks stay blanks (never the defaults).
function withFallbacks(loaded: mail.PanelMailSettings | undefined): mail.PanelMailSettings {
  if (!loaded) return DEFAULT_SETTINGS;
  return {
    driver: loaded.driver || DEFAULT_SETTINGS.driver,
    smtpHost: loaded.smtpHost || "",
    smtpPort: loaded.smtpPort || DEFAULT_SETTINGS.smtpPort,
    smtpEncryption: loaded.smtpEncryption || DEFAULT_SETTINGS.smtpEncryption,
    smtpUsername: loaded.smtpUsername || "",
    smtpPassword: loaded.smtpPassword || "",
    mailFromAddress: loaded.mailFromAddress || "",
    mailFromName: loaded.mailFromName || "",
  };
}

export function MailManager() {
  const queryClient = useQueryClient();
  // `draft` shadows the cached server values until a save is confirmed (or the
  // operator reloads), so an accepted write is always re-read from the panel.
  const [draft, setDraft] = useState<mail.PanelMailSettings | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [testRecipient, setTestRecipient] = useState("");

  const settingsQuery = useQuery({
    queryKey: ["panel-mail-settings"],
    queryFn: mail.getMailSettings,
  });
  const triggersQuery = useQuery({
    queryKey: ["panel-mail-triggers"],
    queryFn: mail.listMailTriggers,
  });

  const settings = draft ?? withFallbacks(settingsQuery.data);
  const setSettings = (next: mail.PanelMailSettings) => setDraft(next);
  const triggers = triggersQuery.data ?? [];

  const loadError = settingsQuery.isError
    ? sanitizeError(settingsQuery.error instanceof Error ? settingsQuery.error.message : "Failed to load mail settings")
    : triggersQuery.isError
      ? sanitizeError(triggersQuery.error instanceof Error ? triggersQuery.error.message : "Failed to load mail triggers")
      : null;

  const saveMut = useMutation({
    mutationFn: (next: mail.PanelMailSettings) => mail.updateMailSettings(next),
    onSuccess: () => {
      setDraft(null);
      setSuccess("Mail settings saved");
      void queryClient.invalidateQueries({ queryKey: ["panel-mail-settings"] });
    },
  });

  const testMut = useMutation({
    mutationFn: (recipient: string) => mail.testMail(recipient),
    onSuccess: (_result, recipient) => setSuccess(`Test email queued for ${recipient}`),
  });

  const actionError = saveMut.isError
    ? sanitizeError(saveMut.error instanceof Error ? saveMut.error.message : "Save failed")
    : testMut.isError
      ? sanitizeError(testMut.error instanceof Error ? testMut.error.message : "Test failed")
      : null;

  function reloadAll() {
    setDraft(null);
    void settingsQuery.refetch();
    void triggersQuery.refetch();
  }

  function dismissError() {
    setError(null);
    setSuccess(null);
    saveMut.reset();
    testMut.reset();
  }

  function handleSave() {
    setError(null);
    setSuccess(null);
    // If password is masked, keep original by sending masked value — backend preserves it
    saveMut.mutate(settings);
  }

  function handleTest() {
    const recipient = testRecipient.trim();
    if (!recipient) {
      setError("Recipient required");
      return;
    }
    setError(null);
    setSuccess(null);
    testMut.mutate(recipient);
  }

  return (
    <AdminPageLayout
      title="Mail Settings"
      description="Panel SMTP configuration (GET/PUT /admin/mail/settings) and test delivery. Driver 'log' bypasses SMTP validation. Password is write-only; reads return a masked placeholder that preserves the stored value when echoed back."
    >
      <OfflineBanner onRetry={reloadAll} />
      {(error || actionError || loadError) && (
        <div role="alert" className="flex items-center justify-between gap-3 rounded-xl border border-red-500/25 bg-red-500/[0.09] p-4 text-sm text-red-200">
          <span>{error ?? actionError ?? loadError}</span>
          <div className="flex items-center gap-2">
            {(settingsQuery.isError || triggersQuery.isError) && (
              <button onClick={reloadAll} className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Retry</button>
            )}
            <button onClick={dismissError} className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
          </div>
        </div>
      )}
      {success && (
        <div role="status" className="flex items-center justify-between gap-3 rounded-xl border border-emerald-500/25 bg-emerald-500/[0.09] p-4 text-sm text-emerald-200">
          <span>{success}</span> <button onClick={() => setSuccess(null)} className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Dismiss</button>
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-2">
        <AdminCard title="SMTP Configuration" description="Validated via mail.ValidateSettings: host required, port 1-65535, mail from must be valid address, encryption none|tls|ssl.">
          {settingsQuery.isPending ? (
            <div className="grid place-items-center rounded-xl border border-dashed border-[var(--line)] bg-black/10 p-6 text-sm text-[var(--text-subtle)]">Loading…</div>
          ) : (
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="text-xs font-bold uppercase tracking-[0.12em] text-[var(--text-subtle)]">Driver</label>
                  <select value={settings.driver} onChange={(e) => setSettings({ ...settings, driver: e.target.value })} className="w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">
                    <option value="smtp">smtp</option>
                    <option value="log">log</option>
                  </select>
                </div>
                <div>
                  <label className="text-xs font-bold uppercase tracking-[0.12em] text-[var(--text-subtle)]">Encryption</label>
                  <select value={settings.smtpEncryption} onChange={(e) => setSettings({ ...settings, smtpEncryption: e.target.value })} className="w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">
                    <option value="none">none</option>
                    <option value="tls">tls (STARTTLS)</option>
                    <option value="ssl">ssl (SMTPS)</option>
                  </select>
                </div>
              </div>
              <div className="grid grid-cols-3 gap-2">
                <div className="col-span-2">
                  <label className="text-xs font-bold uppercase tracking-[0.12em] text-[var(--text-subtle)]">SMTP Host</label>
                  <input value={settings.smtpHost} onChange={(e) => setSettings({ ...settings, smtpHost: e.target.value })} placeholder="smtp.example.com" className="w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
                </div>
                <div>
                  <label className="text-xs font-bold uppercase tracking-[0.12em] text-[var(--text-subtle)]">Port</label>
                  <input type="number" value={settings.smtpPort} onChange={(e) => setSettings({ ...settings, smtpPort: Number(e.target.value) })} className="w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="text-xs font-bold uppercase tracking-[0.12em] text-[var(--text-subtle)]">Username</label>
                  <input value={settings.smtpUsername} onChange={(e) => setSettings({ ...settings, smtpUsername: e.target.value })} className="w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
                </div>
                <div>
                  <label className="text-xs font-bold uppercase tracking-[0.12em] text-[var(--text-subtle)]">Password</label>
                  <input type="password" value={settings.smtpPassword} onChange={(e) => setSettings({ ...settings, smtpPassword: e.target.value })} placeholder="masked if set" className="w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="text-xs font-bold uppercase tracking-[0.12em] text-[var(--text-subtle)]">From Address</label>
                  <input value={settings.mailFromAddress} onChange={(e) => setSettings({ ...settings, mailFromAddress: e.target.value })} className="w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
                </div>
                <div>
                  <label className="text-xs font-bold uppercase tracking-[0.12em] text-[var(--text-subtle)]">From Name</label>
                  <input value={settings.mailFromName} onChange={(e) => setSettings({ ...settings, mailFromName: e.target.value })} className="w-full rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
                </div>
              </div>
              <div className="flex gap-2">
                <button onClick={handleSave} disabled={saveMut.isPending} aria-label="Save mail settings" className="rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-bold text-white hover:bg-[var(--brand-hover)] motion-safe:transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--surface)]">{saveMut.isPending ? "Saving…" : "Save Settings"}</button>
                <button onClick={reloadAll} aria-label="Reload mail settings" className="rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-4 py-2 text-sm text-[var(--text)] hover:bg-[var(--surface-hover)] motion-safe:transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Reload</button>
              </div>
            </div>
          )}
        </AdminCard>

        <div className="space-y-6">
          <AdminCard title="Test Delivery" description="POST /admin/mail/test {recipient}. Queues via MailTriggerService worker or direct mail_outbox.">
            <div className="flex gap-2">
              <input value={testRecipient} onChange={(e) => setTestRecipient(e.target.value)} placeholder="recipient@example.com" className="flex-1 rounded-lg border border-[var(--line-strong)] bg-[var(--surface-input)] px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--text-subtle)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]" />
              <button onClick={handleTest} disabled={testMut.isPending} aria-label="Send test email" className="rounded-lg bg-[var(--surface-raised)] border border-[var(--line)] px-4 py-2 text-xs font-bold text-[var(--text)] hover:bg-[var(--surface-hover)] motion-safe:transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">{testMut.isPending ? "Sending…" : "Send Test"}</button>
            </div>
            <p className="mt-2 text-xs text-[var(--text-subtle)]">Sends subject “Test Email from {settings.mailFromName}” via SMTP sender with 15s timeout, TLS 1.2+, multipart alternative.</p>
          </AdminCard>

          <AdminCard title="Mail Triggers" description="GET /admin/mail/triggers — registry of server/backup/account events mapped to templates.">
            {triggersQuery.isError ? (
              <p role="alert" className="text-sm text-red-200">Mail triggers could not be loaded — the list below is not known to be empty.</p>
            ) : triggers.length === 0 ? (
              <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-[var(--line)] bg-black/10 px-5 py-10 text-center">
                <p className="text-sm font-semibold text-[var(--text)]">No triggers</p>
                <p className="mt-1 text-sm text-[var(--text-subtle)]">No mail triggers registered.</p>
              </div>
            ) : (
              <div className="grid gap-2">
                {triggers.map((t) => (
                  <div key={t.event} className="flex items-center justify-between rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-3 py-2 text-xs motion-safe:transition-colors hover:bg-[var(--surface-hover)]">
                    <div>
                      <p className="font-bold text-[var(--text)]">{t.label}</p>
                      <p className="font-mono text-[var(--text-subtle)]">{t.event} → {t.template}</p>
                    </div>
                    <span className="rounded-full border border-[var(--line)] bg-[var(--surface)] px-2 py-1 text-[11px] text-[var(--text-subtle)]">mail</span>
                  </div>
                ))}
              </div>
            )}
            <button onClick={() => void triggersQuery.refetch()} className="mt-3 rounded-lg border border-[var(--line)] bg-[var(--surface-raised)] px-3 py-1.5 text-xs text-[var(--text)] hover:bg-[var(--surface-hover)] motion-safe:transition-colors motion-reduce:transition-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--focus)]">Refresh Triggers</button>
          </AdminCard>
        </div>
      </div>
    </AdminPageLayout>
  );
}
