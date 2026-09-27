"use client";
/* eslint-disable @next/next/no-img-element -- branding URLs are runtime settings and cannot be statically optimized */

import { ShieldCheck } from "lucide-react";
import { useBranding } from "@/components/branding";
import { ForgeLogo } from "@/components/ui/logo";
import type { ReactNode } from "react";

export function AuthShell({ eyebrow, title, description, children, footer }: { eyebrow?: string; title: string; description: string; children: ReactNode; footer?: ReactNode }) {
  const { companyName, footerText, logoUrl, loginBackgroundUrl } = useBranding();
  return <main className="auth-shell" style={loginBackgroundUrl ? { backgroundImage: `linear-gradient(120deg, rgba(7, 10, 16, .94), rgba(7, 10, 16, .79)), url(\"${loginBackgroundUrl.replace(/["\\\n\r]/g, "")}\")` } : undefined}>
    <div className="auth-grid">
      <section className="auth-brand-panel" aria-label={`${companyName} introduction`}>
        <div className="auth-brand-mark">{logoUrl ? <img alt={`${companyName} logo`} className="max-h-12 max-w-52 object-contain object-left" src={logoUrl} /> : <ForgeLogo size={44} />}</div>
        <div className="max-w-lg"><div className="mb-5 inline-flex items-center gap-2 rounded-full border border-[var(--line)] bg-[var(--overlay-subtle)] px-3 py-1.5 text-xs font-medium text-[var(--text)]"><ShieldCheck className="h-4 w-4 text-[var(--brand-hover)]" />Secure control plane</div><h2 className="text-4xl font-bold leading-tight tracking-tight text-[var(--text)] lg:text-5xl">Your infrastructure,<br /><span className="text-[var(--brand-hover)]">under control.</span></h2><p className="mt-5 max-w-md text-base leading-7 text-[var(--text-subtle)]">Manage servers and account security from a focused, responsive workspace.</p></div>
        <p className="text-xs text-[var(--text-muted)]">{footerText || `© ${new Date().getFullYear()} ${companyName}`}</p>
      </section>
      <section className="auth-form-panel"><div className="w-full max-w-md"><div className="mb-7 lg:hidden"><div className="auth-brand-mark">{logoUrl ? <img alt={`${companyName} logo`} className="max-h-10 max-w-48 object-contain object-left" src={logoUrl} /> : <ForgeLogo size={40} />}</div></div>{eyebrow ? <p className="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-[var(--brand-hover)]">{eyebrow}</p> : null}<h1 className="text-2xl font-bold tracking-tight text-[var(--text)] sm:text-3xl">{title}</h1><p className="mt-2 text-sm leading-6 text-[var(--text-subtle)]">{description}</p><div className="mt-7">{children}</div>{footer ? <div className="mt-6 text-center text-sm text-[var(--text-subtle)]">{footer}</div> : null}</div></section>
    </div>
  </main>;
}
