const requests = new Map<string, number[]>();

const CLEANUP_INTERVAL_MS = 60000;
const WINDOW_MS_DEFAULT = 60000;
const MAX_REQUESTS_DEFAULT = 10;
const MAX_ENTRIES = 10000;

// Evict expired entries periodically
setInterval(() => {
  if (requests.size > MAX_ENTRIES) {
    requests.clear();
    return;
  }
  const now = Date.now();
  for (const [key, timestamps] of requests) {
    const recent = timestamps.filter((t) => now - t < CLEANUP_INTERVAL_MS);
    if (recent.length === 0) {
      requests.delete(key);
    } else {
      requests.set(key, recent);
    }
  }
}, CLEANUP_INTERVAL_MS);

export function checkRateLimit(key: string, maxRequests = MAX_REQUESTS_DEFAULT, windowMs = WINDOW_MS_DEFAULT): boolean {
  const now = Date.now();
  const timestamps = requests.get(key) || [];
  const recent = timestamps.filter((t) => now - t < windowMs);
  if (recent.length >= maxRequests) return false;
  recent.push(now);
  requests.set(key, recent);
  return true;
}
