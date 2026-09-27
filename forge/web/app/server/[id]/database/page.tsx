import { permanentRedirect } from "next/navigation";

/**
 * Compat alias for the old managed-database-services route.
 *
 * `/server/<id>/database` and `/server/<id>/databases` were two different
 * features one character apart: the former listed managed database *services*
 * linked to the server, the latter the server's own databases. Both appeared in
 * the sidebar as "Database" and "Databases", which is not a distinction anyone
 * can act on. The services view now sits under the databases section at
 * `/databases/services`; this alias keeps existing links and bookmarks working.
 */
export default async function ServerDatabaseAlias({
  params,
}: {
  params: Promise<{ id: string }>;
}): Promise<never> {
  const { id } = await params;
  return permanentRedirect(`/server/${id}/databases/services`);
}
