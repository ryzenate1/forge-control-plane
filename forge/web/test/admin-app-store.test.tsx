import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithQuery } from "@/test/render";
import * as api from "@/lib/api/app-store";
import AppStorePage from "@/app/admin/app-store/page";

vi.mock("next/navigation", () => ({ usePathname: () => "/admin/app-store", useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/lib/api/app-store", () => ({ listApps: vi.fn(), listInstalls: vi.fn(), installApp: vi.fn(), uninstallApp: vi.fn(), upgradeApp: vi.fn(), syncBundledTemplates: vi.fn() }));
vi.mock("@/lib/api", () => ({ fetchAllNodes: vi.fn(async () => [{ id: "node-1", name: "Primary node" }]) }));

const app = { id: "redis", key: "redis", name: "Redis", shortDesc: "An in-memory database", description: "Cache for your apps", icon: "", category: "cache", tags: [], version: "7", composeContent: "", params: { PORT: { label: "Port", type: "number", default: 6379, description: "" } }, minMemoryMb: 256, minDiskMb: 1024, maintainer: "", sourceUrl: "", createdAt: "", updatedAt: "" };

beforeEach(() => {
 vi.mocked(api.listApps).mockResolvedValue([app]);
 vi.mocked(api.listInstalls).mockResolvedValue([]);
});

describe("App Store", () => {
 it("loads installed state while browsing", async () => {
  vi.mocked(api.listInstalls).mockResolvedValue([{ id: "install-1", appKey: "redis", name: "cache", status: "running" } as api.AppStoreInstall]);
  renderWithQuery(<AppStorePage />);
  expect(await screen.findByRole("button", { name: /Installed \(1\)/ })).toBeVisible();
  expect(screen.getByRole("button", { name: /Installed Redis/ })).toBeVisible();
 });

 it("requires an explicit node and sends editable configuration once", async () => {
  const user = userEvent.setup();
  vi.mocked(api.installApp).mockRejectedValue(new Error("Deployment failed"));
  renderWithQuery(<AppStorePage />);
  await user.click(await screen.findByRole("button", { name: /Redis An in-memory/ }));
  await user.click(screen.getByRole("button", { name: "Install" }));
  const modal = screen.getByRole("dialog");
  const submit = within(modal).getByRole("button", { name: "Install" });
  expect(submit).toBeDisabled();
  await screen.findByRole("option", { name: "Primary node" });
  await user.selectOptions(within(modal).getByLabelText("Target node"), "node-1");
  expect(within(modal).getByDisplayValue("6379")).toBeVisible();
  await user.click(submit);
  await waitFor(() => expect(api.installApp).toHaveBeenCalledWith(expect.objectContaining({ nodeId: "node-1", params: { PORT: "6379" } })));
  await screen.findByText("Deployment failed");
  expect(api.installApp).toHaveBeenCalledTimes(1);
  expect(submit).toBeEnabled();
 });
});
