import { redirect } from "next/navigation";

/** Legacy group deep-link → Endpoints. 307 so the alias stays repointable. */
export default function InfraNetworkingAlias() {
  redirect("/admin/endpoints");
}
