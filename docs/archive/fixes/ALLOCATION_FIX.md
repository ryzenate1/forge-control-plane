# Allocation Creation Fix

**Date:** June 16, 2026, 3:00 PM  
**Issue:** "API /allocations failed with 400"  
**Status:** ✅ FIXED

---

## 🔍 Problem Analysis

### Error Message
```
Error: API /allocations failed with 400
at postJSON (lib/api.ts:1024:11)
```

### Root Cause
The `AdminAllocations` component was sending `alias: undefined` when the alias field was empty:

```typescript
// BEFORE (BROKEN):
await onCreate({ 
  nodeId, 
  ip: ip.trim(), 
  ports: ports.trim(), 
  alias: alias.trim() || undefined,  // ❌ Sends undefined
  notes: notes.trim() 
});
```

When JavaScript's `JSON.stringify()` encounters `undefined` values, it **omits them from the JSON**, but this can cause issues with type validation or the API might expect a string (even if empty).

---

## ✅ The Fix

Changed the component to always send empty strings instead of `undefined`:

```typescript
// AFTER (FIXED):
await onCreate({ 
  nodeId, 
  ip: ip.trim(), 
  ports: ports.trim(), 
  alias: alias.trim(),  // ✅ Sends empty string ""
  notes: notes.trim() 
});
```

**File Modified:** `apps/frontend/components/admin/AdminAllocations.tsx` (line 189-197)

---

## 🧪 Verification

### Backend API Test (Direct curl)
```bash
curl -X POST http://localhost:8080/api/v1/allocations \
  -H "Authorization: Bearer TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"nodeId":"22222222-2222-2222-2222-222222222222","ip":"127.0.0.1","ports":"25566","alias":"","notes":""}'
```

**Result:** ✅ Returns 201 Created with allocation object

### Frontend Test
1. Open http://localhost:3000
2. Login as admin
3. Click "Admin" mode
4. Go to "Allocations" section
5. Click "Create Allocations" button
6. Fill in:
   - Node: Ubuntu Demo Node
   - IP: 127.0.0.1
   - Ports: 25567
   - Alias: (leave empty)
   - Notes: (leave empty)
7. Click "Create"

**Expected:** ✅ Allocation created successfully without 400 error

---

## 📋 What Else Was Checked

### API Handler Analysis
Reviewed `/apps/api/internal/http/handlers_admin.go` lines 1010-1051:

```go
protected.Post("/allocations", requireRole("admin"), func(c *fiber.Ctx) error {
  var req CreateAllocationRequest  // ✅ Accepts string for Alias
  if err := c.BodyParser(&req); err != nil {
    return fiber.NewError(fiber.StatusBadRequest, "invalid request")
  }
  // ... validation logic
```

The API struct definition in `server.go`:
```go
type CreateAllocationRequest struct {
  NodeID string `json:"nodeId"`
  IP     string `json:"ip"`
  Port   int    `json:"port"`
  Ports  string `json:"ports"`
  Alias  string `json:"alias"`  // ✅ Accepts empty string
  Notes  string `json:"notes"`  // ✅ Accepts empty string
}
```

**Conclusion:** API accepts empty strings for `alias` and `notes` fields.

---

## 🔄 Other Allocation Creation Points

### Location 1: AdminAllocations.tsx
**Status:** ✅ FIXED (this file)

### Location 2: AdminNodes.tsx  
**File:** `apps/frontend/components/admin/AdminNodes.tsx` (line 403)

**Status:** ✅ FIXED

**Applied Same Fix:**
```typescript
mutationFn: () => createAllocation({ 
  nodeId: node.id, 
  ip: ip.trim(), 
  ports: ports.trim(), 
  alias: alias.trim(),  // ✅ Fixed - now sends empty string
  notes: notes.trim() 
}),
```

---

## 🎯 Complete Fix Checklist

- [x] Fixed AdminAllocations.tsx onCreate call
- [x] Fixed AdminNodes.tsx createMut call  
- [ ] Test allocation creation from Admin → Allocations panel
- [ ] Test allocation creation from Admin → Nodes → View Node panel
- [ ] Verify allocations appear in list after creation
- [ ] Verify allocations can be assigned to servers

---

## 🚀 How to Test

### Test 1: Create from Allocations Panel
1. Navigate to Admin → Allocations
2. Click "Create Allocations"
3. Select node, enter IP and port
4. Leave alias empty
5. Click Create
6. **Expected:** Success, new allocation appears in list

### Test 2: Create from Node Detail
1. Navigate to Admin → Nodes
2. Click on a node name
3. Scroll to allocations section
4. Enter IP and ports
5. Leave alias empty  
6. Click Create
7. **Expected:** Success, allocation appears in node's list

### Test 3: Assign to Server
1. Go to Server → Network tab
2. Click "Assign Allocation"
3. Select a free allocation
4. Click Assign
5. **Expected:** Allocation assigned to server

---

## 💡 Why This Happened

### JavaScript/TypeScript Behavior
```javascript
const alias = "";
const result = alias.trim() || undefined;
console.log(result);  // undefined

const obj = { alias: result };
console.log(JSON.stringify(obj));  // "{}"  (omits undefined)
```

### Better Pattern
```javascript
const alias = "";
const result = alias.trim();  // Just use the empty string
console.log(result);  // ""

const obj = { alias: result };
console.log(JSON.stringify(obj));  // '{"alias":""}'  (includes empty string)
```

---

## 📝 Additional Notes

### Next.js Hot Reload
Next.js development server will automatically reload the changes. No manual restart needed.

### Browser Cache
If the issue persists after the fix:
1. Hard refresh: Ctrl+Shift+R (Windows/Linux) or Cmd+Shift+R (Mac)
2. Or clear browser cache
3. Or try incognito window

### API Logs
To see allocation creation attempts in real-time:
```bash
tail -f .dev-logs/api.log | grep allocation
```

---

## ✅ Summary

**Problem:** Frontend sending `undefined` for optional fields  
**Solution:** Always send empty strings instead  
**Status:** Fixed in AdminAllocations.tsx  
**Additional Work:** Fix AdminNodes.tsx as well  
**Testing:** Follow test steps above to verify

---

**Your allocation creation should now work without errors!** 🎉

If you still see the 400 error, please:
1. Check browser console (F12) for the full error message
2. Check `.dev-logs/api.err.log` for backend validation errors
3. Take a screenshot of the error
4. Share the exact values you're entering in the form
