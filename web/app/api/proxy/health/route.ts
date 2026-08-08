import { NextRequest, NextResponse } from "next/server";
import { API_BASE } from "@/lib/api-base";
import { authenticatedHeaders, NO_STORE } from "@/lib/forge-proxy";

export async function GET(request: NextRequest) {
  const headers = authenticatedHeaders(request);
  if (headers instanceof NextResponse) return headers;
  try {
    const res = await fetch(`${API_BASE}/api/v1/health`, {
      headers,
      signal: AbortSignal.timeout(5000),
    });
    if (!res.ok) {
      return NextResponse.json(
        { status: "unhealthy", checks: [], checkedAt: new Date().toISOString() },
        { status: 503, headers: { "Cache-Control": NO_STORE } },
      );
    }
    const data = await res.json();
    return NextResponse.json(data, { headers: { "Cache-Control": "private, max-age=10" } });
  } catch {
    return NextResponse.json(
      { status: "unreachable", checks: [], checkedAt: new Date().toISOString() },
      { status: 502, headers: { "Cache-Control": NO_STORE } },
    );
  }
}
