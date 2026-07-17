import { redirect } from "next/navigation";

// Redirect /admin → /admin/overview to avoid duplicate content.
export default function AdminPage() {
  redirect("/admin/overview");
}
