import { redirect } from "next/navigation";

/** Legacy group deep-link → Nodes. 307 so the alias stays repointable. */
export default function InfraBeaconsAlias() {
  redirect("/admin/nodes");
}
