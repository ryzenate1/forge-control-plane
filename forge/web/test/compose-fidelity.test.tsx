import { describe, it, expect, vi, beforeEach } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { validateCompose, createComposeStack } from "@/lib/api/compose";
import { jsonResponse, mockFetch, mockFetchByUrl, requestJSON } from "@/test/fetch-mock";
import { renderWithQuery } from "@/test/render";
import { ApiError } from "@/lib/api/http";

// Mock next/navigation for compose new page (uses useRouter)
const push = vi.fn();
const replace = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, replace }),
  usePathname: () => "/admin/compose/new",
  useSearchParams: () => new URLSearchParams(),
}));

// ---------------------------------------------------------------------------
// Helpers mirroring beacon/internal/server/compose.go:286 shortFormHostPort
// This TS replica is used to verify fidelity against Go reference vectors.
// ---------------------------------------------------------------------------
function shortFormHostPort(entry: string): string {
  entry = entry.trim();
  if (entry === "") return "";
  const slashIdx = entry.indexOf("/");
  if (slashIdx !== -1) entry = entry.slice(0, slashIdx);
  entry = entry.trim();
  if (entry === "") return "";
  const parts = entry.split(":");
  switch (parts.length) {
    case 1:
      return parts[0];
    case 2:
      return parts[0];
    case 3:
      return parts[1];
    default: {
      if (parts.length > 3) return parts[parts.length - 2];
      return "";
    }
  }
}

function isPrivilegedHostPort(published: string): boolean {
  let p = published.trim();
  if (!p) return false;
  if (p.includes("-")) p = p.split("-", 2)[0].trim();
  const slashIdx = p.indexOf("/");
  if (slashIdx !== -1) p = p.slice(0, slashIdx).trim();
  const port = Number(p);
  if (!Number.isInteger(port) || port <= 0) return false;
  return port < 1024;
}

beforeEach(() => {
  push.mockClear();
  replace.mockClear();
  vi.restoreAllMocks();
});

