import { ServerConsoleLayout } from "@/components/server/server-console-layout";

export default function ServerLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <ServerConsoleLayout>{children}</ServerConsoleLayout>;
}
