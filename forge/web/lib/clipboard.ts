import { toast } from "@/components/ui/sonner";

/**
 * Clipboard hygiene for secrets (API keys, node tokens, database credentials).
 *
 * copySecret(text) writes the secret to the clipboard, shows a toast, and
 * schedules it to be wiped from the clipboard after CLEAR_DELAY_MS. The wipe
 * also runs whenever the window loses focus or the tab is hidden
 * (visibilitychange), so the secret never persists in the clipboard after the
 * user leaves. Starting a new copy cancels the previous wipe timer.
 *
 * When the async Clipboard API is unavailable or denied, falls back to
 * document.execCommand("copy") with the same scheduled wipe. All writes are
 * wrapped in try/catch so a denied or unfocused write can never throw.
 *
 * Returns true when the copy succeeded so callers can flip local "copied" UI
 * state (icon/label swaps).
 */

const CLEAR_DELAY_MS = 15_000;

let clearTimer: number | null = null;

async function writeClipboard(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // Async Clipboard API unavailable or denied — fall back to execCommand.
  }
  try {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.setAttribute("readonly", "");
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    textarea.style.pointerEvents = "none";
    document.body.appendChild(textarea);
    textarea.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(textarea);
    return ok;
  } catch {
    return false;
  }
}

function clearClipboard(): void {
  if (clearTimer) {
    window.clearTimeout(clearTimer);
    clearTimer = null;
  }
  void writeClipboard("").catch(() => undefined);
}

export function copySecret(text: string): Promise<boolean> {
  if (clearTimer) {
    window.clearTimeout(clearTimer);
    clearTimer = null;
  }
  return writeClipboard(text).then((ok) => {
    if (!ok) {
      toast.error("Copy failed — the clipboard is unavailable.");
      return false;
    }
    toast.success("Copied — auto-clears in 15s");
    clearTimer = window.setTimeout(clearClipboard, CLEAR_DELAY_MS);
    return true;
  });
}

if (typeof window !== "undefined") {
  const hide = () => clearClipboard();
  window.addEventListener("blur", hide);
  window.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "hidden") hide();
  });
}
