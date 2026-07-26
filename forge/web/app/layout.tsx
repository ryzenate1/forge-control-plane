import type { Metadata, Viewport } from "next";
import { cookies, headers } from "next/headers";
import { JetBrains_Mono, Plus_Jakarta_Sans } from "next/font/google";
import { Providers } from "@/components/providers";
import { themeScript } from "@/components/theme-provider";
import "./globals.css";

export const metadata: Metadata = {
  title: { default: "Forge Control Plane", template: "%s · Forge Control Plane" },
  description: "Secure game server management control plane",
  applicationName: "Forge Control Plane",
  icons: { icon: "/favicon.ico" },
  robots: { index: false, follow: false },
};

export const viewport: Viewport = {
  themeColor: "#090d14",
};

const rtlLocales = new Set(["ar", "he", "fa", "ur", "yi"]);
const sans = Plus_Jakarta_Sans({ subsets: ["latin"], variable: "--font-sans" });
const mono = JetBrains_Mono({ subsets: ["latin"], variable: "--font-mono" });

export default async function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const cookieStore = await cookies();
  const nonce = (await headers()).get("x-csp-nonce") ?? undefined;
  const locale = cookieStore.get("NEXT_LOCALE")?.value ?? "en";
  const baseLang = locale.split("-")[0];
  const dir = rtlLocales.has(baseLang) ? "rtl" : "ltr";

  return (
    <html lang={locale} dir={dir} suppressHydrationWarning>
      <head>
        <script nonce={nonce} dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body className={`${sans.variable} ${mono.variable}`}>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