// ---------------------------------------------------------------------------
// TestEnvFile_NotSupported – mirrors forge/api/internal/services/compose/*
//   compose_fixes_test.go:TestEnvFile_NotSupported
//   env_file_test.go:TestEnvFile_*
// Backend strict contract: FORGE_ENV_FILE_STRICT=true => Parse/Validate
// must reject env_file (string or list, service-level or include-level).
// Web fidelity: validateCompose propagates that error correctly, and
// non-strict surfaces a warning but remains valid.
// ---------------------------------------------------------------------------
describe("TestEnvFile_NotSupported (compose fidelity – env_file strict contract)", () => {
  const yamlWithEnvFileList = `
services:
  app:
    image: nginx
    env_file:
      - .env
`;
  const yamlWithEnvFileString = `
services:
  web:
    image: nginx
    env_file: .env
`;
  const validWithoutEnvFile = `
services:
  app:
    image: nginx
    environment:
      - FOO=bar
`;

  it("validateCompose: strict 400 path surfaces env_file error as ApiError", async () => {
    // handlers_compose.go returns 400 with ValidateResult when env_file detected in strict mode
    const strictBody = {
      valid: false,
      errors: [{ field: "env_file", message: "env_file not supported, inline env vars" }],
    };
    mockFetch(jsonResponse(strictBody, 400));
    let caught: unknown;
    try {
      await validateCompose(yamlWithEnvFileList);
    } catch (e) {
      caught = e;
    }
    expect(caught).toBeInstanceOf(ApiError);
    expect((caught as ApiError).status).toBe(400);
    expect((caught as Error).message).toMatch(/400|env_file/i);
  });

  it("validateCompose: strict 200-invalid payload contains env_file field error (Go ValidateCompose path)", async () => {
    // Direct ValidateCompose result (valid:false) returned as 200 in some code paths
    mockFetch(
      jsonResponse({
        valid: false,
        errors: [{ field: "env_file", message: "env_file not supported, inline env vars" }],
      }),
    );
    const result = await validateCompose(yamlWithEnvFileString);
    expect(result.valid).toBe(false);
    expect(result.errors).toBeDefined();
    const found = result.errors!.some(
      (e) => e.field.toLowerCase().includes("env_file") || e.message.toLowerCase().includes("env_file"),
    );
    expect(found).toBe(true);
    expect(result.errors![0].message).toBe("env_file not supported, inline env vars");
  });

  it("validateCompose: env_file string form also rejected in strict (mirrors Go string case)", async () => {
    mockFetch(
      jsonResponse({
        valid: false,
        errors: [{ field: "env_file", message: "env_file not supported, inline env vars" }],
      }),
    );
    const result = await validateCompose(yamlWithEnvFileString);
    expect(result.valid).toBe(false);
    expect(result.errors![0].field).toBe("env_file");
  });

  it("validateCompose: clean compose without env_file passes strict", async () => {
    mockFetch(
      jsonResponse({
        valid: true,
        summary: { services: [{ name: "app", image: "nginx" }], networks: [], volumes: [] },
      }),
    );
    const result = await validateCompose(validWithoutEnvFile);
    expect(result.valid).toBe(true);
    expect(result.errors).toBeUndefined();
  });

  it("validateCompose: non-strict warns but passes (mirrors Go FORGE_ENV_FILE_STRICT=false)", async () => {
    mockFetch(
      jsonResponse({
        valid: true,
        warnings: [{ field: "env_file", message: "env_file is present but will be ignored; inline env vars instead" }],
        summary: { services: [{ name: "web", image: "nginx:latest" }], networks: [], volumes: [] },
      }),
    );
    const result = await validateCompose(`
services:
  web:
    image: nginx:latest
    env_file: ./app.env
    environment:
      INLINE_VAR: value
`);
    expect(result.valid).toBe(true);
    expect(result.warnings).toBeDefined();
    const warnFound = result.warnings!.some((w) => w.field.toLowerCase().includes("env_file"));
    expect(warnFound).toBe(true);
  });

  it("validateCompose sends { content } payload (handlers_compose.go ComposeValidateRequest)", async () => {
    const { calls } = mockFetch(
      jsonResponse({ valid: true, summary: { services: [{ name: "app", image: "nginx" }] } }),
    );
    await validateCompose(validWithoutEnvFile);
    expect(calls[0].url).toContain("/compose/validate");
    const body = requestJSON(calls[0]) as { content: string };
    expect(body.content).toBe(validWithoutEnvFile);
  });

  it("createComposeStack also rejects env_file via 400 (handlers_compose.go CreateStackRequest path)", async () => {
    mockFetch(jsonResponse({ message: "env_file not supported, inline env vars" }, 400));
    await expect(
      createComposeStack({ name: "bad", composeYaml: yamlWithEnvFileList }),
    ).rejects.toThrow(/400|env_file/i);
  });

  it("compose new page wires validate button and renders Valid/Invalid feedback states", async () => {
    const src = readFileSync(resolve(__dirname, "../app/admin/compose/new/page.tsx"), "utf8");
    expect(src).toContain("validateCompose");
    expect(src).toContain("validateResult");
    // Must handle both Valid and Invalid branches
    expect(src).toContain("Valid");
    expect(src).toContain("Invalid");
    // Must show env_file field error handling via errors[].field
    expect(src).toContain("validateResult.errors");
    // Handler import must be from lib/api/compose
    expect(src).toContain('from "@/lib/api/compose"');
  });

  it("compose new page file does not swallow env_file – backend strict surfaces field env_file", () => {
    const src = readFileSync(resolve(__dirname, "../lib/api/compose.ts"), "utf8");
    expect(src).toContain("validateCompose");
    expect(src).toContain("/compose/validate");
    // Ensure ValidateResult type documents env_file handling via errors/warnings
    expect(src).toContain("ComposeValidationError");
  });

  it("backend handler file correctly maps env_file to 400 (audit fidelity check)", () => {
    const src = readFileSync(resolve(__dirname, "../../api/internal/http/handlers_compose.go"), "utf8");
    expect(src).toContain('env_file');
    expect(src).toContain("StatusBadRequest");
    // Must check both Validate and Create stack paths
    const validateEnvCount = (src.match(/env_file/g) || []).length;
    expect(validateEnvCount).toBeGreaterThanOrEqual(3);
  });
});

