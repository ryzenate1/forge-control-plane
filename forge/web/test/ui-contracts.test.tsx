import type { ComponentType, ReactNode } from "react";
import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AdminShell } from "@/components/admin/admin-shell";
import { AdminUsers } from "@/components/admin/AdminUsers";
import { adminPagesForRole } from "@/components/admin/admin-registry";
import { BackupsView } from "@/components/server/backups-view";
import { FilesView } from "@/components/server/files-view";
import { NetworkView } from "@/components/server/network-view";
import SetupPage from "@/app/setup/page";
import AccountPage from "@/app/account/page";
import ServersPage from "@/app/servers/page";
import { useServerStore } from "@/stores/use-server-store";
import { jsonResponse, mockFetch } from "@/test/fetch-mock";
import { renderWithQuery } from "@/test/render";

const replace = vi.fn();
const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ replace, push }), usePathname: () => "/admin/overview" }));
vi.mock("next/link", () => ({ default: ({ children, href, ...props }: { children: ReactNode; href: string }) => <a href={href} {...props}>{children}</a> }));
vi.mock("next/dynamic", () => ({ default: (_loader: unknown, options?: { loading?: ComponentType }) => function FakeEditor(props: { value?: string; onChange?: (value: string) => void; options?: { readOnly?: boolean } }) {
  if (props.options?.readOnly && options?.loading) { const Loading = options.loading; return <Loading />; }
  return <textarea aria-label="File content" disabled={props.options?.readOnly} value={props.value} onChange={(event) => props.onChange?.(event.target.value)} />;
} }));

beforeEach(() => {
  replace.mockReset();
  push.mockReset();
  useServerStore.setState({ currentUser: null });
});

describe("admin page registry", () => {
  it("exposes supported admin routes only to administrators and labels metadata-only plugins", () => {
    expect(adminPagesForRole("user")).toEqual([]);
    const pages = adminPagesForRole("admin").flatMap((group) => group.items);
    expect(pages.map((page) => page.href)).toEqual(expect.arrayContaining(["/admin/roles", "/admin/plugins", "/admin/regions", "/admin/operations", "/admin/oauth-clients", "/admin/webhooks"]));
    expect(pages.find((page) => page.href === "/admin/plugins")?.capability).toBe("metadata-only");
  });
});

describe("guard and outage behavior", () => {
  it("keeps setup unavailable during an API outage", async () => {
    mockFetch(jsonResponse({ message: "offline" }, 503));
    renderWithQuery(<SetupPage />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Unable to verify setup status");
    expect(screen.queryByRole("button", { name: /create admin/i })).not.toBeInTheDocument();
  });

  it("hides admin content and redirects a non-admin user", async () => {
    useServerStore.setState({ currentUser: { id: "u1", email: "u@example.com", role: "user" } });
    mockFetch(jsonResponse({ id: "u1", email: "u@example.com", role: "user" }));
    renderWithQuery(<AdminShell><div>ADMIN SECRET</div></AdminShell>);
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/servers"));
    expect(screen.queryByText("ADMIN SECRET")).not.toBeInTheDocument();
  });
});

describe("account security", () => {
  it("changes the password, expires the local session, and keeps email editing unavailable", async () => {
    useServerStore.setState({ currentUser: { id: "u1", email: "u@example.com", role: "user" } });
    const route = (url: string) => {
      if (url.endsWith("/auth/me")) return jsonResponse({ id: "u1", email: "u@example.com", role: "user", useTotp: false });
      if (url.endsWith("/ssh-keys")) return jsonResponse([]);
      if (url.endsWith("/account/password")) return jsonResponse({ status: "ok" });
      return jsonResponse({ message: `Unexpected URL ${url}` }, 500);
    };
    mockFetch(route, route, route, route, route, route, route, route);
    renderWithQuery(<AccountPage />);
    const email = await screen.findByDisplayValue("u@example.com");
    expect(email).toBeDisabled();
    await userEvent.type(screen.getByLabelText("Current password", { selector: "#current-password" }), "old-password");
    await userEvent.type(screen.getByLabelText("New password"), "new-password");
    await userEvent.type(screen.getByLabelText("Confirm new password"), "new-password");
    await userEvent.click(screen.getByRole("button", { name: "Change Password" }));
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/"));
    expect(useServerStore.getState().currentUser).toBeNull();
  });
});

describe("admin mutation protections", () => {
  it("disables user deletion when the user owns a server", async () => {
    mockFetch(
      jsonResponse([{ id: "u1", email: "owner@example.com", role: "user" }]),
      jsonResponse([{ id: "s1", name: "Owned", owner: "owner@example.com", ownerId: "u1", template: "egg", node: "node", status: "offline" }]),
    );
    renderWithQuery(<AdminUsers />);
    await userEvent.click(await screen.findByText("owner@example.com"));
    expect(await screen.findByText("Owned Servers")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /delete/i })).toBeDisabled();
  });
});

