"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";
import { CheckCircle2, Eye, EyeOff } from "lucide-react";
import { resetPassword } from "@/lib/api";
import { AuthShell } from "@/components/ui/auth-shell";
import { Alert, Button, Field, Input } from "@/components/ui/primitives";
import { useT } from "@/components/TranslationProvider";

function ResetForm() {
  const t = useT();
  const params = useSearchParams();
  const token = params.get("token")?.trim() || "";
  const email = params.get("email")?.trim() || "";
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [show, setShow] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [success, setSuccess] = useState(false);

  if (!token || !/^\S+@\S+\.\S+$/.test(email)) return <Alert title={t("resetPassword.invalidLink")} tone="error">{t(token ? "resetPassword.missingEmail" : "resetPassword.missingToken")} <Link className="font-semibold underline" href="/forgot-password">{t("resetPassword.requestNewLink")}</Link>.</Alert>;
  if (success) return <div className="ui-card p-6 text-center"><span className="mx-auto grid h-14 w-14 place-items-center rounded-full bg-emerald-500/10 text-emerald-400"><CheckCircle2 className="h-7 w-7" /></span><h2 className="mt-4 font-semibold text-white">{t("resetPassword.updated")}</h2><p className="mt-2 text-sm leading-6 text-slate-400">{t("resetPassword.updatedDesc")}</p><Link className="ui-button ui-button-primary mt-6 w-full" href="/">{t("resetPassword.continueToSignIn")}</Link></div>;

  async function submit(event: React.FormEvent) {
    event.preventDefault(); setError(null);
    if (password.length < 8) { setError(t("resetPassword.tooShort")); return; }
    if (password !== confirm) { setError(t("validation.passwordMatch")); return; }
    setLoading(true);
    try { await resetPassword(email.toLowerCase(), token, password); setSuccess(true); setPassword(""); setConfirm(""); } catch (caught) { setError(caught instanceof Error ? caught.message : t("resetPassword.resetFailed")); } finally { setLoading(false); }
  }

  return <form className="ui-card space-y-5 p-5 sm:p-6" noValidate onSubmit={submit}><div className="rounded-lg border border-white/[0.07] bg-black/10 px-3.5 py-3"><p className="text-xs text-slate-500">{t("resetPassword.resettingFor")}</p><p className="mt-1 truncate text-sm font-medium text-slate-200">{email}</p></div><Field hint={t("resetPassword.passwordHint")} id="new-password" label={t("resetPassword.newPassword")}><div className="relative"><Input autoComplete="new-password" className="pr-11" id="new-password" minLength={8} onChange={(event) => setPassword(event.target.value)} type={show ? "text" : "password"} value={password} /><button aria-label={show ? t("setupWizard.step2.hidePasswords") : t("setupWizard.step2.showPasswords")} className="ui-icon-button absolute right-1 top-1" onClick={() => setShow((value) => !value)} type="button">{show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}</button></div></Field><Field id="confirm-password" label={t("resetPassword.confirmNewPassword")}><Input autoComplete="new-password" id="confirm-password" onChange={(event) => setConfirm(event.target.value)} type={show ? "text" : "password"} value={confirm} /></Field>{error ? <Alert title={t("resetPassword.notChanged")} tone="error">{error}</Alert> : null}<Button className="w-full" loading={loading} type="submit">{t("resetPassword.submit")}</Button></form>;
}

function ResetPasswordFallback() {
  const t = useT();
  return <div className="ui-card p-6 text-sm text-slate-400" role="status">{t("resetPassword.validatingLink")}</div>;
}

export default function ResetPasswordPage() {
  const t = useT();
  return <AuthShell eyebrow={t("forgotPassword.eyebrow")} title={t("resetPassword.chooseNewPassword")} description={t("resetPassword.linkExpiry")} footer={<Link className="font-medium text-slate-300 hover:text-white" href="/">{t("auth.returnToSignIn")}</Link>}><Suspense fallback={<ResetPasswordFallback />}><ResetForm /></Suspense></AuthShell>;
}
