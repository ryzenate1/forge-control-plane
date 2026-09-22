import { permanentRedirect } from "next/navigation";

export default function GitProvidersAlias() {
  // Provider connect/disconnect lives in the Git Integrations Providers tab;
  // this alias preserves deep links and bookmarks.
  permanentRedirect("/admin/git?tab=providers");
}
