import { NextRequest, NextResponse } from "next/server";
import { API_BASE } from "@/lib/api-base";
import { authenticatedHeaders, NO_STORE } from "@/lib/forge-proxy";

type HostInfo = { os?: string; arch?: string; kernel?: string; cpuCores?: number; uptimeSeconds?: number };
type MemoryInfo = { totalMb?: number; usedMb?: number; freeMb?: number };
type DiskInfo = { mountPoint?: string; totalMb?: number; usedMb?: number; freeMb?: number };

const mib = (value = 0) => value * 1024 * 1024;

export async function GET(request: NextRequest) {
  const headers = authenticatedHeaders(request);
  if (headers instanceof NextResponse) return headers;
  try {
    const options = { headers, signal: AbortSignal.timeout(5000), cache: "no-store" as const };
    const [hostResponse, memoryResponse, diskResponse] = await Promise.all([
      fetch(`${API_BASE}/api/v1/host/info`, options),
      fetch(`${API_BASE}/api/v1/host/memory`, options),
      fetch(`${API_BASE}/api/v1/host/disk`, options),
    ]);
    if (!hostResponse.ok || !memoryResponse.ok || !diskResponse.ok) {
      return NextResponse.json({ error: "System info unavailable" }, { status: 502, headers: { "Cache-Control": NO_STORE } });
    }
    const host = await hostResponse.json() as HostInfo;
    const memory = await memoryResponse.json() as MemoryInfo;
    const partitions = await diskResponse.json() as DiskInfo[];
    const disk = partitions.find((entry) => entry.mountPoint === "/") ?? partitions[0] ?? {};
    return NextResponse.json({
      version: host.kernel ?? "unknown",
      os: host.os ?? "unknown",
      architecture: host.arch ?? "unknown",
      cpuThreads: host.cpuCores ?? 0,
      memory: { total: mib(memory.totalMb), used: mib(memory.usedMb), free: mib(memory.freeMb) },
      disk: { total: mib(disk.totalMb), used: mib(disk.usedMb), free: mib(disk.freeMb) },
      runtime: { reachable: true },
      uptime: host.uptimeSeconds ?? 0,
      activeSessions: 0,
    }, { headers: { "Cache-Control": "private, max-age=30" } });
  } catch {
    return NextResponse.json(
      { error: "System info unreachable" },
      { status: 502, headers: { "Cache-Control": NO_STORE } },
    );
  }
}
