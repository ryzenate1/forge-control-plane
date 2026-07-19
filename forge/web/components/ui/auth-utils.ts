export function safeRedirectPath(value: string | null) {
  if (!value) return null;
  let candidate = value;
  try {
    while (candidate.includes("%5c") || candidate.includes("%5C")) {
      candidate = decodeURIComponent(candidate);
    }
  } catch {
    return null;
  }
  if (!candidate.startsWith("/") || candidate.startsWith("//") || /[\\]/.test(candidate) || candidate.includes("\0")) return null;
  let pathname = candidate;
  let search = "";
  let hash = "";
  const hashIndex = candidate.indexOf("#");
  if (hashIndex >= 0) { hash = candidate.slice(hashIndex); pathname = candidate.slice(0, hashIndex); }
  const queryIndex = pathname.indexOf("?");
  if (queryIndex >= 0) { search = pathname.slice(queryIndex); pathname = pathname.slice(0, queryIndex); }
  try {
    const base = new URL(pathname, "https://panel.invalid");
    if (base.origin !== "https://panel.invalid") return null;
    if (base.pathname.includes("..") || /\\/.test(base.pathname)) return null;
    return `${base.pathname}${search}${hash}`;
  } catch {
    return null;
  }
}
