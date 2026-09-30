"use client";

import { Check, X } from "lucide-react";

/**
 * Client-side mirror of the backend password policy
 * (`ValidatePassword` in forge/api/internal/store/store_users.go):
 * minimum 12 characters with at least one uppercase letter, one lowercase
 * letter, one digit and one special character. The backend treats every
 * non-alphanumeric character as special, so `[^A-Za-z0-9]` matches it here.
 *
 * Rendered as a live checklist under password inputs so requirements are
 * visible while typing — never only as a server error after submit.
 */

export type PasswordRuleKey = "minLength" | "upper" | "lower" | "digit" | "special";

export type PasswordEvaluation = Record<PasswordRuleKey, boolean>;

export function evaluatePassword(password: string): PasswordEvaluation {
  return {
    minLength: password.length >= 12,
    upper: /[A-Z]/.test(password),
    lower: /[a-z]/.test(password),
    digit: /[0-9]/.test(password),
    special: /[^A-Za-z0-9]/.test(password),
  };
}

export function isPasswordValid(password: string): boolean {
  return Object.values(evaluatePassword(password)).every(Boolean);
}

/** First unmet requirement as a human-readable message, or null when valid. */
export function firstPasswordError(password: string): string | null {
  const result = evaluatePassword(password);
  if (!result.minLength) return "Password must be at least 12 characters.";
  if (!result.upper) return "Password must contain at least one uppercase letter.";
  if (!result.lower) return "Password must contain at least one lowercase letter.";
  if (!result.digit) return "Password must contain at least one digit.";
  if (!result.special) return "Password must contain at least one special character.";
  return null;
}

const RULE_LABELS: { key: PasswordRuleKey; label: string }[] = [
  { key: "minLength", label: "At least 12 characters" },
  { key: "upper", label: "One uppercase letter (A–Z)" },
  { key: "lower", label: "One lowercase letter (a–z)" },
  { key: "digit", label: "One number (0–9)" },
  { key: "special", label: "One special character (!@#…)" },
];

export function PasswordRequirements({ password }: { password: string }) {
  const result = evaluatePassword(password);
  return (
    <ul aria-live="polite" className="mt-2 space-y-1">
      {RULE_LABELS.map(({ key, label }) => {
        const met = result[key];
        return (
          <li
            key={key}
            className={`flex items-center gap-1.5 text-xs ${met ? "text-emerald-400" : "text-slate-500"}`}
          >
            {met ? <Check className="h-3.5 w-3.5 shrink-0" /> : <X className="h-3.5 w-3.5 shrink-0" />}
            <span>{label}</span>
          </li>
        );
      })}
    </ul>
  );
}
