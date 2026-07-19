import { fetchJSON, putJSON } from './http';

export type RateLimitSettings = {
  authRequestsPerMinute: number;
  mutationRequestsPerMinute: number;
  readRequestsPerMinute: number;
  loginRateLimitEnabled: boolean;
  loginAttemptThreshold: number;
  accountLockoutMinutes: number;
  signedUrlExpiryMinutes: number;
  maxWebSocketsPerServer: number;
  consoleThrottleEnabled: boolean;
  consoleThrottleLines: number;
  consoleThrottlePeriodMs: number;
};

export const DEFAULT_RATE_LIMIT_SETTINGS: RateLimitSettings = {
  authRequestsPerMinute: 5,
  mutationRequestsPerMinute: 30,
  readRequestsPerMinute: 120,
  loginRateLimitEnabled: true,
  loginAttemptThreshold: 5,
  accountLockoutMinutes: 15,
  signedUrlExpiryMinutes: 5,
  maxWebSocketsPerServer: 30,
  consoleThrottleEnabled: false,
  consoleThrottleLines: 2000,
  consoleThrottlePeriodMs: 100,
};

export async function fetchRateLimitSettings(): Promise<RateLimitSettings> {
  return fetchJSON<RateLimitSettings>('/admin/settings/rate-limits');
}

export async function updateRateLimitSettings(settings: RateLimitSettings): Promise<RateLimitSettings> {
  return putJSON<RateLimitSettings>('/admin/settings/rate-limits', settings);
}
