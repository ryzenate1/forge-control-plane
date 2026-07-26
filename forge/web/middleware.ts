import { NextResponse, type NextRequest } from "next/server";

const SESSION_COOKIES = ["__Host-forge_session", "forge_session"];
const PUBLIC_PATHS = new Set(["/", "/setup", "/forgot-password", "/reset-password", "/favicon.ico"]);
const PROTECTED_PREFIXES = ["/servers", "/server", "/account", "/admin", "/organizations"];
const CSP_HEADER = (nonce: string) => `default-src 'self'; script-src 'self' 'nonce-${nonce}' 'strict-dynamic'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' https://fonts.gstatic.com data:; connect-src 'self' ws: wss:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'`;
const API_INTERNAL_URL = (process.env.API_INTERNAL_URL ?? "http://127.0.0.1:8080").replace(/\/$/, "");

function isProtected(pathname: string) {
  if (PUBLIC_PATHS.has(pathname)) return false;
  if (pathname.startsWith("/_next") || pathname.startsWith("/api/")) return false;
  return PROTECTED_PREFIXES.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`));
}

async function hasValidSession(request: NextRequest): Promise<boolean> {
  try {
    const authPath = API_INTERNAL_URL.endsWith("/api/v1") ? "/auth/me" : "/api/v1/auth/me";
    const response = await fetch(new URL(authPath, `${API_INTERNAL_URL}/`), {
      headers: { cookie: request.headers.get("cookie") ?? "" },
      cache: "no-store",
    });
    return response.ok;
  } catch {
    return false;
  }
}

function withCsp(response: NextResponse, nonce: string): NextResponse {
  response.headers.set("Content-Security-Policy", CSP_HEADER(nonce));
  return response;
}

export async function middleware(request: NextRequest) {
  const { pathname, search } = request.nextUrl;
  if (!isProtected(pathname)) {
    const nonce = crypto.randomUUID().replace(/-/g, "");
    const requestHeaders = new Headers(request.headers);
    requestHeaders.set("x-csp-nonce", nonce);
    return withCsp(NextResponse.next({ request: { headers: requestHeaders } }), nonce);
  }

  const session = SESSION_COOKIES.reduce<string | undefined>((found, name) => found ?? request.cookies.get(name)?.value, undefined);
  if (session && await hasValidSession(request)) {
    const nonce = crypto.randomUUID().replace(/-/g, "");
    const requestHeaders = new Headers(request.headers);
    requestHeaders.set("x-csp-nonce", nonce);
    return withCsp(NextResponse.next({ request: { headers: requestHeaders } }), nonce);
  }

  const loginUrl = request.nextUrl.clone();
  loginUrl.pathname = "/";
  const nextPath = search ? `${pathname}${search}` : pathname;
  loginUrl.search = `?reason=session-expired&next=${encodeURIComponent(nextPath)}`;
  const nonce = crypto.randomUUID().replace(/-/g, "");
  return withCsp(NextResponse.redirect(loginUrl), nonce);
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
