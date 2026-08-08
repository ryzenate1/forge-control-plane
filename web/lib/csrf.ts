/** Read the CSRF token from the __Host-forge_csrf cookie. */
export function getCSRFToken(): string {
  const match = document.cookie.match(/(^|;\s*)__Host-forge_csrf=([^;]*)/);
  return match ? decodeURIComponent(match[2] ?? "") : "";
}
