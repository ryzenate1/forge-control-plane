import { cn } from "@/lib/utils";

/**
 * ForgeLogo — the canonical Forge brand mark.
 *
 * Signature: a heavy "F" letterform in brand red on a cold-steel raised tile,
 * paired with the "Forge" wordmark set in Manrope (inherits --font-sans from
 * the body). The mark uses the UI sans stack via var(--font-sans) — no
 * Georgia/Times serif fallback — to keep the two-font rule (Manrope +
 * JetBrains Mono) intact.
 *
 * Props:
 * - className: extra classes for the wrapper (flex alignment etc.)
 * - size: mark tile size in px (default 32); wordmark scales relative to it
 * - withWordmark: render the "Forge" wordmark (default true)
 *
 * Colors come exclusively from design tokens (var(--*)) — no hardcoded hex.
 */
export function ForgeLogo({
  className,
  size = 32,
  withWordmark = true,
}: {
  className?: string;
  size?: number;
  withWordmark?: boolean;
}) {
  return (
    <span className={cn("inline-flex items-center gap-2.5", className)}>
      <span
        aria-hidden="true"
        className="grid shrink-0 place-items-center rounded-xl border border-[var(--line)] bg-[var(--surface-raised)] text-[var(--brand)]"
        style={{
          width: size,
          height: size,
          fontSize: size * 0.6,
          fontFamily: "var(--font-sans), system-ui, sans-serif",
          fontWeight: 800,
          lineHeight: 1,
          paddingTop: size * 0.02,
        }}
      >
        F
      </span>
      {withWordmark ? (
        <span
          className="font-semibold tracking-tight text-[var(--text)]"
          style={{ fontSize: Math.max(size * 0.5, 14) }}
        >
          Forge
        </span>
      ) : null}
    </span>
  );
}
