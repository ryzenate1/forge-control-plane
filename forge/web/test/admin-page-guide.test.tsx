import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SectionHeader } from "@/components/admin/admin-ui";
import { renderWithQuery } from "@/test/render";

vi.mock("next/navigation", () => ({
  usePathname: () => "/admin/catalog",
}));

describe("admin page guides", () => {
  it("uses the registered sidebar description when a page has no specialized guide", async () => {
    const user = userEvent.setup();
    renderWithQuery(<SectionHeader title="Service Catalog" sub="Provision supported services." />);

    expect(screen.getByRole("heading", { name: "Service Catalog" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "About Service Catalog" }));

    expect(screen.getAllByText("Provision managed services (databases, caches, queues) from the catalog").length).toBeGreaterThanOrEqual(2);
  });
});
