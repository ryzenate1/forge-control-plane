'use client';

import { useState, useEffect, useCallback } from 'react';
import { useParams, useRouter } from 'next/navigation';
import Link from 'next/link';
import type { Organization, Project, TeamMember, Environment } from '@/lib/api/tenancy';

export default function OrganizationDetailPage() {
  const params = useParams<{ slug: string }>();
  const router = useRouter();
  const [org, setOrg] = useState<Organization | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [members, setMembers] = useState<TeamMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<'projects' | 'members'>('projects');

  const fetchOrg = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(`/api/v1/organizations/${params.slug}`, { credentials: 'include' });
      if (!res.ok) throw new Error('Organization not found');
      const data = await res.json();
      setOrg(data);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, [params.slug]);

  useEffect(() => { fetchOrg(); }, [fetchOrg]);

  const fetchProjects = useCallback(async () => {
    if (!org) return;
    try {
      const res = await fetch(`/api/v1/organizations/${org.id}/projects`, { credentials: 'include' });
      if (res.ok) setProjects(await res.json());
    } catch {}
  }, [org]);

  const fetchMembers = useCallback(async () => {
    if (!org) return;
    try {
      const res = await fetch(`/api/v1/organizations/${org.id}/members`, { credentials: 'include' });
      if (res.ok) setMembers(await res.json());
    } catch {}
  }, [org]);

  useEffect(() => { fetchProjects(); fetchMembers(); }, [fetchProjects, fetchMembers]);

  const handleDelete = async () => {
    if (!org || !confirm(`Delete organization "${org.name}"? This cannot be undone.`)) return;
    try {
      const res = await fetch(`/api/v1/organizations/${org.id}`, { method: 'DELETE', credentials: 'include' });
      if (!res.ok) throw new Error(await res.text());
      router.push('/organizations');
    } catch (e: any) {
      setError(e.message);
    }
  };

  if (loading) return <div className="mx-auto max-w-4xl px-4 py-8 text-gray-400">Loading...</div>;
  if (error) return <div className="mx-auto max-w-4xl px-4 py-8 text-red-400">{error}</div>;
  if (!org) return null;

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
          className="rounded-lg border border-red-500/20 px-4 py-2 text-sm text-red-400 hover:bg-red-500/10"
        >
          Delete
        </button>
      </div>

      <div className="mb-6 flex gap-4 border-b border-white/10">
        <button
          onClick={() => setTab('projects')}
          className={`pb-3 text-sm font-medium transition ${tab === 'projects' ? 'border-b-2 border-purple-500 text-purple-400' : 'text-gray-400 hover:text-gray-300'}`}
        >
          Projects ({projects.length})
        </button>
        <button
          onClick={() => setTab('members')}
          className={`pb-3 text-sm font-medium transition ${tab === 'members' ? 'border-b-2 border-purple-500 text-purple-400' : 'text-gray-400 hover:text-gray-300'}`}
        >
          Members ({members.length})
        </button>
      </div>

      {tab === 'projects' && <ProjectsTab orgId={org.id} projects={projects} onRefresh={fetchProjects} />}
      {tab === 'members' && <MembersTab orgId={org.id} members={members} onRefresh={fetchMembers} />}
    </div>
  );
}