describe("server UI truthfulness", () => {
  it("renders unknown server metrics as unknown instead of fabricated values", async () => {
    useServerStore.setState({ currentUser: { id: "u1", email: "u@example.com", role: "user" } });
    mockFetch(
      jsonResponse({ id: "u1", email: "u@example.com", role: "user", useTotp: false }),
      jsonResponse([{ id: "s1", name: "No Telemetry", owner: "u1", template: "egg", node: "node", status: "offline", memory: null, cpu: null, uptime: null }])
    );
    renderWithQuery(<ServersPage />);
    expect(await screen.findByText("No Telemetry")).toBeInTheDocument();
    expect(screen.queryByText(/0%|0 MB|fake/i)).not.toBeInTheDocument();
  });

  it("shows backup status, checksum, and size and disables unsafe actions", async () => {
    mockFetch(jsonResponse({
      data: [
        { uuid: "b1", name: "pending.zip", checksum: "", size: 0, status: "pending", createdAt: "2026-01-01T00:00:00Z" },
        { uuid: "b2", name: "done.zip", checksum: "sha256:abc", size: 2048, status: "completed", createdAt: "2026-01-01T00:00:00Z", completedAt: "2026-01-01T00:01:00Z" },
        { uuid: "b3", name: "failed.zip", checksum: "", size: 10, status: "failed", createdAt: "2026-01-01T00:00:00Z" },
      ],
      pagination: { page: 1, per_page: 20, total: 3, total_pages: 1 }
    }));
    renderWithQuery(<BackupsView server={{ id: "s1", name: "S", owner: "u", template: "e", node: "n", status: "offline" }} />);
    expect(await screen.findByText("pending.zip")).toBeInTheDocument();
    expect(screen.getByText("sha256:abc", { exact: false })).toBeInTheDocument();
    expect(screen.getByText("2.00 kB", { exact: false })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Restore pending.zip" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Delete failed.zip" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Lock done.zip" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Restore done.zip" })).toBeEnabled();
  });

  it("uses the backend primary allocation ID rather than array index", async () => {
    mockFetch(jsonResponse([
      { id: "a1", node: "n1", ip: "127.0.0.1", port: 25565, notes: "first" },
      { id: "a2", node: "n1", ip: "127.0.0.1", port: 25566, notes: "second" },
    ]));
    renderWithQuery(<NetworkView server={{ id: "s1", name: "S", owner: "u", template: "e", node: "n", status: "offline", primaryAllocationId: "a2" }} />);
    expect(await screen.findByText("Primary")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Unassign 127.0.0.1:25566" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Unassign 127.0.0.1:25565" })).toBeEnabled();
  });

  it("keeps file save disabled while content is loading and never reports a fake save", async () => {
    let resolveContent!: (response: Response) => void;
    const content = new Promise<Response>((resolve) => { resolveContent = resolve; });
    mockFetch(jsonResponse([{ name: "server.properties", path: "server.properties", directory: false, size: 20, modTime: "today" }]), () => content);
    renderWithQuery(<FilesView server={{ id: "s1", name: "S", owner: "u", template: "e", node: "n", status: "offline" }} />);
    await userEvent.click(await screen.findByRole("button", { name: "server.properties" }));
    expect(screen.getByRole("button", { name: /save content/i })).toBeDisabled();
    expect(screen.getByText("Loading")).toBeInTheDocument();
    expect(screen.queryByText("Saved")).not.toBeInTheDocument();
    await act(async () => resolveContent(new Response("motd=hello", { status: 200 })));
    await waitFor(() => expect(screen.getByRole("button", { name: /save content/i })).toBeEnabled());
    fireEvent.change(screen.getByLabelText("File content"), { target: { value: "motd=changed" } });
    expect(screen.getByText("Edited")).toBeInTheDocument();
  });
});
