import { notFound } from "next/navigation";
import { DocArticle } from "@/components/docs/doc-article";
import { DocsShell } from "@/components/docs/docs-shell";
import { docBySlug, docs } from "@/lib/docs";

export function generateStaticParams() { return docs.map((page) => ({ slug: page.slug })); }

export default async function DocumentationPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const page = docBySlug(slug);
  if (!page) notFound();
  return <DocsShell slug={page.slug}><DocArticle page={page} /></DocsShell>;
}
