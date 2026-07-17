# Phase 1 Implementation Guide - Critical Security Fixes

**Date:** 2026-06-18  
**Scope:** Production Blocker Fixes  
**Estimated Total Time:** 40-60 hours  
**Priority:** 🔴 CRITICAL

---

## Progress Summary

**Completed:** 37.5% (3/8 tasks)  
**Status:** IN PROGRESS  
**Blockers:** None

---

## ✅ COMPLETED (3/8)

### 1. WebSocket Origin Validation ✅
- Implementation: Complete
- Files: `realtime.go`, `server.go`
- Testing: Required
- Time Spent: 2 hours

### 2. Security Headers ✅
- Implementation: Complete
- Files: `middleware_security.go`, `server.go`
- Testing: Required
- Time Spent: 1.5 hours

### 3. Rate Limiting Middleware ✅
- Implementation: Complete
- Files: `middleware_ratelimit.go`
- Application: NOT YET APPLIED
- Time Spent: 2 hours

---

## 🔄 IN PROGRESS (2/8)

### 4. Apply Rate Limiting to Endpoints

**What's Needed:**

```go
// In forge/api/internal/http/server.go

// Add after app initialization
authLimiter := RateLimiter(GetRateLimitForEndpoint("auth", cfg.Redis))
mutationLimiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis))
readLimiter := RateLimiter(GetRateLimitForEndpoint("read", cfg.Redis))

// Apply to auth endpoints
v1.Post("/auth/login", authLimiter, func(c *fiber.Ctx) error { ... })
v1.Post("/auth/2fa", authLimiter, func(c *fiber.Ctx) error { ... })

// Apply to mutation endpoints (all POST/PUT/PATCH/DELETE)
protected.Post("/servers", mutationLimiter, func(c *fiber.Ctx) error { ... })
protected.Delete("/servers/:id", mutationLimiter, func(c *fiber.Ctx) error { ... })
// ... etc for all mutations

// Apply to read endpoints (all GET)
protected.Get("/servers", readLimiter, func(c *fiber.Ctx) error { ... })
// ... etc for all reads
```

**Steps:**
1. Create rate limiter instances in `NewServer()`
2. Apply `authLimiter` to 4 auth endpoints
3. Apply `mutationLimiter` to ~40 mutation endpoints
4. Apply `readLimiter` to ~30 read endpoints
5. Remove old `checkLoginRateLimit()` function
6. Test all rate limits

**Files to Modify:**
- `forge/api/internal/http/server.go` (main changes)
- `forge/api/internal/http/handlers_auth.go` (if separate file)
- `forge/api/internal/http/handlers_servers.go` (if separate file)

**Estimated Time:** 3-4 hours

---

### 5. Frontend WebSocket Ticket Integration

**Current State:**
- Backend ticket generation: ✅ EXISTS (`handlers_ws_ticket.go`)
- Backend ticket validation: ✅ EXISTS (`realtime.go`)
- Frontend implementation: ❌ STILL USING JWT IN URL

**What's Needed in Frontend:**

```typescript
// forge/web/lib/api.ts

// Add ticket generation function
export async function getWebSocketTicket(serverId: string, stream: 'console' | 'logs' | 'stats'): Promise<string> {
  const response = await fetch(`${API_BASE}/servers/${serverId}/ws/ticket`, {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${getToken()}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ stream }),
  });
  
  if (!response.ok) throw new Error('Failed to get WebSocket ticket');
  
  const data = await response.json();
  return data.ticket;
}

// Update WebSocket connection
export function serverWebSocketURL(serverId: string, stream: string): string {
  const base = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
  
  // First, request a ticket
  const ticket = await getWebSocketTicket(serverId, stream);
  
  // Use ticket instead of JWT
  const wsBase = base.replace('http://', 'ws://').replace('https://', 'wss://');
  return `${wsBase}/api/v1/servers/${serverId}/ws/${stream}?token=${ticket}`;
  
  // Keep JWT in Authorization header (WebSocket upgrade)
}
```

**Files to Modify:**
- `forge/web/lib/api.ts` - Add `getWebSocketTicket()` function
- `forge/web/components/server/console-view.tsx` - Use ticket
- `forge/web/stores/use-server-store.ts` - Update WebSocket connection logic

**Steps:**
1. Add ticket generation API call to `lib/api.ts`
2. Update WebSocket connection in console component
3. Update stats WebSocket connection
4. Update logs WebSocket connection
5. Add error handling for ticket failures
6. Test all WebSocket connections

**Estimated Time:** 4-6 hours

---

