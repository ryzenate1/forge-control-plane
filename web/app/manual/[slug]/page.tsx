import { notFound } from "next/navigation";
import { DocsShell } from "@/components/docs/docs-shell";
import { ManualArticle } from "@/components/manual/manual-article";
import { manualFeatureBySlug, manualFeatures } from "@/lib/features";

export function generateStaticParams() { return manualFeatures.map((feature) => ({ slug: feature.slug })); }

export default async function ManualFeaturePage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const feature = manualFeatureBySlug(slug);
  if (!feature) notFound();
  return <DocsShell><ManualArticle feature={feature} /></DocsShell>;
}
