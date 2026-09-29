import { redirect } from "next/navigation";

/**
 * Alias route: the legacy compatibility-template catalogue lives at
 * `/admin/compatibility-templates`.
 *
 * This page used to render the same component as that route, which meant two
 * URLs for one screen — and only one of them was registered in
 * `admin-registry.ts`, so the unregistered half highlighted nothing in the
 * sidebar and collapsed the breadcrumb to the two-crumb fallback. One route
 * owns the screen; this one forwards to it.
 */
export default function TemplatesAlias() {
  redirect("/admin/compatibility-templates");
}
