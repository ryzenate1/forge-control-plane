import type { Metadata, Viewport } from "next";
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

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
      </head>
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
