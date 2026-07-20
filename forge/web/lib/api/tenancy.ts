export interface Organization {
  id: string;
  name: string;
  slug: string;
  ownerId: string;
  ownerName: string;
  createdAt: string;
}

export interface Project {
  id: string;
  orgId: string;
  name: string;
  slug: string;
  description: string;
  createdAt: string;
}

export interface Environment {
  id: string;
  projectId: string;
  name: string;
  color: string;
  protected: boolean;
  createdAt: string;
}

export interface TeamMember {
  id: string;
  orgId: string;
  userId: string;
  email: string;
  role: 'owner' | 'admin' | 'member' | 'viewer';
  createdAt: string;
}
