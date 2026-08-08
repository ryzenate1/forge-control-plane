import { describe, expect, it, beforeEach } from "vitest";
import { useServerStore } from "./use-server-store";

beforeEach(() => {
  useServerStore.getState().reset();
});

describe("useServerStore", () => {
  describe("initial state", () => {
    it("has expected defaults", () => {
      const state = useServerStore.getState();
      expect(state.currentUser).toBeNull();
      expect(state.mode).toBe("server");
      expect(state.activeTab).toBe("console");
      expect(state.adminTab).toBe("overview");
      expect(state.selectedServerId).toBeNull();
      expect(state.consoleLines).toEqual([]);
      expect(state.consoleStatus).toBe("Disconnected");
      expect(state.liveStats).toBeNull();
      expect(state.cpuHistory).toHaveLength(24);
      expect(state.memoryHistory).toHaveLength(24);
      expect(state.cpuHistory.every((v: number) => v === 0)).toBe(true);
      expect(state.memoryHistory.every((v: number) => v === 0)).toBe(true);
    });
  });

  describe("setCurrentUser", () => {
    it("sets the user", () => {
      const user = { id: "u1", email: "a@b.com", role: "admin" };
      useServerStore.getState().setCurrentUser(user);
      expect(useServerStore.getState().currentUser).toEqual(user);
    });

    it("clears the user", () => {
      useServerStore.getState().setCurrentUser({ id: "u1", email: "a@b.com", role: "admin" });
      useServerStore.getState().setCurrentUser(null);
      expect(useServerStore.getState().currentUser).toBeNull();
    });
  });

  describe("setMode", () => {
    it("sets mode to admin", () => {
      useServerStore.getState().setMode("admin");
      expect(useServerStore.getState().mode).toBe("admin");
    });

    it("sets mode to server", () => {
      useServerStore.getState().setMode("admin");
      useServerStore.getState().setMode("server");
      expect(useServerStore.getState().mode).toBe("server");
    });
  });

  describe("setActiveTab", () => {
    it("sets the active tab", () => {
      useServerStore.getState().setActiveTab("files");
      expect(useServerStore.getState().activeTab).toBe("files");
    });
  });

  describe("setAdminTab", () => {
    it("sets the admin tab", () => {
      useServerStore.getState().setAdminTab("users");
      expect(useServerStore.getState().adminTab).toBe("users");
    });
  });

  describe("setSelectedServerId", () => {
    it("sets the server id", () => {
      useServerStore.getState().setSelectedServerId("s42");
      expect(useServerStore.getState().selectedServerId).toBe("s42");
    });

    it("resets console state and active tab when selecting a server", () => {
      useServerStore.getState().addConsoleLine("old line");
      useServerStore.getState().setActiveTab("files");
      useServerStore.getState().setSelectedServerId("s99");
      expect(useServerStore.getState().consoleLines).toEqual([]);
      expect(useServerStore.getState().consoleStatus).toBe("Connecting");
      expect(useServerStore.getState().activeTab).toBe("console");
    });

    it("clears server id", () => {
      useServerStore.getState().setSelectedServerId("s42");
      useServerStore.getState().setSelectedServerId(null);
      expect(useServerStore.getState().selectedServerId).toBeNull();
    });
  });

  describe("console actions", () => {
    it("addConsoleLine appends a line", () => {
      useServerStore.getState().addConsoleLine("hello");
      expect(useServerStore.getState().consoleLines).toEqual(["hello"]);
    });

    it("addConsoleLine caps at 300 lines", () => {
      for (let i = 0; i < 350; i++) useServerStore.getState().addConsoleLine(`line-${i}`);
      const lines = useServerStore.getState().consoleLines;
      expect(lines).toHaveLength(300);
      expect(lines[0]).toBe("line-50");
      expect(lines[299]).toBe("line-349");
    });

    it("addConsoleLines appends multiple and caps", () => {
      for (let i = 0; i < 290; i++) useServerStore.getState().addConsoleLine(`old-${i}`);
      useServerStore.getState().addConsoleLines(["a", "b", "c"]);
      const lines = useServerStore.getState().consoleLines;
      expect(lines).toHaveLength(293);
      expect(lines[290]).toBe("a");
      expect(lines[292]).toBe("c");
    });

    it("addConsoleLines caps total at 300", () => {
      for (let i = 0; i < 295; i++) useServerStore.getState().addConsoleLine(`old-${i}`);
      useServerStore.getState().addConsoleLines(Array.from({ length: 20 }, (_, i) => `new-${i}`));
      const lines = useServerStore.getState().consoleLines;
      expect(lines).toHaveLength(300);
      expect(lines[0]).toBe("old-15");
    });

    it("clearConsole empties the lines", () => {
      useServerStore.getState().addConsoleLine("a");
      useServerStore.getState().clearConsole();
      expect(useServerStore.getState().consoleLines).toEqual([]);
    });

    it("setConsoleStatus sets the status", () => {
      useServerStore.getState().setConsoleStatus("Connected");
      expect(useServerStore.getState().consoleStatus).toBe("Connected");
    });
  });

  describe("updateStats", () => {
    it("records stats and pushes to history", () => {
      useServerStore.getState().updateStats({ cpuPercent: 50, memoryBytes: 500 * 1024 * 1024, memoryLimit: 1024 * 1024 * 1024, diskBytes: 1000 });
      const state = useServerStore.getState();
      expect(state.liveStats).toEqual({ cpuPercent: 50, memoryBytes: 500 * 1024 * 1024, memoryLimit: 1024 * 1024 * 1024, diskBytes: 1000 });
      expect(state.cpuHistory[23]).toBe(50);
      expect(state.memoryHistory[23]).toBe(49);
    });

    it("caps cpuHistory and memoryHistory at 24 entries", () => {
      for (let i = 0; i < 30; i++) {
        useServerStore.getState().updateStats({ cpuPercent: i, memoryBytes: 0, memoryLimit: 1, diskBytes: 0 });
      }
      expect(useServerStore.getState().cpuHistory).toHaveLength(24);
      expect(useServerStore.getState().memoryHistory).toHaveLength(24);
      expect(useServerStore.getState().cpuHistory[23]).toBe(29);
      expect(useServerStore.getState().cpuHistory[0]).toBe(6);
    });

    it("clamps cpuPercent to 300", () => {
      useServerStore.getState().updateStats({ cpuPercent: 500, memoryBytes: 0, memoryLimit: 1, diskBytes: 0 });
      expect(useServerStore.getState().cpuHistory[23]).toBe(300);
    });

    it("clamps memory percentage to 100", () => {
      useServerStore.getState().updateStats({ cpuPercent: 0, memoryBytes: 2 * 1024 * 1024 * 1024, memoryLimit: 1024 * 1024 * 1024, diskBytes: 0 });
      expect(useServerStore.getState().memoryHistory[23]).toBe(100);
    });

    it("handles zero memoryLimit", () => {
      useServerStore.getState().updateStats({ cpuPercent: 10, memoryBytes: 100, memoryLimit: 0, diskBytes: 0 });
      expect(useServerStore.getState().memoryHistory[23]).toBe(0);
    });
  });

  describe("reset", () => {
    it("restores all fields to initial state", () => {
      useServerStore.getState().setCurrentUser({ id: "u1", email: "a@b.com", role: "admin" });
      useServerStore.getState().setMode("admin");
      useServerStore.getState().setActiveTab("files");
      useServerStore.getState().setSelectedServerId("s1");
      useServerStore.getState().addConsoleLine("test");
      useServerStore.getState().updateStats({ cpuPercent: 90, memoryBytes: 100, memoryLimit: 200, diskBytes: 50 });

      useServerStore.getState().reset();

      const state = useServerStore.getState();
      expect(state.currentUser).toBeNull();
      expect(state.mode).toBe("server");
      expect(state.activeTab).toBe("console");
      expect(state.selectedServerId).toBeNull();
      expect(state.consoleLines).toEqual([]);
      expect(state.consoleStatus).toBe("Disconnected");
      expect(state.liveStats).toBeNull();
      expect(state.cpuHistory).toEqual(Array(24).fill(0));
      expect(state.memoryHistory).toEqual(Array(24).fill(0));
    });
  });
});
