import { cn } from "@/lib/utils";

/**
 * ForgeLogo — the canonical Forge brand mark.
 *
 * Signature: a serif "F" letterform (Georgia / 'Times New Roman' fallback for the
 * editorial serif cut) in brand red on a cold-steel raised tile, paired with the
 * "Forge" wordmark set in Manrope (inherits --font-sans from the body).
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
          fontFamily: "Georgia, 'Times New Roman', serif",
          fontWeight: 700,
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
