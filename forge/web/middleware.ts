import { NextResponse, type NextRequest } from "next/server";

const SESSION_COOKIE = "__Host-forge_session";
const PUBLIC_PATHS = new Set(["/", "/setup", "/forgot-password", "/reset-password", "/favicon.ico"]);
const PROTECTED_PREFIXES = ["/servers", "/server", "/account", "/admin", "/organizations"];

function isProtected(pathname: string) {
  if (PUBLIC_PATHS.has(pathname)) return false;
  if (pathname.startsWith("/_next") || pathname.startsWith("/api/")) return false;
  return PROTECTED_PREFIXES.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`));
}

export function middleware(request: NextRequest) {
  const { pathname, search } = request.nextUrl;
  if (!isProtected(pathname)) return NextResponse.next();

  const session = request.cookies.get(SESSION_COOKIE)?.value;
  if (session) return NextResponse.next();

  const loginUrl = request.nextUrl.clone();
  loginUrl.pathname = "/";
  loginUrl.search = `?reason=session-expired&next=${encodeURIComponent(`${pathname}${search}`)}`;
  return NextResponse.redirect(loginUrl);
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
