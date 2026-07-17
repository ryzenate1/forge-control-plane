# 🔌 Frontend Wiring Checklist

**Purpose:** Wire all 15 components into the application  
**Estimated Time:** 2-3 days  
**Status:** Ready to begin

---

## 📋 Wiring Tasks

### Phase 1: Account Pages (2 components)

#### `/account/security` Page
- [ ] Create `apps/frontend/pages/account/security.tsx`
- [ ] Import `TwoFactorSetup` from `@/components/account/TwoFactorSetup`
- [ ] Import `SSHKeyManager` from `@/components/account/SSHKeyManager`
- [ ] Add authentication guard
- [ ] Add to navigation menu

```tsx
import TwoFactorSetup from '@/components/account/TwoFactorSetup';
import SSHKeyManager from '@/components/account/SSHKeyManager';

export default function SecurityPage() {
  return (
    <div className="space-y-6">
      <TwoFactorSetup />
      <SSHKeyManager />
    </div>
  );
}
```

---

### Phase 2: Admin Pages (5 components)

#### `/admin/activity` Page
- [ ] Create `apps/frontend/pages/admin/activity.tsx`
- [ ] Import `ActivityLogViewer` from `@/components/admin/ActivityLogViewer`
- [ ] Add admin authentication guard
- [ ] Add to admin navigation menu

```tsx
import ActivityLogViewer from '@/components/admin/ActivityLogViewer';

export default function ActivityPage() {
  return <ActivityLogViewer />;
}
```

#### `/admin/webhooks` Page
- [ ] Create `apps/frontend/pages/admin/webhooks.tsx`
- [ ] Import `WebhookManager` from `@/components/admin/WebhookManager`
- [ ] Add admin authentication guard
- [ ] Add to admin navigation menu

```tsx
import WebhookManager from '@/components/admin/WebhookManager';

export default function WebhooksPage() {
  return <WebhookManager />;
}
```

#### `/admin/monitoring` Page
- [ ] Create `apps/frontend/pages/admin/monitoring.tsx`
- [ ] Import `NodePerformance` from `@/components/admin/NodePerformance`
- [ ] Add admin authentication guard
- [ ] Add to admin navigation menu

```tsx
import NodePerformance from '@/components/admin/NodePerformance';

export default function MonitoringPage() {
  return <NodePerformance />;
}
```

#### `/admin/mounts` Page
- [ ] Create `apps/frontend/pages/admin/mounts.tsx`
- [ ] Import `MountManager` from `@/components/admin/MountManager`
- [ ] Add admin authentication guard
- [ ] Add to admin navigation menu

```tsx
import MountManager from '@/components/admin/MountManager';

export default function MountsPage() {
  return <MountManager />;
}
```

#### `/admin/api-usage` Page
- [ ] Create `apps/frontend/pages/admin/api-usage.tsx`
- [ ] Import `ApiDashboard` from `@/components/admin/ApiDashboard`
- [ ] Add admin authentication guard
- [ ] Add to admin navigation menu

```tsx
import ApiDashboard from '@/components/admin/ApiDashboard';

export default function ApiUsagePage() {
  return <ApiDashboard />;
}
```

---

### Phase 3: Server Pages (8 components)

#### `/servers/[id]/schedules` Page
- [ ] Create `apps/frontend/pages/servers/[id]/schedules.tsx`
- [ ] Import `ScheduleEditor` from `@/components/server/ScheduleEditor`
- [ ] Pass `serverId` prop from route params
- [ ] Add authentication guard
- [ ] Add to server navigation tabs

```tsx
import { useParams } from 'next/navigation';
import ScheduleEditor from '@/components/server/ScheduleEditor';

export default function SchedulesPage() {
  const { id } = useParams();
  return <ScheduleEditor serverId={id as string} />;
}
```

#### `/servers/[id]/subusers` Page
- [ ] Create `apps/frontend/pages/servers/[id]/subusers.tsx`
- [ ] Import `SubuserManager` from `@/components/server/SubuserManager`
- [ ] Pass `serverId` prop
- [ ] Add authentication guard
- [ ] Add to server navigation tabs

```tsx
import { useParams } from 'next/navigation';
import SubuserManager from '@/components/server/SubuserManager';

export default function SubusersPage() {
  const { id } = useParams();
  return <SubuserManager serverId={id as string} />;
}
```

#### `/servers/[id]/backups` Page (Replace Existing)
- [ ] Update `apps/frontend/pages/servers/[id]/backups.tsx`
- [ ] Replace old component with `EnhancedBackupsView`
- [ ] Import from `@/components/server/enhanced-backups-view`
- [ ] Pass `serverId` prop
- [ ] Test backup operations

