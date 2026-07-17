import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ActivityView } from "./activity-view";
import { DatabasesView } from "./databases-view";
import { FilesView } from "./files-view";
import { ServerSettingsView } from "./settings-view";
import { StartupView } from "./startup-view";
import { jsonResponse, mockFetch, requestJSON } from "@/test/fetch-mock";
import { renderWithQuery } from "@/test/render";

vi.mock("next/dynamic", () => ({ default: () => function Editor() { return <textarea aria-label="File content" />; } }));

const server = { id: "s1", name: "Game", owner: "owner@example.com", template: "egg", node: "Node A", status: "offline", databaseLimit: 2, backupLimit: 2, allocationLimit: 2 };

beforeEach(() => {
  Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText: vi.fn().mockResolvedValue(undefined) } });
});

describe("server database secrets", () => {
  it("reveals a newly created password once and sends connection limits", async () => {
    const mocked = mockFetch(
      jsonResponse([]),
      jsonResponse({ id: "db1", database: "world", username: "u1", remote: "%", engine: "mysql", host: "db.local", port: 3306, maxConnections: 12, password: "one-time-secret" }),
      jsonResponse([{ id: "db1", database: "world", username: "u1", remote: "%", engine: "mysql", host: "db.local", port: 3306, maxConnections: 12 }]),
    );
    renderWithQuery(<DatabasesView server={server} />);
    await screen.findByText("No databases have been created.");
    await userEvent.type(screen.getByLabelText("Database name"), "world");
    await userEvent.type(screen.getByLabelText("Max connections"), "12");
    await userEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(await screen.findByText("one-time-secret")).toBeInTheDocument();
    expect(requestJSON(mocked.calls[1])).toEqual({ database: "world", remote: "%", maxConnections: 12 });
    await userEvent.click(screen.getByRole("button", { name: "Dismiss permanently" }));
    expect(screen.queryByText("one-time-secret")).not.toBeInTheDocument();
  });
});

describe("startup validation", () => {
  it("blocks invalid variable values and saves using the environment key", async () => {
    const mocked = mockFetch(
      jsonResponse({ startup_command: "java -Xmx2G -jar server.jar", raw_startup_command: "java {{MEMORY}}", docker_images: { Java: "java:21" }, variables: [{ name: "Port", description: "Game port", env_variable: "SERVER_PORT", default_value: "25565", server_value: "25565", is_editable: true, rules: "required|integer|min:1024|max:65535" }] }),
      jsonResponse({}),
      jsonResponse({ startup_command: "java -Xmx2G -jar server.jar", raw_startup_command: "java {{MEMORY}}", docker_images: { Java: "java:21" }, variables: [{ name: "Port", description: "Game port", env_variable: "SERVER_PORT", default_value: "25565", server_value: "25566", is_editable: true, rules: "required|integer|min:1024|max:65535" }] }),
    );
    renderWithQuery(<StartupView server={server} />);
    const input = await screen.findByLabelText("Port");
    await userEvent.clear(input);
    await userEvent.type(input, "80");
    expect(screen.getByText("Minimum value is 1024.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save variable" })).toBeDisabled();
    await userEvent.clear(input);
    await userEvent.type(input, "25566");
    await userEvent.click(screen.getByRole("button", { name: "Save variable" }));
    await waitFor(() => expect(requestJSON(mocked.calls[1])).toEqual({ key: "SERVER_PORT", value: "25566" }));
  });
});

describe("safe server settings", () => {
  it("updates only the server name and description", async () => {
    const mocked = mockFetch(jsonResponse({ ...server, name: "Renamed", description: "Updated" }));
    renderWithQuery(<ServerSettingsView node={{ id: "n1", name: "Node A", region: "eu", status: "online", fqdn: "node.example.com", daemonSftp: 2022 }} server={server} />);
    const name = screen.getByLabelText("Server name");
    await userEvent.clear(name);
    await userEvent.type(name, "Renamed");
    await userEvent.type(screen.getByLabelText("Description"), "Updated");
    await userEvent.click(screen.getByRole("button", { name: "Save details" }));
    await waitFor(() => expect(mocked.calls).toHaveLength(1));
    expect(requestJSON(mocked.calls[0])).toEqual({ name: "Renamed", description: "Updated" });
  });
});

describe("file mass actions", () => {
  it("deletes selected items through one bounded batch endpoint", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const mocked = mockFetch(
      jsonResponse([{ name: "a.txt", path: "a.txt", directory: false, size: 1, modTime: "now" }, { name: "b.txt", path: "b.txt", directory: false, size: 1, modTime: "now" }]),
      jsonResponse({ ok: true, deleted: 2 }),
      jsonResponse([]),
    );
    renderWithQuery(<FilesView server={server} />);
    await userEvent.click(await screen.findByLabelText("Select a.txt"));
    await userEvent.click(screen.getByLabelText("Select b.txt"));
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(mocked.calls.some((call) => call.url.includes("/files/delete-batch"))).toBe(true));
    expect(requestJSON(mocked.calls[1])).toEqual({ paths: ["a.txt", "b.txt"] });
  });
});

describe("activity filtering and pagination", () => {
  it("paginates an unpaginated backend response without claiming server pagination", async () => {
    mockFetch(jsonResponse(Array.from({ length: 21 }, (_, index) => ({ id: String(index), action: `server:file.write.${index}`, targetType: "file", metadata: JSON.stringify({ path: `file-${index}.txt` }), actorEmail: "user@example.com", createdAt: `2026-01-01T00:${String(index).padStart(2, "0")}:00Z` }))));
    renderWithQuery(<ActivityView server={server} />);
    expect(await screen.findByText(/Page 1 of 2/)).toHaveTextContent("client-side pagination");
    await userEvent.click(screen.getByRole("button", { name: "Next activity page" }));
    expect(screen.getByText(/Page 2 of 2/)).toBeInTheDocument();
  });
});
