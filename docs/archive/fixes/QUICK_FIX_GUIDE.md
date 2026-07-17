# Quick Fix Guide - Getting GamePanel Working

## Current Issues

1. ❌ "missing bearer token" - Auth not working
2. ❌ "missing panel signature" - Daemon communication issue  
3. ❌ UI components not showing - Views need integration
4. ❌ Can't create servers, allocations, etc.

## Root Cause

The system is running but:
- Database might not have the default admin user
- Auth tokens not being saved properly
- New components I created are NOT wired into the app (they're standalone files)

## Solution Steps

### Step 1: Check Database & Create Admin User

```bash
# Connect to the database
docker exec -it docker-postgres-1 psql -U postgres -d gamepanel

# Check if users table has data
SELECT id, email, role FROM users;

# If no admin user exists, create one:
INSERT INTO users (id, email, password, role, created_at, updated_at)
VALUES (
  gen_random_uuid(),
  'admin@example.com',
  -- password hash for 'admin123' (bcrypt)
  '$2a$10$N9qo8uLOickgx2ZMRZoMye

IH9JaPFSnKa/dF4DJPBnLtQp1QphK1C',
  'admin',
  now(),
  now()
);

# Exit postgres
\q
```

### Step 2: Fix Environment Variables

Create `/Users/riyaz/gamepanel/apps/frontend/.env.local`:

```bash
NEXT_PUBLIC_API_URL=http://localhost:8080/api/v1
NEXT_PUBLIC_WS_URL=ws://localhost:8080
```

### Step 3: Verify Services

```bash
# Check all services are running
curl http://localhost:8080/api/v1/health
curl http://localhost:9090/health
curl http://localhost:3000

# Check API responds
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"admin123"}'
```

## Understanding the Architecture

### Current Setup (WORKING)
- `apps/frontend/components/dashboard.tsx` - Main routing component
- `apps/frontend/components/server/*-view.tsx` - Existing views (WORKING)
- `apps/frontend/components/admin/*` - Admin panels

### New Components I Created (NOT WIRED)
These are standalone and NOT integrated:
- `components/account/TwoFactorSetup.tsx`
- `components/account/SSHKeyManager.tsx`
- `components/admin/ActivityLogViewer.tsx`
- `components/admin/WebhookManager.tsx`
- `components/admin/NodePerformance.tsx`
- `components/admin/MountManager.tsx`
- `components/admin/ApiDashboard.tsx`
- `components/server/ScheduleEditor.tsx`
- `components/server/SubuserManager.tsx`
- `components/server/enhanced-backups-view.tsx`
- `components/server/enhanced-network-view.tsx`
- `components/server/CronBuilder.tsx`
- `components/admin/ServerClone.tsx`

## Integration Plan

To use the new components, we need to:

### Option A: Keep Both (Recommended)
Add new components as ADDITIONAL tabs/features alongside existing ones

### Option B: Replace Specific Views
Replace old views with enhanced versions one by one

## Quick Test After Fixes

1. Login at http://localhost:3000
2. Check if you can see servers list
3. Try creating an allocation
4. Try starting/stopping a server

## If Still Not Working

Check logs:
```bash
# API logs
tail -f .dev-logs/api.log
tail -f .dev-logs/api.err.log

# Frontend logs
tail -f .dev-logs/frontend.log

# Daemon logs
tail -f .dev-logs/daemon.log
```

## Common Issues

### "missing bearer token"
- Token not in localStorage
- Check browser DevTools → Application → Local Storage
- Key should be: `modern-game-panel-token`

### "missing panel signature"  
- Daemon can't authenticate with API
- Check daemon token configuration

### Can't create allocation
- No nodes available
- No IP addresses configured on nodes

### Can't create server
- No allocations available
- No eggs/nests configured

## Next Steps After Basic Fixes

Once everything is working:
1. I can integrate the new enhanced components
2. Add the new features (2FA, SSH keys, webhooks, etc.)
3. Improve UI/UX with the new components

Would you like me to:
- A) Fix the immediate auth/database issues first?
- B) Start integrating new components alongside existing?
- C) Replace specific views with enhanced versions?
