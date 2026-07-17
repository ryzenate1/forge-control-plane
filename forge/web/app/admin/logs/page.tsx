import { permanentRedirect } from "next/navigation";

export default function LogsPage() {
  // Logs are consolidated under activity; retain this URL as a permanent alias.
  permanentRedirect("/admin/activity");
}