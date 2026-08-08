"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Eye, EyeOff, KeyRound, ShieldCheck } from "lucide-react";
import { login, loginCheckpoint, fetchSetupStatus, type LoginResponse } from "@/lib/api";
import { useServerStore } from "@/stores/use-server-store";
import { AuthShell } from "@/components/ui/auth-shell";
import { Alert, Button, Field, Input } from "@/components/ui/primitives";
import { safeRedirectPath } from "@/components/ui/auth-utils";
import { useT } from "@/components/TranslationProvider";

type SetupStatus = "ready" | "required" | "unreachable";

function LoginContent() {
  const t = useT();
  const router = useRouter();
  const replaceRoute = router.replace;
  const params = useSearchParams();
  const qc = useQueryClient();
  const { currentUser, setCurrentUser } = useServerStore();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [errors, setErrors] = useState<{ email?: string; password?: string; form?: string }>({});
  const [checkpointToken, setCheckpointToken] = useState<string | null>(null);
  const [code, setCode] = useState("");
  const [isRecovery, setIsRecovery] = useState(false);
  const [cooldown, setCooldown] = useState(0);
  const [setupStatus, setSetupStatus] = useState<SetupStatus>("ready");

  useEffect(() => {
    let cancelled = false;
    fetchSetupStatus()
      .then((data) => {
        if (cancelled) return;
        if (data.required) {
          setSetupStatus("required");
          replaceRoute("/setup");
        } else {
          setSetupStatus("ready");
        }
      })
      .catch((err) => {
        if (cancelled) return;
        console.error("setup fetch error:", err);
        setSetupStatus("unreachable");
      });
    return () => { cancelled = true; };
  }, [replaceRoute]);

  useEffect(() => { if (currentUser && setupStatus === "ready") { replaceRoute(currentUser.role === "admin" ? "/admin/overview" : "/servers"); } }, [currentUser, replaceRoute, setupStatus]);
  useEffect(() => { if (cooldown <= 0) return; const timer = window.setInterval(() => setCooldown((value) => Math.max(0, value - 1)), 1000); return () => window.clearInterval(timer); }, [cooldown]);

  function finishLogin(data: LoginResponse) {
      if (!data.complete || !data.user) { setErrors({ form: t("auth.incompleteResponse") }); return; }
      setCurrentUser(data.user); qc.setQueryData(["current-user"], data.user);
      const requested = safeRedirectPath(params.get("next"));
      replaceRoute(requested || (data.user.role === "admin" ? "/admin/overview" : "/servers"));
    }

  function mutationError(error: unknown, fallback: string) {
    const message = error instanceof Error ? error.message : fallback;
    if (/too many/i.test(message)) setCooldown(30);
    setErrors({ form: message });
  }

  const loginMutation = useMutation({ mutationFn: () => login(email.trim().toLowerCase(), password), onSuccess: (data) => { if (!data.complete && data.confirmationToken) { setCheckpointToken(data.confirmationToken); setPassword(""); setErrors({}); return; } finishLogin(data); }, onError: (error) => mutationError(error, t("auth.unableToSignIn")) });
  const checkpointMutation = useMutation({ mutationFn: () => isRecovery ? loginCheckpoint(checkpointToken!, undefined, code.trim()) : loginCheckpoint(checkpointToken!, code.trim()), onSuccess: finishLogin, onError: (error) => mutationError(error, t("auth.unableToVerifyCode")) });

  if (checkpointToken) return <AuthShell eyebrow={t("auth.securityCheckpoint")} title={t("auth.verifyItsYou")} description={isRecovery ? t("auth.enterUnusedRecoveryCode") : t("auth.enterSixDigitCode")} footer={<button className="font-medium text-slate-300 hover:text-white" onClick={() => { setCheckpointToken(null); setCode(""); setErrors({}); }} type="button">{t("auth.returnToSignIn")}</button>}>
    <form className="ui-card space-y-5 p-5 sm:p-6" noValidate onSubmit={(event) => { event.preventDefault(); setErrors({}); const normalized = code.trim(); if ((!isRecovery && !/^\d{6}$/.test(normalized)) || (isRecovery && !normalized)) { setErrors({ form: isRecovery ? t("auth.enterRecoveryCode") : t("auth.enterSixDigitAuthCode") }); return; } checkpointMutation.mutate(); }}>
      <div className="flex items-center gap-3 rounded-lg border border-white/[0.07] bg-white/[0.025] p-3"><span className="grid h-9 w-9 place-items-center rounded-lg bg-emerald-500/10 text-emerald-400">{isRecovery ? <KeyRound className="h-5 w-5" /> : <ShieldCheck className="h-5 w-5" />}</span><div><p className="text-sm font-semibold text-slate-200">{isRecovery ? t("auth.recoveryCode") : t("auth.authenticatorCode")}</p><p className="text-xs text-slate-500">{isRecovery ? t("auth.useSavedBackupCode") : t("auth.timeBasedOtp")}</p></div></div>
      <Field id="checkpoint-code" label={isRecovery ? t("auth.recoveryCode") : t("auth.twoFactorCode")}><Input autoComplete="one-time-code" autoFocus id="checkpoint-code" inputMode={isRecovery ? "text" : "numeric"} maxLength={isRecovery ? 128 : 6} onChange={(event) => setCode(isRecovery ? event.target.value : event.target.value.replace(/\D/g, ""))} placeholder={isRecovery ? "xxxx-xxxx-xxxx" : "000000"} value={code} className={isRecovery ? "font-mono" : "text-center font-mono text-lg tracking-[0.35em]"} /></Field>
      {errors.form ? <Alert tone={cooldown ? "warning" : "error"}>{errors.form}{cooldown ? " " + t("common.tryAgainIn", { seconds: cooldown }) : ""}</Alert> : null}
      <Button className="w-full" disabled={cooldown > 0 || !code.trim()} loading={checkpointMutation.isPending} type="submit">{t("auth.verifyAndContinue")}</Button>
      <button className="w-full text-sm font-medium text-slate-400 hover:text-white" onClick={() => { setIsRecovery((value) => !value); setCode(""); setErrors({}); }} type="button">{isRecovery ? t("auth.useAuthenticatorInstead") : t("auth.useRecoveryCodeInstead")}</button>
    </form>
  </AuthShell>;

  return <AuthShell eyebrow={t("auth.welcomeBack")} title={t("auth.login")} description={t("auth.useCredentialsToContinue")} footer={<>{t("auth.needAccessHelp")}</>}>
    <form className="ui-card space-y-5 p-5 sm:p-6" noValidate onSubmit={(event) => { event.preventDefault(); const nextErrors: typeof errors = {}; if (!/^\S+@\S+\.\S+$/.test(email.trim())) nextErrors.email = t("auth.enterValidEmail"); if (!password) nextErrors.password = t("auth.enterYourPassword"); setErrors(nextErrors); if (!nextErrors.email && !nextErrors.password) loginMutation.mutate(); }}>
      {setupStatus === "unreachable" ? <Alert actions={<Button onClick={() => { setSetupStatus("ready"); }} variant="secondary">{t("common.retry")}</Button>} className="mb-4" title={t("auth.unableToReachApi")} tone="warning">{t("auth.outageNotConfigured")}</Alert> : null}
      {params.get("setup") === "complete" ? <Alert tone="success" title={t("auth.administratorCreated")}>{t("auth.setupCompleteSignIn")}</Alert> : null}
      {params.get("reason") === "session-expired" ? <Alert tone="warning" title={t("auth.sessionExpiredTitle")}>{t("auth.signInAgainToContinue")}</Alert> : null}
      <Field error={errors.email} id="email" label={t("auth.email")}><Input aria-describedby={errors.email ? "email-error" : undefined} autoComplete="email" autoFocus id="email" invalid={Boolean(errors.email)} onChange={(event) => { setEmail(event.target.value); if (errors.email) setErrors((value) => ({ ...value, email: undefined })); }} placeholder="you@example.com" type="email" value={email} /></Field>
      <Field error={errors.password} id="password" label={t("auth.password")}><div className="relative"><Input aria-describedby={errors.password ? "password-error" : undefined} autoComplete="current-password" className="pr-11" id="password" invalid={Boolean(errors.password)} onChange={(event) => { setPassword(event.target.value); if (errors.password) setErrors((value) => ({ ...value, password: undefined })); }} placeholder={t("auth.enterYourPasswordPlaceholder")} type={showPassword ? "text" : "password"} value={password} /><button aria-label={showPassword ? t("auth.hidePassword") : t("auth.showPassword")} aria-pressed={showPassword} className="ui-icon-button absolute right-1 top-1" onClick={() => setShowPassword((value) => !value)} type="button">{showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}</button></div></Field>
      <div className="flex justify-end"><Link className="text-sm font-medium text-slate-400 hover:text-white" href="/forgot-password">{t("auth.forgotPassword")}</Link></div>
      {errors.form ? <Alert tone={cooldown ? "warning" : "error"}>{errors.form}{cooldown ? " " + t("common.tryAgainIn", { seconds: cooldown }) : ""}</Alert> : null}
      <Button className="w-full" disabled={cooldown > 0} loading={loginMutation.isPending} type="submit">{cooldown ? t("common.tryAgainIn", { seconds: cooldown }) : t("auth.login")}</Button>
    </form>
  </AuthShell>;
}

function LoginFallback() {
  const t = useT();
  return <AuthShell title={t("auth.preparingSignIn")} description={t("auth.loadingSecureForm")}><div className="ui-card p-6 text-sm text-slate-400">{t("common.loading")}</div></AuthShell>;
}

export default function LoginPage() { return <Suspense fallback={<LoginFallback />}><LoginContent /></Suspense>; }
