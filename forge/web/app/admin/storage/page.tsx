import { redirect } from "next/navigation";

/** Vocabulary alias: "Storage" resolved to the mounts list. 307, repointable. */
export default function StorageAlias() {
  redirect("/admin/mounts");
}
