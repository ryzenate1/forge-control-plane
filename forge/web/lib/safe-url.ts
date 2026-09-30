const SAFE_PROTOCOLS = new Set(["http:", "https:"]);

/**
 * Returns the URL for safe use in an anchor href, or null when the value is
 * absent or uses a non-http(s) scheme. API-controlled strings must never be
 * rendered into href attributes without this gate: browsers execute
 * `javascript:` URLs, and data:/vbscript: variants bypass naive checks.
 */
export function safeExternalUrl(raw: string | null | undefined): string | null {
  const value = raw?.trim();
  if (!value) return null;
  try {
    const url = new URL(value);
    return SAFE_PROTOCOLS.has(url.protocol) ? url.toString() : null;
  } catch {
    return null;
  }
}
