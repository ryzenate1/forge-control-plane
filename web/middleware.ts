import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

// Trust model: The session cookie (__Host-forge_session) is the sole auth gate
// at the edge. Public paths (/, /features, /docs, /manual) are accessible without
// authentication — they serve the documentation site. All other paths (e.g. /health,
// /system, /backups) require a valid session cookie; unauthenticated requests are
// redirected to '/'. API proxy routes (/api/proxy/*) are exempt because the backend
// validates auth independently via the forwarded cookie.
const publicPaths = ['/', '/features', '/docs', '/manual'];

export function middleware(request: NextRequest) {
  // API proxy routes handle their own auth via cookie forwarding to the backend
  if (request.nextUrl.pathname.startsWith('/api/proxy/')) {
    return NextResponse.next();
  }

  const session = request.cookies.get('__Host-forge_session');
  const pathname = request.nextUrl.pathname;
  const isPublic = publicPaths.some(p => pathname === p || pathname.startsWith('/docs/') || pathname.startsWith('/manual/'));

  if (!session && !isPublic) {
    return NextResponse.redirect(new URL('/', request.url));
  }
  return NextResponse.next();
}

export const config = {
  matcher: ['/((?!_next|favicon\\.svg|robots\\.txt).*)'],
};
