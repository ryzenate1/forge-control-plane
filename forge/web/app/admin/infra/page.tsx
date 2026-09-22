import { redirect } from "next/navigation";

/** Legacy group deep-link; see `app/admin/beacons/page.tsx` for why 307. */
export default function InfraAlias() {
  redirect("/admin/nodes");
}
