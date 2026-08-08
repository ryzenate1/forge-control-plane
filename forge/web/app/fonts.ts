import { JetBrains_Mono, Manrope } from "next/font/google";

export const sans = Manrope({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-sans",
});

export const mono = JetBrains_Mono({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-mono",
});
