"use client";

/**
 * Code, command and terminal surfaces.
 *
 * Every technical string in Forge — an id, a path, a command, a log line —
 * renders in JetBrains Mono on a well surface. That contrast is how an operator
 * tells "text about the system" from "text from the system".
 */

import * as React from "react";
import { Check, Copy, Download, Terminal as TerminalIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { ForgeIconButton } from "./controls";

/* -------------------------------------------------------------------------- */
/* Inline code / keys                                                         */
/* -------------------------------------------------------------------------- */

/** Inline technical token: a flag, a path fragment, an env var. */
export function ForgeCode({
  className,
  children,
  ...rest
}: React.HTMLAttributes<HTMLElement>) {
  return (
    <code className={cn("ui-code-inline", className)} {...rest}>
      {children}
    </code>
  );
}

/** A keyboard key. Use for shortcuts only, never for arbitrary labels. */
export function ForgeKbd({
  className,
  children,
  ...rest
}: React.HTMLAttributes<HTMLElement>) {
  return (
    <kbd className={cn("ui-kbd", className)} {...rest}>
      {children}
    </kbd>
  );
}

/* -------------------------------------------------------------------------- */
/* Copy button                                                               */
/* -------------------------------------------------------------------------- */

/**
 * Copy-to-clipboard control.
 *
 * Only shows the success tick when the clipboard write actually resolved. A
 * failed copy surfaces as a title on the control rather than a false success.
 */
export function ForgeCopyButton({
  value,
  label = "Copy",
  className,
}: {
  value: string;
  label?: string;
  className?: string;
}) {
  const [state, setState] = React.useState<"idle" | "copied" | "failed">("idle");
  const timerRef = React.useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  React.useEffect(() => () => clearTimeout(timerRef.current), []);

  const copy = async () => {
    clearTimeout(timerRef.current);
    try {
      await navigator.clipboard.writeText(value);
      setState("copied");
    } catch {
      setState("failed");
    }
    timerRef.current = setTimeout(() => setState("idle"), 1800);
  };

  return (
    <ForgeIconButton
      className={className}
      icon={
        state === "copied" ? (
          <Check aria-hidden="true" className="size-3.5 text-ok" />
        ) : (
          <Copy aria-hidden="true" className="size-3.5" />
        )
      }
      label={state === "copied" ? "Copied" : state === "failed" ? "Copy failed" : label}
      onClick={copy}
      size="sm"
    />
  );
}

/* -------------------------------------------------------------------------- */
/* Code block                                                                 */
/* -------------------------------------------------------------------------- */

export type ForgeCodeBlockProps = {
  /** The code. Passed as a string so copy and line numbers stay accurate. */
  code: string;
  /** Shown in the header rail — a filename, a language, or a command name. */
  filename?: React.ReactNode;
  language?: string;
  showLineNumbers?: boolean;
  /** Set false for blocks that must be read in full. */
  wrap?: boolean;
  maxHeight?: number | string;
  /** Extra header controls, placed before the copy button. */
  actions?: React.ReactNode;
  copyable?: boolean;
  className?: string;
};

export function ForgeCodeBlock({
  code,
  filename,
  language,
  showLineNumbers = false,
  wrap = false,
  maxHeight = 420,
  actions,
  copyable = true,
  className,
}: ForgeCodeBlockProps) {
  const lines = React.useMemo(() => code.replace(/\n$/, "").split("\n"), [code]);
  const hasHeader = Boolean(filename || language || actions || copyable);

  return (
    <div className={cn("overflow-hidden rounded-lg border border-line bg-surface-well", className)}>
      {hasHeader ? (
        <div className="flex items-center justify-between gap-3 border-b border-line bg-overlay-subtle px-3 py-1.5">
          <p className="min-w-0 truncate font-mono text-meta text-text-subtle">
            {filename ?? language}
          </p>
          <div className="flex shrink-0 items-center gap-1">
            {actions}
            {copyable ? <ForgeCopyButton label="Copy code" value={code} /> : null}
          </div>
        </div>
      ) : null}
      <div className="overflow-auto" style={maxHeight ? { maxHeight } : undefined}>
        <pre
          className={cn(
            "px-3 py-2.5 font-mono text-meta leading-relaxed text-text",
            wrap ? "whitespace-pre-wrap break-words" : "whitespace-pre"
          )}
        >
          {showLineNumbers ? (
            <code className="grid grid-cols-[auto_1fr] gap-x-3">
              {lines.map((line, index) => (
                <React.Fragment key={index}>
                  <span aria-hidden="true" className="select-none text-right text-text-muted">
                    {index + 1}
                  </span>
                  <span>{line || " "}</span>
                </React.Fragment>
              ))}
            </code>
          ) : (
            <code>{code}</code>
          )}
        </pre>
      </div>
    </div>
  );
}

/** A single shell command, presented as something to copy and run. */
export function ForgeCommand({
  command,
  /** Prompt glyph. `$` for user, `#` for root. */
  prompt = "$",
  className,
}: {
  command: string;
  prompt?: string;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 rounded-md border border-line bg-surface-well px-2.5 py-1.5",
        className
      )}
    >
      <span aria-hidden="true" className="select-none font-mono text-meta text-brand">
        {prompt}
      </span>
      <code className="min-w-0 flex-1 overflow-x-auto whitespace-pre font-mono text-meta text-text">
        {command}
      </code>
      <ForgeCopyButton label="Copy command" value={command} />
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/* Terminal / console surface                                                 */
/* -------------------------------------------------------------------------- */

/**
 * Chrome for a live console, log tail or xterm mount.
 *
 * `status` is rendered by the caller (a `ForgeStatusBadge`) so the surface
 * never has to guess at a connection state it cannot observe.
 */
export function ForgeTerminal({
  title = "Console",
  status,
  actions,
  /** Renders a download control when provided. */
  onDownload,
  /** Set when the body mounts its own scroller, e.g. xterm. */
  unscrolled = false,
  height,
  className,
  bodyClassName,
  children,
}: {
  title?: React.ReactNode;
  status?: React.ReactNode;
  actions?: React.ReactNode;
  onDownload?: () => void;
  unscrolled?: boolean;
  height?: number | string;
  className?: string;
  bodyClassName?: string;
  children?: React.ReactNode;
}) {
  return (
    <div className={cn("ui-terminal", className)}>
      <div className="flex items-center justify-between gap-3 border-b border-line bg-overlay-subtle px-3 py-2">
        <div className="flex min-w-0 items-center gap-2">
          <TerminalIcon aria-hidden="true" className="size-3.5 shrink-0 text-text-muted" />
          <p className="t-eyebrow min-w-0 truncate">{title}</p>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          {status}
          {onDownload ? (
            <ForgeIconButton
              icon={<Download aria-hidden="true" className="size-3.5" />}
              label="Download output"
              onClick={onDownload}
              size="sm"
            />
          ) : null}
          {actions}
        </div>
      </div>
      <div
        className={cn(
          "ui-terminal-body",
          unscrolled ? "overflow-hidden" : "overflow-auto",
          bodyClassName
        )}
        style={height ? { height } : undefined}
      >
        {children}
      </div>
    </div>
  );
}

export type ForgeLogLineTone = "default" | "muted" | "warn" | "danger" | "ok";

const logLineTone: Record<ForgeLogLineTone, string> = {
  default: "text-text",
  muted: "text-text-muted",
  warn: "text-warn",
  danger: "text-danger",
  ok: "text-ok",
};

/** One log row. Timestamps stay mono and dimmed so the message reads first. */
export function ForgeLogLine({
  timestamp,
  tone = "default",
  source,
  className,
  children,
}: {
  timestamp?: React.ReactNode;
  tone?: ForgeLogLineTone;
  /** Stream or component name, e.g. `stderr`, `beacon`. */
  source?: React.ReactNode;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div className={cn("flex gap-2 px-0.5 font-mono text-meta leading-relaxed", className)}>
      {timestamp ? (
        <span className="shrink-0 select-none tabular-nums text-text-muted">{timestamp}</span>
      ) : null}
      {source ? <span className="shrink-0 text-text-subtle">{source}</span> : null}
      <span className={cn("min-w-0 whitespace-pre-wrap break-words", logLineTone[tone])}>
        {children}
      </span>
    </div>
  );
}
