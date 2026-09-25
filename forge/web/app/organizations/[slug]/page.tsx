'use client';

import { useState } from 'react';
import { useParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  fetchOrganization,
  fetchProjects,
  createProject,
  fetchTeamMembers,
  addTeamMember,
  removeTeamMember,
  deleteOrganization,
  fetchOrgServers,
} from '@/lib/api/tenancy';
import { Pagination } from '@/components/ui/primitives';

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

const PROJECTS_KEY = ['organization-projects'] as const;
const MEMBERS_KEY = ['organization-members'] as const;

export default function OrganizationDetailPage() {
  const params = useParams<{ slug: string }>();
  const router = useRouter();
  const [tab, setTab] = useState<'projects' | 'members' | 'servers'>('projects');

  const orgQuery = useQuery({
    queryKey: ['organization', params.slug],
    queryFn: () => fetchOrganization(params.slug),
  });
  const org = orgQuery.data ?? null;
  const orgId = org?.id ?? '';

  // Counted in the tab labels, listed by the tabs themselves — both read the
  // same cache entries, so there is one request per collection.
  const projectsQuery = useQuery({
    queryKey: [...PROJECTS_KEY, orgId],
    queryFn: () => fetchProjects(orgId),
    enabled: !!org,
  });
  const membersQuery = useQuery({
    queryKey: [...MEMBERS_KEY, orgId],
    queryFn: () => fetchTeamMembers(orgId),
    enabled: !!org,
  });

  const deleteMut = useMutation({
    mutationFn: () => deleteOrganization(orgId),
    onSuccess: () => router.push('/organizations'),
  });

  const handleDelete = () => {
    if (!org) return;
    if (!confirm(`Delete organization "${org.name}"? This cannot be undone.`)) return;
    deleteMut.mutate();
  };

  if (orgQuery.isPending) return <div className="mx-auto max-w-4xl px-4 py-8 text-gray-400">Loading...</div>;
  if (orgQuery.isError) {
    return (
      <div className="mx-auto max-w-4xl space-y-4 px-4 py-8">
        <div className="rounded-lg border border-red-500/20 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          Could not load this organization: {errorMessage(orgQuery.error)}
        </div>
        <div className="flex gap-3">
          <button
            onClick={() => void orgQuery.refetch()}
            className="rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--brand-hover)]"
          >
            Retry
          </button>
          <Link href="/organizations" className="rounded-lg border border-white/10 px-4 py-2 text-sm text-gray-300 hover:bg-white/5">
            Back to organizations
          </Link>
        </div>
      </div>
    );
  }
  if (!org) return null;

  const projectCount = projectsQuery.data?.length ?? 0;
  const memberCount = membersQuery.data?.length ?? 0;

  return (
    <div className="mx-auto max-w-4xl px-4 py-8">
      <div className="mb-6 flex items-start justify-between">
        <div>
          <Link href="/organizations" className="text-sm text-gray-500 hover:text-gray-300">← Organizations</Link>
          <h1 className="mt-2 text-2xl font-bold text-white">{org.name}</h1>
          <p className="text-sm text-gray-400">Owner: {org.ownerName}</p>
        </div>
        <button
          onClick={handleDelete}
          disabled={deleteMut.isPending}
          className="rounded-lg border border-red-500/20 px-4 py-2 text-sm text-red-400 hover:bg-red-500/10 disabled:opacity-50"
        >
          {deleteMut.isPending ? 'Deleting…' : 'Delete'}
        </button>
      </div>

      {deleteMut.isError && (
        <div className="mb-6 rounded-lg border border-red-500/20 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          Could not delete the organization: {errorMessage(deleteMut.error)}
        </div>
      )}

      <div className="mb-6 flex gap-4 border-b border-white/10">
        <button
          onClick={() => setTab('projects')}
          className={`pb-3 text-sm font-medium transition ${tab === 'projects' ? 'border-b-2 text-[var(--brand)] border-[var(--brand)]' : 'text-gray-400 hover:text-gray-300'}`}
        >
          Projects ({projectCount})
        </button>
        <button
          onClick={() => setTab('members')}
          className={`pb-3 text-sm font-medium transition ${tab === 'members' ? 'border-b-2 text-[var(--brand)] border-[var(--brand)]' : 'text-gray-400 hover:text-gray-300'}`}
        >
          Members ({memberCount})
        </button>
        <button
          onClick={() => setTab('servers')}
          className={`pb-3 text-sm font-medium transition ${tab === 'servers' ? 'border-b-2 text-[var(--brand)] border-[var(--brand)]' : 'text-gray-400 hover:text-gray-300'}`}
        >
          Servers
        </button>
      </div>

      {tab === 'projects' && <ProjectsTab orgId={orgId} />}
      {tab === 'members' && <MembersTab orgId={orgId} />}
      {tab === 'servers' && <OrgServersTab orgId={orgId} />}
    </div>
  );
}

