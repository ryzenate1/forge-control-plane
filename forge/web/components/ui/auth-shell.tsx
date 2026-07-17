"use client";
/* eslint-disable @next/next/no-img-element -- branding URLs are runtime settings and cannot be statically optimized */

import { Gamepad2, ShieldCheck } from "lucide-react";
import { useBranding } from "@/components/branding";
import type { ReactNode } from "react";

export function AuthShell({ eyebrow, title, description, children, footer }: { eyebrow?: string; title: string; description: string; children: ReactNode; footer?: ReactNode }) {
  const { companyName, footerText, logoUrl, loginBackgroundUrl } = useBranding();
  return <main className="auth-shell" style={loginBackgroundUrl ? { backgroundImage: `linear-gradient(120deg, rgba(7, 10, 16, .94), rgba(7, 10, 16, .79)), url(\"${loginBackgroundUrl.replace(/["\\\n\r]/g, "")}\")` } : undefined}>
    <div className="auth-grid">
      <section className="auth-brand-panel" aria-label={`${companyName} introduction`}>
        <div className="auth-brand-mark">{logoUrl ? <img alt={`${companyName} logo`} className="max-h-12 max-w-52 object-contain object-left" src={logoUrl} /> : <><span className="grid h-11 w-11 place-items-center rounded-xl bg-red-600 text-white shadow-lg shadow-red-950/40"><Gamepad2 className="h-6 w-6" /></span><span className="text-xl font-bold text-white">{companyName}</span></>}</div>
        <div className="max-w-lg"><div className="mb-5 inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.04] px-3 py-1.5 text-xs font-medium text-slate-300"><ShieldCheck className="h-4 w-4 text-red-400" />Secure control plane</div><h2 className="text-4xl font-bold leading-tight tracking-tight text-white lg:text-5xl">Your infrastructure,<br /><span className="text-red-400">under control.</span></h2><p className="mt-5 max-w-md text-base leading-7 text-slate-400">Manage servers and account security from a focused, responsive workspace.</p></div>
        <p className="text-xs text-slate-500">{footerText || `© ${new Date().getFullYear()} ${companyName}`}</p>
      </section>
      <section className="auth-form-panel"><div className="w-full max-w-md"><div className="mb-7 lg:hidden"><div className="auth-brand-mark">{logoUrl ? <img alt={`${companyName} logo`} className="max-h-10 max-w-48 object-contain object-left" src={logoUrl} /> : <><span className="grid h-10 w-10 place-items-center rounded-xl bg-red-600"><Gamepad2 className="h-5 w-5" /></span><span className="text-lg font-bold text-white">{companyName}</span></>}</div></div>{eyebrow ? <p className="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-red-400">{eyebrow}</p> : null}<h1 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">{title}</h1><p className="mt-2 text-sm leading-6 text-slate-400">{description}</p><div className="mt-7">{children}</div>{footer ? <div className="mt-6 text-center text-sm text-slate-400">{footer}</div> : null}</div></section>
    </div>
  </main>;
}
