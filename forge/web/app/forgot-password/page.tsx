"use client";

import Link from "next/link";
import { useState } from "react";
import { MailCheck } from "lucide-react";
import { requestPasswordReset } from "@/lib/api";
import { AuthShell } from "@/components/ui/auth-shell";
import { Alert, Button, Field, Input } from "@/components/ui/primitives";
import { useT } from "@/components/TranslationProvider";

export default function ForgotPasswordPage() {
  const t = useT();
  const [email, setEmail] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState(false);

  async function submit(event: React.FormEvent) {
    event.preventDefault(); setError(null);
    if (!/^\S+@\S+\.\S+$/.test(email.trim())) { setError(t("auth.enterValidEmail")); return; }
    setLoading(true);
    try { await requestPasswordReset(email.trim().toLowerCase()); setSent(true); } catch (caught) { setError(caught instanceof Error ? caught.message : t("forgotPassword.requestFailed")); } finally { setLoading(false); }
  }

  return <AuthShell eyebrow={t("forgotPassword.eyebrow")} title={sent ? t("forgotPassword.checkInbox") : t("forgotPassword.title")} description={sent ? t("forgotPassword.checkInboxDesc") : t("forgotPassword.description")} footer={<Link className="font-medium text-slate-300 hover:text-white" href="/">{t("auth.returnToSignIn")}</Link>}>
    {sent ? <div className="ui-card p-6 text-center"><span className="mx-auto grid h-14 w-14 place-items-center rounded-full bg-emerald-500/10 text-emerald-400"><MailCheck className="h-7 w-7" /></span><p className="mt-4 text-sm leading-6 text-slate-400">{t("forgotPassword.privacyNoticePrefix")} <strong className="font-medium text-slate-200">{email.trim().toLowerCase()}</strong> {t("forgotPassword.privacyNoticeSuffix")}</p><Button className="mt-6 w-full" onClick={() => { setSent(false); setError(null); }} variant="secondary">{t("forgotPassword.sendAnother")}</Button></div> : <form className="ui-card space-y-5 p-5 sm:p-6" noValidate onSubmit={submit}><Field error={error || undefined} hint={t("forgotPassword.mailHint")} id="recovery-email" label={t("auth.email")}><Input aria-describedby={error ? "recovery-email-error" : "recovery-email-hint"} autoComplete="email" autoFocus id="recovery-email" invalid={Boolean(error)} onChange={(event) => { setEmail(event.target.value); setError(null); }} placeholder="you@example.com" type="email" value={email} /></Field>{error ? <Alert tone="error" title={t("forgotPassword.notSentTitle")}>{error}</Alert> : null}<Button className="w-full" loading={loading} type="submit">{t("forgotPassword.sendLink")}</Button></form>}
  </AuthShell>;
}
