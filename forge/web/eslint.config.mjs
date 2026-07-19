import { FlatCompat } from "@eslint/eslintrc";
import { dirname } from "path";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const compat = new FlatCompat({
  baseDirectory: __dirname,
});

const eslintConfig = [
  {
    ignores: [
      ".next/**",
      "coverage/**",
      "next-env.d.ts",
      "tsconfig.tsbuildinfo",
      "lib/api.ts",
      "lib/api/*.ts",
      "components/admin/AdminMounts.tsx",
      "components/admin/AdminServers.tsx",
      "components/server/schedules-view.tsx",
      "components/admin/AdminOperations.tsx",
      "components/admin/AdminOverview.tsx",
      "components/admin/AdminWebhooks.tsx",
    ],
  },
  ...compat.extends("next/core-web-vitals", "next/typescript"),
  {
    files: [
      "lib/api.ts",
      "lib/api/*.ts",
      "components/admin/AdminMounts.tsx",
      "components/admin/AdminServers.tsx",
      "components/server/schedules-view.tsx",
      "components/server/startup-view.tsx",
      "components/server/users-view.tsx",
      "components/admin/AdminOperations.tsx",
      "components/admin/AdminOverview.tsx",
      "components/admin/AdminWebhooks.tsx",
    ],
    rules: {
      "@typescript-eslint/no-explicit-any": "off",
      "@typescript-eslint/no-unused-vars": "warn",
    },
  },
];

export default eslintConfig;
