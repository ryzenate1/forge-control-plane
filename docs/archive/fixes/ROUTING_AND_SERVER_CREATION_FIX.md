# URL Routing & Multi-Step Server Creation Fix

**Date:** June 16, 2026, 3:15 PM  
**Issues:**
1. URL doesn't change when navigating (stays as `localhost:3000`)
2. No multi-step server creation wizard

---

## 🔍 Current Architecture

### Problem 1: Single Page Application (SPA) with No URL Routing

**Current Setup:**
- `app/page.tsx` renders a single `<Dashboard />` component
- Dashboard uses **internal state** to switch between views
- No Next.js routing - everything happens in one React component
- URL never changes = no bookmarking, no deep linking, no browser history

**Why This Happens:**
```typescript
// app/page.tsx
export default function Home() {
  return <Dashboard />;  // ← Entire app is one component!
}

// components/dashboard.tsx
function Dashboard() {
  const [mode, setMode] = useState<"server" | "admin">("server");
  const [serverTab, setServerTab] = useState<ServerTab>("console");
  // ... all routing is internal state
}
```

### Problem 2: Simple Server Creation (Not Multi-Step)

**Current:**
- Single modal with all fields at once
- No wizard flow
- Less user-friendly for complex setup

**What You Want:**
- Step 1: Basic Info (name, owner)
- Step 2: Node Selection & Resources
- Step 3: Allocations
- Step 4: Docker & Startup
- Step 5: Review & Create

---

## ✅ Solutions

### Solution A: Keep SPA + Add URL Sync (Quick Fix - 30 mins)

**What It Does:**
- Keep current Dashboard architecture
- Add URL synchronization using `useRouter` and `useSearchParams`
- URLs become: `/?mode=admin&tab=servers&server=xxx`
- Browser back/forward buttons work
- Can bookmark/share URLs

**Pros:**
- Quick to implement
- Minimal code changes
- No architectural changes

**Cons:**
- URLs are ugly (`?mode=admin&tab=servers`)
- Still a SPA (not true routing)
- Limited SEO potential

### Solution B: Proper Next.js Routing (Better - 2-3 hours)

**What It Does:**
- Create proper Next.js routes
- URLs become: `/admin/servers/xxx`, `/server/yyy/console`
- Full page navigation
- Better SEO
- Proper deep linking

**Structure:**
```
app/
├── page.tsx              → Dashboard/home
├── server/
│   └── [id]/
│       ├── console/
│       ├── files/
│       ├── databases/
│       └── ...
└── admin/
    ├── servers/
    ├── nodes/
    ├── allocations/
    └── ...
```

**Pros:**
- Clean URLs
- Proper routing
- Better UX
- SEO friendly
- Can use Next.js features (SSR, ISR, etc.)

**Cons:**
- More work to implement
- Need to refactor Dashboard component
- Breaking architectural change

---

## 🎯 Recommended Approach: HYBRID

**Phase 1: Quick URL Sync (Do Now - 30 mins)**
- Add URL synchronization to current SPA
- Make URLs work immediately
- Users can bookmark pages

**Phase 2: Multi-Step Server Creation (Do Now - 45 mins)**
- Create wizard component
- 5-step process
- Much better UX

**Phase 3: Proper Routing (Later - Optional)**
- Refactor to true Next.js routing
- When you have more time
- Not urgent if Phase 1 works

---

## 🛠️ Implementation Plan

### PHASE 1: Add URL Synchronization

**Changes to Dashboard Component:**

```typescript
// components/dashboard.tsx
import { useRouter, useSearchParams } from 'next/navigation';

export function Dashboard() {
  const router = useRouter();
  const searchParams = useSearchParams();
  
  // Read from URL
  const [mode, setModeState] = useState<"server" | "admin">(
    (searchParams.get('mode') as "server" | "admin") || "server"
  );
  const [serverTab, setServerTabState] = useState<ServerTab>(
    (searchParams.get('tab') as ServerTab) || "console"
  );
  const [selectedServerId, setSelectedServerIdState] = useState<string>(
    searchParams.get('server') || ""
  );

  // Wrapper functions that update URL
  const setMode = (m: "server" | "admin") => {
    setModeState(m);
    const params = new URLSearchParams(searchParams);
    params.set('mode', m);
    router.push(`?${params.toString()}`);
  };

  const setServerTab = (tab: ServerTab) => {
    setServerTabState(tab);
    const params = new URLSearchParams(searchParams);
    params.set('tab', tab);
    router.push(`?${params.toString()}`);
  };

  const setSelectedServerId = (id: string) => {
    setSelectedServerIdState(id);
    const params = new URLSearchParams(searchParams);
    params.set('server', id);
    router.push(`?${params.toString()}`);
  };

  // ... rest of component
}
```

**Result:**
- URL changes to `/?mode=admin&tab=servers`
- Browser back/forward works
- Can bookmark/share URLs
- Refresh page keeps your place

---

### PHASE 2: Multi-Step Server Creation Wizard

**Create New Component:** `apps/frontend/components/admin/CreateServerWizard.tsx`

