import { redirect } from "next/navigation";

/** Legacy group deep-link → Storage Mounts. 307 so the alias stays repointable. */
export default function InfraStorageAlias() {
  redirect("/admin/mounts");
}
