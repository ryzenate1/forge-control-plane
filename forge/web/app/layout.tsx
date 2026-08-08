import type { Metadata, Viewport } from "next";
import { cookies, headers } from "next/headers";
import { Providers } from "@/components/providers";
import { themeScript } from "@/components/theme-provider";
import { mono, sans } from "./fonts";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "Forge Control Plane", template: "%s · Forge Control Plane" },
  description: "Forge — secure game server management control plane",
  applicationName: "Forge Control Plane",
  icons: { icon: "/favicon.svg", apple: "/favicon.svg" },
  openGraph: {
    title: "Forge Control Plane",
    description: "Forge — secure game server management control plane",
    type: "website",
    siteName: "Forge Control Plane",
    images: [{ url: "/og.svg", width: 1200, height: 630, alt: "Forge Control Plane" }],
  },
  robots: { index: false, follow: false },
};

export const viewport: Viewport = {
  themeColor: "#0a0e16",
};

const rtlLocales = new Set(["ar", "he", "fa", "ur", "yi"]);

export default async function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const cookieStore = await cookies();
  const nonce = (await headers()).get("x-csp-nonce") ?? undefined;
  const locale = cookieStore.get("NEXT_LOCALE")?.value ?? "en";
  const baseLang = locale.split("-")[0];
  const dir = rtlLocales.has(baseLang) ? "rtl" : "ltr";

  return (
    <html lang={locale} dir={dir} suppressHydrationWarning>
      <head>
        <script nonce={nonce} dangerouslySetInnerHTML={{ __html: themeScript }} suppressHydrationWarning />
      </head>
      <body className={`${sans.variable} ${mono.variable}`}>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
