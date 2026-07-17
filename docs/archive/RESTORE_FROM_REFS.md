# Restore UI from Reference Panels

**Date:** June 16, 2026, 3:30 PM  
**Task:** Copy working UI from Pterodactyl/Pelican panels  
**Status:** In Progress

---

## 🎯 Objective

The current UI is broken due to:
1. Missing shadcn/ui components
2. Incomplete component implementations
3. Features not working (backups, allocations, network, schedules, etc.)

**Solution:** Copy the working UI components from the reference implementations in `/refs/` folder.

---

## 📁 Reference Panels Available

### 1. Pterodactyl Panel
- **Path:** `/refs/petrodactylpanel/`
- **Frontend:** React + TypeScript
- **Location:** `resources/scripts/components/`
- **Features:** Full server management UI

### 2. Pelican Panel
- **Path:** `/refs/pelicanpanel/`
- **Frontend:** Vue/React + TypeScript  
- **Location:** `resources/`
- **Features:** Enhanced Pterodactyl fork

### 3. Puffer Panel
- **Path:** `/refs/pufferpanel/`
- **Frontend:** Vue
- **Location:** `client/`
- **Features:** Lightweight panel

---

## 🗺️ Component Mapping

### Server Management Components

| Feature | Our Path | Pterodactyl Path | Status |
|---------|----------|------------------|--------|
| Console | `components/server/console-view.tsx` | `components/server/console/` | ⏳ Copy |
| Files | `components/server/files-view.tsx` | `components/server/files/` | ⏳ Copy |
| Databases | `components/server/databases-view.tsx` | `components/server/databases/` | ⏳ Copy |
| Backups | `components/server/backups-view.tsx` | `components/server/backups/` | ⏳ Copy |
| Network | `components/server/network-view.tsx` | `components/server/network/` | ⏳ Copy |
| Schedules | `components/server/schedules-view.tsx` | `components/server/schedules/` | ⏳ Copy |
| Users | `components/server/users-view.tsx` | `components/server/users/` | ⏳ Copy |
| Startup | `components/server/startup-view.tsx` | `components/server/startup/` | ⏳ Copy |
| Settings | `components/server/settings-view.tsx` | `components/server/settings/` | ⏳ Copy |
| Activity | `components/server/activity-view.tsx` | `components/server/ServerActivityLogContainer.tsx` | ⏳ Copy |

---

## 🔧 Implementation Plan

### Phase 1: Copy Base Components (30 mins)

**Step 1:** Copy Pterodactyl's element components
```bash
cp -r refs/petrodactylpanel/resources/scripts/components/elements/* \
  apps/frontend/components/elements/
```

**Step 2:** Copy helper utilities
```bash
cp refs/petrodactylpanel/resources/scripts/lib/*.ts \
  apps/frontend/lib/
```

**Step 3:** Copy API helpers
```bash
cp refs/petrodactylpanel/resources/scripts/api/http.ts \
  apps/frontend/lib/http.ts
```

### Phase 2: Copy Server Components (1 hour)

For each server management component:

1. **Console**
   - Copy: `refs/petrodactylpanel/resources/scripts/components/server/console/`
   - Adapt: WebSocket connection to our API
   - Test: Command execution, power controls

2. **Files**
   - Copy: `refs/petrodactylpanel/resources/scripts/components/server/files/`
   - Adapt: File API endpoints
   - Test: Upload, download, edit, delete

3. **Databases**
   - Copy: `refs/petrodactylpanel/resources/scripts/components/server/databases/`
   - Adapt: Database creation API
   - Test: Create, rotate password, delete

4. **Backups**
   - Copy: `refs/petrodactylpanel/resources/scripts/components/server/backups/`
   - Adapt: Backup API endpoints
   - Test: Create, restore, download, delete

5. **Network**
   - Copy: `refs/petrodactylpanel/resources/scripts/components/server/network/`
   - Adapt: Allocation API
   - Test: Assign, set primary, unassign

6. **Schedules**
   - Copy: `refs/petrodactylpanel/resources/scripts/components/server/schedules/`
   - Adapt: Schedule CRUD API
   - Test: Create, edit, run now, delete

7. **Users**
   - Copy: `refs/petrodactylpanel/resources/scripts/components/server/users/`
   - Adapt: Subuser API
   - Test: Add, edit permissions, remove

8. **Startup**
   - Copy: `refs/petrodactylpanel/resources/scripts/components/server/startup/`
   - Adapt: Variable update API
   - Test: Edit variables, update

9. **Settings**
   - Copy: `refs/petrodactylpanel/resources/scripts/components/server/settings/`
   - Adapt: Server update/delete API
   - Test: Rename, reinstall, delete

10. **Activity**
    - Copy: `refs/petrodactylpanel/resources/scripts/components/server/ServerActivityLogContainer.tsx`
    - Adapt: Activity log API
    - Test: View logs, filter

### Phase 3: Copy Admin Components (45 mins)

**Admin panels needed:**
- Servers list & creation
- Nodes management
- Allocations management
- Users management
- Database hosts
- Mounts

### Phase 4: Test & Fix (30 mins)

Test each component:
- [ ] Can create server
- [ ] Can create allocation
- [ ] Can create database
- [ ] Can create backup
- [ ] Can assign network
- [ ] Can create schedule
- [ ] Power controls work
- [ ] File manager works
- [ ] Console connects
- [ ] All admin functions work

---

## 📝 Adaptation Strategy

### 1. API Endpoints
Pterodactyl uses `/api/client/servers/:id/...`  
We use `/api/v1/servers/:id/...`

**Find & Replace:**
```typescript
// OLD (Pterodactyl):
http.get(`/api/client/servers/${uuid}/backups`)

// NEW (Ours):
http.get(`/api/v1/servers/${id}/backups`)
```

### 2. Authentication
Pterodactyl uses cookies  
We use Bearer tokens

**Adapt:**
```typescript
// Add to all requests:
headers: {
  'Authorization': `Bearer ${localStorage.getItem('modern-game-panel-token')}`
}
```

### 3. WebSocket URLs
Pterodactyl: `wss://panel.example.com/api/client/servers/:uuid/websocket`  
Ours: `ws://localhost:8080/api/v1/servers/:id/ws/console`

### 4. Component Imports
Pterodactyl uses absolute imports with `@/`  
Keep our import structure

### 5. Styling
Pterodactyl uses Tailwind CSS  
We also use Tailwind - styles should work!

---

## 🚀 Quick Start

I'll systematically:
1. Copy each working component from Pterodactyl
2. Adapt API calls to our backend
3. Test functionality
4. Move to next component

This ensures we get ALL the features working:
- ✅ Multi-step server creation
- ✅ All node options (FQDN, etc.)
- ✅ Working backups
- ✅ Working allocations
- ✅ Working schedules
- ✅ Working everything!

---

## 📊 Progress Tracking

### Components Copied: 0/10
- [ ] Console
- [ ] Files  
- [ ] Databases
- [ ] Backups
- [ ] Network
- [ ] Schedules
- [ ] Users
- [ ] Startup
- [ ] Settings
- [ ] Activity

### Admin Panels Copied: 0/6
- [ ] Servers
- [ ] Nodes
- [ ] Allocations
- [ ] Users
- [ ] Database Hosts
- [ ] Mounts

---

**Starting implementation now!** 🚀
