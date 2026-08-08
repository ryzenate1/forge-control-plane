const SANITIZE_MAX_LENGTH = 200;

/** Strip HTML tags first, then escape HTML entities to prevent XSS via error messages. */
export function sanitizeError(msg: string): string {
  if (!msg) return '';
  return msg
    .replace(/<[^>]*>/g, '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#x27;')
    .substring(0, SANITIZE_MAX_LENGTH);
}
