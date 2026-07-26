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
        "lib/**/*.ts",
        "lib/**/*.tsx",
        "components/**/*.tsx",
        "stores/**/*.ts",
        "app/**/*.tsx",
        "middleware.ts",
      ],
      exclude: [
        "test/**",
        "coverage/**",
        "node_modules/**",
        "**/*.d.ts",
        "**/*.config.*",
        "app/api/**",
        "next-env.d.ts",
      ],
      thresholds: {
        lines: 25,
        functions: 20,
        branches: 15,
        statements: 25,
      },
    },
  },
});
