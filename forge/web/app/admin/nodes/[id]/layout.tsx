import type { Metadata } from "next";

export const metadata: Metadata = { title: "Beacon — Forge Admin" };

export default function AdminNodeDetailLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