## ❌ NOT STARTED (3/8)

### 6. Runtime Smoke Testing

**Blockers:**
- `start-dev.sh` process management issues
- Need to fix script before testing

**Steps:**
1. Fix `start-dev.sh` to keep processes alive
2. Start full development environment
3. Open browser to `http://localhost:3000`
4. Test login flow
5. Test console WebSocket connection
6. Test file upload/download
7. Test power actions
8. Document all test steps

**Files to Fix:**
- `scripts/start-dev.sh`
- Maybe: `scripts/stop-dev.sh`, `scripts/status.sh`

**Estimated Time:** 8-12 hours

---

### 7. Database Provisioning Decision

**Options:**

**Option A: Remove Feature (RECOMMENDED FOR NOW)**
- Disable database creation in frontend
- Keep admin database host CRUD
- Document as future feature
- Time: 2 hours

**Option B: Implement Full Provisioning**
- MySQL driver implementation
- PostgreSQL driver implementation
- Connection testing
- Database CRUD operations
- Time: 40-60 hours

**Recommendation:** Option A now, Option B in Phase 2

**Steps (Option A):**
1. Update frontend database tab to show "Coming Soon"
2. Return clear message from 501 endpoints
3. Document in PROJECT_STATE.md
4. Add to Phase 2 roadmap

**Files to Modify (Option A):**
- `forge/web/components/server/database-list.tsx`
- `forge/api/internal/http/handlers_servers.go` (improve 501 message)
- `docs/PROJECT_STATE.md`

**Estimated Time:** 2 hours (Option A)

---

### 8. Comprehensive Testing

**Prerequisites:**
- All above tasks complete
- Development environment working
- Browser available

**Test Plan:**

1. **Security Headers Test**
   ```bash
   curl -I http://localhost:8080/api/v1/health
   # Verify all headers present:
   # - Content-Security-Policy
   # - X-Frame-Options: DENY
   # - X-Content-Type-Options: nosniff
   # - X-XSS-Protection: 1; mode=block
   # - Referrer-Policy: strict-origin-when-cross-origin
   # - Permissions-Policy
   ```

2. **WebSocket Origin Test**
   ```javascript
   // In browser console
   // Test allowed origin (should work)
   const ws = new WebSocket('ws://localhost:8080/api/v1/servers/xxx/ws/console');
   
   // Test disallowed origin (should fail)
   // Change browser Origin header or use different domain
   ```

3. **Rate Limiting Test**
   ```bash
   # Test auth rate limit (5/min)
   for i in {1..10}; do
     curl -X POST http://localhost:8080/api/v1/auth/login \
       -H "Content-Type: application/json" \
       -d '{"email":"test@test.com","password":"wrong"}'
     echo ""
   done
   # Should see 429 after 5 requests
   
   # Verify headers
   # X-RateLimit-Limit: 5
   # X-RateLimit-Remaining: 0
   # X-RateLimit-Reset: <timestamp>
   # Retry-After: 60
   ```

4. **WebSocket Ticket Test**
   ```bash
   # Get ticket
   TICKET=$(curl -X POST http://localhost:8080/api/v1/servers/xxx/ws/ticket \
     -H "Authorization: Bearer $JWT" \
     -H "Content-Type: application/json" \
     -d '{"stream":"console"}' | jq -r '.ticket')
   
   # Use ticket (should work once)
   wscat -c "ws://localhost:8080/api/v1/servers/xxx/ws/console?token=$TICKET"
   
   # Try to reuse ticket (should fail)
   wscat -c "ws://localhost:8080/api/v1/servers/xxx/ws/console?token=$TICKET"
   ```

5. **End-to-End Browser Test**
   - Login with correct credentials
   - Navigate to server console
   - Verify WebSocket connects
   - Send command
   - Verify output appears
   - Check network tab for security headers
   - Try file upload
   - Try power actions

**Estimated Time:** 6-8 hours

---

## Implementation Order

**Week 1 (20-24 hours):**
1. ✅ WebSocket origin validation (DONE)
2. ✅ Security headers (DONE)
3. ✅ Rate limiting middleware (DONE)
4. 🔄 Apply rate limiting to endpoints (3-4 hours)
5. 🔄 Frontend WebSocket tickets (4-6 hours)
6. Fix development scripts (2-3 hours)
7. Runtime smoke testing (8-12 hours)

**Week 2 (20-24 hours):**
8. Database provisioning decision (2 hours)
9. Comprehensive testing (6-8 hours)
10. Fix any issues found (8-12 hours)
11. Documentation updates (4-6 hours)