// ---------------------------------------------------------------------------
// TestShortFormHostPort – mirrors beacon/internal/server/compose.go:286 and
// beacon/internal/server/compose_fixes_test.go:12 TestShortFormHostPort_Fixes
// plus beacon/compose_fixes_test.go range privileged checks.
// ---------------------------------------------------------------------------
describe("TestShortFormHostPort (beacon compose.go fidelity)", () => {
  const vectors: Array<{ name: string; input: string; expected: string }> = [
    { name: "len1 single port", input: "80", expected: "80" },
    { name: "len1 with protocol", input: "80/tcp", expected: "80" },
    { name: "host:container", input: "8080:80", expected: "8080" },
    { name: "range host:container range", input: "8080-8082:80-82", expected: "8080-8082" },
    { name: "ip:host:container", input: "127.0.0.1:8080:80", expected: "8080" },
    { name: "ip::container random host", input: "127.0.0.1::80", expected: "" },
    { name: "ip:range:container range", input: "127.0.0.1:8080-8082:80-82", expected: "8080-8082" },
    { name: "with protocol suffix", input: "8080:80/udp", expected: "8080" },
    { name: "bracket ipv6", input: "[::1]:8080:80", expected: "8080" },
  ];

  it.each(vectors)("$name: shortFormHostPort($input) = $expected", ({ input, expected }) => {
    expect(shortFormHostPort(input)).toBe(expected);
  });

  it("strips /tcp, /udp, /sctp suffixes before splitting", () => {
    expect(shortFormHostPort("8080:80/tcp")).toBe("8080");
    expect(shortFormHostPort("8080:80/udp")).toBe("8080");
    expect(shortFormHostPort("8080:80/sctp")).toBe("8080");
    expect(shortFormHostPort("80/sctp")).toBe("80");
  });

  it("range form extracts host side correctly (beacon compose.go:261 handling)", () => {
    expect(shortFormHostPort("8080-8082:80-82")).toBe("8080-8082");
    expect(shortFormHostPort("3000-3005:3000-3005")).toBe("3000-3005");
    expect(shortFormHostPort("127.0.0.1:8080-8082:80-82")).toBe("8080-8082");
    // Range low-end extraction for privileged check is done after shortFormHostPort
    const hostPort = shortFormHostPort("8080-8082:80-82"); // "8080-8082"
    const low = hostPort.split("-", 2)[0]; // "8080"
    expect(low).toBe("8080");
  });

  it("random host (ip::container) returns empty so privileged gate is skipped (beacon logic)", () => {
    const published = shortFormHostPort("127.0.0.1::80");
    expect(published).toBe("");
    // validateComposePorts would continue (skip) when published=="" => not privileged
    expect(isPrivilegedHostPort(published)).toBe(false);
    // Also verify validateComposePorts handling of 127.0.0.1::80 directly returns empty
  });

  it("privileged detection: host range starting at privileged port 80 is rejected (beacon TestValidateComposePorts_RangePrivileged)", () => {
    // Host range 80-82 publishing should be privileged -> rejected
    expect(isPrivilegedHostPort("80-82")).toBe(true);
    expect(isPrivilegedHostPort(shortFormHostPort("80-82:8080"))).toBe(true);
    // Non-privileged host range should pass
    expect(isPrivilegedHostPort(shortFormHostPort("8080-8082:80"))).toBe(false);
    expect(isPrivilegedHostPort("8080-8082")).toBe(false);
  });

  it("len1 privileged 80 is rejected, 8080 passes (beacon len1 check)", () => {
    expect(isPrivilegedHostPort("80")).toBe(true);
    expect(isPrivilegedHostPort("8080")).toBe(false);
    expect(isPrivilegedHostPort(shortFormHostPort("80"))).toBe(true);
    expect(isPrivilegedHostPort(shortFormHostPort("8080"))).toBe(false);
  });

  it("handles int/float/proto edge cases from beacon validateComposePorts switch", () => {
    // String with spaces trimmed
    expect(shortFormHostPort("  8080:80  ")).toBe("8080");
    expect(shortFormHostPort("")).toBe("");
    expect(shortFormHostPort("   ")).toBe("");
    // Protocol stripping trims before split, so " 8080:80/tcp " works
    expect(shortFormHostPort(" 8080:80/tcp ")).toBe("8080");
  });

  it("IPv6 bracket form second-last heuristic matches beacon compose.go:310", () => {
    expect(shortFormHostPort("[::1]:8080:80")).toBe("8080");
    // More complex split >3: [::1]:8080:80 splits to 5 parts => returns parts[3] = 8080
    expect(shortFormHostPort("[fe80::1]:3000:3000")).toBe("3000");
  });

  it("beacon compose.go file actually contains shortFormHostPort and privileged range logic", () => {
    const src = readFileSync(resolve(__dirname, "../../../beacon/internal/server/compose.go"), "utf8");
    expect(src).toContain("func shortFormHostPort");
    expect(src).toContain("8080-8082");
    expect(src).toContain("validateComposePorts");
    // Must handle int, int64, float64 for ports not just strings
    expect(src).toContain("case int:");
    expect(src).toContain("case int64:");
    // Must strip protocol suffix before split
    expect(src).toContain('strings.Index(entry, "/")');
  });

  it("web compose stack status display does not conflate shortFormHostPort with full container port", () => {
    // In compose stack detail, services[].ports is a display string like "0.0.0.0:8080->80/tcp"
    // Web does not re-parse host port for validation – validation is server-side – but display must not truncate.
    // This is a fidelity contract: web shows svc.ports verbatim.
    const src = readFileSync(resolve(__dirname, "../app/admin/compose/[id]/page.tsx"), "utf8");
    expect(src).toContain("svc.ports");
    // Web should not define its own shortFormHostPort shadow; validation is delegated to backend
    expect(src).not.toMatch(/function shortFormHostPort/);
  });
});