function ProjectsTab({ orgId, projects, onRefresh }: { orgId: string; projects: Project[]; onRefresh: () => void }) {
  const [name, setName] = useState('');
  const [desc, setDesc] = useState('');
  const [creating, setCreating] = useState(false);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    setCreating(true);
    try {
      const res = await fetch(`/api/v1/organizations/${orgId}/projects`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim(), description: desc.trim() }),
      });
      if (!res.ok) throw new Error(await res.text());
      setName(''); setDesc(''); onRefresh();
    } catch {}
    setCreating(false);
  };

  return (
    <div>
      <form onSubmit={handleCreate} className="mb-6 flex gap-3">
        <input
          value={name} onChange={(e) => setName(e.target.value)} placeholder="Project name"
          className="flex-1 rounded-lg border border-white/10 bg-black/30 px-4 py-2 text-sm text-white placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none" required
        />
        <input
          value={desc} onChange={(e) => setDesc(e.target.value)} placeholder="Description"
          className="w-48 rounded-lg border border-white/10 bg-black/30 px-4 py-2 text-sm text-white placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none"
        />
        <button type="submit" disabled={creating || !name.trim()} className="rounded-lg bg-purple-600 px-4 py-2 text-sm font-medium text-white hover:bg-purple-500 disabled:opacity-50">
          {creating ? '...' : 'Add'}
        </button>
      </form>

      {projects.length === 0 && <p className="text-sm text-gray-500">No projects yet.</p>}
      <div className="grid gap-3 sm:grid-cols-2">
        {projects.map((p) => (
          <div key={p.id} className="rounded-lg border border-white/10 bg-white/5 p-4">
            <h3 className="font-semibold text-white">{p.name}</h3>
            {p.description && <p className="mt-1 text-xs text-gray-400">{p.description}</p>}
          </div>
        ))}
      </div>
    </div>
  );
}

function MembersTab({ orgId, members, onRefresh }: { orgId: string; members: TeamMember[]; onRefresh: () => void }) {
  const [userId, setUserId] = useState('');
  const [role, setRole] = useState('member');
  const [adding, setAdding] = useState(false);

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!userId.trim()) return;
    setAdding(true);
    try {
      const res = await fetch(`/api/v1/organizations/${orgId}/members`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ userId: userId.trim(), role }),
      });
      if (!res.ok) throw new Error(await res.text());
      setUserId(''); setRole('member'); onRefresh();
    } catch {}
    setAdding(false);
  };

  const handleRemove = async (userId: string) => {
    try {
      const res = await fetch(`/api/v1/organizations/${orgId}/members/${userId}`, { method: 'DELETE', credentials: 'include' });
      if (!res.ok) throw new Error(await res.text());
      onRefresh();
    } catch {}
  };

  return (
    <div>
      <form onSubmit={handleAdd} className="mb-6 flex gap-3">
        <input
          value={userId} onChange={(e) => setUserId(e.target.value)} placeholder="User ID"
          className="flex-1 rounded-lg border border-white/10 bg-black/30 px-4 py-2 text-sm text-white placeholder:text-gray-500 focus:border-purple-500/50 focus:outline-none" required
        />
        <select value={role} onChange={(e) => setRole(e.target.value)} className="rounded-lg border border-white/10 bg-black/30 px-3 py-2 text-sm text-white">
          <option value="member">Member</option>
          <option value="admin">Admin</option>
          <option value="viewer">Viewer</option>
        </select>
        <button type="submit" disabled={adding || !userId.trim()} className="rounded-lg bg-purple-600 px-4 py-2 text-sm font-medium text-white hover:bg-purple-500 disabled:opacity-50">
          {adding ? '...' : 'Add'}
        </button>
      </form>

      {members.length === 0 && <p className="text-sm text-gray-500">No members yet.</p>}
      <div className="space-y-2">
        {members.map((m) => (
          <div key={m.id} className="flex items-center justify-between rounded-lg border border-white/10 bg-white/5 px-4 py-3">
            <div>
              <span className="text-sm font-medium text-white">{m.email}</span>
              <span className={`ml-3 inline-block rounded-full px-2 py-0.5 text-xs font-medium ${
                m.role === 'owner' ? 'bg-yellow-500/20 text-yellow-400' :
                m.role === 'admin' ? 'bg-purple-500/20 text-purple-400' :
                m.role === 'viewer' ? 'bg-gray-500/20 text-gray-400' :
                'bg-blue-500/20 text-blue-400'
              }`}>{m.role}</span>
            </div>
            {m.role !== 'owner' && (
              <button onClick={() => handleRemove(m.userId)} className="text-xs text-red-400 hover:text-red-300">Remove</button>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
