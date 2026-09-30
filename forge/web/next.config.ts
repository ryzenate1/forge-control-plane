import type { NextConfig } from "next";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const apiInternalUrl = (process.env.API_INTERNAL_URL ?? "http://127.0.0.1:8080").replace(/\/$/, "");
const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");

const nextConfig: NextConfig = {
  reactStrictMode: true,
  output: "standalone",
  outputFileTracingRoot: projectRoot,
  images: {
    // No remote optimizer patterns: branding artwork renders through <img>
    // and app-store icons pass `unoptimized`, so every remote host must be
    // allow-listed here before next/image is used for it. A '**' wildcard
    // would let the optimizer fetch from any host on the internet.
    remotePatterns: [],
  },
  async rewrites() {
    return [
      {
        source: "/api/i18n/:path*",
        destination: `/api/i18n/:path*`,
      },
      {
        source: "/api/:path*",
        destination: `${apiInternalUrl}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
