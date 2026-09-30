import { redirect } from "next/navigation";

export default async function AdminTemplatesPage({ searchParams }: { searchParams: Promise<{ nestId?: string }> }) {
  const { nestId } = await searchParams;
  redirect(`/admin/compatibility-templates${nestId ? `?nestId=${encodeURIComponent(nestId)}` : ""}`);
}
