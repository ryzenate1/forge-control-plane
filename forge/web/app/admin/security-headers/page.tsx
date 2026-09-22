import { redirect } from "next/navigation";

/**
 * Alias route: security-header policy is managed under Access, at
 * `/admin/security`. The route moved when header policy was recognised as an
 * authorization surface rather than a traffic surface; this keeps old deep
 * links and bookmarks loading instead of 404-ing behind a correctly
 * highlighted sidebar row.
 */
export default function SecurityHeadersAlias() {
  redirect("/admin/security");
}
