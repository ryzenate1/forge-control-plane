import type { Metadata } from "next";
import { Manrope, DM_Mono, Newsreader } from "next/font/google";
import "./globals.css";
import "./extra.css";

const manrope = Manrope({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-manrope",
  weight: ["400", "500", "600", "700", "800"]
});

const dmMono = DM_Mono({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-dm-mono",
  weight: ["400", "500"]
});

const newsreader = Newsreader({
  subsets: ["latin"],
  display: "swap",
  variable: "--font-newsreader",
  weight: ["500", "600"]
});

export const metadata: Metadata = {
  title: {
    default: "Forge Documentation",
    template: "%s | Forge",
  },
  description: "The operator guide for the Forge Control Plane.",
  icons: {
    icon: "/favicon.svg",
  },
  openGraph: {
    title: "Forge Documentation",
    description: "The operator guide for the Forge Control Plane.",
    type: "website",
    siteName: "Forge",
    url: '/',
  },
  twitter: {
    card: 'summary',
  },
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" className={`${manrope.variable} ${dmMono.variable} ${newsreader.variable}`}>
      <body>
        <a
          href="#main-content"
          className="sr-only focus:not-sr-only focus:absolute focus:top-2 focus:left-2 focus:z-50 focus:bg-white focus:px-4 focus:py-2 focus:border focus:rounded focus:text-ink"
        >
          Skip to content
        </a>
        {children}
      </body>
    </html>
  );
}