function ProjectsTab({ orgId }: { orgId: string }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState('');
  const [desc, setDesc] = useState('');

  const projectsQuery = useQuery({
    queryKey: [...PROJECTS_KEY, orgId],
    queryFn: () => fetchProjects(orgId),
  });
  const projects = projectsQuery.data ?? [];

  const createMut = useMutation({
    mutationFn: (input: { name: string; description?: string }) => createProject(orgId, input.name, undefined, input.description),
    onSuccess: () => {
      // Only cleared/refreshed once the API confirmed the project exists, so a
      // failed create never looks like a success.
      setName('');
      setDesc('');
      void queryClient.invalidateQueries({ queryKey: [...PROJECTS_KEY, orgId] });
    },
  });

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    createMut.mutate({ name: name.trim(), description: desc.trim() || undefined });
  };

  return (
    <div>
      <form onSubmit={handleCreate} className="mb-6 flex gap-3">
        <input
          value={name} onChange={(e) => setName(e.target.value)} placeholder="Project name"
          className="flex-1 rounded-lg border border-white/10 bg-black/30 px-4 py-2 text-sm text-white placeholder:text-gray-500 focus:border-[var(--brand)]/50 focus:outline-none" required
        />
        <input
          value={desc} onChange={(e) => setDesc(e.target.value)} placeholder="Description"
          className="w-48 rounded-lg border border-white/10 bg-black/30 px-4 py-2 text-sm text-white placeholder:text-gray-500 focus:border-[var(--brand)]/50 focus:outline-none"
        />
        <button type="submit" disabled={createMut.isPending || !name.trim()} className="rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--brand-hover)] disabled:opacity-50">
          {createMut.isPending ? '...' : 'Add'}
        </button>
      </form>

      {createMut.isError && (
        <div className="mb-6 rounded-lg border border-red-500/20 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          Could not create the project: {errorMessage(createMut.error)}
        </div>
      )}

      {projectsQuery.isError ? (
        <div className="rounded-lg border border-red-500/20 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          Could not load projects: {errorMessage(projectsQuery.error)}{' '}
          <button onClick={() => void projectsQuery.refetch()} className="underline hover:text-red-300">Retry</button>
        </div>
      ) : projectsQuery.isPending ? (
        <p className="text-sm text-gray-400">Loading projects…</p>
      ) : projects.length === 0 ? (
        <p className="text-sm text-gray-500">No projects yet.</p>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {projects.map((p) => (
            <div key={p.id} className="rounded-lg border border-white/10 bg-white/5 p-4">
              <h3 className="font-semibold text-white">{p.name}</h3>
              {p.description && <p className="mt-1 text-xs text-gray-400">{p.description}</p>}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function MembersTab({ orgId }: { orgId: string }) {
  const queryClient = useQueryClient();
  const [userId, setUserId] = useState('');
  const [role, setRole] = useState('member');

  const membersQuery = useQuery({
    queryKey: [...MEMBERS_KEY, orgId],
    queryFn: () => fetchTeamMembers(orgId),
  });
  const members = membersQuery.data ?? [];

  const addMut = useMutation({
    mutationFn: (input: { userId: string; role: string }) => addTeamMember(orgId, input.userId, input.role),
    onSuccess: () => {
      setUserId('');
      setRole('member');
      void queryClient.invalidateQueries({ queryKey: [...MEMBERS_KEY, orgId] });
    },
  });

  const removeMut = useMutation({
    mutationFn: (memberUserId: string) => removeTeamMember(orgId, memberUserId),
    onSuccess: () => { void queryClient.invalidateQueries({ queryKey: [...MEMBERS_KEY, orgId] }); },
  });

  const handleAdd = (e: React.FormEvent) => {
    e.preventDefault();
    if (!userId.trim()) return;
    addMut.mutate({ userId: userId.trim(), role });
  };

  return (
    <div>
      <form onSubmit={handleAdd} className="mb-6 flex gap-3">
        <input
          value={userId} onChange={(e) => setUserId(e.target.value)} placeholder="User ID"
          className="flex-1 rounded-lg border border-white/10 bg-black/30 px-4 py-2 text-sm text-white placeholder:text-gray-500 focus:border-[var(--brand)]/50 focus:outline-none" required
        />
        <select value={role} onChange={(e) => setRole(e.target.value)} className="rounded-lg border border-white/10 bg-black/30 px-3 py-2 text-sm text-white">
          <option value="member">Member</option>
          <option value="admin">Admin</option>
          <option value="viewer">Viewer</option>
        </select>
        <button type="submit" disabled={addMut.isPending || !userId.trim()} className="rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--brand-hover)] disabled:opacity-50">
          {addMut.isPending ? '...' : 'Add'}
        </button>
      </form>

      {addMut.isError && (
        <div className="mb-4 rounded-lg border border-red-500/20 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          Could not add the member: {errorMessage(addMut.error)}
        </div>
      )}
      {removeMut.isError && (
        <div className="mb-4 rounded-lg border border-red-500/20 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          Could not remove the member: {errorMessage(removeMut.error)}
        </div>
      )}

      {membersQuery.isError ? (
        <div className="rounded-lg border border-red-500/20 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          Could not load members: {errorMessage(membersQuery.error)}{' '}
          <button onClick={() => void membersQuery.refetch()} className="underline hover:text-red-300">Retry</button>
        </div>
      ) : membersQuery.isPending ? (
        <p className="text-sm text-gray-400">Loading members…</p>
      ) : members.length === 0 ? (
        <p className="text-sm text-gray-500">No members yet.</p>
      ) : (
        <div className="space-y-2">
          {members.map((m) => (
            <div key={m.id} className="flex items-center justify-between rounded-lg border border-white/10 bg-white/5 px-4 py-3">
              <div>
                <span className="text-sm font-medium text-white">{m.email}</span>
                <span className={`ml-3 inline-block rounded-full px-2 py-0.5 text-xs font-medium ${
                  m.role === 'owner' ? 'bg-yellow-500/20 text-yellow-400' :
                  m.role === 'admin' ? 'bg-[var(--brand)]/20 text-[var(--brand)]' :
                  m.role === 'viewer' ? 'bg-gray-500/20 text-gray-400' :
                  'bg-blue-500/20 text-blue-400'
                }`}>{m.role}</span>
              </div>
              {m.role !== 'owner' && (
                <button onClick={() => removeMut.mutate(m.userId)} disabled={removeMut.isPending} className="text-xs text-red-400 hover:text-red-300 disabled:opacity-50">Remove</button>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function OrgServersTab({ orgId }: { orgId: string }) {
  const [page, setPage] = useState(1);
  const [perPage] = useState(10);
  const [search, setSearch] = useState('');
  const [searchInput, setSearchInput] = useState('');

  const serversQuery = useQuery({
    queryKey: ['organization-servers', orgId, page, perPage, search],
    queryFn: () => fetchOrgServers(orgId, { page, per_page: perPage, search: search || undefined }),
  });

  const servers = serversQuery.data?.data ?? [];
  const pagination = serversQuery.data?.meta?.pagination;
  const total = pagination?.total ?? servers.length;
  const pageCount = Math.max(1, Math.ceil(total / perPage));

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    setPage(1);
    setSearch(searchInput.trim());
  };

  return (
    <div>
      <form onSubmit={handleSearch} className="mb-4 flex gap-2">
        <input
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          placeholder="Search servers..."
          className="flex-1 rounded-lg border border-white/10 bg-black/30 px-4 py-2 text-sm text-white placeholder:text-gray-500 focus:border-[var(--brand)]/50 focus:outline-none"
        />
        <button type="submit" className="rounded-lg bg-[var(--brand)] px-4 py-2 text-sm font-medium text-white hover:bg-[var(--brand-hover)]">Search</button>
        {search && (
          <button type="button" onClick={() => { setSearch(''); setSearchInput(''); setPage(1); }} className="rounded-lg border border-white/10 px-4 py-2 text-sm text-gray-300 hover:bg-white/5">Clear</button>
        )}
      </form>

      {serversQuery.isPending ? (
        <div className="p-6 text-sm text-slate-400">Loading servers...</div>
      ) : serversQuery.isError ? (
        <div className="rounded-lg border border-red-500/20 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          {errorMessage(serversQuery.error)}{' '}
          <button onClick={() => void serversQuery.refetch()} className="underline hover:text-red-300">Retry</button>
        </div>
      ) : servers.length === 0 ? (
        <p className="text-sm text-gray-500">No servers in this organization.</p>
      ) : (
        <>
          <div className="space-y-2">
            {servers.map((s) => (
              <Link
                key={s.id}
                href={`/server/${s.id}`}
                className="flex items-center justify-between rounded-lg border border-white/10 bg-white/5 px-4 py-3 transition hover:border-[var(--brand)]/30 hover:bg-white/[0.07]"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium text-white">{s.name}</span>
                    <span className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${
                      s.status === 'running' ? 'bg-emerald-500/20 text-emerald-400' :
                      s.status === 'offline' ? 'bg-gray-500/20 text-gray-400' :
                      s.status === 'installing' ? 'bg-amber-500/20 text-amber-400' :
                      'bg-blue-500/20 text-blue-400'
                    }`}>{s.status}</span>
                  </div>
                  {s.description && <p className="mt-1 truncate text-xs text-gray-400">{s.description}</p>}
                  <p className="mt-1 text-xs text-gray-500">{s.node} • {s.allocation ?? s.primaryAllocationId ?? ''}</p>
                </div>
                <span className="ml-3 shrink-0 text-xs text-gray-400">→</span>
              </Link>
            ))}
          </div>
          <Pagination page={page} pageCount={pageCount} onPageChange={setPage} label="Organization servers pagination" />
          <p className="mt-2 text-center text-xs text-gray-500">{total} total • page {page} of {pageCount}</p>
        </>
      )}
    </div>
  );
}