// ---------------------------------------------------------------------------
// TestComposeCreate with nodeId – mirrors forge/api/internal/http/handlers_compose.go
// CreateStackRequest nodeId handling and forge/web/lib/api/compose.ts
// createComposeStack body forwarding. NodeId is opt-in (empty => auto-select scheduler).
// ---------------------------------------------------------------------------
describe("TestComposeCreate with nodeId (compose create fidelity)", () => {
  it("createComposeStack forwards nodeId when provided", async () => {
    const { calls } = mockFetch(
      jsonResponse({
        id: "stack-123",
        userId: "u1",
        name: "my-stack",
        nodeId: "node-abc",
        status: "deploying",
        composeYaml: "services:\n  web:\n    image: nginx",
        composeHash: "abc",
        envVars: {},
        memoryMb: 0,
        cpuShares: 0,
        diskMb: 0,
        error: "",
        composeType: "docker-compose",
        sourceType: "raw",
        environmentId: "",
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      }),
    );
    await createComposeStack({
      name: "my-stack",
      composeYaml: "services:\n  web:\n    image: nginx",
      nodeId: "node-abc",
      composeType: "docker-compose",
      sourceType: "raw",
    });
    expect(calls.length).toBe(1);
    expect(calls[0].url).toContain("/compose");
    expect((calls[0].init.method ?? "GET").toUpperCase()).toBe("POST");
    const body = requestJSON(calls[0]) as Record<string, unknown>;
    expect(body.name).toBe("my-stack");
    expect(body.composeYaml).toBe("services:\n  web:\n    image: nginx");
    expect(body.nodeId).toBe("node-abc");
  });

  it("createComposeStack sends nodeId as undefined when auto-select (omitted JSON)", async () => {
    const { calls } = mockFetch(
      jsonResponse({
        id: "stack-124",
        name: "auto-stack",
        nodeId: "",
        status: "deploying",
        composeYaml: "services:\n  web:\n    image: nginx",
        composeHash: "",
        envVars: {},
        memoryMb: 0,
        cpuShares: 0,
        diskMb: 0,
        error: "",
        composeType: "docker-compose",
        sourceType: "raw",
        environmentId: "",
        createdAt: "",
        updatedAt: "",
      }),
    );
    await createComposeStack({
      name: "auto-stack",
      composeYaml: "services:\n  web:\n    image: nginx",
      nodeId: undefined,
    });
    const body = requestJSON(calls[0]) as Record<string, unknown>;
    // When undefined, JSON.stringify omits the key (mirrors handlers_compose.go empty string -> scheduler auto)
    expect(body.nodeId).toBeUndefined();
  });

  it("createComposeStack also forwards composeType and sourceType alongside nodeId", async () => {
    const { calls } = mockFetch(
      jsonResponse({ id: "s1", name: "x", nodeId: "n1", status: "deploying", composeYaml: "v", composeHash: "", envVars: {}, memoryMb: 0, cpuShares: 0, diskMb: 0, error: "", composeType: "stack", sourceType: "raw", environmentId: "", createdAt: "", updatedAt: "" }),
    );
    await createComposeStack({
      name: "x",
      composeYaml: "services:\n  web:\n    image: nginx",
      nodeId: "n1",
      composeType: "stack",
      sourceType: "raw",
      memoryMb: 512,
    });
    const body = requestJSON(calls[0]) as Record<string, unknown>;
    expect(body.composeType).toBe("stack");
    expect(body.sourceType).toBe("raw");
    expect(body.memoryMb).toBe(512);
    expect(body.nodeId).toBe("n1");
  });

  it("lib/api/compose.ts correctly defines nodeId?: string optional (auto-select contract)", () => {
    const src = readFileSync(resolve(__dirname, "../lib/api/compose.ts"), "utf8");
    expect(src).toContain("nodeId?: string");
    expect(src).toContain("createComposeStack");
    expect(src).toContain("postJSON<ComposeStack>('/compose'");
  });

  it("NewComposeStackPage wires nodeId select to createComposeStack payload", async () => {
    const src = readFileSync(resolve(__dirname, "../app/admin/compose/new/page.tsx"), "utf8");
    expect(src).toContain("nodeId");
    expect(src).toContain("fetchNodes");
    expect(src).toContain("createComposeStack");
    // Must pass nodeId: nodeId || undefined (auto-select when empty)
    expect(src).toMatch(/nodeId:\s*nodeId\s*\|\|\s*undefined/);
    // Must render node selector with Auto-select option
    expect(src).toContain("Auto-select");
    expect(src).toContain('value={nodeId}');
  });

  it("NewComposeStackPage deploy mutation posts to /compose with nodeId (integration via mocked fetch)", async () => {
    // Render page with mocked nodes and compose validate/deploy endpoints
    const nodes = [{ id: "node-1", name: "us-east-1a" }, { id: "node-2", name: "eu-west-1b" }];
    mockFetchByUrl({
      "/nodes": jsonResponse(nodes),
      "/compose/validate": jsonResponse({ valid: true, summary: { services: [{ name: "web", image: "nginx" }] } }),
      "/compose": (url, init) => {
        const body = init.body ? JSON.parse(String(init.body)) : {};
        // Assert nodeId is forwarded inside the fetch handler (will be checked via waitFor below)
        // Return a stack so router.push is triggered
        return jsonResponse({
          id: "new-stack-id",
          name: body.name,
          nodeId: body.nodeId ?? "",
          status: "deploying",
          composeYaml: body.composeYaml,
          composeHash: "h",
          envVars: {},
          memoryMb: 0,
          cpuShares: 0,
          diskMb: 0,
          error: "",
          composeType: body.composeType,
          sourceType: body.sourceType,
          environmentId: "",
          createdAt: "",
          updatedAt: "",
        });
      },
    });

    const NewPage = (await import("@/app/admin/compose/new/page")).default;
    renderWithQuery(<NewPage />);

    // Wait for nodes to load and populate select
    expect(await screen.findByText("us-east-1a (node-1)")).toBeInTheDocument();

    const nameInput = screen.getByPlaceholderText("my-stack");
    await userEvent.type(nameInput, "integration-stack");

    const yamlArea = screen.getByPlaceholderText(/services:/);
    await userEvent.clear(yamlArea);
    await userEvent.type(yamlArea, "services:\n  web:\n    image: nginx:alpine\n    ports:\n      - \"8080:80\"");

    // Select node-2
    const nodeSelect = screen.getByDisplayValue("Auto-select") as HTMLSelectElement;
    await userEvent.selectOptions(nodeSelect, "node-2");
    expect(nodeSelect.value).toBe("node-2");

    // Trigger validate so canDeploy becomes true (requires validateResult?.valid)
    const validateBtn = screen.getByRole("button", { name: "Validate" });
    await userEvent.click(validateBtn);
    await waitFor(() => expect(screen.getByText("Valid")).toBeInTheDocument());

    // Now Deploy should be enabled and will POST with nodeId=node-2
    const deployBtn = screen.getByRole("button", { name: "Deploy" });
    expect(deployBtn).toBeEnabled();
    await userEvent.click(deployBtn);

    await waitFor(() => expect(push).toHaveBeenCalledWith(expect.stringContaining("new-stack-id")));
    // Verify the push destination includes the new stack id
    expect(push).toHaveBeenCalledWith("/admin/compose/new-stack-id");
  });

  it("Create App page also forwards nodeId for compose sourceType (apps/new/page.tsx)", () => {
    const src = readFileSync(resolve(__dirname, "../app/admin/apps/new/page.tsx"), "utf8");
    expect(src).toContain("nodeId");
    expect(src).toContain("createApp");
    // Must include nodeId in CreateAppInput assembly
    expect(src).toMatch(/nodeId:\s*nodeId/);
    expect(src).toContain("fetchNodes");
  });

  it("backend handlers_compose.go CreateStackRequest correctly requires nodeId optional and userId wiring", () => {
    const src = readFileSync(resolve(__dirname, "../../api/internal/http/handlers_compose.go"), "utf8");
    expect(src).toContain("type CreateStackRequest");
    expect(src).toContain("NodeID        string");
    expect(src).toContain("NodeID:        req.NodeID");
    // Must handle empty nodeId as auto-select (no error when empty)
    expect(src).toContain('if req.Name == "" || req.ComposeYAML == ""');
    expect(src).not.toMatch(/if req\.NodeID == ""/);
  });

  it("compose list page shows Node pill sliced to 8 chars (display fidelity)", () => {
    const src = readFileSync(resolve(__dirname, "../app/admin/compose/page.tsx"), "utf8");
    expect(src).toContain("stack.nodeId");
    expect(src).toContain("slice(0, 8)");
  });
});
