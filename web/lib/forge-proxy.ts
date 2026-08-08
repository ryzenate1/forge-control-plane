import { timingSafeEqual } from "node:crypto";
import { NextRequest, NextResponse } from "next/server";

export const NO_STORE = "no-store, no-cache, must-revalidate";

export function authenticatedHeaders(request: NextRequest, mutation = false): Headers | NextResponse {
  const session = request.cookies.get("__Host-forge_session")?.value;
  const cookieHeader = request.headers.get("cookie");
  if (!session || !cookieHeader) {
    return NextResponse.json({ error: "Authentication required" }, { status: 401, headers: { "Cache-Control": NO_STORE } });
  }

  const headers = new Headers({ Accept: "application/json", Cookie: cookieHeader });
  if (mutation) {
    const supplied = request.headers.get("x-csrf-token") ?? "";
    const expected = request.cookies.get("__Host-forge_csrf")?.value ?? "";
    const suppliedBytes = Buffer.from(supplied);
    const expectedBytes = Buffer.from(expected);
    if (!supplied || !expected || suppliedBytes.length !== expectedBytes.length || !timingSafeEqual(suppliedBytes, expectedBytes)) {
      return NextResponse.json({ error: "Invalid CSRF token" }, { status: 403, headers: { "Cache-Control": NO_STORE } });
    }
    headers.set("Content-Type", "application/json");
    headers.set("X-CSRF-Token", supplied);
  }
  return headers;
}

export function safeUpstreamError(status: number, action: string): NextResponse {
  const clientStatus = status >= 400 && status < 500 ? status : 502;
  return NextResponse.json(
    { error: `${action} failed` },
    { status: clientStatus, headers: { "Cache-Control": NO_STORE } },
  );
}
