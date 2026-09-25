'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { fetchOrganizations, createOrganization } from '@/lib/api/tenancy';
import type { Organization } from '@/lib/api/tenancy';
import { useT } from '@/components/TranslationProvider';

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export default function OrganizationsPage() {
  const t = useT();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');

  // Server state lives in react-query only — same ["organizations"] key the
  // TenancyHydrator already populates, so this screen shares the cached list
  // instead of re-fetching into a duplicate zustand copy.
  const orgsQuery = useQuery({
    queryKey: ['organizations'],
    queryFn: fetchOrganizations,
    staleTime: 60_000,
  });

  const createMut = useMutation({
    mutationFn: (input: { name: string; slug?: string }) =>
      createOrganization(input.name, input.slug),
    // The draft is cleared and the list refreshed only once the API has
    // confirmed the create; a rejected request keeps the form and surfaces
    // the error so a failed operation never looks like a success.
    onSuccess: (org) => {
      void queryClient.invalidateQueries({ queryKey: ['organizations'] });
      setName('');
      setSlug('');
      router.push(`/organizations/${org.slug}`);
    },
  });

  const organizations = orgsQuery.data ?? [];
  const loading = orgsQuery.isPending;
  const creating = createMut.isPending;
  const error = orgsQuery.isError
    ? errorMessage(orgsQuery.error)
    : createMut.isError
      ? errorMessage(createMut.error)
      : null;

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    createMut.mutate({ name: name.trim(), slug: slug.trim() || undefined });
  };

  return (
    <div className="mx-auto max-w-4xl px-4 py-8">
      <div className="mb-8 flex items-center justify-between">
        <h1 className="text-2xl font-bold text-white">{t('organizations.title')}</h1>
      </div>

      {error && (
        <div role="alert" className="mb-4 flex items-center justify-between gap-3 rounded-lg border border-red-500/20 bg-red-500/10 px-4 py-3 text-red-400 text-sm">
          <span>{error}</span>
          {orgsQuery.isError && (
            <button
              type="button"
              onClick={() => void orgsQuery.refetch()}
              className="rounded px-2 py-1 text-xs underline hover:bg-white/[0.06]"
            >
              {t('common.retry')}
            </button>
          )}
        </div>
      )}

      <form onSubmit={handleCreate} className="mb-8 rounded-xl border border-white/10 bg-white/5 p-6">
        <h2 className="mb-4 text-lg font-semibold text-white">{t('organizations.create.title')}</h2>
        <div className="flex gap-3">
          <input
            type="text"
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              if (!slug) setSlug(e.target.value.toLowerCase().replace(/\s+/g, '-'));
            }}
            placeholder={t('organizations.create.namePlaceholder')}
            className="flex-1 rounded-lg border border-white/10 bg-black/30 px-4 py-2 text-white text-sm placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none"
            required
          />
          <input
            type="text"
            value={slug}
            onChange={(e) => setSlug(e.target.value.toLowerCase().replace(/\s+/g, '-'))}
            placeholder={t('organizations.create.slugPlaceholder')}
            className="w-48 rounded-lg border border-white/10 bg-black/30 px-4 py-2 text-white text-sm placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none"
          />
          <button
            type="submit"
            disabled={creating || !name.trim()}
            className="rounded-lg bg-purple-600 px-6 py-2 text-sm font-medium text-white hover:bg-purple-500 disabled:opacity-50"
          >
            {creating ? t('organizations.create.creating') : t('common.create')}
          </button>
        </div>
      </form>

      {loading && (
        <div className="text-center text-gray-400 py-12">{t('common.loading')}</div>
      )}

      {!loading && !orgsQuery.isError && organizations.length === 0 && (
        <div className="rounded-xl border border-white/10 bg-white/5 p-12 text-center text-gray-400">
          {t('organizations.list.emptyState')}
        </div>
      )}

      <div className="grid gap-4 sm:grid-cols-2">
        {organizations.map((org: Organization) => (
          <Link
            key={org.id}
            href={`/organizations/${org.slug}`}
            className="rounded-xl border border-white/10 bg-white/5 p-5 transition hover:border-purple-500/30 hover:bg-white/[0.07]"
          >
            <h3 className="text-lg font-semibold text-white">{org.name}</h3>
            <p className="mt-1 text-sm text-gray-400">{org.slug}</p>
            <p className="mt-2 text-xs text-gray-500">{t('organizations.ownerLabel', { name: org.ownerName })}</p>
          </Link>
        ))}
      </div>
    </div>
  );
}
