const SANITIZE_MAX_LENGTH = 200;

/** Escape HTML entities to prevent XSS via error messages. Text-only escaping (no tag stripping) so sanitization is complete. */
export function sanitizeError(msg: string): string {
  if (!msg) return '';
  const out = msg
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#x27;')
    .replace(/`/g, '&#x60;')
    .replace(/\//g, '&#x2F;');
  return out.length > SANITIZE_MAX_LENGTH ? out.slice(0, SANITIZE_MAX_LENGTH) : out;
}
