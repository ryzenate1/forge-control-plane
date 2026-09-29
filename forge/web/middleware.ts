import { NextResponse, type NextRequest } from "next/server";
import { isProtectedPath } from "@/lib/auth/protected-paths";

const PUBLIC_PATHS = new Set(["/", "/setup", "/forgot-password", "/reset-password", "/favicon.ico"]);
const IS_DEV = process.env.NODE_ENV === "development";
// next/font self-hosts every family at build time, so there is no runtime
// fetch to fonts.gstatic.com — font-src stays 'self' + data: only.
const CSP_HEADER = (nonce: string) => `default-src 'self'; script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${IS_DEV ? " 'unsafe-eval'" : ""}; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self' ws: wss:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'`;
const API_INTERNAL_URL = (process.env.API_INTERNAL_URL ?? "http://127.0.0.1:8080").replace(/\/$/, "");

function isProtected(pathname: string) {
  if (PUBLIC_PATHS.has(pathname)) return false;
  if (pathname.startsWith("/_next") || pathname.startsWith("/api/")) return false;
  return isProtectedPath(pathname);
}

/**
 * __Host- cookies are bound to the secure origin (Secure + Path=/ + no
 * Domain). Accepting a bare `forge_session` fallback in production would let a
 * non-__Host- cookie satisfy the gate, defeating that binding. Only the dev
 * server (plain http://localhost, where Secure cookies cannot be set) accepts
 * the bare fallback.
 */
function getSessionCookie(request: NextRequest): string | undefined {
  const hardened = request.cookies.get("__Host-forge_session")?.value;
  if (hardened) return hardened;
  if (IS_DEV) return request.cookies.get("forge_session")?.value;
  return undefined;
}

async function isSetupRequired(): Promise<boolean | null> {
  try {
    const statusPath = API_INTERNAL_URL.endsWith("/api/v1") ? "/setup/status" : "/api/v1/setup/status";
    const response = await fetch(new URL(statusPath, `${API_INTERNAL_URL}/`), { cache: "no-store", signal: AbortSignal.timeout(3000) });
    if (!response.ok) return null;
    const data = (await response.json()) as { required?: boolean };
    return Boolean(data.required);
  } catch {
    return null;
  }
}

async function hasValidSession(request: NextRequest): Promise<boolean | null> {
  try {
    const authPath = API_INTERNAL_URL.endsWith("/api/v1") ? "/auth/me" : "/api/v1/auth/me";
    const response = await fetch(new URL(authPath, `${API_INTERNAL_URL}/`), {
      headers: { cookie: request.headers.get("cookie") ?? "" },
      cache: "no-store",
      signal: AbortSignal.timeout(3000),
    });
    // 401/403 is a definite "no session". A 5xx or any other unexpected
    // status means the API could not answer authoritatively — report unknown
    // (null) rather than false so the caller does not bounce a valid session
    // to login just because the backend is down.
    if (response.status === 401 || response.status === 403) return false;
    if (response.ok) return true;
    return null;
  } catch {
    // Network error / timeout / API down: unknown, not invalid.
    return null;
  }
}

function withCsp(response: NextResponse, nonce: string): NextResponse {
  response.headers.set("Content-Security-Policy", CSP_HEADER(nonce));
  return response;
}

export async function middleware(request: NextRequest) {
  const { pathname, search } = request.nextUrl;
  if (!isProtected(pathname)) {
    // /setup stays public so first-run can bootstrap, but once setup has
    // completed it is no longer a valid destination — send visitors home
    // instead of rendering a wizard that will immediately bounce them.
    if (pathname === "/setup" && (await isSetupRequired()) === false) {
      const homeUrl = request.nextUrl.clone();
      homeUrl.pathname = "/";
      homeUrl.search = "?setup=complete";
      const nonce = crypto.randomUUID().replace(/-/g, "");
      return withCsp(NextResponse.redirect(homeUrl), nonce);
    }
    const nonce = crypto.randomUUID().replace(/-/g, "");
    const requestHeaders = new Headers(request.headers);
    requestHeaders.set("x-csp-nonce", nonce);
    return withCsp(NextResponse.next({ request: { headers: requestHeaders } }), nonce);
  }

  const session = getSessionCookie(request);
  if (!session) {
    const loginUrl = request.nextUrl.clone();
    loginUrl.pathname = "/";
    const nextPath = search ? `${pathname}${search}` : pathname;
    loginUrl.search = `?reason=session-expired&next=${encodeURIComponent(nextPath)}`;
    const nonce = crypto.randomUUID().replace(/-/g, "");
    return withCsp(NextResponse.redirect(loginUrl), nonce);
  }
  const validity = await hasValidSession(request);
  if (validity === false) {
    const loginUrl = request.nextUrl.clone();
    loginUrl.pathname = "/";
    const nextPath = search ? `${pathname}${search}` : pathname;
    loginUrl.search = `?reason=session-expired&next=${encodeURIComponent(nextPath)}`;
    const nonce = crypto.randomUUID().replace(/-/g, "");
    return withCsp(NextResponse.redirect(loginUrl), nonce);
  }
  // validity === null means the API could not be reached authoritatively
  // (down, 5xx, timeout). Fail open: let the request through and let the
  // client-side SessionLoader verify once the API answers, instead of
  // bouncing a potentially-valid session to login on every API blip.
  const nonce = crypto.randomUUID().replace(/-/g, "");
  const requestHeaders = new Headers(request.headers);
  requestHeaders.set("x-csp-nonce", nonce);
  return withCsp(NextResponse.next({ request: { headers: requestHeaders } }), nonce);
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