---

## Quick Start Guide

To continue implementation:

### Step 1: Apply Rate Limiting

```bash
# Edit forge/api/internal/http/server.go
cd /Users/riyaz/gamepanel/forge/api
nano internal/http/server.go

# Add after app initialization:
authLimiter := RateLimiter(GetRateLimitForEndpoint("auth", cfg.Redis))
mutationLimiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis))
readLimiter := RateLimiter(GetRateLimitForEndpoint("read", cfg.Redis))

# Apply to each endpoint group
# Save and test
go run ./cmd/api
```

### Step 2: Update Frontend

```bash
# Edit forge/web/lib/api.ts
cd /Users/riyaz/gamepanel/forge/web
nano lib/api.ts

# Add getWebSocketTicket() function
# Update serverWebSocketURL() to use tickets

# Test
npm run dev
# Open browser, test console
```

### Step 3: Test Everything

```bash
# Run full test suite
./scripts/start-dev.sh

# Manual browser testing
# Automated tests
# Security scans
```

---

## Code Snippets

### Apply Rate Limiter to Auth Endpoints

```go
// In NewServer() after creating limiters
v1.Post("/auth/login", authLimiter, func(c *fiber.Ctx) error {
    // existing login handler
})

v1.Post("/auth/2fa", authLimiter, func(c *fiber.Ctx) error {
    // existing 2FA handler
})

v1.Post("/auth/logout", authLimiter, func(c *fiber.Ctx) error {
    // existing logout handler
})

v1.Post("/auth/refresh", authLimiter, func(c *fiber.Ctx) error {
    // existing refresh handler (if exists)
})
```

### Apply Rate Limiter to Mutation Endpoints

```go
// Server mutations
protected.Post("/servers", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error { ... })
protected.Delete("/servers/:id", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error { ... })
protected.Post("/servers/:id/power", mutationLimiter, requireServerPermission(cfg, store.PermControlStart), func(c *fiber.Ctx) error { ... })

// User mutations
protected.Post("/users", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error { ... })
protected.Put("/users/:id", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error { ... })
protected.Delete("/users/:id", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error { ... })

// Continue for all POST/PUT/PATCH/DELETE endpoints
```

### Frontend WebSocket with Tickets

```typescript
// forge/web/lib/api.ts

export interface WSTicketResponse {
  ticket: string;
  expiresAt: string;
}

export async function getWebSocketTicket(
  serverId: string,
  stream: 'console' | 'logs' | 'stats'
): Promise<string> {
  const response = await fetch(
    `${API_BASE}/servers/${serverId}/ws/ticket`,
    {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${getToken()}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ stream }),
    }
  );

  if (!response.ok) {
    throw new Error(`Failed to get WebSocket ticket: ${response.statusText}`);
  }

  const data: WSTicketResponse = await response.json();
  return data.ticket;
}

// Update WebSocket connection
export async function connectServerWebSocket(
  serverId: string,
  stream: 'console' | 'logs' | 'stats'
): Promise<WebSocket> {
  // Get short-lived ticket
  const ticket = await getWebSocketTicket(serverId, stream);
  
  // Connect with ticket
  const base = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
  const wsBase = base.replace('http://', 'ws://').replace('https://', 'wss://');
  const url = `${wsBase}/api/v1/servers/${serverId}/ws/${stream}?token=${ticket}`;
  
  const ws = new WebSocket(url);
  
  // Set Authorization header (WebSocket doesn't support custom headers directly)
  // The ticket already authorizes this connection
  
  return ws;
}
```

---

## Success Criteria

Phase 1 is complete when:

- [x] WebSocket origin validation implemented
- [x] Security headers implemented
- [x] Rate limiting middleware created
- [ ] Rate limiting applied to all endpoints
- [ ] Frontend uses WebSocket tickets
- [ ] Development environment works
- [ ] Runtime smoke testing complete
- [ ] All security tests pass
- [ ] Documentation updated

**Current: 3/9 (33%)**  
**Target: 9/9 (100%)**

---

## Next Actions

**Immediate (Today):**
1. Apply rate limiting to all endpoints in server.go
2. Test rate limiting works
3. Commit security fixes

**Tomorrow:**
1. Update frontend to use WebSocket tickets
2. Test WebSocket connections
3. Fix development environment scripts

**This Week:**
1. Complete runtime smoke testing
2. Make database provisioning decision
3. Document all changes

---

**Last Updated:** 2026-06-18  
**Status:** 37.5% Complete, On Track  
**Next Milestone:** Rate limiting application (Tomorrow)
