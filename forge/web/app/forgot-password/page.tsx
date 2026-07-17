"use client";

import Link from "next/link";
import { useState } from "react";
import { MailCheck } from "lucide-react";
import { requestPasswordReset } from "@/lib/api";
import { AuthShell } from "@/components/ui/auth-shell";
import { Alert, Button, Field, Input } from "@/components/ui/primitives";

export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState(false);

  async function submit(event: React.FormEvent) {
    event.preventDefault(); setError(null);
    if (!/^\S+@\S+\.\S+$/.test(email.trim())) { setError("Enter a valid email address."); return; }
    setLoading(true);
    try { await requestPasswordReset(email.trim().toLowerCase()); setSent(true); } catch (caught) { setError(caught instanceof Error ? caught.message : "The reset request could not be sent."); } finally { setLoading(false); }
  }

  return <AuthShell eyebrow="Account recovery" title={sent ? "Check your inbox" : "Reset your password"} description={sent ? "If an account matches that address, password reset instructions have been queued." : "Enter your account email and we’ll send a single-use reset link."} footer={<Link className="font-medium text-slate-300 hover:text-white" href="/">Back to sign in</Link>}>
    {sent ? <div className="ui-card p-6 text-center"><span className="mx-auto grid h-14 w-14 place-items-center rounded-full bg-emerald-500/10 text-emerald-400"><MailCheck className="h-7 w-7" /></span><p className="mt-4 text-sm leading-6 text-slate-400">For privacy, the response is the same whether or not <strong className="font-medium text-slate-200">{email.trim().toLowerCase()}</strong> belongs to an account. Reset links expire after 30 minutes.</p><Button className="mt-6 w-full" onClick={() => { setSent(false); setError(null); }} variant="secondary">Send another request</Button></div> : <form className="ui-card space-y-5 p-5 sm:p-6" noValidate onSubmit={submit}><Field error={error || undefined} hint="Mail delivery must be configured by a panel administrator." id="recovery-email" label="Email address"><Input aria-describedby={error ? "recovery-email-error" : "recovery-email-hint"} autoComplete="email" autoFocus id="recovery-email" invalid={Boolean(error)} onChange={(event) => { setEmail(event.target.value); setError(null); }} placeholder="you@example.com" type="email" value={email} /></Field>{error ? <Alert tone="error" title="Reset email not sent">{error}</Alert> : null}<Button className="w-full" loading={loading} type="submit">Send reset link</Button></form>}
  </AuthShell>;
}