```tsx
import { useParams } from 'next/navigation';
import EnhancedBackupsView from '@/components/server/enhanced-backups-view';

export default function BackupsPage() {
  const { id } = useParams();
  return <EnhancedBackupsView serverId={id as string} />;
}
```

#### `/servers/[id]/network` Page (Replace Existing)
- [ ] Update `apps/frontend/pages/servers/[id]/network.tsx`
- [ ] Replace old component with `EnhancedNetworkView`
- [ ] Import from `@/components/server/enhanced-network-view`
- [ ] Pass `serverId` prop
- [ ] Test allocation operations

```tsx
import { useParams } from 'next/navigation';
import EnhancedNetworkView from '@/components/server/enhanced-network-view';

export default function NetworkPage() {
  const { id } = useParams();
  return <EnhancedNetworkView serverId={id as string} />;
}
```

#### `/servers/[id]/files` Page (Replace Existing)
- [ ] Update `apps/frontend/pages/servers/[id]/files.tsx`
- [ ] Replace old component with `EnhancedFileManager`
- [ ] Import from `@/components/server/EnhancedFileManager`
- [ ] Pass `serverId` prop
- [ ] Test file operations

```tsx
import { useParams } from 'next/navigation';
import EnhancedFileManager from '@/components/server/EnhancedFileManager';

export default function FilesPage() {
  const { id } = useParams();
  return <EnhancedFileManager serverId={id as string} />;
}
```

#### `/servers/[id]/console` Page (Replace Existing)
- [ ] Update `apps/frontend/pages/servers/[id]/console.tsx`
- [ ] Replace old component with `EnhancedConsole`
- [ ] Import from `@/components/server/EnhancedConsole`
- [ ] Pass `serverId` prop
- [ ] Test WebSocket connection

```tsx
import { useParams } from 'next/navigation';
import EnhancedConsole from '@/components/server/EnhancedConsole';

export default function ConsolePage() {
  const { id } = useParams();
  return <EnhancedConsole serverId={id as string} />;
}
```

#### `/servers/[id]/clone` Page
- [ ] Create `apps/frontend/pages/servers/[id]/clone.tsx`
- [ ] Import `ServerClone` from `@/components/admin/ServerClone`
- [ ] Pass `serverId` prop
- [ ] Add authentication guard
- [ ] Add to server actions menu

```tsx
import { useParams } from 'next/navigation';
import ServerClone from '@/components/admin/ServerClone';

export default function ClonePage() {
  const { id } = useParams();
  return <ServerClone serverId={id as string} />;
}
```

#### CronBuilder Integration (Used in ScheduleEditor)
- [ ] CronBuilder is already imported in `ScheduleEditor.tsx`
- [ ] No separate page needed
- [ ] Can be used standalone if needed

---

### Phase 4: Navigation Updates

#### Main Navigation
- [ ] Add "Security" under Account dropdown
- [ ] Add Admin section if user is admin:
  - [ ] Activity Logs
  - [ ] Webhooks
  - [ ] Monitoring
  - [ ] Mounts
  - [ ] API Usage

#### Server Navigation Tabs
- [ ] Add "Schedules" tab
- [ ] Add "Subusers" tab
- [ ] Update "Backups" to use new component
- [ ] Update "Network" to use new component
- [ ] Update "Files" to use new component
- [ ] Update "Console" to use new component
- [ ] Add "Clone Server" to actions dropdown

#### Admin Navigation
- [ ] Create admin-only sidebar/menu
- [ ] Add all 5 admin pages
- [ ] Add access control checks

---

### Phase 5: Authentication Guards

#### Create Auth Guards
```tsx
// middleware/auth.ts
export function requireAuth() {
  const token = localStorage.getItem('token');
  if (!token) {
    window.location.href = '/login';
    return false;
  }
  return true;
}

export async function requireAdmin() {
  if (!requireAuth()) return false;
  
  const response = await fetch('/api/v1/user', {
    headers: { 'Authorization': `Bearer ${localStorage.getItem('token')}` }
  });
  
  if (!response.ok) return false;
  const user = await response.json();
  
  if (!user.is_admin) {
    window.location.href = '/';
    return false;
  }
  
  return true;
}
```

#### Apply Guards
- [ ] Add `requireAuth()` to all user pages
- [ ] Add `requireAdmin()` to all admin pages
- [ ] Add `requireAuth()` to all server pages

---

### Phase 6: Testing Checklist

#### Account & Security
- [ ] Test 2FA setup flow (all 4 steps)
- [ ] Test 2FA QR code display
- [ ] Test recovery code generation
- [ ] Test SSH key addition
- [ ] Test SSH key deletion
- [ ] Test copy functionality

