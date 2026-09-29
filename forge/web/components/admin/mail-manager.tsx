"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Mail, Send, Truck, Zap } from "lucide-react";
import {
  AdminErrorState,
  AdminSelect,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  SectionHeader,
} from "@/components/admin/admin-ui";
import { DataState, FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { AdminPageLayout } from "@/components/admin/admin-ui";
import { OfflineBanner } from "@/components/shared/states-offline";
import * as mail from "@/lib/api/mail";
import { sanitizeError } from "@/lib/sanitize";

/**
 * This page is the `/admin/mail/settings` surface.
 *
 * Platform Settings has a Mail tab that writes a *different* record
 * (`/admin/settings/mail`, `handlers_settings_extras.go:25-57`) with a different
 * field spelling and no `driver`. They must not share a cache key — the tab now
 * uses `["panel-settings-mail"]`, so neither form can be filled from the other's
 * read. Which of the two endpoints should exist at all is an API decision
 * (see the impl report).
 */
export const MAIL_SETTINGS_QUERY_KEY = ["panel-mail-settings"] as const;

type Fields = {
  driver: string;
  smtpHost: string;
  smtpPort: string;
  smtpEncryption: string;
  smtpUsername: string;
  smtpPassword: string;
  mailFromAddress: string;
  mailFromName: string;
};

const EMPTY: Fields = {
  driver: "smtp",
  smtpHost: "",
  smtpPort: "587",
  smtpEncryption: "tls",
  smtpUsername: "",
  smtpPassword: "",
  mailFromAddress: "",
  mailFromName: "",
};

/**
 * Field-by-field hydration with **no invented defaults**.
 *
 * The old version fell back to `mailFromAddress: "noreply@example.com"` and
 * `mailFromName: "Forge"` whenever the read had not answered, and Save posted the
 * object verbatim — so a load failure followed by Save wrote a placeholder sender
 * address into production mail. The only fallbacks kept are the two the server
 * itself treats as its defaults, and Save stays disabled until a read succeeds.
 */
function fieldsFrom(loaded: mail.PanelMailSettings | undefined): Fields {
  if (!loaded) return EMPTY;
  return {
    driver: loaded.driver || EMPTY.driver,
    smtpHost: loaded.smtpHost ?? "",
    smtpPort: loaded.smtpPort ? String(loaded.smtpPort) : "",
    smtpEncryption: loaded.smtpEncryption || "",
    smtpUsername: loaded.smtpUsername ?? "",
    smtpPassword: loaded.smtpPassword ?? "",
    mailFromAddress: loaded.mailFromAddress ?? "",
    mailFromName: loaded.mailFromName ?? "",
  };
}

function toPayload(fields: Fields): mail.PanelMailSettings {
  return {
    driver: fields.driver,
    smtpHost: fields.smtpHost,
    smtpPort: Number(fields.smtpPort),
    smtpEncryption: fields.smtpEncryption,
    smtpUsername: fields.smtpUsername,
    // The API's masked sentinel is echoed back verbatim, which the handler
    // interprets as "keep what you have" (handlers_mail_settings.go).
    smtpPassword: fields.smtpPassword,
    mailFromAddress: fields.mailFromAddress,
    mailFromName: fields.mailFromName,
  };
}

export function MailManager() {
  const queryClient = useQueryClient();
  // `draft` shadows the cached server values until a save is confirmed (or the
  // operator reloads), so an accepted write is always re-read from the panel.
  const [draft, setDraft] = useState<Fields | null>(null);
  const [dismissed, setDismissed] = useState(false);
  const [testRecipient, setTestRecipient] = useState("");

  const settingsQuery = useQuery({ queryKey: MAIL_SETTINGS_QUERY_KEY, queryFn: mail.getMailSettings });
  const triggersQuery = useQuery({ queryKey: ["panel-mail-triggers"], queryFn: mail.listMailTriggers });

  const loaded = !settingsQuery.data || Object.keys(settingsQuery.data).length === 0 ? undefined : settingsQuery.data;
  const fields = draft ?? fieldsFrom(settingsQuery.data);
  const setField = (key: keyof Fields, value: string) => setDraft({ ...fields, [key]: value });

  const loadError = settingsQuery.isError
    ? sanitizeError(settingsQuery.error instanceof Error ? settingsQuery.error.message : "Failed to load mail settings")
    : null;
  const triggersError = triggersQuery.isError
    ? sanitizeError(triggersQuery.error instanceof Error ? triggersQuery.error.message : "Failed to load mail triggers")
    : null;

  const portInvalid = fields.smtpPort.trim() !== "" && (!Number.isInteger(Number(fields.smtpPort)) || Number(fields.smtpPort) < 1 || Number(fields.smtpPort) > 65535);
  const saveMut = useMutation({
    mutationFn: async (next: mail.PanelMailSettings) => {
      const result = await mail.updateMailSettings(next);
      if (!result.ok) throw new Error("The server reported the mail settings update did not complete.");
      return result;
    },
    onSuccess: () => {
      setDraft(null);
      setDismissed(false);
      void queryClient.invalidateQueries({ queryKey: MAIL_SETTINGS_QUERY_KEY });
    },
  });

  const testMut = useMutation({
    mutationFn: async (recipient: string) => {
      const result = await mail.testMail(recipient);
      if (!result.ok) throw new Error(result.message || "The server reported the test email was not sent.");
      return result;
    },
    onSuccess: (_result, recipient) => setDismissed(false),
  });

  const actionError = saveMut.isError
    ? sanitizeError(saveMut.error instanceof Error ? saveMut.error.message : "Save failed")
    : testMut.isError
      ? sanitizeError(testMut.error instanceof Error ? testMut.error.message : "Test failed")
      : null;

  const success = saveMut.isSuccess && !draft
    ? "Mail settings saved and re-read from the control plane."
    : testMut.isSuccess
      ? `Test email queued for ${testRecipient.trim()}. Queued is not delivered — check the recipient's inbox or the mail log.`
      : null;

  function reloadAll() {
    setDraft(null);
    setDismissed(false);
    saveMut.reset();
    testMut.reset();
    void settingsQuery.refetch();
    void triggersQuery.refetch();
  }

  const dirty = draft !== null;
  const passwordIsStored = fields.smtpPassword === mail.MAIL_MASKED_SECRET;
  const smtpIncomplete = fields.driver === "smtp" && fields.smtpHost.trim() === "";
  const canSave = Boolean(loaded) && !settingsQuery.isError && !portInvalid && fields.smtpHost.trim() !== "" && fields.mailFromAddress.trim() !== "";
  const triggers = triggersQuery.data ?? [];

  return (
    <AdminPageLayout className="space-y-6">
      <SectionHeader
        sub="Outbound mail for the whole panel: the SMTP (or log-only) driver, the sender identity, a test delivery, and the triggers that decide which events send mail."
        status={<FreshnessBadge state={sourceState(settingsQuery)} />}
        action={
          <Btn onClick={reloadAll} tone="ghost">Reload</Btn>
        }
        info={{
          title: "Mail",
          triggerLabel: "About mail settings",
          description: "How this page reads, writes and tests outbound mail.",
          sections: [
            {
              title: "Write-only password",
              content: "The API returns a masked placeholder for the SMTP password. Echoing that placeholder back keeps the stored value; typing over it replaces it. Save is disabled until the settings have actually been read, so a failed load can never write defaults over a live configuration.",
            },
            {
              title: "Driver log",
              content: "With driver=log the panel does not open an SMTP connection at all: messages are written to the host log instead. Test delivery stays available, but no recipient will receive anything while this driver is selected.",
            },
            {
              title: "Two mail surfaces",
              content: "Platform Settings has its own Mail tab that writes a different record (/admin/settings/mail) and has no driver field. This page is /admin/mail/settings. Use one of them per deployment; they are separate documents with differently named fields.",
            },
            {
              title: "Testing",
              content: "A test queues a real message through the configured driver and reports what the queue accepted, not what the provider delivered. A green message here is not proof of an inbox.",
            },
          ],
        }}
      />

      <OfflineBanner onRetry={reloadAll} />

      {actionError ? <AdminErrorState message={actionError} retry={saveMut.isError ? () => saveMut.mutate(toPayload(fields)) : undefined} /> : null}
      {loadError ? <AdminErrorState message={`${loadError} Saving is disabled until the stored settings can be read.`} retry={() => void settingsQuery.refetch()} /> : null}
      {success && !dismissed ? (
        <div className="ui-alert ui-alert-success flex-wrap items-center justify-between gap-3" role="status">
          <span>{success}</span>
          <Btn onClick={() => setDismissed(true)} size="sm" tone="ghost">Dismiss</Btn>
        </div>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader icon={Mail} title="SMTP configuration" />
          <DataState
            emptyMessage="The panel returned an empty settings document."
            emptyTitle="Nothing stored yet"
            isEmpty={Boolean(loaded) && !dirty && fields.smtpHost === ""}
            loadingLabel="Reading mail settings…"
            onRetry={() => void settingsQuery.refetch()}
            state={sourceState(settingsQuery)}
          >
            <div className="space-y-4 p-4">
              <div className="grid gap-3 sm:grid-cols-2">
                <div>
                  <AdminSelect
                    label="Driver"
                    onChange={(value) => setField("driver", value)}
                    options={[{ label: "smtp — send through the host below", value: "smtp" }, { label: "log — write to the host log, send nothing", value: "log" }]}
                    value={fields.driver}
                  />
                  {fields.driver === "log" ? (
                    <p className="mt-1 text-[11px] leading-5 text-warn" role="status">
                      Driver is <code className="font-mono">log</code>: outbound SMTP is bypassed and no message leaves this host, whatever the fields below say. Skip validation, and mail-dependent flows will appear to succeed without delivering.
                    </p>
                  ) : null}
                </div>
                <AdminSelect
                  label="Encryption"
                  onChange={(value) => setField("smtpEncryption", value)}
                  options={[{ label: "None", value: "none" }, { label: "STARTTLS", value: "tls" }, { label: "Implicit TLS (SMTPS)", value: "ssl" }]}
                  value={fields.smtpEncryption}
                />
              </div>
              <div className="grid gap-3 sm:grid-cols-3">
                <div className="sm:col-span-2">
                  <Input label="SMTP host" onChange={(value) => setField("smtpHost", value)} placeholder="smtp.example.com" required value={fields.smtpHost} />
                  {!fields.smtpHost.trim() ? <p className="mt-1 text-[11px] text-warn">Required by the server before saving.</p> : null}
                </div>
                <Input label="Port" onChange={(value) => setField("smtpPort", value)} type="number" value={fields.smtpPort} />
              </div>
              {portInvalid ? <p className="text-[11px] text-warn" role="alert">Port must be a whole number between 1 and 65535.</p> : null}
              <div className="grid gap-3 sm:grid-cols-2">
                <Input autoComplete="off" label="Username" onChange={(value) => setField("smtpUsername", value)} value={fields.smtpUsername} />
                <Input autoComplete="new-password" label="Password" onChange={(value) => setField("smtpPassword", value)} placeholder={passwordIsStored ? "stored — leave as-is to keep it" : "not set"} type="password" value={fields.smtpPassword} />
                <Input label="From address" onChange={(value) => setField("mailFromAddress", value)} required type="email" value={fields.mailFromAddress} />
                <Input label="From name" onChange={(value) => setField("mailFromName", value)} value={fields.mailFromName} />
              </div>
              <div className="flex flex-wrap items-center justify-end gap-2">
                <span className="text-[11px] text-text-muted">{dirty ? "Unsaved changes" : "Matches the stored settings"}</span>
                <Btn disabled={!canSave || saveMut.isPending || !dirty} loading={saveMut.isPending} onClick={() => saveMut.mutate(toPayload(fields))}>
                  Save settings
                </Btn>
                <Btn onClick={reloadAll} tone="ghost">Reload</Btn>
              </div>
            </div>
          </DataState>
        </Card>

        <div className="space-y-6">
          <Card>
            <CardHeader icon={Send} title="Test delivery" />
            <div className="space-y-3 p-4">
              <div className="flex flex-wrap items-end gap-2">
                <div className="min-w-48 flex-1">
                  <Input label="Recipient" onChange={setTestRecipient} placeholder="operator@example.com" type="email" value={testRecipient} />
                </div>
                <Btn disabled={testMut.isPending || !testRecipient.trim() || smtpIncomplete} loading={testMut.isPending} onClick={() => testMut.mutate(testRecipient.trim())} title={smtpIncomplete ? "Set an SMTP host first, or switch the driver to log" : undefined}>
                  Send test
                </Btn>
              </div>
              <p className="text-[11px] leading-5 text-text-muted">
                {fields.driver === "log"
                  ? "Driver is log: this records the message in the host log and delivers nothing."
                  : "Queues the message through the configured sender. The response reports what the queue accepted, not what the provider delivered."}
              </p>
              {testMut.isSuccess ? <p className="text-[11px] text-info" role="status">Queued for {testRecipient.trim()} — delivery is not confirmed by this response.</p> : null}
            </div>
          </Card>

          <Card>
            <CardHeader icon={Zap} title="Mail triggers" />
            <div className="p-4">
              <DataState
                emptyMessage="No mail triggers are registered with the control plane."
                emptyTitle="No triggers"
                isEmpty={triggers.length === 0}
                loadingLabel="Loading triggers…"
                onRetry={() => void triggersQuery.refetch()}
                state={sourceState(triggersQuery)}
              >
                {triggersError ? <p className="mb-3 text-[11px] leading-5 text-warn" role="alert">The trigger list is only as current as the last successful read.</p> : null}
                <div className="grid gap-2">
                  {triggers.map((t) => (
                    <div className="flex items-center justify-between gap-3 rounded-lg border border-line bg-overlay-subtle px-3 py-2 text-xs" key={t.event}>
                      <div className="min-w-0">
                        <p className="font-semibold text-text">{t.label}</p>
                        <p className="truncate font-mono text-text-subtle">{t.event} → {t.template}</p>
                      </div>
                      <Truck aria-hidden="true" className="shrink-0 text-text-muted" size={14} />
                    </div>
                  ))}
                </div>
              </DataState>
            </div>
          </Card>
        </div>
      </div>

      {settingsQuery.isSuccess && !loaded ? <EmptyState icon={Mail} title="Empty settings document" message="The read succeeded but returned no fields, so nothing below is known to be configured." /> : null}
    </AdminPageLayout>
  );
}
