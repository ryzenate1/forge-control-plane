import { redirect } from "next/navigation";

/**
 * Alias route: the private certificate authority and mutual-TLS identities live
 * at `/admin/mtls`. Kept so an old bookmark resolves rather than 404-ing.
 */
export default function MtlsCaAlias() {
  redirect("/admin/mtls");
}
