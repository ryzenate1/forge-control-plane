"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Eye, EyeOff, KeyRound, ShieldCheck } from "lucide-react";
import { fetchSetupStatus, login, loginCheckpoint, type LoginResponse } from "@/lib/api";
import { useServerStore } from "@/stores/use-server-store";
import { AuthShell } from "@/components/ui/auth-shell";
import { Alert, Button, Field, Input } from "@/components/ui/primitives";
import { safeRedirectPath } from "@/components/ui/auth-utils";

function LoginContent() {
  const router = useRouter();
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

  const setupQuery = useQuery({ queryKey: ["setup-status"], queryFn: fetchSetupStatus, retry: 1, staleTime: 60_000 });
  useEffect(() => { if (currentUser && !setupQuery.data?.required) { router.replace(currentUser.role === "admin" ? "/admin/overview" : "/servers"); } }, [currentUser, router, setupQuery.data]);
  useEffect(() => { if (setupQuery.data?.required) router.replace("/setup"); }, [router, setupQuery.data]);
  useEffect(() => { if (cooldown <= 0) return; const timer = window.setInterval(() => setCooldown((value) => Math.max(0, value - 1)), 1000); return () => window.clearInterval(timer); }, [cooldown]);

  function finishLogin(data: LoginResponse) {
      if (!data.complete || !data.user) { setErrors({ form: "The sign-in response was incomplete. Please try again." }); return; }
      // No token needed - HttpOnly cookie is set by server
      setCurrentUser(data.user); qc.setQueryData(["current-user"], data.user);
      const requested = safeRedirectPath(params.get("next"));
      router.replace(requested || (data.user.role === "admin" ? "/admin/overview" : "/servers"));
    }

  function mutationError(error: unknown, fallback: string) {
    const message = error instanceof Error ? error.message : fallback;
    if (/too many/i.test(message)) setCooldown(30);
    setErrors({ form: message });
  }

  const loginMutation = useMutation({ mutationFn: () => login(email.trim().toLowerCase(), password), onSuccess: (data) => { if (!data.complete && data.confirmationToken) { setCheckpointToken(data.confirmationToken); setPassword(""); setErrors({}); return; } finishLogin(data); }, onError: (error) => mutationError(error, "Unable to sign in.") });
  const checkpointMutation = useMutation({ mutationFn: () => isRecovery ? loginCheckpoint(checkpointToken!, undefined, code.trim()) : loginCheckpoint(checkpointToken!, code.trim()), onSuccess: finishLogin, onError: (error) => mutationError(error, "Unable to verify your code.") });

  if (setupQuery.isPending) return <AuthShell title="Preparing sign in" description="Verifying that the panel is ready."><div className="ui-card p-6 text-sm text-slate-400" role="status">Checking panel status…</div></AuthShell>;
  if (setupQuery.isError) return <AuthShell title="Panel unavailable" description="Sign in is paused until setup status can be verified."><Alert actions={<Button loading={setupQuery.isFetching} onClick={() => void setupQuery.refetch()} variant="secondary">Retry</Button>} title="Unable to reach the panel API" tone="error">An outage is not treated as a configured panel. Check the API connection and retry.</Alert></AuthShell>;
  if (setupQuery.data?.required) return <AuthShell title="Opening setup" description="This panel still needs its first administrator."><div className="ui-card p-6 text-sm text-slate-400" role="status">Redirecting to first-run setup…</div></AuthShell>;

  if (checkpointToken) return <AuthShell eyebrow="Security checkpoint" title="Verify it’s you" description={isRecovery ? "Enter one unused recovery code. It can only be used once." : "Enter the six-digit code from your authenticator app."} footer={<button className="font-medium text-slate-300 hover:text-white" onClick={() => { setCheckpointToken(null); setCode(""); setErrors({}); }} type="button">Return to sign in</button>}>
    <form className="ui-card space-y-5 p-5 sm:p-6" noValidate onSubmit={(event) => { event.preventDefault(); setErrors({}); const normalized = code.trim(); if ((!isRecovery && !/^\d{6}$/.test(normalized)) || (isRecovery && !normalized)) { setErrors({ form: isRecovery ? "Enter a recovery code." : "Enter the six-digit authentication code." }); return; } checkpointMutation.mutate(); }}>
      <div className="flex items-center gap-3 rounded-lg border border-white/[0.07] bg-white/[0.025] p-3"><span className="grid h-9 w-9 place-items-center rounded-lg bg-emerald-500/10 text-emerald-400">{isRecovery ? <KeyRound className="h-5 w-5" /> : <ShieldCheck className="h-5 w-5" />}</span><div><p className="text-sm font-semibold text-slate-200">{isRecovery ? "Recovery code" : "Authenticator code"}</p><p className="text-xs text-slate-500">{isRecovery ? "Use a saved backup code" : "Time-based one-time password"}</p></div></div>
      <Field id="checkpoint-code" label={isRecovery ? "Recovery code" : "Authentication code"}><Input autoComplete="one-time-code" autoFocus id="checkpoint-code" inputMode={isRecovery ? "text" : "numeric"} maxLength={isRecovery ? 128 : 6} onChange={(event) => setCode(isRecovery ? event.target.value : event.target.value.replace(/\D/g, ""))} placeholder={isRecovery ? "xxxx-xxxx-xxxx" : "000000"} value={code} className={isRecovery ? "font-mono" : "text-center font-mono text-lg tracking-[0.35em]"} /></Field>
      {errors.form ? <Alert tone={cooldown ? "warning" : "error"}>{errors.form}{cooldown ? ` Try again in ${cooldown} seconds.` : ""}</Alert> : null}
      <Button className="w-full" disabled={cooldown > 0 || !code.trim()} loading={checkpointMutation.isPending} type="submit">Verify and continue</Button>
      <button className="w-full text-sm font-medium text-slate-400 hover:text-white" onClick={() => { setIsRecovery((value) => !value); setCode(""); setErrors({}); }} type="button">{isRecovery ? "Use an authenticator code instead" : "Use a recovery code instead"}</button>
    </form>
  </AuthShell>;

  return <AuthShell eyebrow="Welcome back" title="Sign in to your account" description="Use your administrator or server account credentials to continue." footer={<>Need access help? Contact your panel administrator.</>}>
    <form className="ui-card space-y-5 p-5 sm:p-6" noValidate onSubmit={(event) => { event.preventDefault(); const nextErrors: typeof errors = {}; if (!/^\S+@\S+\.\S+$/.test(email.trim())) nextErrors.email = "Enter a valid email address."; if (!password) nextErrors.password = "Enter your password."; setErrors(nextErrors); if (!nextErrors.email && !nextErrors.password) loginMutation.mutate(); }}>
      {params.get("setup") === "complete" ? <Alert tone="success" title="Administrator created">Setup is complete. Sign in with the credentials you just created.</Alert> : null}
      {params.get("reason") === "session-expired" ? <Alert tone="warning" title="Your session expired">Sign in again to return to your work.</Alert> : null}
      <Field error={errors.email} id="email" label="Email address"><Input aria-describedby={errors.email ? "email-error" : undefined} autoComplete="email" autoFocus id="email" invalid={Boolean(errors.email)} onChange={(event) => { setEmail(event.target.value); if (errors.email) setErrors((value) => ({ ...value, email: undefined })); }} placeholder="you@example.com" type="email" value={email} /></Field>
      <Field error={errors.password} id="password" label="Password"><div className="relative"><Input aria-describedby={errors.password ? "password-error" : undefined} autoComplete="current-password" className="pr-11" id="password" invalid={Boolean(errors.password)} onChange={(event) => { setPassword(event.target.value); if (errors.password) setErrors((value) => ({ ...value, password: undefined })); }} placeholder="Enter your password" type={showPassword ? "text" : "password"} value={password} /><button aria-label={showPassword ? "Hide password" : "Show password"} aria-pressed={showPassword} className="ui-icon-button absolute right-1 top-1" onClick={() => setShowPassword((value) => !value)} type="button">{showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}</button></div></Field>
      <div className="flex justify-end"><Link className="text-sm font-medium text-slate-400 hover:text-white" href="/forgot-password">Forgot password?</Link></div>
      {errors.form ? <Alert tone={cooldown ? "warning" : "error"}>{errors.form}{cooldown ? ` Try again in ${cooldown} seconds.` : ""}</Alert> : null}
      <Button className="w-full" disabled={cooldown > 0} loading={loginMutation.isPending} type="submit">{cooldown ? `Try again in ${cooldown}s` : "Sign in"}</Button>
    </form>
  </AuthShell>;
}

export default function LoginPage() { return <Suspense fallback={<AuthShell title="Preparing sign in" description="Loading the secure sign-in form."><div className="ui-card p-6 text-sm text-slate-400">Loading…</div></AuthShell>}><LoginContent /></Suspense>; }
