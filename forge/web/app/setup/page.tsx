"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Check, Database, Eye, EyeOff, ShieldCheck } from "lucide-react";
import { fetchSetupStatus, runSetup } from "@/modules/platform/api";
import { AuthShell } from "@/components/ui/auth-shell";
import { Alert, Button, Field, Input } from "@/components/ui/primitives";

export default function SetupPage() {
  const router = useRouter();
  const [step, setStep] = useState<1 | 2 | 3>(1);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [errors, setErrors] = useState<{ email?: string; password?: string; confirm?: string; form?: string }>({});
  const statusQuery = useQuery({ queryKey: ["setup-status"], queryFn: fetchSetupStatus, retry: false });
  useEffect(() => { if (statusQuery.data && !statusQuery.data.required && step !== 3) router.replace("/"); }, [router, statusQuery.data, step]);
  const setupMutation = useMutation({ mutationFn: () => runSetup({ email: email.trim().toLowerCase(), password }), onSuccess: () => setStep(3), onError: (error) => setErrors({ form: error instanceof Error ? error.message : "Setup could not be completed." }) });

  if (statusQuery.isPending) return <AuthShell eyebrow="First-run setup" title="Checking readiness" description="Connecting to the panel API before setup begins."><div className="ui-card p-6 text-sm text-slate-400" role="status">Verifying environment…</div></AuthShell>;
  if (statusQuery.isError) return <AuthShell eyebrow="First-run setup" title="Readiness check failed" description="Setup remains locked until the API confirms its state."><Alert actions={<Button loading={statusQuery.isFetching} onClick={() => void statusQuery.refetch()} variant="secondary">Retry</Button>} title="Unable to verify setup status" tone="error">No setup state has been assumed. Confirm the API and database services are reachable, then retry.</Alert></AuthShell>;
  if (!statusQuery.data?.required && step !== 3) return <AuthShell title="Setup already complete" description="This panel already has an administrator."><div className="ui-card p-6 text-sm text-slate-400" role="status">Returning to sign in…</div></AuthShell>;

  return <AuthShell eyebrow="First-run setup" title={step === 1 ? "Panel readiness" : step === 2 ? "Create the first administrator" : "Setup complete"} description={step === 1 ? "Review the state reported by the setup API before creating an account." : step === 2 ? "These credentials will have full administrative access to the panel." : "Your administrator account is ready. Sign in to start configuring the panel."}>
    <ol aria-label="Setup progress" className="mb-5 grid grid-cols-2 gap-2 text-xs">
      {[{ n: 1, label: "Readiness" }, { n: 2, label: "Administrator" }].map((item) => <li aria-current={step === item.n ? "step" : undefined} className={`flex items-center gap-2 rounded-lg border px-3 py-2 ${step >= item.n ? "border-red-500/30 bg-red-500/10 text-red-200" : "border-white/10 text-slate-500"}`} key={item.n}><span className="grid h-5 w-5 place-items-center rounded-full border border-current text-[10px]">{step > item.n ? <Check className="h-3 w-3" /> : item.n}</span>{item.label}</li>)}
    </ol>
    {step === 1 ? <div className="ui-card p-5 sm:p-6"><div className="flex items-start gap-4"><span className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-emerald-500/10 text-emerald-400"><Database className="h-5 w-5" /></span><div><h2 className="font-semibold text-slate-100">API is ready</h2><p className="mt-1 text-sm leading-6 text-slate-400">The API reports that setup is required and no administrator exists.</p><dl className="mt-4 grid grid-cols-2 gap-3 text-xs"><div className="rounded-lg bg-black/20 p-3"><dt className="text-slate-500">Version</dt><dd className="mt-1 font-mono text-slate-200">{statusQuery.data.appVersion || "Not reported"}</dd></div><div className="rounded-lg bg-black/20 p-3"><dt className="text-slate-500">Administrator</dt><dd className="mt-1 text-slate-200">{statusQuery.data.hasAdmin ? "Present" : "Not created"}</dd></div></dl></div></div><Alert className="mt-5" tone="info">This check confirms only the setup API state. It does not test optional services such as mail delivery or node connectivity.</Alert><Button className="mt-5 w-full" onClick={() => setStep(2)}>Continue</Button></div> : null}
    {step === 2 ? <form className="ui-card space-y-5 p-5 sm:p-6" noValidate onSubmit={(event) => { event.preventDefault(); const next: typeof errors = {}; if (!/^\S+@\S+\.\S+$/.test(email.trim())) next.email = "Enter a valid email address."; if (password.length < 8) next.password = "Use at least 8 characters."; else if (password === email) next.password = "Choose a password that differs from your email."; if (confirm !== password) next.confirm = "Passwords do not match."; setErrors(next); if (!next.email && !next.password && !next.confirm) setupMutation.mutate(); }}>
      <Field error={errors.email} id="setup-email" label="Administrator email"><Input autoComplete="email" autoFocus id="setup-email" invalid={Boolean(errors.email)} onChange={(event) => setEmail(event.target.value)} placeholder="admin@example.com" type="email" value={email} /></Field>
      <Field error={errors.password} hint="At least 8 characters. A longer, unique passphrase is recommended." id="setup-password" label="Password"><div className="relative"><Input autoComplete="new-password" className="pr-11" id="setup-password" invalid={Boolean(errors.password)} minLength={8} onChange={(event) => setPassword(event.target.value)} type={showPassword ? "text" : "password"} value={password} /><button aria-label={showPassword ? "Hide passwords" : "Show passwords"} className="ui-icon-button absolute right-1 top-1" onClick={() => setShowPassword((value) => !value)} type="button">{showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}</button></div></Field>
      <Field error={errors.confirm} id="setup-confirm" label="Confirm password"><Input autoComplete="new-password" id="setup-confirm" invalid={Boolean(errors.confirm)} onChange={(event) => setConfirm(event.target.value)} type={showPassword ? "text" : "password"} value={confirm} /></Field>
      {errors.form ? <Alert tone="error">{errors.form}</Alert> : null}
      <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-between"><Button onClick={() => setStep(1)} type="button" variant="ghost">Back</Button><Button loading={setupMutation.isPending} type="submit">Create administrator</Button></div>
    </form> : null}
    {step === 3 ? <div className="ui-card p-6 text-center"><span className="mx-auto grid h-14 w-14 place-items-center rounded-full bg-emerald-500/10 text-emerald-400"><ShieldCheck className="h-7 w-7" /></span><h2 className="mt-4 text-lg font-semibold text-white">Administrator created</h2><p className="mt-2 text-sm leading-6 text-slate-400">Setup does not create a browser session. Sign in with <strong className="font-medium text-slate-200">{email.trim().toLowerCase()}</strong> to continue.</p><Link className="ui-button ui-button-primary mt-6 w-full" href="/?setup=complete">Continue to sign in</Link></div> : null}
  </AuthShell>;
}
