import { describe, expect, expectTypeOf, it } from "vitest";
import {
  createNode,
  fetchNodeLifecycle,
  updateNode,
  type ApiNode,
  type ApiNodeLifecycle,
  type CreateNodeInput,
  type UpdateNodeInput,
} from "@/lib/api";
import { jsonResponse, mockFetch, requestJSON } from "@/test/fetch-mock";

describe("node API contract", () => {
  it("returns the one-time create token alongside the created node", async () => {
    const node = { id: "node-1", name: "Amsterdam" } as ApiNode;
    const { calls } = mockFetch(jsonResponse({ node, token: "onboarding-token" }));

    const result = await createNode({
      name: "Amsterdam",
      region: "ams",
      regionId: "region-1",
      locationId: "location-1",
      fqdn: "node.example.test",
      baseUrl: "https://node.example.test",
    });

    expectTypeOf(result).toEqualTypeOf<{ node: ApiNode; token: string }>();
    expect(result).toEqual({ node, token: "onboarding-token" });
    expect(calls[0].url).toContain("/nodes");
    expect(calls[0].init.method).toBe("POST");
    expect(requestJSON(calls[0])).toEqual({
      name: "Amsterdam",
      region: "ams",
      regionId: "region-1",
      locationId: "location-1",
      fqdn: "node.example.test",
      baseUrl: "https://node.example.test",
    });
  });

  it("sends the supplied node patch verbatim, including false and zero values", async () => {
    const patch: UpdateNodeInput = {
      name: "Draining node",
      locationId: "location-2",
      behindProxy: false,
      daemonListen: 0,
      memoryMb: 0,
      draining: false,
      maintenanceMode: false,
    };
    const { calls } = mockFetch(jsonResponse({ id: "node/1" }));

    await updateNode("node/1", patch);

    expect(calls[0].url).toContain("/nodes/node%2F1");
    expect(calls[0].init.method).toBe("PATCH");
    expect(requestJSON(calls[0])).toEqual(patch);
  });

  it("fetches and returns the node lifecycle resource without reshaping it", async () => {
    const lifecycle: ApiNodeLifecycle = {
      node: { id: "node/1", name: "Amsterdam", region: "ams", status: "online" },
      health: { cpu: "healthy", memory: "healthy", disk: "healthy", network: "healthy", runtime: "healthy" },
      healthScore: { cpu: 100, memory: 100, disk: 100, heartbeat: 100, status: 100, total: 100 },
      capacity: {
        nodeId: "node/1",
        allocated_cpu: 1,
        available_cpu: 7,
        allocated_memory: 1_000,
        available_memory: 11_000,
        allocated_disk: 100,
        available_disk: 900,
        server_count: 1,
        updated_at: "2026-07-17T00:00:00Z",
      },
      draining: false,
      maintenance: false,
      placementEligible: true,
    };
    const { calls } = mockFetch(jsonResponse(lifecycle));

    await expect(fetchNodeLifecycle("node/1")).resolves.toStrictEqual(lifecycle);

    expect(calls[0].url).toContain("/nodes/node%2F1/lifecycle");
    expect(calls[0].init.method).toBeUndefined();
  });

  it("keeps the public create and update input types distinct", () => {
    expectTypeOf<CreateNodeInput>().toMatchTypeOf<Required<Pick<CreateNodeInput, "name" | "region" | "fqdn">>>();
    expectTypeOf<UpdateNodeInput>().not.toMatchTypeOf<CreateNodeInput>();
  });
});