#### Admin Pages
- [ ] Test activity log filtering
- [ ] Test activity log CSV export
- [ ] Test webhook creation
- [ ] Test webhook delivery testing
- [ ] Test webhook history
- [ ] Test node performance monitoring
- [ ] Test mount creation/deletion
- [ ] Test API dashboard metrics

#### Server Management
- [ ] Test schedule creation
- [ ] Test task addition/deletion
- [ ] Test schedule execution (run now)
- [ ] Test cron builder presets
- [ ] Test cron builder visual mode
- [ ] Test subuser invitation
- [ ] Test permission management
- [ ] Test backup creation
- [ ] Test backup locking/unlocking
- [ ] Test backup restore
- [ ] Test allocation assignment
- [ ] Test allocation notes
- [ ] Test set primary allocation
- [ ] Test server cloning
- [ ] Test template saving
- [ ] Test template deployment

#### Enhanced Components
- [ ] Test file manager bulk operations
- [ ] Test file manager archive/extract
- [ ] Test console command history (↑↓)
- [ ] Test console auto-reconnect
- [ ] Test console search

---

### Phase 7: Environment Configuration

#### API Endpoints
- [ ] Verify all API endpoints are accessible
- [ ] Check CORS configuration
- [ ] Verify WebSocket endpoints
- [ ] Test rate limiting

#### Environment Variables
```bash
# .env.local
NEXT_PUBLIC_API_URL=http://localhost:8080
NEXT_PUBLIC_WS_URL=ws://localhost:8080
```

---

## 🎯 Quick Start Guide

### Step 1: Create Page Files
```bash
cd apps/frontend

# Account pages
mkdir -p pages/account
touch pages/account/security.tsx

# Admin pages
mkdir -p pages/admin
touch pages/admin/activity.tsx
touch pages/admin/webhooks.tsx
touch pages/admin/monitoring.tsx
touch pages/admin/mounts.tsx
touch pages/admin/api-usage.tsx

# Server pages (if not exists)
mkdir -p pages/servers/[id]
touch pages/servers/[id]/schedules.tsx
touch pages/servers/[id]/subusers.tsx
touch pages/servers/[id]/clone.tsx
```

### Step 2: Copy Template Code
Use the code snippets provided above for each page.

### Step 3: Update Navigation
Add links to your main navigation component.

### Step 4: Test Each Component
Go through the testing checklist above.

---

## 📊 Progress Tracking

### Completion Status
- [ ] Phase 1: Account Pages (0/2)
- [ ] Phase 2: Admin Pages (0/5)
- [ ] Phase 3: Server Pages (0/8)
- [ ] Phase 4: Navigation Updates (0/3)
- [ ] Phase 5: Authentication Guards (0/3)
- [ ] Phase 6: Testing (0/30)
- [ ] Phase 7: Environment Config (0/4)

### Overall Progress: 0% → 100%

---

## 🚨 Common Issues & Solutions

### Issue 1: Import Errors
**Solution:** Verify path aliases in `tsconfig.json`:
```json
{
  "compilerOptions": {
    "paths": {
      "@/components/*": ["./components/*"],
      "@/pages/*": ["./pages/*"]
    }
  }
}
```

### Issue 2: API 401 Errors
**Solution:** Check token in localStorage:
```js
console.log(localStorage.getItem('token'));
```

### Issue 3: WebSocket Connection Failed
**Solution:** Verify WS URL in environment:
```bash
NEXT_PUBLIC_WS_URL=ws://localhost:8080
```

### Issue 4: CORS Errors
**Solution:** Update backend CORS config:
```go
c := cors.New(cors.Options{
    AllowedOrigins:   []string{"http://localhost:3000"},
    AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
    AllowedHeaders:   []string{"Authorization", "Content-Type"},
    AllowCredentials: true,
})
```

---

## 📚 Additional Resources

### Component Documentation
- All components have inline JSDoc comments
- TypeScript interfaces define prop types
- Refer to `FRONTEND_COMPLETE_100_PERCENT.md` for feature lists

### API Documentation
- Backend handlers in `apps/api/internal/http/`
- Refer to `PRODUCTION_READY_STATUS.md` for endpoint details

### Testing Guide
- Test each component individually first
- Then test workflows (create → edit → delete)
- Finally test integration between components

---

## ✅ Final Checklist

Before considering wiring complete:

- [ ] All 15 components accessible via routes
- [ ] Navigation menus updated
- [ ] Authentication guards in place
- [ ] All features tested manually
- [ ] No console errors
- [ ] No TypeScript errors
- [ ] API calls working
- [ ] WebSocket connections stable
- [ ] Mobile responsive
- [ ] Cross-browser tested

---

**Once complete, the application will be fully production-ready! 🚀**
