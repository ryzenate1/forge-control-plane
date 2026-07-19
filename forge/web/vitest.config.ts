import path from "node:path";
import { defineConfig } from "vitest/config";

export default defineConfig({
  esbuild: { jsx: "automatic" },
  resolve: {
    alias: { "@": path.resolve(__dirname) },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./test/setup.ts"],
    css: false,
    coverage: {
      provider: "v8",
      reporter: ["text", "json-summary", "html"],
      include: [
        "lib/api.ts",
        "components/providers.tsx",
        "components/admin/admin-shell.tsx",
        "components/server/{backups-view,network-view,files-view,users-view}.tsx",
        "app/{page,setup/page}.tsx",
      ],
      thresholds: {
        lines: 25,
        functions: 20,
        branches: 30,
        statements: 25,
      },
    },
  },
});
