'use client';

import { useState, useEffect, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { useTenancyStore } from '@/stores/use-tenancy-store';
import { fetchOrganizations, createOrganization } from '@/lib/api/tenancy';
import type { Organization } from '@/lib/api/tenancy';

export default function OrganizationsPage() {
  const router = useRouter();
  const {
    organizations, setOrganizations,
    loading, setLoading, error, setError,
  } = useTenancyStore();
  const [name, setName] = useState('');
  const [slug, setSlug] = useState('');
  const [creating, setCreating] = useState(false);

  const fetchOrgs = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await fetchOrganizations();
      setOrganizations(data);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [setLoading, setError, setOrganizations]);

  useEffect(() => { fetchOrgs(); }, [fetchOrgs]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setCreating(true);
    try {
      const org = await createOrganization(name.trim(), slug.trim() || undefined);
      router.push(`/organizations/${org.slug}`);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="mx-auto max-w-4xl px-4 py-8">
      <div className="mb-8 flex items-center justify-between">
        <h1 className="text-2xl font-bold text-white">Organizations</h1>
      </div>

      {error && (
        <div className="mb-4 rounded-lg border border-red-500/20 bg-red-500/10 px-4 py-3 text-red-400 text-sm">
          {error}
        </div>
      )}

      <form onSubmit={handleCreate} className="mb-8 rounded-xl border border-white/10 bg-white/5 p-6">
        <h2 className="mb-4 text-lg font-semibold text-white">Create Organization</h2>
        <div className="flex gap-3">
          <input
            type="text"
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              if (!slug) setSlug(e.target.value.toLowerCase().replace(/\s+/g, '-'));
            }}
            placeholder="Organization name"
            className="flex-1 rounded-lg border border-white/10 bg-black/30 px-4 py-2 text-white text-sm placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none"
            required
          />
          <input
            type="text"
            value={slug}
            onChange={(e) => setSlug(e.target.value.toLowerCase().replace(/\s+/g, '-'))}
            placeholder="slug"
            className="w-48 rounded-lg border border-white/10 bg-black/30 px-4 py-2 text-white text-sm placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none"
          />
          <button
            type="submit"
            disabled={creating || !name.trim()}
            className="rounded-lg bg-purple-600 px-6 py-2 text-sm font-medium text-white hover:bg-purple-500 disabled:opacity-50"
          >
            {creating ? 'Creating...' : 'Create'}
          </button>
        </div>
      </form>

      {loading && organizations.length === 0 && (
        <div className="text-center text-gray-400 py-12">Loading...</div>
      )}

      {!loading && organizations.length === 0 && (
        <div className="rounded-xl border border-white/10 bg-white/5 p-12 text-center text-gray-400">
          No organizations yet. Create one to get started.
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
            <p className="mt-2 text-xs text-gray-500">Owner: {org.ownerName}</p>
          </Link>
        ))}
      </div>
    </div>
  );
}
