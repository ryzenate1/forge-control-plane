// Fonts follow the AGENTS.md convention: Manrope (UI) + JetBrains Mono
// (technical values). They are wired through next/font so they are self-served
// by the Next.js runtime (no external Google Fonts requests at page runtime).
//
// Self-hosting via `next/font/local` requires the Manrope / JetBrains Mono
// woff2 files to be vendored into the repo under `public/fonts/`; they are not
// present in this checkout, so we use `next/font/google` here to avoid
// fabricating import paths that would break the build. TODO: vendor the woff2
// files and migrate these to `next/font/local` for full self-hosting.
//
// There is no third display face: headings, numbers and code all use the two
// tokens above (`--font-sans` / `--font-mono`). `display` below is kept as an
// alias of `sans` so existing `display.variable` class hooks keep working
// without pulling a Space_Grotesk payload.
import { Manrope, JetBrains_Mono } from "next/font/google";

export const sans = Manrope({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
  display: "swap",
  variable: "--font-sans",
});

export const display = Manrope({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
  display: "swap",
  variable: "--font-display",
});

export const mono = JetBrains_Mono({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-mono",
});

