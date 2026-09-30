import { describe, expect, it } from "vitest";
import { getTotalPages } from "@/lib/api";

// Guards the fetchAllNodes/fetchAllServers page-count contract: the backend
// emits total_records (record count) and total_pages separately, and the
// client must derive pages as ceil(total_records / per_page) — never treat
// the record count itself as a page count.
describe("getTotalPages", () => {
  it("returns 1 when pagination metadata is absent", () => {
    expect(getTotalPages(undefined)).toBe(1);
  });

  it("derives pages from total_records / per_page", () => {
    expect(getTotalPages({ per_page: 100, total_records: 0 } as never)).toBe(1);
    expect(getTotalPages({ per_page: 100, total_records: 1 } as never)).toBe(1);
    expect(getTotalPages({ per_page: 100, total_records: 100 } as never)).toBe(1);
    expect(getTotalPages({ per_page: 100, total_records: 101 } as never)).toBe(2);
    expect(getTotalPages({ per_page: 50, total_records: 750 } as never)).toBe(15);
    expect(getTotalPages({ per_page: 100, total_records: 5000 } as never)).toBe(50);
  });

  it("falls back to explicit total_pages when records are unknown", () => {
    expect(getTotalPages({ total_pages: 7 } as never)).toBe(7);
    expect(getTotalPages({ total: 4 } as never)).toBe(4);
  });

  it("prefers total_records over stale total_pages", () => {
    // A record count of 250 at per_page=100 is 3 pages even if a cached
    // envelope still says something else.
    expect(getTotalPages({ per_page: 100, total_records: 250, total_pages: 2 } as never)).toBe(3);
  });
});
