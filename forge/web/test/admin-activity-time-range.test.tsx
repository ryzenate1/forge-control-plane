import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithQuery } from "@/test/render";
import { AdminActivityLog } from "@/components/admin/AdminActivityLog";
import { fetchAdminActivity } from "@/lib/api";

vi.mock("next/navigation", () => ({ usePathname: () => "/admin/activity" }));
vi.mock("@/lib/api", () => ({
  fetchAdminActivity: vi.fn(async () => ({ events: [], total: 0 })),
  fetchActivityStats: vi.fn(async () => ({})),
  exportAdminActivity: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

describe("Activity time range", () => {
  it("uses exact preset timestamps and keeps the selected range visible", async () => {
    const user = userEvent.setup();
    renderWithQuery(<AdminActivityLog />);
    const before = Date.now();
    await user.selectOptions(screen.getByRole("combobox", { name: "Select time range" }), "1h");
    await waitFor(() => expect(fetchAdminActivity).toHaveBeenLastCalledWith(expect.objectContaining({ from: expect.any(String), to: expect.any(String), offset: 0 })));
    const filter = vi.mocked(fetchAdminActivity).mock.calls.at(-1)![0]!;
    expect(Date.parse(filter.to!) - Date.parse(filter.from!)).toBe(3_600_000);
    expect(Date.parse(filter.to!)).toBeGreaterThanOrEqual(before);
    expect(Date.parse(filter.to!)).toBeLessThanOrEqual(Date.now());
    expect(screen.getByRole("combobox", { name: "Select time range" })).toHaveValue("1h");
    await user.selectOptions(screen.getByRole("combobox", { name: "Select time range" }), "all");
    await waitFor(() => expect(fetchAdminActivity).toHaveBeenLastCalledWith(expect.objectContaining({ from: undefined, to: undefined, offset: 0 })));
  });
});