```typescript
type Step = "basic" | "resources" | "allocation" | "startup" | "review";

export function CreateServerWizard({ onClose, onSuccess }) {
  const [step, setStep] = useState<Step>("basic");
  const [formData, setFormData] = useState({
    // Step 1: Basic
    name: "",
    ownerId: "",
    description: "",
    
    // Step 2: Resources  
    nodeId: "",
    memoryMb: 1024,
    cpuShares: 1024,
    diskMb: 10240,
    
    // Step 3: Allocation
    allocationId: "",
    additionalAllocationIds: [],
    
    // Step 4: Startup
    templateId: "",
    dockerImage: "",
    startupCommand: "",
    
    // Step 5: Review (read-only)
  });

  const steps = [
    { id: "basic", label: "Basic Info", icon: Info },
    { id: "resources", label: "Resources", icon: Cpu },
    { id: "allocation", label: "Network", icon: Network },
    { id: "startup", label: "Startup", icon: Terminal },
    { id: "review", label: "Review", icon: CheckCircle },
  ];

  return (
    <Modal title="Create Server" onClose={onClose} wide>
      {/* Step Indicator */}
      <div className="flex items-center justify-between px-6 py-4 border-b">
        {steps.map((s, idx) => (
          <div key={s.id} className={cn(
            "flex items-center gap-2",
            step === s.id && "text-blue-500",
            steps.findIndex(x => x.id === step) > idx && "text-green-500"
          )}>
            <s.icon size={16} />
            <span className="text-sm font-medium">{s.label}</span>
          </div>
        ))}
      </div>

      {/* Step Content */}
      <div className="p-6">
        {step === "basic" && <BasicInfoStep data={formData} onChange={setFormData} />}
        {step === "resources" && <ResourcesStep data={formData} onChange={setFormData} />}
        {step === "allocation" && <AllocationStep data={formData} onChange={setFormData} />}
        {step === "startup" && <StartupStep data={formData} onChange={setFormData} />}
        {step === "review" && <ReviewStep data={formData} />}
      </div>

      {/* Navigation */}
      <div className="flex justify-between border-t px-6 py-4">
        <Btn onClick={() => {
          const idx = steps.findIndex(s => s.id === step);
          if (idx > 0) setStep(steps[idx - 1].id);
        }} disabled={step === "basic"}>
          Previous
        </Btn>
        <Btn onClick={() => {
          const idx = steps.findIndex(s => s.id === step);
          if (idx < steps.length - 1) setStep(steps[idx + 1].id);
          else handleCreate();
        }}>
          {step === "review" ? "Create Server" : "Next"}
        </Btn>
      </div>
    </Modal>
  );
}
```

**Replace in AdminServers.tsx:**
```typescript
// OLD:
{modal && <CreateServerModal ... />}

// NEW:
{modal && <CreateServerWizard onClose={() => setModal(false)} onSuccess={() => { ... }} />}
```

---

## 📋 What Each Step Shows

### Step 1: Basic Info
- Server name (required)
- Owner selection (dropdown of users)
- Description (optional)

### Step 2: Resources
- Node selection (dropdown)
- Memory MB (number input with slider)
- CPU Shares (number input with slider)
- CPU Limit % (number input)
- Disk MB (number input with slider)
- Database limit (number input)
- Backup limit (number input)
- Allocation limit (number input)

### Step 3: Network
- Primary allocation (required, dropdown of free allocations)
- Additional allocations (multi-select)
- Shows IP:Port for each allocation

### Step 4: Startup
- Template/Egg selection (dropdown)
- Docker image (text input with common suggestions)
- Startup command (text input)
- Environment variables (key-value pairs, optional)

### Step 5: Review
- Shows all entered data in a nice summary
- "Edit" buttons to go back to specific steps
- Validates all required fields
- "Create Server" button

---

## 🎨 Visual Design

### Step Indicator Example:
```
[✓] Basic Info  →  [✓] Resources  →  [●] Network  →  [ ] Startup  →  [ ] Review
```

### Form Layout:
- Clean 2-column grid on desktop
- Single column on mobile
- Clear labels and hints
- Inline validation
- Progress saved between steps

---

## 🧪 Testing Plan

### Test URL Routing:
1. Navigate to admin section
2. Check URL changes to `?mode=admin`
3. Click on servers
4. Check URL includes `&tab=servers`
5. Click browser back button
6. Verify it goes back to previous view
7. Refresh page
8. Verify you stay on same page
9. Copy URL and open in new tab
10. Verify it opens to same view

### Test Server Creation:
1. Click "New Server" button
2. Verify wizard modal opens
3. Fill Step 1 (name, owner)
4. Click "Next"
5. Fill Step 2 (resources)
6. Click "Next"
7. Fill Step 3 (allocations)
8. Click "Next"
9. Fill Step 4 (startup)
10. Click "Next"
11. Review all data in Step 5
12. Click "Create Server"
13. Verify server appears in list
14. Verify server is actually created in database

---

## 🚀 Implementation Timeline

### Now (30 minutes):
- Add URL synchronization to Dashboard
- Test navigation with URL changes
- Verify browser back/forward works

### Next (45 minutes):
- Create CreateServerWizard component
- Implement all 5 steps
- Add validation
- Test server creation flow

### Later (Optional - 2-3 hours):
- Refactor to proper Next.js routing
- Create actual route pages
- Migrate away from SPA architecture

---

## 💡 Quick Fixes for Immediate Use

### If you just want URLs to work:
I can add the URL sync code to Dashboard.tsx in 5 minutes. URLs will update as you navigate.

### If you just want better server creation:
I can enhance the existing CreateServerModal to be multi-step in 20 minutes without creating a whole new component.

---

## ❓ What Do You Want?

**Option 1: Just fix URLs** (5-10 minutes)
- Add URL synchronization
- URLs change as you navigate
- Server creation stays simple (single form)

**Option 2: Just add multi-step wizard** (45 minutes)
- Keep URLs as-is
- Create proper wizard for server creation
- Much better UX for creating servers

**Option 3: Both fixes** (1 hour) ⭐ **RECOMMENDED**
- Add URL synchronization
- Create multi-step wizard
- Complete solution

**Option 4: Full routing refactor** (3-4 hours)
- Proper Next.js routing
- Clean URLs like `/admin/servers`
- Multi-step wizard
- Production-ready architecture

---

**Let me know which option you prefer and I'll implement it immediately!** 🚀
