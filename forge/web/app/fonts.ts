// Fonts follow the AGENTS.md convention: Manrope (UI) + JetBrains Mono
// (technical values). They are wired through next/font so they are self-served
// by the Next.js runtime (no external Google Fonts requests at page runtime).
//
// Self-hosting via `next/font/local` requires the Manrope / JetBrains Mono
// woff2 files to be vendored into the repo; they are not present under
// `public/` or anywhere in the tree, so we use `next/font/google` here to avoid
// fabricating import paths that would break the build. TODO: vendor the woff2
// files and migrate these to `next/font/local` for full self-hosting.
import { Manrope, JetBrains_Mono, Space_Grotesk } from "next/font/google";

export const sans = Manrope({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
  display: "swap",
  variable: "--font-sans",
});

export const display = Space_Grotesk({
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

